# quick-start

Prerequisites
- Kubernetes cluster
- helm
- kubectl

## install

1. Clone the repository `git clone --depth=1 https://github.com/AliyunContainerService/terway-qos.git`
2. Package the chart and deploy it to the cluster `helm package ./charts/terway-qos && helm install -nkube-system terway-qos .`
3. You can check the QoS configuration in the ConfigMap `kubectl get cm terway-qos -nkube-system -oyaml`

## test the qos work

Deploy the following YAML template to get three Pods with different priorities.

```shell
priority=("server" "burstable" "guaranteed" "best-effort")

for prio in "${priority[@]}"
do
  echo "$prio"
  kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: $prio
spec:
  selector:
    app: $prio
  clusterIP: None 
--- 
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: $prio
spec:
  serviceName: $prio
  selector:
    matchLabels:
      app: $prio
  replicas: 1
  template:
    metadata:
      name: $prio
      labels:
        app: $prio
      annotations:
        k8s.aliyun.com/qos-class: $prio
    spec:
      containers:
        - name: stress
          image: registry.aliyuncs.com/wangbs/netperf:wrk
EOF
done
```

### Test mixed deployment bandwidth limitation

#### No contention scenario

When there is no contention, the "best-effort" business can have a maximum bandwidth of "300MBps" as defined.

Run the following command to test the bandwidth of "best-effort-0" separately.

```shell
kubectl exec -it server-0 -- bash -c "iperf3 -s -p 5000"

kubectl exec -it best-effort-0 -- bash -c "iperf3 -c server -p 5000 -t 5"
```

```shell
root@best-effort-0:/# iperf3 -c server -p 5000 -t 5
Connecting to host server, port 5000
[  4] local 172.16.1.198 port 49186 connected to 172.16.1.202 port 5000
[ ID] Interval           Transfer     Bandwidth       Retr  Cwnd
[  4]   0.00-1.00   sec   281 MBytes  2.36 Gbits/sec    0    652 KBytes
[  4]   1.00-2.00   sec   274 MBytes  2.30 Gbits/sec    0    684 KBytes
[  4]   2.00-3.00   sec   274 MBytes  2.30 Gbits/sec    0    684 KBytes
[  4]   3.00-4.00   sec   274 MBytes  2.30 Gbits/sec    0    684 KBytes
[  4]   4.00-5.00   sec   274 MBytes  2.30 Gbits/sec    0    684 KBytes
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID] Interval           Transfer     Bandwidth       Retr
[  4]   0.00-5.00   sec  1.34 GBytes  2.31 Gbits/sec    0             sender
[  4]   0.00-5.00   sec  1.34 GBytes  2.29 Gbits/sec                  receiver

iperf Done.
```

#### Contention scenario

In a contention scenario, the "guaranteed-0" business can use bandwidth resources as much as possible, while also reserving the minimum bandwidth for the "best-effort" and "burstable" businesses.

