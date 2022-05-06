package kustomize

import (
	"context"
	"fmt"

	api "github.com/fluxcd/kustomize-controller/api/v1beta2"
	"github.com/kinvolk/flux-libs/lib"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KustomizeConfig struct {
	c          client.Client
	kubeconfig []byte
}

type kustomizeConfigOpt func(*KustomizeConfig)

var scheme *runtime.Scheme

func init() {
	scheme = runtime.NewScheme()
	_ = api.AddToScheme(scheme)
}

// kustomizeCfg := lib.NewKustomizeConfig(
//     lib.WithKubeconfig(kc),
//	   lib.WithFoobar(fb),
// )
func NewKustomizeConfig(fns ...kustomizeConfigOpt) (*KustomizeConfig, error) {
	var ret KustomizeConfig

	for _, fn := range fns {
		fn(&ret)
	}

	var err error

	ret.c, err = lib.GetKubernetesClient(ret.kubeconfig, scheme)
	if err != nil {
		return nil, err
	}

	return &ret, nil
}

func WithKubeconfig(kubeconfig []byte) kustomizeConfigOpt {
	return func(kc *KustomizeConfig) {
		kc.kubeconfig = kubeconfig
	}
}

func (k *KustomizeConfig) Get(name, ns string) (*api.Kustomization, error) {
	var got api.Kustomization

	if err := k.c.Get(context.Background(), types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}, &got); err != nil {
		return nil, fmt.Errorf("getting kustomization: %w", err)
	}

	return &got, nil
}

func (k *KustomizeConfig) List(listOpts *client.ListOptions) (*api.KustomizationList, error) {
	var got api.KustomizationList

	if err := k.c.List(context.Background(), &got, listOpts); err != nil {
		return nil, fmt.Errorf("listing kustomization: %w", err)
	}

	return &got, nil
}

func (k *KustomizeConfig) CreateOrUpdate(kc *api.Kustomization) error {
	var got api.Kustomization

	if err := k.c.Get(context.Background(), types.NamespacedName{
		Namespace: kc.GetNamespace(),
		Name:      kc.GetName(),
	}, &got); err != nil {
		if errors.IsNotFound(err) {
			// Create the object since it does not exists.
			if err := k.c.Create(context.Background(), kc); err != nil {
				return fmt.Errorf("creating kustomization: %w", err)
			}

			return nil
		}

		return fmt.Errorf("looking up kustomization: %w", err)
	}

	kc.ResourceVersion = got.ResourceVersion

	if err := k.c.Update(context.Background(), kc); err != nil {
		return fmt.Errorf("updating kustomization: %w", err)
	}

	return nil
}

func (k *KustomizeConfig) Delete(kc *api.Kustomization) error {
	if err := k.c.Delete(context.Background(), kc); err != nil {
		return fmt.Errorf("deleting kustomization: %w", err)
	}

	return nil
}
