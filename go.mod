module github.com/ppc64le-cloud/powervs-csi-driver

go 1.17

require (
	github.com/IBM-Cloud/bluemix-go v0.0.0-20201019071904-51caa09553fb
	github.com/IBM-Cloud/power-go-client v1.0.53
	github.com/container-storage-interface/spec v1.5.0
	github.com/davecgh/go-spew v1.1.1
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	google.golang.org/grpc v1.38.0
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v1.22.1
	k8s.io/klog/v2 v2.9.0
	k8s.io/mount-utils v0.22.1
	k8s.io/utils v0.0.0-20210707171843-4b05e18ac7d9
)

require (
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a // indirect
	github.com/go-logr/logr v0.4.0 // indirect
	github.com/go-openapi/analysis v0.19.5 // indirect
	github.com/go-openapi/errors v0.19.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.5 // indirect
	github.com/go-openapi/loads v0.19.4 // indirect
	github.com/go-openapi/runtime v0.19.11 // indirect
	github.com/go-openapi/spec v0.19.3 // indirect
	github.com/go-openapi/strfmt v0.19.4 // indirect
	github.com/go-openapi/swag v0.19.14 // indirect
	github.com/go-openapi/validate v0.19.6 // indirect
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.5 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.11 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	go.mongodb.org/mongo-driver v1.5.1 // indirect
	golang.org/x/net v0.0.0-20210520170846-37e1c6afe023 // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/sys v0.0.0-20210616094352-59db8d763f22 // indirect
	golang.org/x/term v0.0.0-20210220032956-6a3ed077a48d // indirect
	golang.org/x/text v0.3.6 // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac // indirect
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c // indirect
	google.golang.org/protobuf v1.26.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/api v0.22.1 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.1.2 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.22.1
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.22.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.1
	k8s.io/apiserver => k8s.io/apiserver v0.22.1
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.22.1
	k8s.io/client-go => k8s.io/client-go v0.22.1
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.22.1
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.22.1
	k8s.io/code-generator => k8s.io/code-generator v0.22.1
	k8s.io/component-base => k8s.io/component-base v0.22.1
	k8s.io/component-helpers => k8s.io/component-helpers v0.22.1
	k8s.io/controller-manager => k8s.io/controller-manager v0.22.1
	k8s.io/cri-api => k8s.io/cri-api v0.22.1
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.22.1
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.22.1
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.22.1
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.22.1
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.22.1
	k8s.io/kubectl => k8s.io/kubectl v0.22.1
	k8s.io/kubelet => k8s.io/kubelet v0.22.1
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.22.1
	k8s.io/metrics => k8s.io/metrics v0.22.1
	k8s.io/mount-utils => k8s.io/mount-utils v0.22.1
	k8s.io/node-api => k8s.io/node-api v0.22.1
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.22.1
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.22.1
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.22.1
	k8s.io/sample-controller => k8s.io/sample-controller v0.22.1
	vbom.ml/util => github.com/fvbommel/util v0.0.0-20180919145318-efcd4e0f9787
)
