/*
 * Copyright (c) 2023, Alibaba Group;
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package k8s

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"time"

	"github.com/AliyunContainerService/terway-qos/pkg/bandwidth"
	"github.com/AliyunContainerService/terway-qos/pkg/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Interface interface {
	PodByUID() *corev1.Pod
}

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func StartPodHandler(ctx context.Context, syncer types.SyncPod) error {
	options := ctrl.Options{
		Scheme: scheme,
	}

	options.NewCache = cache.BuilderWithOptions(cache.Options{
		SelectorsByObject: cache.SelectorsByObject{
			&corev1.Pod{}: {
				Field: fields.SelectorFromSet(fields.Set{"spec.nodeName": os.Getenv("K8S_NODE_NAME")}),
			},
		}},
	)
	mgr, err := ctrl.NewManager(config.GetConfigOrDie(), options)
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}, builder.WithPredicates(&predicateForPod{})).
		Complete(&reconcilePod{
			client: mgr.GetClient(),
			syncer: syncer,
		})
	if err != nil {
		return err
	}
	return mgr.Start(ctx)
}

// reconcilePod reconciles ReplicaSets
type reconcilePod struct {
	// client can be used to retrieve objects from the APIServer.
	client client.Client

	syncer types.SyncPod
}

// Implement reconcile.Reconciler so the controller can reconcile objects
var _ reconcile.Reconciler = &reconcilePod{}

func (r *reconcilePod) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	pod := corev1.Pod{}
	err := r.client.Get(ctx, client.ObjectKey{
		Namespace: request.Namespace,
		Name:      request.Name,
	}, &pod)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("pod %s/%s has been deleted", request.Namespace, request.Name)
			return reconcile.Result{}, r.syncer.DeletePod(request.String())
		}
		return reconcile.Result{}, err
	}

	if !pod.DeletionTimestamp.IsZero() {
		t := time.Since(pod.DeletionTimestamp.Time)
		if t < 0 {
			// Reconciliation is level-based, meaning action isn't driven off changes in
			// individual Events. Requeue the result at least once to make sure the bpf map
			// will be deleted in time. Because the pod object may exist but its ip address
			// has been allocated to another pod. e.g. pod deletion is blocked by a
			// time-consuming finalizer.
			klog.Infof("pod %s/%s requeue deletion at %s",
				pod.Namespace, pod.Name, pod.DeletionTimestamp.Time)
			return reconcile.Result{RequeueAfter: -t}, nil
		} else {
			// IP addresses are expected to have been reclaimed
			// See https://github.com/kubernetes/kubernetes/issues/109414#issuecomment-1125233538
			klog.Infof("pod %s/%s IP addresses are expected to have been reclaimed",
				pod.Namespace, pod.Name)
			return reconcile.Result{}, r.syncer.DeletePod(request.String())
		}
	}

	v4, v6 := getIPs(&pod)
	if !v4.IsValid() && !v6.IsValid() {
		return reconcile.Result{}, fmt.Errorf("pod %s/%s has no ip", pod.Namespace, pod.Name)
	}

	ingress, egress, err := bandwidth.ExtractPodBandwidthResources(pod.Annotations)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error extract bandwidth resources, %w", err)
	}

	update := &types.PodConfig{
		PodID:       fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
		PodUID:      string(pod.UID),
		IPv4:        v4,
		IPv6:        v6,
		HostNetwork: pod.Spec.HostNetwork,
	}

	if ingress != nil {
		v := uint64(ingress.Value())
		update.RxBps = &(v)
	}
	if egress != nil {
		v := uint64(egress.Value())
		update.TxBps = &(v)
	}
	switch pod.Annotations["k8s.aliyun.com/qos-class"] {
	case "best-effort":
		update.Prio = func(a uint32) *uint32 {
			return &a
		}(2)
	case "burstable":
		update.Prio = func(a uint32) *uint32 {
			return &a
		}(1)
	case "guaranteed":
		update.Prio = func(a uint32) *uint32 {
			return &a
		}(0)
	}

	return reconcile.Result{}, r.syncer.UpdatePod(update)
}

func getIPs(pod *corev1.Pod) (v4 netip.Addr, v6 netip.Addr) {
	for _, ip := range pod.Status.PodIPs {
		addr, err := netip.ParseAddr(ip.IP)
		if err != nil {
			continue
		}
		if addr.Is4() {
			v4 = addr
		} else {
			v6 = addr
		}
	}

	return
}
