package config

import (
	"net/netip"
)

type Node struct {
	HwTxBpsMax uint64 `json:"hw_tx_bps_max" yaml:"hw_tx_bps_max"`
	HwRxBpsMax uint64 `json:"hw_rx_bps_max" yaml:"hw_rx_bps_max"`
	L0TxBpsMin uint64 `json:"l0_tx_bps_min" yaml:"l0_tx_bps_min"`
	L0TxBpsMax uint64 `json:"l0_tx_bps_max" yaml:"l0_tx_bps_max"`
	L0RxBpsMin uint64 `json:"l0_rx_bps_min" yaml:"l0_rx_bps_min"`
	L0RxBpsMax uint64 `json:"l0_rx_bps_max" yaml:"l0_rx_bps_max"`
	L1TxBpsMin uint64 `json:"l1_tx_bps_min" yaml:"l1_tx_bps_min"`
	L1TxBpsMax uint64 `json:"l1_tx_bps_max" yaml:"l1_tx_bps_max"`
	L1RxBpsMin uint64 `json:"l1_rx_bps_min" yaml:"l1_rx_bps_min"`
	L1RxBpsMax uint64 `json:"l1_rx_bps_max" yaml:"l1_rx_bps_max"`
	L2TxBpsMin uint64 `json:"l2_tx_bps_min" yaml:"l2_tx_bps_min"`
	L2TxBpsMax uint64 `json:"l2_tx_bps_max" yaml:"l2_tx_bps_max"`
	L2RxBpsMin uint64 `json:"l2_rx_bps_min" yaml:"l2_rx_bps_min"`
	L2RxBpsMax uint64 `json:"l2_rx_bps_max" yaml:"l2_rx_bps_max"`
}

type Pod struct {
	PodName      string `json:"podName"`
	PodNamespace string `json:"podNamespace"`
	PodUID       string `json:"podUID"`
	Prio         int    `json:"prio"`

	IPv4 netip.Addr `json:"ipv4"`
	IPv6 netip.Addr `json:"ipv6"`

	HostNetwork bool `json:"hostNetwork"`

	CgroupDir string `json:"cgroupDir"`
}
