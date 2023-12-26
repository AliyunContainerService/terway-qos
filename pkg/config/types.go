package config

import (
	"net/netip"
)

// Node represents the qos config for a node
// all the unit in the struct is bit per second
type Node struct {
	TotalNetworkBandwidth uint64 `json:"totalNetworkBandwidth"`

	// Leveled is the qos config for each level, which is indexed by koordinator level name extension.QoSClass
	Leveled map[string]QoS `json:"leveled"`
}

type QoS struct {
	IngressRequestBps uint64 `json:"ingressRequestBps"`
	IngressLimitBps   uint64 `json:"ingressLimitBps"`
	EgressRequestBps  uint64 `json:"egressRequestBps"`
	EgressLimitBps    uint64 `json:"egressLimitBps"`
}

type Pod struct {
	PodName      string `json:"podName"`
	PodNamespace string `json:"podNamespace"`
	PodUID       string `json:"podUID"`
	QoSClass     string `json:"qosClass"`

	IPv4 netip.Addr `json:"ipv4"`
	IPv6 netip.Addr `json:"ipv6"`

	HostNetwork bool `json:"hostNetwork"`

	CgroupDir string `json:"cgroupDir"`
}