```shell
root@best-effort-0:

[ ID] Interval           Transfer     Bandwidth       Retr  Cwnd
[  4]   0.00-1.00   sec   282 MBytes  2.37 Gbits/sec    0    652 KBytes
[  4]   1.00-2.00   sec   274 MBytes  2.30 Gbits/sec    0    652 KBytes
[  4]   2.00-3.00   sec   272 MBytes  2.29 Gbits/sec    0    725 KBytes
[  4]   3.00-4.00   sec   274 MBytes  2.30 Gbits/sec    0    725 KBytes
[  4]   4.00-5.00   sec   270 MBytes  2.26 Gbits/sec    0    911 KBytes
[  4]   5.00-6.00   sec   274 MBytes  2.30 Gbits/sec    0   1.09 MBytes
[  4]   6.00-7.00   sec   272 MBytes  2.29 Gbits/sec    0   1.25 MBytes
[  4]   7.00-8.00   sec   274 MBytes  2.30 Gbits/sec    0   1.25 MBytes
[  4]   8.00-9.00   sec   191 MBytes  1.60 Gbits/sec    0   1.25 MBytes
[  4]   9.00-10.00  sec  91.2 MBytes   765 Mbits/sec    0   1.25 MBytes
[  4]  10.00-11.00  sec  91.2 MBytes   765 Mbits/sec    0   1.25 MBytes
[  4]  11.00-12.00  sec  91.2 MBytes   765 Mbits/sec    0   1.25 MBytes
[  4]  12.00-13.00  sec  91.2 MBytes   765 Mbits/sec    0   1.25 MBytes
[  4]  13.00-14.00  sec  91.2 MBytes   765 Mbits/sec    0   1.25 MBytes
[  4]  14.00-15.00  sec  91.2 MBytes   765 Mbits/sec    0   1.25 MBytes

---
root@guaranteed-0:

[ ID] Interval           Transfer     Bandwidth       Retr  Cwnd
[  4]   0.00-1.00   sec   831 MBytes  6.97 Gbits/sec   11   7.30 MBytes
[  4]   1.00-2.00   sec   711 MBytes  5.97 Gbits/sec    0   7.30 MBytes
[  4]   2.00-3.00   sec   639 MBytes  5.36 Gbits/sec    0   7.30 MBytes
[  4]   3.00-4.00   sec   639 MBytes  5.36 Gbits/sec    0   7.30 MBytes
[  4]   4.00-5.00   sec   639 MBytes  5.36 Gbits/sec    0   7.30 MBytes
[  4]   5.00-6.00   sec   640 MBytes  5.37 Gbits/sec    0   7.30 MBytes
[  4]   6.00-7.00   sec   638 MBytes  5.35 Gbits/sec    0   7.30 MBytes
[  4]   7.00-8.00   sec   640 MBytes  5.37 Gbits/sec    0   7.30 MBytes

---
root@burstable-0:

[ ID] Interval           Transfer     Bandwidth       Retr  Cwnd
[  4]   0.00-1.00   sec   191 MBytes  1.60 Gbits/sec    0    530 KBytes
[  4]   1.00-2.00   sec   181 MBytes  1.52 Gbits/sec    0    648 KBytes
[  4]   2.00-3.00   sec   181 MBytes  1.52 Gbits/sec    0    727 KBytes
[  4]   3.00-4.00   sec   182 MBytes  1.53 Gbits/sec    0    727 KBytes
[  4]   4.00-5.00   sec   182 MBytes  1.53 Gbits/sec    0    727 KBytes
[  4]   5.00-6.00   sec   182 MBytes  1.53 Gbits/sec    0    727 KBytes
[  4]   6.00-7.00   sec   130 MBytes  1.09 Gbits/sec    0    727 KBytes
[  4]   7.00-8.00   sec  91.2 MBytes   765 Mbits/sec    0    727 KBytes
[  4]   8.00-9.00   sec  91.2 MBytes   765 Mbits/sec    0    727 KBytes
[  4]   9.00-10.00  sec  91.2 MBytes   765 Mbits/sec    0    727 KBytes
[  4]  10.00-11.00  sec  91.2 MBytes   765 Mbits/sec    0    727 KBytes
[  4]  11.00-12.00  sec  91.2 MBytes   765 Mbits/sec    0    727 KBytes
[  4]  12.00-13.00  sec  91.2 MBytes   765 Mbits/sec    0    727 KBytes
```

In a contention scenario, the "best-effort" business has the least priority.

