# quick-start

前提条件
- Kubernetes 集群
- helm
- kubectl

## 安装

1. git clone 仓库 `git clone --depth=1 https://github.com/AliyunContainerService/terway-qos.git`
2. 打包 chart 并部署到集群 `helm package ./charts/terway-qos && helm install -nkube-system terway-qos .`
3. 你可以在 ConfigMap 中检查 QoS 配置 `kubectl get cm terway-qos -nkube-system -oyaml`

## 测试 QoS 功能

部署下面的 `YAML` 模板，你将得到三个不同优先级的 Pod。

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

### 测试混部场景限速

#### 无争抢场景

当没有争抢时，`best-effort` 业务可以其定义的最大 `300Mbps` 带宽。

运行下面命令，将单独测试 `best-effort-0` 的带宽。

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

#### 争抢场景

争抢场景下 `guaranteed-0` 业务可以尽可能使用带宽资源，同时也会保留 `best-effort`、`burstable` 业务的最小带宽。

```
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

争抢场景，优先抢占 `best-effort` 业务的带宽。

```
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


## 通过 cli 检查配置

### 查看 Pod 优先级

在 terway-qos 容器内，可以查看节点上容器优先级配置，如下所示：
`class_id` 取值 0-2 ， 0 为最高优先级， 2 为最低优先级。

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

### 查看节点 QoS 配置

在 terway-qos 容器内，可以查看节点 QoS 配置，如下所示：

```shell
/# qos config global get
       | L0        | L1        | L2
Rx-Max | 0         | 0         | 0
Rx-Min | 0         | 0         | 0
Tx-Max | 900000000 | 200000000 | 300000000
Tx-Min | 700000000 | 100000000 | 100000000
```