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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type predicateForPod struct {
	predicate.Funcs
}

func (p *predicateForPod) Create(e event.CreateEvent) bool {
	pod, ok := e.Object.(*corev1.Pod)
	if !ok {
		return false
	}

	v4, v6 := getIPs(pod)
	if !v4.IsValid() && !v6.IsValid() {
		return false
	}

	return true
}

func (p *predicateForPod) Update(e event.UpdateEvent) bool {
	pod, ok := e.ObjectNew.(*corev1.Pod)
	if !ok {
		return false
	}

	v4, v6 := getIPs(pod)
	if !v4.IsValid() && !v6.IsValid() {
		return false
	}

	return true
}

func (p *predicateForPod) Delete(e event.DeleteEvent) bool {
	pod, ok := e.Object.(*corev1.Pod)
	if !ok {
		return false
	}

	v4, v6 := getIPs(pod)
	if !v4.IsValid() && !v6.IsValid() {
		return false
	}

	return true
}
