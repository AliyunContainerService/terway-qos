# terway-qos

## 介绍

terway-qos 的诞生是为了解决混部场景下，容器网络带宽争抢问题。支持按单Pod、按业务类型限制带宽。

相比于其他方案，terway-qos 有以下优势：

1. 支持按业务类型限制带宽，支持多种业务类型混部
2. 支持 Pod 带宽限制动态调整

## 功能介绍

带宽限制分为

- 整机带宽限制
- Pod带宽限制

### 整机带宽限制

混部场景下，我们期望在线业务有最大带宽的保证，从而避免争抢。在空闲时，离线业务也能尽可能使用全部带宽资源。  
由此用户可为业务流量定义三种优先级，L0，L1，L2。其优先级顺序依次递减。

争抢场景定义： 当 `L0 + L1 + L2` 总流量大于整机带宽

限制策略：

- L0 最大带宽依据 L1， L2 实时流量而动态调整。最大为整机带宽，最小为 `整机带宽- L1 最小带宽- L2 最小带宽`。
- 任何情况下，L1、L2 其带宽不超过各自带宽上限。
- 争抢场景下， L1、L2 其带宽不会低于各自带宽下限。
- 争抢场景下，将按照 L2 、L1 、L0 的顺序对带宽进行限制。

#### Pod 优先级定义

通过为 Pod 配置下面 Annotation

| key                        | 参数                                                                     |
|----------------------------|------------------------------------------------------------------------|
| `k8s.aliyun.com/qos-class` | `guaranteed` 在线业务 L0 <br>`burstable` 离线业务 L1 <br>`best-effort` 离线业务 L2 |

#### 带宽限制配置

对需混部的节点，需配置宽限制，配置路径 `/var/lib/terway/qos/global_bps_config`。

| 配置路径                                    | 参数                                                                                                                                                                                                                                               |
|-----------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `/var/lib/terway/qos/global_bps_config` | `hw_tx_bps_max`  节点的最大tx带宽 <br>`hw_rx_bps_max` 节点的最大rx带宽 <br>`offline_l1_tx_bps_min` 入方向离线l1 业务的最小带宽保证 <br>`offline_l1_tx_bps_max` 入方向离线l1 业务的最大带宽占用 <br>`offline_l2_tx_bps_min` 入方向离线l2 业务的最小带宽保证 <br>`offline_l2_tx_bps_max` 入方向离线l2 业务的最大带宽占用 |

示例如下

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: terway-qos
data:
  global_bps_config: |
    hw_tx_bps_max 900000000
    hw_rx_bps_max 900000000
    offline_l1_tx_bps_min 100000000
    offline_l1_tx_bps_max 200000000
    offline_l2_tx_bps_min 100000000
    offline_l2_tx_bps_max 300000000
    offline_l1_rx_bps_min 100000000
    offline_l1_rx_bps_max 200000000
    offline_l2_rx_bps_min 100000000
    offline_l2_rx_bps_max 300000000
```

> 带宽单位 Bytes/s , 带宽限制精度至少 1MB 以上

### Pod 带宽限制配置

支持 Kubernetes 标准的 Annotation

- `kubernetes.io/egress-bandwidth`
- `kubernetes.io/ingress-bandwidth`

支持热更新 Annotation 来调整 Pod 带宽限制

需注意，CNI 插件可能支持 Kubernetes 标准的 Annotation ，从而会影响热更新，这种情况下可以选择关闭 CNI 插件的带宽限制功能。

## 快速开始

[快速开始](docs/quick-start-zh_CN.md)

## License

terway-qos是由阿里巴巴开发的，采用Apache License（版本2.0）许可证。
本产品包含其他开源许可证下的各种第三方组件。
更多信息请参阅[NOTICE](NOTICE)文件。