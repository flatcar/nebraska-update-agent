package updater

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/kinvolk/flux-libs/lib"
	"github.com/kinvolk/flux-libs/lib/kustomize"
	gitrepocontroller "github.com/kinvolk/flux-libs/lib/source-controller/git-repo-controller"
	"github.com/kinvolk/nebraska/updater"

	kustomizeapi "github.com/fluxcd/kustomize-controller/api/v1beta2"
	"github.com/fluxcd/pkg/apis/meta"
	sourceapi "github.com/fluxcd/source-controller/api/v1beta1"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/yaml"
)

const (
	namespace      = "flux-system"
	defaultVersion = "0.0.0"
)

type Config struct {
	Kubeconfig     string
	ApplicationID  string
	Interval       int64
	Dev            bool
	NebraskaServer string
	Channel        string

	gitRepoCfg     *gitrepocontroller.GitRepoConfig
	kustomizeCfg   *kustomize.KustomizeConfig
	nbsClient      updater.Updater
	clusterID      string
	currentVersion string

	kustomization *kustomizeapi.Kustomization
	gitRepository *sourceapi.GitRepository
}
type Package struct {
	Spec *kustomizeapi.KustomizationSpec `json:"spec"`
}

var fluxInstallInterval = metav1.Duration{Duration: 5 * time.Minute} //nolint:gomnd

func Reconcile(cfg *Config) error {
	kubeconfig, err := ioutil.ReadFile(cfg.Kubeconfig)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading kubeconfig: %w", err)
	}

	cfg.gitRepoCfg, err = gitrepocontroller.NewGitRepoConfig(
		gitrepocontroller.WithKubeconfig(kubeconfig),
	)
	if err != nil {
		return fmt.Errorf("initializing GitRepository client: %w", err)
	}

	cfg.kustomizeCfg, err = kustomize.NewKustomizeConfig(
		kustomize.WithKubeconfig(kubeconfig),
	)
	if err != nil {
		return fmt.Errorf("initializing Kustomization client: %w", err)
	}

	if err = cfg.getClusterID(); err != nil {
		return fmt.Errorf("retrieving cluster id: %w", err)
	}

	cfg.currentVersion = defaultVersion

	if err := cfg.setupNebraskaClient(); err != nil {
		return fmt.Errorf("setting up nebraska client: %w", err)
	}

	log.Debug("initialization complete")

	_ = wait.PollInfinite(time.Duration(cfg.Interval)*time.Second, func() (done bool, err error) {
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

func (cfg *Config) getClusterID() error {
	// Return random UUID when using dev mode.
	if cfg.Dev {
		cfg.clusterID = string(uuid.NewUUID())
		return nil
	}

	kubeconfig, err := ioutil.ReadFile(cfg.Kubeconfig)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading kubeconfig: %w", err)
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

func (cfg *Config) getUpdateConfig(info *updater.UpdateInfo) error {
	var err error

	updateCfgFile := info.URL()

	if err = cfg.generateConfigs(updateCfgFile); err != nil {
		return fmt.Errorf("parsing update config: %w", err)
	}

	return nil
}

func (cfg *Config) createOrUpdateNamespace() error {
	kubeconfig, err := ioutil.ReadFile(cfg.Kubeconfig)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading kubeconfig: %w", err)
	}

	c, err := lib.GetKubernetesClient(kubeconfig, nil)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	namespace := cfg.gitRepository.Namespace
	var got corev1.Namespace
	if err := c.Get(context.Background(), types.NamespacedName{Name: namespace}, &got); err != nil {
		if errors.IsNotFound(err) {
			// Create the namespace since it does not exists.
			if err := c.Create(context.Background(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}); err != nil {
				return fmt.Errorf("creating namespace %s: %w", namespace, err)
			}

			return nil
		}

		return fmt.Errorf("getting namespace %s: %w", namespace, err)
	}

	// This means the namespace already exists.
	return nil
}

func (cfg *Config) updateFluxCRs() error {
	// Check if the namespace exists, if not then create one.
	if err := cfg.createOrUpdateNamespace(); err != nil {
		return fmt.Errorf("creating/updating namespace: %w", err)
	}

	if err := cfg.gitRepoCfg.CreateOrUpdate(cfg.gitRepository); err != nil {
		return fmt.Errorf("creating/updating GitRepository: %w", err)
	}

	if err := cfg.kustomizeCfg.CreateOrUpdate(cfg.kustomization); err != nil {
		return fmt.Errorf("creating/updating Kustomization: %w", err)
	}

	log.Info("updated all the Flux configs")

	return nil
}

func (cfg *Config) waitForKustomizationReadiness() error {
	log.Debug("checking the Kustomization readiness.")

	// Poll for ten minutes every ten seconds.
	if err := wait.PollImmediate(time.Second*10, time.Minute*10, func() (done bool, err error) {
		ready := true

		name := cfg.kustomization.Name
		namespace := cfg.kustomization.Namespace

		kc, err := cfg.kustomizeCfg.Get(name, namespace)
		if err != nil {
			return false, fmt.Errorf("getting the Kustomization %s: %w", name, err)
		}

		// Not ready yet.
		if kc.Generation != kc.Status.ObservedGeneration || !apimeta.IsStatusConditionTrue(kc.Status.Conditions, meta.ReadyCondition) {
			ready = false
		}

		// No need to poll any more, all the HelmReleases are ready.
		if ready {
			return true, nil
		}

		return false, nil
	}); err != nil {
		return fmt.Errorf("waiting for the Kustomization to be ready: %w", err)
	}

	log.Info("Kustomization is ready with the new version")

	return nil
}

func (cfg *Config) setupNebraskaClient() error {
	var err error

	nbsConfig := updater.Config{
		OmahaURL:        cfg.NebraskaServer,
		AppID:           cfg.ApplicationID,
		Channel:         cfg.Channel,
		InstanceID:      cfg.clusterID,
		InstanceVersion: removeVFromVersion(cfg.currentVersion),
		// Debug:           true,
	}

	cfg.nbsClient, err = updater.New(nbsConfig)
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
		log.Debugf("got this response: %#v", info.OmahaResponse().Apps[0])

		return nil
	}

	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressDownloadStarted)

	// There is a new update.
	version := info.Version

	log.Debugf("update available: %s", version)

	if err := cfg.getUpdateConfig(info); err != nil {
		_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)

		return fmt.Errorf("getting the update config provided in Nebraska update: %w", err)
	}

	if err := cfg.updateFluxCRs(); err != nil {
		_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)

		return fmt.Errorf("updating flux CRs: %w", err)
	}

	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressDownloadFinished)
	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressInstallationStarted)

	if err := cfg.waitForKustomizationReadiness(); err != nil {
		_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressError)

		return err
	}

	// Update the current version to the new one.
	cfg.currentVersion = version

	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressInstallationFinished)
	_ = cfg.nbsClient.ReportProgress(ctx, updater.ProgressUpdateComplete)

	cfg.nbsClient.SetInstanceVersion(info.Version)

	return nil
}

