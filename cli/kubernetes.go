package cli

import (
	"github.com/kinvolk/lokomotive-update-controller/pkg/updater"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var kubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Manage kubernetes updates",
	Run:   runKubernetes,
}

func init() {
	RootCmd.AddCommand(kubernetesCmd)
}

func runKubernetes(cmd *cobra.Command, args []string) {

	cfg := updater.Config{
		Kubeconfig:     kubeconfig,
		ApplicationID:  appId,
		Interval:       interval,
		Dev:            dev,
		NebraskaServer: nebraskaServer,
		Channel:        channel,
	}

	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	if err := updater.Reconcile(&cfg); err != nil {
		log.Fatalf("reconciling: %v", err)
	}
}
