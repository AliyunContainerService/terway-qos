// Code generated by bpf2go; DO NOT EDIT.
//go:build arm64be || armbe || mips || mips64 || mips64p32 || ppc64 || s390 || s390x || sparc || sparc64

package bpf

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"

	"github.com/cilium/ebpf"
)

// loadQos_tc returns the embedded CollectionSpec for qos_tc.
func loadQos_tc() (*ebpf.CollectionSpec, error) {
	reader := bytes.NewReader(_Qos_tcBytes)
	spec, err := ebpf.LoadCollectionSpecFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("can't load qos_tc: %w", err)
	}

	return spec, err
}

// loadQos_tcObjects loads qos_tc and converts it into a struct.
//
// The following types are suitable as obj argument:
//
//	*qos_tcObjects
//	*qos_tcPrograms
//	*qos_tcMaps
//
// See ebpf.CollectionSpec.LoadAndAssign documentation for details.
func loadQos_tcObjects(obj interface{}, opts *ebpf.CollectionOptions) error {
	spec, err := loadQos_tc()
	if err != nil {
		return err
	}

	return spec.LoadAndAssign(obj, opts)
}

// qos_tcSpecs contains maps and programs before they are loaded into the kernel.
//
// It can be passed ebpf.CollectionSpec.Assign.
type qos_tcSpecs struct {
	qos_tcProgramSpecs
	qos_tcMapSpecs
}

// qos_tcSpecs contains programs before they are loaded into the kernel.
//
// It can be passed ebpf.CollectionSpec.Assign.
type qos_tcProgramSpecs struct {
	QosCgroup      *ebpf.ProgramSpec `ebpf:"qos_cgroup"`
	QosGlobal      *ebpf.ProgramSpec `ebpf:"qos_global"`
	QosProgEgress  *ebpf.ProgramSpec `ebpf:"qos_prog_egress"`
	QosProgIngress *ebpf.ProgramSpec `ebpf:"qos_prog_ingress"`
}

// qos_tcMapSpecs contains maps before they are loaded into the kernel.
//
// It can be passed ebpf.CollectionSpec.Assign.
type qos_tcMapSpecs struct {
	CgroupRateMap   *ebpf.MapSpec `ebpf:"cgroup_rate_map"`
	GlobalRateMap   *ebpf.MapSpec `ebpf:"global_rate_map"`
	PodMap          *ebpf.MapSpec `ebpf:"pod_map"`
	QosProgMap      *ebpf.MapSpec `ebpf:"qos_prog_map"`
	TerwayGlobalCfg *ebpf.MapSpec `ebpf:"terway_global_cfg"`
	TerwayNetStat   *ebpf.MapSpec `ebpf:"terway_net_stat"`
}

// qos_tcObjects contains all objects after they have been loaded into the kernel.
//
// It can be passed to loadQos_tcObjects or ebpf.CollectionSpec.LoadAndAssign.
type qos_tcObjects struct {
	qos_tcPrograms
	qos_tcMaps
}

func (o *qos_tcObjects) Close() error {
	return _Qos_tcClose(
		&o.qos_tcPrograms,
		&o.qos_tcMaps,
	)
}

// qos_tcMaps contains all maps after they have been loaded into the kernel.
//
// It can be passed to loadQos_tcObjects or ebpf.CollectionSpec.LoadAndAssign.
type qos_tcMaps struct {
	CgroupRateMap   *ebpf.Map `ebpf:"cgroup_rate_map"`
	GlobalRateMap   *ebpf.Map `ebpf:"global_rate_map"`
	PodMap          *ebpf.Map `ebpf:"pod_map"`
	QosProgMap      *ebpf.Map `ebpf:"qos_prog_map"`
	TerwayGlobalCfg *ebpf.Map `ebpf:"terway_global_cfg"`
	TerwayNetStat   *ebpf.Map `ebpf:"terway_net_stat"`
}

func (m *qos_tcMaps) Close() error {
	return _Qos_tcClose(
		m.CgroupRateMap,
		m.GlobalRateMap,
		m.PodMap,
		m.QosProgMap,
		m.TerwayGlobalCfg,
		m.TerwayNetStat,
	)
}

// qos_tcPrograms contains all programs after they have been loaded into the kernel.
//
// It can be passed to loadQos_tcObjects or ebpf.CollectionSpec.LoadAndAssign.
type qos_tcPrograms struct {
	QosCgroup      *ebpf.Program `ebpf:"qos_cgroup"`
	QosGlobal      *ebpf.Program `ebpf:"qos_global"`
	QosProgEgress  *ebpf.Program `ebpf:"qos_prog_egress"`
	QosProgIngress *ebpf.Program `ebpf:"qos_prog_ingress"`
}

func (p *qos_tcPrograms) Close() error {
	return _Qos_tcClose(
		p.QosCgroup,
		p.QosGlobal,
		p.QosProgEgress,
		p.QosProgIngress,
	)
}

func _Qos_tcClose(closers ...io.Closer) error {
	for _, closer := range closers {
		if err := closer.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Do not access this directly.
//
//go:embed qos_tc_bpfeb.o
var _Qos_tcBytes []byte