// base64Decode decodes base64 encoded strings.
// A golang version of:
// echo '' | base64 -d
func base64Decode(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("got empty string")
	}

	decodeBytes, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("decoding string: %w", err)
	}

	return string(decodeBytes), nil
}

// generateConfigs will convert an URL like the following into corresponding GitRepository and Kustomization configs.
// https://github.com/surajssd/test-flux?nua_commit=OWZmZWYxOTY5Njc3MDU3ZTIxZGZlOTlhY2NiZjIyZjM0M2Y5NjMwMA%3D%3D&nua_kustomize=CnNwZWM6CiAgaW50ZXJ2YWw6IDE1bQogIHBhdGg6ICIuL2s4cyIKICBwcnVuZTogdHJ1ZQogIHNvdXJjZVJlZjoKICAgIGtpbmQ6IEdpdFJlcG9zaXRvcnkKICAgIG5hbWU6IG15LWFwcAoK&nua_namespace=bmV3
func (cfg *Config) generateConfigs(encodedURL string) error {
	u, err := url.Parse(encodedURL)
	if err != nil {
		return fmt.Errorf("parsing given URL: %w", err)
	}

	// Get commit, namespace and kustomization spec config from the URL.
	encodedCommit := u.Query().Get("nua_commit")
	encodedNamespace := u.Query().Get("nua_namespace")
	encodedKustomizeCfg := u.Query().Get("nua_kustomize_config")

	// Extract the https://github.com/surajssd/test-flux from the URL.
	repoURL := path.Join(u.Host, u.Path)
	repoURL = "https://" + repoURL

	commit, err := base64Decode(encodedCommit)
	if err != nil {
		return fmt.Errorf("decoding commit: %w", err)
	}

	namespace, err := base64Decode(encodedNamespace)
	if err != nil {
		return fmt.Errorf("decoding repo sub-path: %w", err)
	}

	kustomizeCfg, err := base64Decode(encodedKustomizeCfg)
	if err != nil {
		return fmt.Errorf("decoding kustomize config: %w", err)
	}

	log.Debugf("Nebraska update URL decoded successfully")

	// Convert the YAML string into object.
	pkg, err := parseKustomizeConfig(kustomizeCfg)
	if err != nil {
		return fmt.Errorf("parsing kustomize config: %w", err)
	}

	/*
	   apiVersion: kustomize.toolkit.fluxcd.io/v1beta2
	   kind: Kustomization
	   metadata:
	     name: my-app
	     namespace: default
	   spec:
	     interval: 15m
	     path: "./k8s"
	     prune: true
	     sourceRef:
	       kind: GitRepository
	       name: my-app
	*/

	name := pkg.Spec.SourceRef.Name
	cfg.kustomization = &kustomizeapi.Kustomization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: *pkg.Spec,
	}

	/*
	   apiVersion: source.toolkit.fluxcd.io/v1beta2
	   kind: GitRepository
	   metadata:
	     name: my-app
	     namespace: default
	   spec:
	     interval: 5m
	     url: https://github.com/surajssd/test-flux
	     ref:
	       commit: 9ffef1969677057e21dfe99accbf22f343f96300
	*/
	cfg.gitRepository = &sourceapi.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: sourceapi.GitRepositorySpec{
			URL: repoURL,
			Reference: &sourceapi.GitRepositoryRef{
				Commit: commit,
			},
		},
	}

	return nil
}

// parseKustomizeConfig parses the string into Package object.
func parseKustomizeConfig(config string) (*Package, error) {
	var ret Package

	if err := yaml.Unmarshal([]byte(config), &ret); err != nil {
		return nil, fmt.Errorf("unmarshalling response into Package: %w. \nGiven config:\n%s\n", err, config)
	}

	return &ret, nil
}
