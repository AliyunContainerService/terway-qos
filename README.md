# qos

## Introduction

terway-qos is developed to solve the problem of container network bandwidth contention in mixed deployment scenarios. It
supports bandwidth limitation based on individual Pods and business types.

Compared to other solutions, terway-qos has the following advantages:

1. Supports bandwidth limitation based on business types, allowing for mixed deployment of multiple business types.
2. Supports dynamic adjustment of Pod bandwidth limitation.

## Functionality

Bandwidth limitation can be divided into:

- Host bandwidth limitation
- Pod bandwidth limitation

### Host bandwidth limitation

In mixed deployment scenarios, we expect to guarantee maximum bandwidth for online business to avoid contention. During
idle periods, offline business should also be able to utilize the full bandwidth resources as much as possible.
For this purpose, users can define three priority levels for business traffic: L0, L1, and L2. The priority order is
L0 > L1 > L2.

Definition of contention scenario: When the total traffic of L0, L1, and L2 exceeds the host bandwidth.

Limitation strategy:

- The maximum bandwidth of L0 is dynamically adjusted based on the real-time traffic of L1 and L2. The maximum value is
  the host bandwidth, and the minimum value is `host bandwidth - minimum L1 bandwidth - minimum L2 bandwidth`.
- Under any circumstances, the bandwidth of L1 and L2 should not exceed their respective upper limits.
- In a contention scenario, the bandwidth of L1 and L2 should not be lower than their respective lower limits.
- In a contention scenario, the bandwidth is limited in the order of L2, L1, and L0.

Supports hot update of annotations to adjust Pod bandwidth limitation.

Please note that the CNI plugin may also support Kubernetes standard annotations, which may affect the hot update. In
this case, you can choose to disable the bandwidth limitation feature of the CNI plugin.

### Pod priority definition

Configure the following annotation for Pods:

| key                        | Parameters                                                                                                                |
|----------------------------|---------------------------------------------------------------------------------------------------------------------------|
| `k8s.aliyun.com/qos-class` | `guaranteed` for online business L0 <br>`burstable` for offline business L1<br>`best-effort` for offline business L2 <br> |

### Bandwidth limitation configuration

For nodes requiring mixed deployment, configure the grace limits in the path `/var/lib/terway/qos/global_bps_config`.

| Configuration Path	                     | Parameters                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
|-----------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `/var/lib/terway/qos/global_bps_config` | `hw_tx_bps_max`  maximum tx bandwidth for the node <br>`hw_rx_bps_max` maximum rx bandwidth for the node  <br>`offline_l1_tx_bps_min` minimum guaranteed bandwidth for inbound L1 offline business <br>`offline_l1_tx_bps_max` maximum bandwidth usage for inbound L1 offline business <br>`offline_l2_tx_bps_min` minimum guaranteed bandwidth for inbound L2 offline business <br>`offline_l2_tx_bps_max` maximum bandwidth usage for inbound L2 offline business |

Here is an example:

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: terway-qos
data:
  global_bps_config: |
    hw_tx_bps_max 900000000
    hw_rx_bps_max 0
    offline_l1_tx_bps_min 100000000
    offline_l1_tx_bps_max 200000000
    offline_l2_tx_bps_min 100000000
    offline_l2_tx_bps_max 300000000
    offline_l1_rx_bps_min 0
    offline_l1_rx_bps_max 0
    offline_l2_rx_bps_min 0
    offline_l2_rx_bps_max 0
```

> The bandwidth unit is Bytes/s, and the bandwidth limitation precision is at least 1MB or higher.

### Pod bandwidth limitation configuration

Supports Kubernetes standard annotations:

- `kubernetes.io/egress-bandwidth`
- `kubernetes.io/ingress-bandwidth`

Supports hot update of annotations to adjust Pod bandwidth limitation.

Please note that the CNI plugin may also support Kubernetes standard annotations, which may affect the hot update. In
this case, you can choose to disable the bandwidth limitation feature of the CNI plugin.

Annotations use bits/s (for example, `80M` means 10,000,000 Bytes/s); file configuration uses Bytes/s.
For each bandwidth direction and priority, annotations take precedence over `pod.json`, which takes precedence over
`per_cgroup_bps_limit` for the same cgroup path. Removing an annotation restores the file value; removing a file
preserves settings from other sources. Invalid file configuration retains the last valid snapshot.

## Testing

Use Go 1.27.3. The project uses Kubernetes libraries v0.37.2 with controller-runtime v0.25.2.

Run `go test -race ./...` on Linux. To also load BPF programs and test real kernel maps, run
`TERWAY_QOS_BPF_TEST=1 go test -race ./...` with BPF privileges. These tests use unpinned maps and do not attach programs
to network interfaces.

To test the controller against an isolated Kubernetes API server and etcd, install the test binaries and run:

```sh
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.25.2
export KUBEBUILDER_ASSETS="$(setup-envtest use 1.37.x -p path)"
make test-api
```

This checks node filtering, Pod creation/deletion, and annotation changes without using an existing cluster.

## License

terway-qos developed by Alibaba Group and licensed under the Apache License (Version 2.0)
This product contains various third-party components under other open source licenses.
See the [NOTICE](NOTICE) file for more information.
