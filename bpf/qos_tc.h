#include "common.h"
#include <bpf_endian.h>
#include <bpf_helpers.h>

#ifndef __RATE_LIMIT_TC__
#define __RATE_LIMIT_TC__

#undef NSEC_PER_SEC
#undef NSEC_PER_MSEC

#define NSEC_PER_SEC (1000 * 1000 * 1000ULL)
#define NSEC_PER_MSEC (1000 * 1000ULL)

#define T_HORIZON_DROP (2000 * 1000 * 1000ULL)

#define MEGABYTE (1000 * 1000ULL)

#define MAX_PROG 30

#define PRIO_ONLINE 0
#define PRIO_OFFLINE_L1 1
#define PRIO_OFFLINE_L2 2

#define INGRESS_TRAFFIC 0
#define EGRESS_TRAFFIC 1

#define PROG_TC_CGROUP 0
#define PROG_TC_GLOBAL 1

struct rate_info {
	__u64 bps;
	__u64 t_last;
	__u64 slot3;
};

struct global_rate_cfg {
	__u64 interval; // the interval to adjust rate
	__u64 hw_min_bps;
	__u64 hw_max_bps;

	__u64 l0_min_bps;
	__u64 l0_max_bps;

	__u64 l1_min_bps;
	__u64 l1_max_bps;
	__u64 l2_min_bps;
	__u64 l2_max_bps;
};

struct global_rate_info {
	__u64 t_last;

	__u64 t_l0_last;
	__u64 l0_bps;
	__u64 l0_slot;

	__u64 t_l1_last;
	__u64 l1_bps;
	__u64 l1_slot;

	__u64 t_l2_last;
	__u64 l2_bps;
	__u64 l2_slot;
};

struct ip_addr {
	__u32 d1;
	__u32 d2;
	__u32 d3;
	__u32 d4;
};

struct cgroup_info {
	__u32 class_id; // cgroup classid
	__u32 pad1;
	__u64 inode; // cgroup inode id
};

struct cgroup_rate_id {
	__u64 inode;
	__u32 direction;
	__u32 pad;
};

struct net_stat {
	__u64 index;
	__u64 ts;
	__u64 val;
};

/* Global map to jump into terway qos program */
struct {
	__uint(type, BPF_MAP_TYPE_PROG_ARRAY);
	__uint(max_entries, MAX_PROG);
	__uint(key_size, sizeof(__u32));
	__uint(value_size, sizeof(__u32));
	__uint(pinning, LIBBPF_PIN_BY_NAME);
} qos_prog_map SEC(".maps");

/* per pod rate limit begin */

/* Global map for pod config, index by pod ip */
struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(key_size, sizeof(struct ip_addr));
	__uint(value_size, sizeof(struct cgroup_info));
	__uint(max_entries, 65535);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
} pod_map SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(key_size, sizeof(struct cgroup_rate_id));
	__uint(value_size, sizeof(struct rate_info));
	__uint(max_entries, 65535);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
} cgroup_rate_map SEC(".maps");
/* per pod rate limit end */

/* global rate limit begin */
struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(key_size, sizeof(__u32));
	__uint(value_size, sizeof(struct global_rate_cfg));
	__uint(max_entries, 2);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
} terway_global_cfg SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(key_size, sizeof(__u32));
	__uint(value_size, sizeof(struct global_rate_info));
	__uint(max_entries, 2);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
} global_rate_map SEC(".maps");
/* global rate limit end*/

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(key_size, sizeof(__u32));
	__uint(value_size, sizeof(struct net_stat));
	__uint(max_entries, 20);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
} terway_net_stat SEC(".maps");

#endif /* __RATE_LIMIT_TC__ */