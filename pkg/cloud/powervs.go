package cloud

import (
	"context"
	goErrors "errors"
	"fmt"
	gohttp "net/http"
	"os"
	"strings"
	"time"

	"github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev2/controllerv2"
	"github.com/IBM-Cloud/bluemix-go/authentication"
	"github.com/IBM-Cloud/bluemix-go/http"
	"github.com/IBM-Cloud/bluemix-go/rest"
	bxsession "github.com/IBM-Cloud/bluemix-go/session"
	"github.com/IBM-Cloud/power-go-client/clients/instance"
	"github.com/IBM-Cloud/power-go-client/errors"
	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM-Cloud/power-go-client/power/client/p_cloud_volumes"
	"github.com/davecgh/go-spew/spew"
	"github.com/dgrijalva/jwt-go"
	"github.com/ppc64le-cloud/powervs-csi-driver/pkg/util"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ Cloud = &powerVSCloud{}

const (
	PollTimeout          = 120 * time.Second
	PollInterval         = 10 * time.Second
	TIMEOUT              = 60 * time.Minute
	VolumeInUseState     = "in-use"
	VolumeAvailableState = "available"
)

var (
	ErrPVMInstanceNotFound = goErrors.New("PVM instance not found")
	ErrDiskNotFound        = goErrors.New("disk Not Found")
)

type PowerVSClient interface {
}

type powerVSCloud struct {
	bxSess    *bxsession.Session
	piSession *ibmpisession.IBMPISession

	cloudInstanceID string

	imageClient        *instance.IBMPIImageClient
	pvmInstancesClient *instance.IBMPIInstanceClient
	resourceClient     controllerv2.ResourceServiceInstanceRepository
	volClient          *instance.IBMPIVolumeClient
}

type User struct {
	ID         string
	Email      string
	Account    string
	cloudName  string `default:"bluemix"`
	cloudType  string `default:"public"`
	generation int    `default:"2"`
}

type PVMInstance struct {
	ID      string
	ImageID string
	Name    string
}

type PVMImage struct {
	ID       string
	Name     string
	DiskType string
}

func authenticateAPIKey(sess *bxsession.Session) error {
	config := sess.Config
	tokenRefresher, err := authentication.NewIAMAuthRepository(config, &rest.Client{
		DefaultHeader: gohttp.Header{
			"User-Agent": []string{http.UserAgent()},
		},
	})
	if err != nil {
		return err
	}
	return tokenRefresher.AuthenticateAPIKey(config.BluemixAPIKey)
}

func fetchUserDetails(sess *bxsession.Session, generation int) (*User, error) {
	config := sess.Config
	user := User{}
	var bluemixToken string

	if strings.HasPrefix(config.IAMAccessToken, "Bearer") {
		bluemixToken = config.IAMAccessToken[7:len(config.IAMAccessToken)]
	} else {
		bluemixToken = config.IAMAccessToken
	}

	token, err := jwt.Parse(bluemixToken, func(token *jwt.Token) (interface{}, error) {
		return "", nil
	})
	if err != nil && !strings.Contains(err.Error(), "key is of invalid type") {
		return &user, err
	}

	claims := token.Claims.(jwt.MapClaims)
	if email, ok := claims["email"]; ok {
		user.Email = email.(string)
	}
	user.ID = claims["id"].(string)
	user.Account = claims["account"].(map[string]interface{})["bss"].(string)
	iss := claims["iss"].(string)
	if strings.Contains(iss, "https://iam.cloud.ibm.com") {
		user.cloudName = "bluemix"
	} else {
		user.cloudName = "staging"
	}
	user.cloudType = "public"

	user.generation = generation
	return &user, nil
}

func NewPowerVSCloud(cloudInstanceID string, debug bool) (Cloud, error) {
	return newPowerVSCloud(cloudInstanceID, debug)
}

func newPowerVSCloud(cloudInstanceID string, debug bool) (Cloud, error) {
	apikey := os.Getenv("IBMCLOUD_API_KEY")
	bxSess, err := bxsession.New(&bluemix.Config{BluemixAPIKey: apikey})
	if err != nil {
		return nil, err
	}

	err = authenticateAPIKey(bxSess)
	if err != nil {
		return nil, err
	}

	user, err := fetchUserDetails(bxSess, 2)
	if err != nil {
		return nil, err
	}

	ctrlv2, err := controllerv2.New(bxSess)
	if err != nil {
		return nil, err
	}

	resourceClient := ctrlv2.ResourceServiceInstanceV2()

	in, err := resourceClient.GetInstance(cloudInstanceID)
	if err != nil {
		return nil, err
	}

	zone := in.RegionID
	region, err := getRegion(zone)
	if err != nil {
		return nil, err
	}
	piSession, err := ibmpisession.New(bxSess.Config.IAMAccessToken, region, debug, TIMEOUT, user.Account, zone)
	if err != nil {
		return nil, err
	}

	volClient := instance.NewIBMPIVolumeClient(piSession, cloudInstanceID)
	pvmInstancesClient := instance.NewIBMPIInstanceClient(piSession, cloudInstanceID)
	imageClient := instance.NewIBMPIImageClient(piSession, cloudInstanceID)

	return &powerVSCloud{
		bxSess:             bxSess,
		piSession:          piSession,
		cloudInstanceID:    cloudInstanceID,
		imageClient:        imageClient,
		pvmInstancesClient: pvmInstancesClient,
		resourceClient:     resourceClient,
		volClient:          volClient,
	}, nil
}

func (p *powerVSCloud) GetPVMInstanceByName(name string) (*PVMInstance, error) {
	in, err := p.pvmInstancesClient.GetAll(p.cloudInstanceID, TIMEOUT)
	if err != nil {
		return nil, err
	}
	for _, pvmInstance := range in.PvmInstances {
		if name == *pvmInstance.ServerName {
			return &PVMInstance{
				ID:      *pvmInstance.PvmInstanceID,
				ImageID: *pvmInstance.ImageID,
				Name:    *pvmInstance.ServerName,
			}, nil
		}
	}
	return nil, ErrPVMInstanceNotFound
}

func (p *powerVSCloud) GetPVMInstanceByID(id string) (*PVMInstance, error) {
	in, err := p.pvmInstancesClient.Get(id, p.cloudInstanceID, TIMEOUT)
	if err != nil {
		return nil, err
	}

	return &PVMInstance{
		ID:      *in.PvmInstanceID,
		ImageID: *in.ImageID,
		Name:    *in.ServerName,
	}, nil
}

func (p *powerVSCloud) GetImageByID(id string) (*PVMImage, error) {
	image, err := p.imageClient.Get(id, p.cloudInstanceID)
	if err != nil {
		return nil, err
	}
	return &PVMImage{
		ID:       *image.ImageID,
		Name:     *image.Name,
		DiskType: *image.StorageType,
	}, nil
}

func (p *powerVSCloud) CreateDisk(ctx context.Context, volumeName string, diskOptions *DiskOptions) (disk *Disk, err error) {
	var volumeType string
	capacityGiB := float64(util.BytesToGiB(diskOptions.CapacityBytes))

	switch diskOptions.VolumeType {
	case VolumeTypeTier1, VolumeTypeTier3:
		volumeType = diskOptions.VolumeType
	case "":
		volumeType = DefaultVolumeType
	default:
		return nil, fmt.Errorf("invalid PowerVS VolumeType %q", diskOptions.VolumeType)
	}

	v, err := p.volClient.Create(volumeName, capacityGiB, volumeType, diskOptions.Shareable, p.cloudInstanceID, TIMEOUT)
	if err != nil {
		return nil, err
	}

	return &Disk{CapacityGB: diskOptions.CapacityGigaBytes, VolumeID: *v.VolumeID, DiskType: v.DiskType, WWN: strings.ToLower(v.Wwn)}, nil
}

func (p *powerVSCloud) DeleteDisk(ctx context.Context, volumeID string) (success bool, err error) {
	err = p.volClient.DeleteVolume(volumeID, p.cloudInstanceID, TIMEOUT)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (p *powerVSCloud) AttachDisk(ctx context.Context, volumeID string, nodeID string) (devicePath string, err error) {
	_, err = p.volClient.Attach(nodeID, volumeID, p.cloudInstanceID, TIMEOUT)
	if err != nil {
		return "", err
	}

	err = p.WaitForAttachmentState(ctx, volumeID, VolumeInUseState)
	if err != nil {
		return "", err
	}
	return "", nil
}

func (p *powerVSCloud) DetachDisk(ctx context.Context, volumeID string, nodeID string) (err error) {
	_, err = p.volClient.Detach(nodeID, volumeID, p.cloudInstanceID, TIMEOUT)
	if err != nil {
		return err
	}
	err = p.WaitForAttachmentState(ctx, volumeID, VolumeAvailableState)
	if err != nil {
		return err
	}
	return nil
}

func (p *powerVSCloud) ResizeDisk(ctx context.Context, volumeID string, reqSize int64) (newSize int64, err error) {
	disk, err := p.GetDiskByID(ctx, volumeID)
	if err != nil {
		return 0, err
	}
	v, err := p.volClient.Update(volumeID, disk.Name, float64(reqSize), disk.Shareable, p.cloudInstanceID, TIMEOUT)
	if err != nil {
		return 0, err
	}
	return int64(*v.Size), nil
}

func (p *powerVSCloud) WaitForAttachmentState(ctx context.Context, volumeID, state string) error {
	err := wait.PollImmediate(PollInterval, PollTimeout, func() (bool, error) {
		v, err := p.volClient.Get(volumeID, p.cloudInstanceID, TIMEOUT)
		if err != nil {
			return false, err
		}
		spew.Dump(v)
		return v.State == state, nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *powerVSCloud) GetDiskByName(ctx context.Context, name string, capacityBytes int64) (disk *Disk, err error) {
	//TODO: remove capacityBytes
	params := p_cloud_volumes.NewPcloudCloudinstancesVolumesGetallParamsWithTimeout(TIMEOUT).WithCloudInstanceID(p.cloudInstanceID)
	resp, err := p.piSession.Power.PCloudVolumes.PcloudCloudinstancesVolumesGetall(params, ibmpisession.NewAuth(p.piSession, p.cloudInstanceID))
	if err != nil {
		return nil, errors.ToError(err)
	}
	for _, v := range resp.Payload.Volumes {
		if name == *v.Name {
			return &Disk{
				Name:      *v.Name,
				DiskType:  *v.DiskType,
				VolumeID:  *v.VolumeID,
				WWN:       strings.ToLower(*v.Wwn),
				Shareable: *v.Shareable,
			}, nil
		}
	}

	return nil, ErrDiskNotFound
}

func (p *powerVSCloud) GetDiskByID(ctx context.Context, volumeID string) (disk *Disk, err error) {
	v, err := p.volClient.Get(volumeID, p.cloudInstanceID, TIMEOUT)
	if err != nil {
		return nil, err
	}
	return &Disk{
		Name:      *v.Name,
		DiskType:  v.DiskType,
		VolumeID:  *v.VolumeID,
		WWN:       strings.ToLower(v.Wwn),
		Shareable: *v.Shareable,
	}, nil
}

func (p *powerVSCloud) IsExistInstance(ctx context.Context, nodeID string) (success bool) {
	panic("implement me")
}

func (p *powerVSCloud) CreateSnapshot(ctx context.Context, volumeID string, snapshotOptions *SnapshotOptions) (snapshot *Snapshot, err error) {
	panic("implement me")
}

func (p *powerVSCloud) DeleteSnapshot(ctx context.Context, snapshotID string) (success bool, err error) {
	panic("implement me")
}

func (p *powerVSCloud) GetSnapshotByName(ctx context.Context, name string) (snapshot *Snapshot, err error) {
	panic("implement me")
}

func (p *powerVSCloud) GetSnapshotByID(ctx context.Context, snapshotID string) (snapshot *Snapshot, err error) {
	panic("implement me")
}

func (p *powerVSCloud) ListSnapshots(ctx context.Context, volumeID string, maxResults int64, nextToken string) (listSnapshotsResponse *ListSnapshotsResponse, err error) {
	panic("implement me")
}

func getRegion(zone string) (region string, err error) {
	err = nil
	switch {
	case strings.HasPrefix(zone, "us-south"):
		region = "us-south"
	case strings.HasPrefix(zone, "us-east"):
		region = "us-east"
	case strings.HasPrefix(zone, "tor"):
		region = "tor"
	case strings.HasPrefix(zone, "eu-de-"):
		region = "eu-de"
	case strings.HasPrefix(zone, "lon"):
		region = "lon"
	case strings.HasPrefix(zone, "syd"):
		region = "syd"
	default:
		return "", fmt.Errorf("region not found for the zone, talk to the developer to add the support into the tool: %s", zone)
	}
	return
}
