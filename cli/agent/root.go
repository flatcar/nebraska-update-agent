package agentcli

import (
	"os"

	"github.com/kinvolk/lokomotive-update-controller/pkg/agent"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "nebraska-agent",
	Short: "Run nebraska agent",
	Run:   runController,
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var (
	docker       bool
	envPath      string
	appId        string
	interval     int64
	verbose      bool
	dev          bool
	updateServer string
	channel      string
)

func init() {
	RootCmd.PersistentFlags().BoolVarP(&docker, "docker", "d", false, "enable docker mode.")
	RootCmd.PersistentFlags().StringVar(&envPath, "envpath", "", "env file path for systemd service.")
	RootCmd.PersistentFlags().StringVar(&appId, "app-id", "", "Nebraska assigned application ID.")
	RootCmd.PersistentFlags().StringVar(&updateServer, "update-server", "", "Nebraska server URL.")
	RootCmd.PersistentFlags().StringVar(&channel, "channel", "stable", "Channel to subscribe to for this application [stable | beta | alpha].")
	RootCmd.PersistentFlags().Int64Var(&interval, "interval", 1, "Polling interval for Nebraska server.")
	RootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Output verbose logs.")
	RootCmd.PersistentFlags().BoolVar(&dev, "dev", false, "God mode.")
	RootCmd.MarkFlagRequired("updateServer")
	RootCmd.MarkFlagRequired("appId")
	RootCmd.MarkFlagRequired("envpath")
}

func runController(cmd *cobra.Command, args []string) {

	cfg := agent.Config{
		ApplicationID: appId,
		Dev:           dev,
		UpdateServer:  updateServer,
		Channel:       channel,
		Interval:      interval,
		Docker:        docker,
		EnvPath:       envPath,
	}

	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	if err := agent.ReconcileContainer(&cfg); err != nil {
		log.Fatalf("reconciling: %v", err)
	}
}
