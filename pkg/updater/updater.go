package updater

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"time"

	helmreleaseapi "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/fluxcd/pkg/apis/meta"
	sourceapi "github.com/fluxcd/source-controller/api/v1beta1"

	"github.com/kinvolk/nebraska/updater"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kinvolk/fluxlib/lib"
	helmrelease "github.com/kinvolk/fluxlib/lib/helm-release"
	sourcecontroller "github.com/kinvolk/fluxlib/lib/source-controller"
)

const (
	namespace      = "flux-system"
	defaultVersion = "0.0.1"
)

type Config struct {
	Kubeconfig     string
	ApplicationID  string
	Interval       int64
	Dev            bool
	NebraskaServer string
	Channel        string

	grc            *sourcecontroller.GitRepoConfig
	hrc            *helmrelease.HelmReleaseConfig
	nbsClient      *updater.Updater
	clusterID      string
	currentVersion string
}

var fluxInstallInterval = metav1.Duration{Duration: 5 * time.Minute} //nolint:gomnd

func generateGitRepository(version string) *sourceapi.GitRepository {
	return &sourceapi.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lokomotive-" + version,
			Namespace: namespace,
		},
		Spec: sourceapi.GitRepositorySpec{
			Interval: fluxInstallInterval,
			Reference: &sourceapi.GitRepositoryRef{
				Tag: version,
			},
			URL: "https://github.com/kinvolk/lokomotive/",
		},
	}
}

func Reconcile(cfg *Config) error {
	kubeconfig, err := ioutil.ReadFile(cfg.Kubeconfig)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading kubeconfig: %w", err)
	}

	cfg.grc, err = sourcecontroller.NewGitRepoConfig(
		sourcecontroller.WithKubeconfig(kubeconfig),
	)
	if err != nil {
		return fmt.Errorf("initializing GitRepository client: %w", err)
	}

	cfg.hrc, err = helmrelease.NewHelmReleaseConfig(
		helmrelease.WithKubeconfig(kubeconfig),
	)
	if err != nil {
		return fmt.Errorf("initializing HelmRelease config: %w", err)
	}

	if err = cfg.getClusterID(); err != nil {
		return fmt.Errorf("retrieving cluster id: %w", err)
	}

	if err = wait.PollInfinite(time.Second*10, func() (done bool, err error) {
		cfg.currentVersion, err = cfg.getCurrentVersion()
		if err != nil {
			log.Errorf("getting current version from latest GitRepository: %v", err)

			return false, nil
		}

		log.Debugf("got version: '%s'", cfg.currentVersion)

		return true, nil
	}); err != nil {
		return fmt.Errorf("waiting for GitRepository to be available: %w", err)
	}

	if err := cfg.setupNebraskaClient(); err != nil {
		return fmt.Errorf("setting up nebraska client: %w", err)
	}

	log.Debug("initialization complete")

	_ = wait.PollInfinite(time.Duration(cfg.Interval)*time.Minute, func() (done bool, err error) {
		log.Debug("reconciling infinitely!")

		if err := cfg.reconcile(); err != nil {
			log.Error(err)
		}

		return false, nil
	})

	return nil
}

func addVToVersion(version string) string {
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	return version
}

func removeVFromVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}

func (cfg *Config) getCurrentVersion() (string, error) {
	gitRepoList, err := cfg.grc.List(&client.ListOptions{Namespace: namespace})
	if err != nil {
		return "", fmt.Errorf("getting GitRepositoryList: %w", err)
	}

	grs := gitRepoList.Items

	if len(grs) == 0 {
		return "", fmt.Errorf("no GitRepository installed in the cluster")
	}

	sort.Slice(grs, func(i, j int) bool {
		return grs[i].CreationTimestamp.Before(&grs[j].CreationTimestamp)
	})

	// Get the latest (the last in the sorted list) item.
	gr := grs[len(grs)-1]
	tag := gr.Spec.Reference.Tag

	if tag == "" {
		log.Debugf("Found empty 'tag' in GitRepository: '%s'. Source reference: %+v.", gr.Name, gr.Spec.Reference)

		return defaultVersion, nil
	}

	// Remove the leading 'v' from the version.
	return removeVFromVersion(tag), nil
}

