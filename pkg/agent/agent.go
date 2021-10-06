package agent

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/kinvolk/fluxlib/lib"
	"github.com/kinvolk/nebraska/updater"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
)

const (
	defaultVersion = "0.0.1"
)

type Config struct {
	Kubeconfig    string
	ApplicationID string
	Dev           bool
	UpdateServer  string
	Channel       string
	Interval      int64
	Docker        bool
	EnvPath       string

	clusterID      string
	currentVersion string
	nbsClient      updater.Updater
}

func (cfg *Config) getClusterID(kubeconfig []byte) error {
	// Return random UUID when using dev mode.
	if cfg.Dev {
		cfg.clusterID = string(uuid.NewUUID())
		return nil
	}

	c, err := lib.GetKubernetesClient(kubeconfig, nil)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	var got corev1.Namespace
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "kube-system"}, &got); err != nil {
		return fmt.Errorf("getting kube-system namespace: %w", err)
	}

	cfg.clusterID = string(got.UID)
	log.Debugf("got cluster id: %s", cfg.clusterID)

	return nil
}

func (cfg *Config) setupNebraskaClient() error {
	var err error

	nbsConfig := updater.Config{
		OmahaURL:        cfg.UpdateServer,
		AppID:           cfg.ApplicationID,
		Channel:         cfg.Channel,
		InstanceID:      cfg.clusterID,
		InstanceVersion: removeVFromVersion(cfg.currentVersion),
	}

	cfg.nbsClient, err = updater.New(nbsConfig)
	if err != nil {
		return fmt.Errorf("initializing nebraska client: %w", err)
	}

	return nil
}

func removeVFromVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}

func ReconcileContainer(cfg *Config) error {
	kubeconfig, err := ioutil.ReadFile(cfg.Kubeconfig)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading kubeconfig: %w", err)
	}

	if err := cfg.getClusterID(kubeconfig); err != nil {
		return fmt.Errorf("retrieving cluster id: %w", err)
	}
	cfg.currentVersion = defaultVersion

	if err := cfg.setupNebraskaClient(); err != nil {
		return fmt.Errorf("setting up nebraska client: %w", err)
	}

	log.Debug("initialization complete")

	if cfg.Docker {
		_ = wait.PollInfinite(time.Duration(cfg.Interval)*time.Minute, func() (done bool, err error) {
			log.Debug("reconciling infinitely!")

			if err := cfg.reconcileContainer(); err != nil {
				log.Error(err)
			}

			return false, nil
		})
	} else {
		if err := cfg.systemdService(); err != nil {
			log.Error(err)
		}
	}

	return nil
}

func (cfg *Config) reconcileContainer() error {
	ctx := context.TODO()

	// Let us check if there is an update.
	info, err := cfg.nbsClient.CheckForUpdates(ctx)
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}

	// There is no update hence return.
	if !info.HasUpdate {
		log.Info("no update available")

		// Print the response just in case.
		log.Debugf("got this response: %#v", info.OmahaResponse().Apps[0])

		return nil
	}

	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressDownloadStarted)

	// There is a new update.
	version := info.Version
	link := info.URL()
	name := info.Package().Name
	containerStopDuration := time.Minute

	log.Debugf("update available: %s", version, link, name)

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)
		return fmt.Errorf("setting up docker client: %w", err)
	}

	containers, err := cli.ContainerList(context.Background(), dockertypes.ContainerListOptions{})
	if err != nil {
		_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)
		return fmt.Errorf("listing containers: %w", err)
	}

	for _, container := range containers {

		imageName := strings.Split(container.Names[0], "/")

		if imageName[1] == name {

			log.Debug("Stopping and restaring the container")

			err := cli.ContainerStop(context.Background(), container.ID[:10], &containerStopDuration)
			if err != nil {
				_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)
				return fmt.Errorf("stopping container: %w", err)
			}

			err = cli.ContainerRemove(context.Background(), container.ID[:10], dockertypes.ContainerRemoveOptions{})
			if err != nil {
				_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)
				return fmt.Errorf("removing container: %w", err)
			}

			// Now run a container
			resp, err := cli.ContainerCreate(ctx, &dockercontainer.Config{
				Image: name + ":" + version,
			}, nil, nil, nil, name)
			if err != nil {
				_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)
				return fmt.Errorf("creating container: %w", err)
			}

			if err := cli.ContainerStart(ctx, resp.ID, dockertypes.ContainerStartOptions{}); err != nil {
				_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)
				return fmt.Errorf("starting container: %w", err)
			}
		} else {
			continue
		}
	}

	// Update the current version to the new one.
	cfg.currentVersion = version

	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressInstallationFinished)
	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressUpdateComplete)

	cfg.nbsClient.SetInstanceVersion(info.Version)

	return nil
}

func (cfg *Config) systemdService() error {
	ctx := context.TODO()

	// Let us check if there is an update.
	info, err := cfg.nbsClient.CheckForUpdates(ctx)
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}

	// There is no update hence return.
	if !info.HasUpdate {
		// Print the response just in case.
		log.Debugf("got this response: %#v", info.OmahaResponse().Apps[0])

		return nil
	}

	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressDownloadStarted)

	// There is a new update.
	version := info.Version
	// link := info.URL()
	// name := info.Package().Name

	// Read the contents of env file
	envFile, err := ioutil.ReadFile(cfg.EnvPath)
	if err != nil && !os.IsNotExist(err) {
		_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)
		return fmt.Errorf("reading env file: %w", err)
	}

	envData := strings.Split(string(envFile), "\n")
	versionEnv := strings.Split(envData[1], "=")

	if versionEnv[1] == version {
		log.Info("no update available")
		return nil
	}

	log.Debugf("update available: %s", version)

	// update env file.
	indexVersion := int64(strings.Index(string(envFile), "VERSION=")) + 8
	file, err := os.OpenFile(cfg.EnvPath, os.O_RDWR, 0644)

	if err != nil {
		_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)
		return fmt.Errorf("failed opening file: %w", err)
	}

	defer file.Close()

	_, err = file.WriteAt([]byte(version), indexVersion)
	if err != nil {
		return fmt.Errorf("failed writing to file: %w", err)
	}

	log.Info("Updated env file to latest version.")

	// Update the current version to the new one.
	cfg.currentVersion = version

	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressInstallationFinished)
	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressUpdateComplete)

	cfg.nbsClient.SetInstanceVersion(info.Version)

	return nil
}
