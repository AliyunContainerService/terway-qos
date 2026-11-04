package k8s

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/AliyunContainerService/terway-qos/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type podEvent struct {
	id  string
	pod *types.PodConfig
}

type eventSyncer struct {
	events    chan podEvent
	snapshots chan map[string]struct{}
}

func (s *eventSyncer) PrunePods(live map[string]struct{}) error {
	select {
	case s.snapshots <- live:
	default:
	}
	return nil
}

func (s *eventSyncer) UpdatePod(p *types.PodConfig) error {
	s.events <- podEvent{id: p.PodID, pod: p}
	return nil
}
func (s *eventSyncer) DeletePod(id string) error {
	s.events <- podEvent{id: id}
	return nil
}

// Start an isolated API server and etcd; no existing cluster is used.
func TestPodControllerWithAPIServer(t *testing.T) {
	if os.Getenv("TERWAY_QOS_ENVTEST") != "1" {
		t.Skip("set TERWAY_QOS_ENVTEST=1 and KUBEBUILDER_ASSETS for API-server integration tests")
	}
	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Error(err)
		}
	})
	opts := podManagerOptions("test-node")
	opts.Metrics.BindAddress = "0"
	opts.HealthProbeBindAddress = "0"
	mgr, err := ctrl.NewManager(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	syncer := &eventSyncer{events: make(chan podEvent, 100), snapshots: make(chan map[string]struct{}, 10)}
	if err := registerPodController(mgr, syncer); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("manager did not stop")
		}
	})
	api, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	if err := api.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "qos-test"}}); err != nil {
		t.Fatal(err)
	}
	createPod := func(name, node string) *corev1.Pod {
		t.Helper()
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "qos-test", Name: name,
			Annotations: map[string]string{"kubernetes.io/egress-bandwidth": "80M"}},
			Spec: corev1.PodSpec{NodeName: node, Containers: []corev1.Container{{Name: "test", Image: "registry.k8s.io/pause:3.10"}}}}
		if err := api.Create(ctx, pod); err != nil {
			t.Fatal(err)
		}
		pod.Status.PodIP = "10.0.0.2"
		pod.Status.PodIPs = []corev1.PodIP{{IP: "10.0.0.2"}}
		if err := api.Status().Update(ctx, pod); err != nil {
			t.Fatal(err)
		}
		return pod
	}
	createPod("other-node", "other-node")
	pod := createPod("local", "test-node")
	waitFor := func(match func(podEvent) bool) {
		t.Helper()
		timer := time.NewTimer(20 * time.Second)
		defer timer.Stop()
		for {
			select {
			case event := <-syncer.events:
				if event.id != "qos-test/local" {
					t.Fatalf("controller observed off-node Pod: %s", event.id)
				}
				if match(event) {
					return
				}
			case <-timer.C:
				t.Fatal("timed out waiting for Pod reconciliation")
			}
		}
	}
	waitFor(func(e podEvent) bool { return e.pod != nil && e.pod.TxBps != nil && *e.pod.TxBps == 10000000 })
	// GC uses the same node-scoped, synchronized cache as reconciliation.
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
snapshotLoop:
	for {
		select {
		case live := <-syncer.snapshots:
			if _, ok := live[string(pod.UID)]; ok {
				if len(live) != 1 {
					t.Fatalf("GC included off-node Pods: %v", live)
				}
				break snapshotLoop
			}
		case <-timer.C:
			t.Fatal("GC did not receive a synchronized Pod snapshot")
		}
	}
	if err := api.Get(ctx, client.ObjectKeyFromObject(pod), pod); err != nil {
		t.Fatal(err)
	}
	pod.Annotations["kubernetes.io/egress-bandwidth"] = "160M"
	if err := api.Update(ctx, pod); err != nil {
		t.Fatal(err)
	}
	waitFor(func(e podEvent) bool { return e.pod != nil && e.pod.TxBps != nil && *e.pod.TxBps == 20000000 })
	if err := api.Get(ctx, client.ObjectKeyFromObject(pod), pod); err != nil {
		t.Fatal(err)
	}
	pod.Annotations = nil
	if err := api.Update(ctx, pod); err != nil {
		t.Fatal(err)
	}
	waitFor(func(e podEvent) bool { return e.pod != nil && e.pod.TxBps == nil })
	if err := api.Delete(ctx, pod, client.GracePeriodSeconds(0)); err != nil {
		t.Fatal(err)
	}
	waitFor(func(e podEvent) bool { return e.pod == nil })
}
