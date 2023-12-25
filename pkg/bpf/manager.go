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

package bpf

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/rlimit"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	ctrl "sigs.k8s.io/controller-runtime"
)

var log = ctrl.Log.WithName("bpf")

const (
	tcProgName = "terway_qos"
	pinPath    = "/sys/fs/bpf/terway"
)

var objs *qos_tcObjects
var once sync.Once

func getBpfObj(enableCORE bool) *qos_tcObjects {
	once.Do(func() {
		err := rlimit.RemoveMemlock()
		if err != nil {
			log.Error(err, "remove memlock failed")
			os.Exit(1)
		}
		err = os.MkdirAll(pinPath, os.ModeDir)
		if err != nil {
			log.Error(err, "mkdir failed")
			os.Exit(1)
		}

		featEDT := false
		err = features.HaveProgramHelper(ebpf.SchedCLS, asm.FnSkbEcnSetCe)
		if err != nil {
			if !errors.Is(err, ebpf.ErrNotSupported) {
				log.Error(err, "check kernel version failed")
				os.Exit(1)
			}
		} else {
			featEDT = true
		}

		objs = &qos_tcObjects{}

		opts := &ebpf.CollectionOptions{
			Maps: ebpf.MapOptions{
				PinPath:        pinPath,
				LoadPinOptions: ebpf.LoadPinOptions{},
			},
			Programs:        ebpf.ProgramOptions{},
			MapReplacements: nil,
		}

		if enableCORE {
			err := loadQos_tcObjects(objs, opts)
			if err != nil {
				log.Error(err, "load bpf objects failed")
				os.Exit(1)
			}
		} else {
			err := Compile(featEDT)
			if err != nil {
				log.Error(err, "compile bpf failed")
				os.Exit(1)
			}

			spec, err := ebpf.LoadCollectionSpec(progPath)
			if err != nil {
				log.Error(err, "load bpf objects failed")
				os.Exit(1)
			}
			err = spec.LoadAndAssign(objs, opts)
			if err != nil {
				log.Error(err, "load bpf objects failed")
				os.Exit(1)
			}
		}

	})
	return objs
}

type validateDeviceFunc = func(link netlink.Link) bool

type Mgr struct {
	nlEvent chan netlink.LinkUpdate

	enableIngress, enableEgress bool

	obj *qos_tcObjects

	validate validateDeviceFunc
}

func NewBpfMgr(enableIngress, enableEgress, enableCORE bool, validate validateDeviceFunc) (*Mgr, error) {
	return &Mgr{
		nlEvent:       make(chan netlink.LinkUpdate),
		obj:           getBpfObj(enableCORE),
		enableEgress:  enableEgress,
		enableIngress: enableIngress,
		validate:      validate,
	}, nil
}

func (m *Mgr) Start(ctx context.Context) error {
	links, err := netlink.LinkList()
	if err != nil {
		return err
	}
	for _, link := range links {
		err = m.ensureBpfProg(link)
		if err != nil {
			log.Error(err, "attach bpf prog failed")
			return err
		}
	}

	err = netlink.LinkSubscribe(m.nlEvent, ctx.Done())
	if err != nil {
		return err
	}

	go func() {
		for e := range m.nlEvent {
			err = m.ensureBpfProg(e.Link)
			if err != nil {
				log.Error(err, "attach bpf prog failed")
			}
		}
	}()

	return nil
}

func (m *Mgr) Close() {
	if m.obj != nil {
		m.obj.Close()
	}
}

func (m *Mgr) ensureBpfProg(link netlink.Link) error {
	if !m.validate(link) {
		return nil
	}

	err := ensureQdisc([]netlink.Link{link})
	if err != nil {
		return err
	}

	ingressFilter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_MIN_INGRESS,
			Handle:    netlink.MakeHandle(0, 1),
			Protocol:  unix.ETH_P_ALL,
			Priority:  90,
		},
		Fd:           int(m.obj.qos_tcPrograms.QosProgIngress.FD()),
		Name:         tcProgName,
		DirectAction: true,
	}
	if m.enableIngress {
		err = netlink.FilterReplace(ingressFilter)
		if err != nil {
			return err
		}

		log.Info("set bpf ingress", "dev", link.Attrs().Name)

		err = m.obj.QosProgMap.Put(uint32(0), uint32(m.obj.QosCgroup.FD()))
		if err != nil {
			return err
		}
		err = m.obj.QosProgMap.Put(uint32(1), uint32(m.obj.QosGlobal.FD()))
		if err != nil {
			return err
		}
	} else {
		err = netlink.FilterDel(ingressFilter)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				log.Error(err, "delete bpf prog failed")
			}
		}
	}

	egressFilter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_MIN_EGRESS,
			Handle:    netlink.MakeHandle(0, 1),
			Protocol:  unix.ETH_P_ALL,
			Priority:  90,
		},
		Fd:           int(m.obj.qos_tcPrograms.QosProgEgress.FD()),
		Name:         tcProgName,
		DirectAction: true,
	}
	if m.enableEgress {
		err = netlink.FilterReplace(egressFilter)
		if err != nil {
			return err
		}

		log.Info("set bpf egress", "dev", link.Attrs().Name)

		err = m.obj.QosProgMap.Put(uint32(0), uint32(m.obj.QosCgroup.FD()))
		if err != nil {
			return err
		}
		err = m.obj.QosProgMap.Put(uint32(1), uint32(m.obj.QosGlobal.FD()))
		if err != nil {
			return err
		}
	} else {
		err = netlink.FilterDel(egressFilter)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				log.Error(err, "delete bpf prog failed")
			}
		}
	}

	return nil
}

func ensureQdisc(links []netlink.Link) error {
	for _, link := range links {
		qdisc := &netlink.GenericQdisc{
			QdiscAttrs: netlink.QdiscAttrs{
				LinkIndex: link.Attrs().Index,
				Parent:    netlink.HANDLE_CLSACT,
				Handle:    netlink.HANDLE_CLSACT & 0xffff0000,
			},
			QdiscType: "clsact",
		}
		err := netlink.QdiscReplace(qdisc)
		if err != nil {
			return err
		}
	}
	return nil
}
