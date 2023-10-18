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

package config

import (
	"fmt"

	"github.com/AliyunContainerService/terway-qos/pkg/types"

	"k8s.io/client-go/tools/cache"
)

type PodCache struct {
	cache.Indexer
}

const (
	indexPodID      = "podID"
	indexPodUID     = "podUID"
	indexCgroupPath = "cgroupPath"
)

func NewPodCache() *PodCache {
	return &PodCache{
		Indexer: cache.NewIndexer(func(obj interface{}) (string, error) {
			r, ok := obj.(*types.PodConfig)
			if !ok {
				return "", fmt.Errorf("not type *Record")
			}
			return r.PodID, nil
		}, cache.Indexers{
			indexPodID: func(obj interface{}) ([]string, error) {
				r, ok := obj.(*types.PodConfig)
				if !ok {
					return nil, fmt.Errorf("not type *Record")
				}
				return []string{r.PodID}, nil
			},
			indexPodUID: func(obj interface{}) ([]string, error) {
				r, ok := obj.(*types.PodConfig)
				if !ok {
					return nil, fmt.Errorf("not type *Record")
				}
				return []string{r.PodUID}, nil
			},
			indexCgroupPath: func(obj interface{}) ([]string, error) {
				r, ok := obj.(*types.PodConfig)
				if !ok {
					return nil, fmt.Errorf("not type *Record")
				}
				return []string{r.CgroupInfo.Path}, nil
			},
		}),
	}
}

func (r *PodCache) ByPodID(id string) *types.PodConfig {
	objs, err := r.ByIndex(indexPodID, id)
	if err != nil {
		panic(err)
	}
	if len(objs) == 0 {
		return nil
	}
	return objs[0].(*types.PodConfig)
}

func (r *PodCache) ByPodUID(id string) *types.PodConfig {
	objs, err := r.ByIndex(indexPodUID, id)
	if err != nil {
		panic(err)
	}
	if len(objs) == 0 {
		return nil
	}
	return objs[0].(*types.PodConfig)
}

func (r *PodCache) ByCgroupPath(id string) *types.PodConfig {
	objs, err := r.ByIndex(indexCgroupPath, id)
	if err != nil {
		panic(err)
	}
	if len(objs) == 0 {
		return nil
	}
	return objs[0].(*types.PodConfig)
}

func (r *PodCache) AddIfNotPresent(config *types.PodConfig) error {
	_, ok, err := r.Indexer.Get(config)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return r.Indexer.Add(config)
}

func (r *PodCache) Del(config *types.PodConfig) error {
	return r.Indexer.Delete(config)
}

func (r *PodCache) DelByPodID(id string) error {
	return r.Indexer.Delete(&types.PodConfig{PodID: id})
}
