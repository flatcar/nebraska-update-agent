module github.com/kinvolk/nebraska-update-agent

go 1.16

require (
	github.com/fluxcd/kustomize-controller/api v0.25.0
	github.com/fluxcd/pkg/apis/meta v0.13.0
	github.com/fluxcd/source-controller/api v0.22.3
	github.com/kinvolk/flux-libs v0.0.0-20220506103121-9ca81861812f
	github.com/kinvolk/nebraska/updater v0.0.0-20220324162709-c5f72decb5ad
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	k8s.io/api v0.23.5
	k8s.io/apimachinery v0.23.5
	sigs.k8s.io/yaml v1.3.0
)