```shell
root@best-effort-0:

[  4]  10.00-11.00  sec   274 MBytes  2.30 Gbits/sec    0   1.31 MBytes
[  4]  11.00-12.00  sec   274 MBytes  2.30 Gbits/sec    0   1.31 MBytes
[  4]  12.00-13.00  sec   150 MBytes  1.26 Gbits/sec    0   1.31 MBytes
[  4]  13.00-14.00  sec  96.2 MBytes   807 Mbits/sec    0   1.31 MBytes
[  4]  14.00-15.00  sec  91.2 MBytes   765 Mbits/sec    0   1.31 MBytes
[  4]  15.00-16.00  sec  91.2 MBytes   765 Mbits/sec    0   1.31 MBytes
[  4]  16.00-17.00  sec  91.2 MBytes   765 Mbits/sec    0   1.31 MBytes
[  4]  17.00-18.00  sec  91.2 MBytes   765 Mbits/sec    0   1.31 MBytes
[  4]  18.00-19.00  sec  91.2 MBytes   765 Mbits/sec    0   1.31 MBytes

root@burstable-0:

[  4]   8.00-9.00   sec   182 MBytes  1.53 Gbits/sec    0    792 KBytes
[  4]   9.00-10.00  sec   182 MBytes  1.53 Gbits/sec    0    792 KBytes
[  4]  10.00-11.00  sec   161 MBytes  1.35 Gbits/sec    0    836 KBytes
[  4]  11.00-12.00  sec   136 MBytes  1.14 Gbits/sec    0    836 KBytes
[  4]  12.00-13.00  sec   154 MBytes  1.29 Gbits/sec    0    836 KBytes
[  4]  13.00-14.00  sec   125 MBytes  1.05 Gbits/sec    0    836 KBytes
[  4]  14.00-15.00  sec   120 MBytes  1.01 Gbits/sec    0    836 KBytes
[  4]  15.00-16.00  sec   144 MBytes  1.21 Gbits/sec    0    836 KBytes
[  4]  16.00-17.00  sec   141 MBytes  1.18 Gbits/sec    0    836 KBytes

root@guaranteed-0:

[  4]   0.00-1.00   sec   590 MBytes  4.95 Gbits/sec    0   1.97 MBytes
[  4]   1.00-2.00   sec   585 MBytes  4.91 Gbits/sec    0   2.85 MBytes
[  4]   2.00-3.00   sec   587 MBytes  4.93 Gbits/sec    0   4.31 MBytes
[  4]   3.00-4.00   sec   599 MBytes  5.02 Gbits/sec    0   5.86 MBytes
[  4]   4.00-5.00   sec   618 MBytes  5.18 Gbits/sec    0   4.24 MBytes
[  4]   5.00-6.00   sec   594 MBytes  4.98 Gbits/sec    0   4.50 MBytes
[  4]   6.00-7.00   sec   574 MBytes  4.81 Gbits/sec    0   4.70 MBytes
[  4]   7.00-8.00   sec   618 MBytes  5.19 Gbits/sec    0   4.87 MBytes
[  4]   8.00-9.00   sec   599 MBytes  5.02 Gbits/sec    0   5.05 MBytes
```

## Checking Configuration via CLI

### Viewing Pod Priority

Inside the terway-qos container, you can check the priority configuration of the containers on the node as shown below:
`class_id` takes values from 0 to 2, where 0 represents the highest priority and 2 represents the lowest priority.

```shell
/# qos pod list
ip                                     | class_id | inode
::ffff:172.16.1.192                    | 0        | 39
::ffff:172.16.1.199                    | 0        | 444
2408:4005:***:****:1001:****:****:e13a | 2        | 282
::ffff:172.16.1.198                    | 2        | 282
2408:4005:***:****:1001:****:****:e136 | 0        | 138
::ffff:172.16.1.194                    | 0        | 147
2408:4005:***:****:1001:****:****:e135 | 0        | 147
::ffff:172.16.1.197                    | 0        | 273
2408:4005:***:****:1001:****:****:e13b | 0        | 444
2408:4005:***:****:1001:****:****:e139 | 0        | 273
2408:4005:***:****:1001:****:****:e134 | 0        | 39
::ffff:172.16.1.196                    | 1        | 255
::ffff:172.16.1.193                    | 0        | 138
::ffff:172.16.1.195                    | 0        | 228
2408:4005:***:****:1001:****:****:e138 | 1        | 255
2408:4005:***:****:1001:****:****:e137 | 0        | 228
```

### Viewing Node QoS Configuration

Inside the terway-qos container, you can view the QoS configuration of the node as shown below:

```shell
/# qos config global get
       | L0        | L1        | L2
Rx-Max | 0         | 0         | 0
Rx-Min | 0         | 0         | 0
Tx-Max | 900000000 | 200000000 | 300000000
Tx-Min | 700000000 | 100000000 | 100000000
```