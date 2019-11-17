module github.com/kanisterio/kanister

go 1.12

replace (
	cloud.google.com/go => github.com/GoogleCloudPlatform/google-cloud-go v0.1.1-0.20160913182117-3b1ae45394a2
	github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309
	github.com/graymeta/stow => github.com/kastenhq/stow v0.1.2-kasten
	github.com/rook/operator-kit => github.com/kastenhq/operator-kit v0.0.0-20180316185208-859e831cc18d
)

require (
	contrib.go.opencensus.io/exporter/ocagent v0.4.12 // indirect
	github.com/Azure/azure-sdk-for-go v31.1.0+incompatible // indirect
	github.com/Azure/go-autorest v13.3.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/to v0.2.0 // indirect
	github.com/BurntSushi/toml v0.3.1
	github.com/IBM-Cloud/ibm-cloud-cli-sdk v0.3.0 // indirect
	github.com/IBM/ibmcloud-storage-volume-lib v1.0.2-beta02.0.20190828145158-1da4543a60af
	github.com/Masterminds/semver v1.4.2
	github.com/Masterminds/sprig v2.15.0+incompatible
	github.com/aokoli/goutils v1.1.0 // indirect
	github.com/aws/aws-sdk-go v1.20.12
	github.com/cheekybits/is v0.0.0-20150225183255-68e9c0620927 // indirect
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/elazarl/goproxy v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-openapi/strfmt v0.19.0
	github.com/gofrs/flock v0.7.1
	github.com/graymeta/stow v0.0.0-00010101000000-000000000000
	github.com/jarcoal/httpmock v1.0.4 // indirect
	github.com/jpillora/backoff v0.0.0-20170918002102-8eab2debe79d
	github.com/json-iterator/go v1.1.8
	github.com/kelseyhightower/envconfig v1.4.0 // indirect
	github.com/kubernetes-csi/external-snapshotter v1.2.1-0.20191115155915-abbefcd7fa2f
	github.com/lib/pq v1.2.0
	github.com/luci/go-render v0.0.0-20160219211803-9a04cc21af0f
	github.com/mitchellh/mapstructure v1.1.2
	github.com/pkg/errors v0.8.1
	github.com/renier/xmlrpc v0.0.0-20170708154548-ce4a1a486c03 // indirect
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.4.2
	github.com/softlayer/softlayer-go v0.0.0-20190615201252-ba6e7f295217 // indirect
	github.com/spf13/cobra v0.0.5
	github.com/vmware/govmomi v0.21.1-0.20191008161538-40aebf13ba45
	go.uber.org/atomic v1.4.0 // indirect
	go.uber.org/zap v1.10.0
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	google.golang.org/api v0.3.1
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce // indirect
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.2.4
	helm.sh/helm/v3 v3.0.0
	k8s.io/api v0.0.0-20191115135540-bbc9463b57e5
	k8s.io/apiextensions-apiserver v0.0.0-20191117020858-b615a37f53e7
	k8s.io/apimachinery v0.0.0-20191116203941-08e4eafd6d11
	k8s.io/client-go v0.0.0-20191115215802-0a8a1d7b7fae
	k8s.io/kubectl v0.0.0-20191115222826-fbc5d36fee2d // indirect
)
