package k8s

import (
	"context"
	"errors"
	"testing"

	"github.com/AliyunContainerService/terway-qos/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type podClient struct {
	client.Client
	pod corev1.Pod
}

type snapshotReader struct {
	client.Reader
	pods corev1.PodList
	err  error
}

func (r *snapshotReader) List(_ context.Context, out client.ObjectList, _ ...client.ListOption) error {
	*out.(*corev1.PodList) = r.pods
	return r.err
}

type captureGC struct {
	calls int
	live  map[string]struct{}
}

func (g *captureGC) PrunePods(live map[string]struct{}) error { g.calls++; g.live = live; return nil }

func TestPrunePodSnapshot(t *testing.T) {
	reader := &snapshotReader{pods: corev1.PodList{Items: []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{UID: "pending"}}, // No IP or successful reconcile required.
		{ObjectMeta: metav1.ObjectMeta{UID: "annotated"}},
	}}}
	gc := &captureGC{}
	if err := prunePodState(context.Background(), reader, gc); err != nil {
		t.Fatal(err)
	}
	if len(gc.live) != 2 {
		t.Fatalf("incomplete live Pod snapshot: %v", gc.live)
	}
	reader.err = errors.New("list failed")
	if err := prunePodState(context.Background(), reader, gc); err == nil {
		t.Fatal("ignored API list error")
	}
	if gc.calls != 1 {
		t.Fatal("pruned after API list failure")
	}
}

func (c *podClient) Get(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	*obj.(*corev1.Pod) = *c.pod.DeepCopy()
	return nil
}

type captureSyncer struct{ pod *types.PodConfig }

func (s *captureSyncer) UpdatePod(p *types.PodConfig) error { s.pod = p; return nil }
func (s *captureSyncer) DeletePod(string) error             { return nil }

func TestReconcileBandwidthUnitsAndRemoval(t *testing.T) {
	c := &podClient{pod: corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns", UID: "uid", Annotations: map[string]string{
			"kubernetes.io/ingress-bandwidth": "80M",
			"kubernetes.io/egress-bandwidth":  "160M",
		}},
		Status: corev1.PodStatus{PodIPs: []corev1.PodIP{{IP: "10.0.0.1"}}},
	}}
	s := &captureSyncer{}
	r := &reconcilePod{client: c, syncer: s}
	request := reconcile.Request{NamespacedName: ktypes.NamespacedName{Namespace: "ns", Name: "pod"}}
	if _, err := r.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if *s.pod.RxBps != 10000000 || *s.pod.TxBps != 20000000 {
		t.Fatalf("incorrect byte rates: %+v", s.pod)
	}
	c.pod.Annotations = nil
	if _, err := r.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if s.pod.RxBps != nil || s.pod.TxBps != nil {
		t.Fatal("removed annotation retained a limit")
	}
	c.pod.Annotations = map[string]string{"kubernetes.io/egress-bandwidth": "bad"}
	if _, err := r.Reconcile(context.Background(), request); err == nil {
		t.Fatal("accepted invalid bandwidth")
	}
}
