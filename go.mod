module github.com/kinvolk/nebraska-update-controller

go 1.16

require (
	github.com/fluxcd/pkg/apis/meta v0.10.0
	github.com/fluxcd/source-controller/api v0.15.4
	github.com/kinvolk/flux-libs v0.0.0-20211007140918-b5d53df56575
	github.com/kinvolk/nebraska/updater v0.0.0-20211006140741-b0a56c5037ac
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	sigs.k8s.io/yaml v1.2.0
)