func (cfg *Config) getClusterID() error {
	// Return random UUID when using dev mode.
	if cfg.Dev {
		cfg.clusterID = string(uuid.NewUUID())
		return nil
	}

	c, err := lib.GetKubernetesClient([]byte(cfg.Kubeconfig), nil)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	var got corev1.Namespace
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "kube-system"}, &got); err != nil {
		return fmt.Errorf("getting kube-system namespace: %w", err)
	}

	cfg.clusterID = string(got.UID)
	log.Debugf("got cluster id: '%s'", cfg.clusterID)

	return nil
}

func (cfg *Config) updateFluxCRs(version string) (*helmreleaseapi.HelmReleaseList, error) {
	// For the new version create new GitRespository.
	gr := generateGitRepository(version)
	if err := cfg.grc.CreateOrUpdate(gr); err != nil {
		return nil, fmt.Errorf("creating/updating GitRepository for version '%s': %w", version, err)
	}

	log.Debugf("Created/Updated the GitRepository for the version '%s'.", version)

	// Get all the HelmReleases.
	helmReleaseList, err := cfg.hrc.List(&client.ListOptions{Namespace: namespace})
	if err != nil {
		return nil, fmt.Errorf("getting HelmReleaseList: %w", err)
	}

	// Update all HelmReleases with the new GitRepository.
	for _, h := range helmReleaseList.Items {
		hr := h.DeepCopy()
		hr.Spec.Chart.Spec.SourceRef.Name = gr.Name
		if err := cfg.hrc.CreateOrUpdate(hr); err != nil {
			return nil, fmt.Errorf("updating HelmRelease '%s': %w", hr.Name, err)
		}
	}

	log.Infof("Updated all the HelmReleases to version '%s'.", version)

	return helmReleaseList, nil
}

func (cfg *Config) waitForHelmReleaseReadiness(hrl *helmreleaseapi.HelmReleaseList) error {
	log.Debug("checking the HelmRelease readiness.")

	// Poll for ten minutes every ten seconds.
	if err := wait.PollImmediate(time.Second*10, time.Minute*10, func() (done bool, err error) {
		ready := true

		for _, h := range hrl.Items {
			hr, err := cfg.hrc.Get(h.Name, h.Namespace)
			if err != nil {
				return false, fmt.Errorf("getting the HelmRelease '%s': %w", hr.Name, err)
			}

			// Not ready yet.
			if hr.Generation != hr.Status.ObservedGeneration || !apimeta.IsStatusConditionTrue(hr.Status.Conditions, meta.ReadyCondition) {
				ready = false
			}
		}

		// No need to poll any more, all the HelmReleases are ready.
		if ready {
			return true, nil
		}

		return false, nil
	}); err != nil {
		return fmt.Errorf("waiting for the HelmReleases to be ready: %w", err)
	}

	log.Info("All the HelmReleases are ready with the new version.")

	return nil
}

func (cfg *Config) setupNebraskaClient() error {
	var err error

	cfg.nbsClient, err = updater.New(cfg.NebraskaServer, cfg.ApplicationID, cfg.Channel, cfg.clusterID, cfg.currentVersion)
	if err != nil {
		return fmt.Errorf("initializing nebraska client: %w", err)
	}

	return nil
}

func (cfg *Config) reconcile() error {
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
		log.Debugf("got this object: %#v", info.GetOmahaResponse().Apps[0])

		// Ensure that all the helm releases are updated to the latest version we got from the
		// Nebraska. But if we haven't received any update yet then just return.
		if cfg.currentVersion == defaultVersion {
			return nil
		}

		if _, err = cfg.updateFluxCRs(cfg.currentVersion); err != nil {
			return fmt.Errorf("ensuring all HelmReleases are to the latest version: %w", err)
		}

		return nil
	}

	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressDownloadStarted)

	// There is a new update.
	version := info.GetVersion()
	version = addVToVersion(version)

	log.Debugf("update available: '%s'", version)

	helmReleaseList, err := cfg.updateFluxCRs(version)
	if err != nil {
		_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)

		return fmt.Errorf("updating flux CRs: %w", err)
	}

	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressDownloadFinished)
	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressInstallationStarted)

	if err := cfg.waitForHelmReleaseReadiness(helmReleaseList); err != nil {
		_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)

		return err
	}

	// Update the current version to the new one.
	cfg.currentVersion = version

	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressInstallationFinished)
	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressUpdateComplete)

	cfg.nbsClient.SetInstanceVersion(info.GetVersion())

	return nil
}
