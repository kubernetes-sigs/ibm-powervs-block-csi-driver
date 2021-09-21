package cloud

// MetadataService represents Power VS metadata service.
type MetadataService interface {
	GetCloudInstanceId() string
	GetPvmInstanceId() string
}
