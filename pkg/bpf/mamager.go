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
	"fmt"
	"net"
	"os"

	"github.com/cilium/ebpf"
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

type Mgr struct {
	nlEvent chan netlink.LinkUpdate

	obj *qos_tcObjects
}

func NewBpfMgr() (*Mgr, error) {
	err := rlimit.RemoveMemlock()
	if err != nil {
		return nil, err
	}
	err = Compile()
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(pinPath, os.ModeDir)
	if err != nil {
		return nil, err
	}

	spec, err := ebpf.LoadCollectionSpec(progPath)
	if err != nil {
		return nil, fmt.Errorf("load spec %w", err)
	}
	objs := &qos_tcObjects{}
	err = spec.LoadAndAssign(objs, &ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{
			PinPath:        pinPath,
			LoadPinOptions: ebpf.LoadPinOptions{},
		},
		Programs:        ebpf.ProgramOptions{},
		MapReplacements: nil,
	})
	if err != nil {
		return nil, err
	}

	return &Mgr{nlEvent: make(chan netlink.LinkUpdate), obj: objs}, nil
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
	if !validDevice(link) {
		return nil
	}

	err := ensureQdisc([]netlink.Link{link})
	if err != nil {
		return err
	}

	filter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_MIN_EGRESS,
			Handle:    netlink.MakeHandle(0, 1),
			Protocol:  unix.ETH_P_ALL,
			Priority:  90,
		},
		Fd:           int(m.obj.qos_tcPrograms.QosProg.FD()),
		Name:         tcProgName,
		DirectAction: true,
	}
	err = netlink.FilterReplace(filter)
	if err != nil {
		return err
	}

	log.Info("set bpf", "dev", link.Attrs().Name)

	err = m.obj.QosProgMap.Put(uint32(0), uint32(m.obj.QosCgroup.FD()))
	if err != nil {
		return err
	}
	err = m.obj.QosProgMap.Put(uint32(1), uint32(m.obj.QosGlobal.FD()))
	if err != nil {
		return err
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

func validDevice(link netlink.Link) bool {
	dev, ok := link.(*netlink.Device)
	if !ok {
		return false
	}
	if dev.Attrs().Flags&net.FlagUp == 0 {
		return false
	}
	return dev.EncapType != "loopback"
}
