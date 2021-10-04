package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "luc",
	Short: "Manage Lokomotive Update Controller",
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
