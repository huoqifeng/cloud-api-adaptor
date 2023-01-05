package e2e

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"os"
	"path"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"time"
)

// CloudProvision defines operations to provision the environment on cloud providers.
type CloudProvision interface {
	CreateCluster(ctx context.Context, cfg *envconf.Config) error
	CreateVPC(ctx context.Context, cfg *envconf.Config) error
	DeleteVPC(ctx context.Context, cfg *envconf.Config) error
	DeleteCluster(ctx context.Context, cfg *envconf.Config) error
}

type PeerPods struct {
	cloudProvider string
	namespace     string
}

func NewPeerPods(provider string) (p *PeerPods) {
	return &PeerPods{cloudProvider: provider,
		namespace: "confidential-containers-system"}
}

func (p *PeerPods) Delete(ctx context.Context, cfg *envconf.Config) error {
	// TODO: implement me.
	return nil
}

// Deploy installs Peer Pods on the cluster.
func (p *PeerPods) Deploy(ctx context.Context, cfg *envconf.Config) error {
	client, err := cfg.NewClient()
	if err != nil {
		return err
	}

	err = decoder.DecodeEachFile(ctx, os.DirFS("../../install/yamls"), "deploy.yaml", decoder.CreateIgnoreAlreadyExists(client.Resources()))
	if err != nil {
		return err
	}

	if err := AllPodsRunning(ctx, cfg, p.namespace); err != nil {
		return err
	}

	overlayDir := path.Join("../../install/overlays", p.cloudProvider)
	kustomizeHelper := &KustomizeHelper{configDir: overlayDir}
	if err := kustomizeHelper.Apply(ctx, cfg); err != nil {
		return err
	}

	if err := AllPodsRunning(ctx, cfg, p.namespace); err != nil {
		return err
	}

	return nil
}

func (p *PeerPods) DoKustomize(ctx context.Context, cfg *envconf.Config) {
}

// TODO: convert this into a klient/wait/conditions
func AllPodsRunning(ctx context.Context, cfg *envconf.Config, namespace string) error {
	client, err := cfg.NewClient()
	if err != nil {
		return err
	}
	resources := client.Resources(namespace)
	objList := &corev1.PodList{}
	err = resources.List(context.TODO(), objList)
	if err != nil {
		return err
	}
	metaList, _ := meta.ExtractList(objList)
	for _, o := range metaList {
		obj, _ := o.(k8s.Object)
		if err = wait.For(conditions.New(resources).PodRunning(obj), wait.WithTimeout(time.Second*600)); err != nil {
			return err
		}
	}
	return nil
}
