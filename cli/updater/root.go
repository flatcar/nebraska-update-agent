package updatercli

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/kinvolk/lokomotive-update-controller/pkg/updater"
)

var RootCmd = &cobra.Command{
	Use:   "luc",
	Short: "Manage Lokomotive Update Controller",
	Run:   runController,
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var (
	kubeconfig     string
	appId          string
	interval       int64
	verbose        bool
	dev            bool
	nebraskaServer string
	channel        string
)

func init() {
	RootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "$HOME/.kube/config", "Path to Kubeconfig file.")
	RootCmd.PersistentFlags().StringVar(&appId, "app-id", "", "Nebraska assigned application ID.")
	RootCmd.PersistentFlags().StringVar(&nebraskaServer, "nebraska-server", "", "Nebraska server URL.")
	RootCmd.PersistentFlags().StringVar(&channel, "channel", "stable", "Channel to subscribe to for this application [stable | beta | alpha].")
	RootCmd.PersistentFlags().Int64Var(&interval, "interval", 1, "Polling interval for Nebraska server.")
	RootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Output verbose logs.")
	RootCmd.PersistentFlags().BoolVar(&dev, "dev", false, "God mode.")
	RootCmd.MarkFlagRequired("nebraskaServer")
	RootCmd.MarkFlagRequired("appId")
}

func runController(cmd *cobra.Command, args []string) {

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
