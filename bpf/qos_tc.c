#include "qos_tc.h"
#include "common.h"
#include <bpf_endian.h>
#include <bpf_helpers.h>

static __always_inline void mark_ingress(struct __sk_buff *skb) {
	skb->cb[0] &= 0xfffffff0;
	skb->cb[0] |= 0x8;
}

static __always_inline void mark_egress(struct __sk_buff *skb) {
	skb->cb[0] &= 0xfffffff0;
	skb->cb[0] |= 0x4;
}

// 0 for ingress, 1 for egress
static __always_inline __u32 get_direction(struct __sk_buff *skb) {
	if ((skb->cb[0] & 0x8)) {
		return INGRESS_TRAFFIC;
	}
	return EGRESS_TRAFFIC;
}

static __always_inline __u32 index_shift(__u32 direction) {
	return direction * 10;
}

static __always_inline __u32 ctx_wire_len(struct __sk_buff *skb) {
#if LINUX_VERSION_CODE >= KERNEL_VERSION(5, 0, 0)
 	return skb->wire_len;
#else
    return skb->len;
#endif
}

// cal_rate cal package transferred
static __always_inline void cal_rate(__u64 len, __u32 direction) {
	__u64 now              = bpf_ktime_get_ns();
	const __u32 meta_index = index_shift(direction);
	struct net_stat *meta;

	__u64 index;
	__u64 t_last, last_bytes;
	__u64 total_bytes;
	__u64 cur_rate;

	// use index 0 for the global rate cal
	meta = bpf_map_lookup_elem(&terway_net_stat, &meta_index);
	if (meta == NULL) {
		return;
	}

	index      = READ_ONCE(meta->index);
	t_last     = READ_ONCE(meta->ts);
	last_bytes = READ_ONCE(meta->val);

	if (index % 10 == 0) {
		// init it
		WRITE_ONCE(meta->index, meta_index + 1);
		WRITE_ONCE(meta->ts, now);
		WRITE_ONCE(meta->val, 0);
		index++;
	} else if ((index + 1) % 10 == 1) {
		index = meta_index + 1;
	}

	total_bytes = last_bytes + len;

	// aggregate every 100ms for actual bps
	// as we calculate all package, this should always be valid...
	if ((now - t_last) < 100 * NSEC_PER_MSEC) {
		WRITE_ONCE(meta->val, total_bytes);
		return;
	}

	// always reset meta
	WRITE_ONCE(meta->ts, now);
	WRITE_ONCE(meta->val, 0);

	if ((now - t_last) > 300 * NSEC_PER_MSEC) {
		// the rate has no meaning
		cur_rate = 0;
	} else {
		cur_rate = total_bytes * NSEC_PER_SEC / (now - t_last);
	}

	// set current bps , update by index
	struct net_stat *cur;
	cur = bpf_map_lookup_elem(&terway_net_stat, &index);
	if (cur == NULL) {
		return;
	}
	WRITE_ONCE(cur->val, cur_rate);
	WRITE_ONCE(cur->ts, now);

	// move index
	WRITE_ONCE(meta->index, index + 1);
}

static __always_inline __u64 get_average_rate(__u32 direction) {
	__u32 i;
	__u64 now      = bpf_ktime_get_ns();
	__u64 cur_rate = 0;

#pragma unroll
	for (i = 0; i < 9; i++) {
		struct net_stat *info;
		__u32 index = index_shift(direction) + i;

		info = bpf_map_lookup_elem(&terway_net_stat, &index);
		if (info == NULL)
			continue;

		if ((now - READ_ONCE(info->ts)) < 1000 * NSEC_PER_MSEC)
			cur_rate += READ_ONCE(info->val);
	}
	return cur_rate / 9;
}

static __always_inline int accept(__u64 wire_len, __u64 *tokens, __u64 *t_last, __u64 byte_per_seconds) {
	__u64 now = bpf_ktime_get_ns();
	__u64 t   = *tokens;

	if (byte_per_seconds < MEGABYTE) {
		return TC_ACT_OK;
	}

	__u64 elapsed_time = (now - *t_last) / 1000; // microseconds
	if (elapsed_time > 0) {
		t += (byte_per_seconds * elapsed_time / MEGABYTE);
		if (t > byte_per_seconds) {
			t = byte_per_seconds;
		}
	}

	*t_last = now;

	if (t >= wire_len) {
		t -= wire_len;

		*tokens = t;
		return TC_ACT_OK;
	} else {
		*tokens = t;
		return TC_ACT_SHOT;
	}
}

static __always_inline int tb_rate_limit(struct __sk_buff *skb, struct rate_info *info) {
	__u64 tokens, t_last, byte_per_seconds;
	__u32 rt = 0;

	tokens           = READ_ONCE(info->slot3);
	t_last           = READ_ONCE(info->t_last);
	byte_per_seconds = READ_ONCE(info->bps);

	rt = accept(ctx_wire_len(skb), &tokens, &t_last, byte_per_seconds);

	WRITE_ONCE(info->slot3, tokens);
	WRITE_ONCE(info->t_last, t_last);

	return rt;
}

static __always_inline int global_tb_rate_limit(struct __sk_buff *skb, struct global_rate_info *rate_info) {
	switch (skb->priority) {
	case PRIO_ONLINE: {
		__u64 tokens, t_last, byte_per_seconds;
		__u32 rt = 0;

		tokens           = READ_ONCE(rate_info->l0_slot);
		t_last           = READ_ONCE(rate_info->t_l0_last);
		byte_per_seconds = READ_ONCE(rate_info->l0_bps);

		rt = accept(ctx_wire_len(skb), &tokens, &t_last, byte_per_seconds);

		WRITE_ONCE(rate_info->l0_slot, tokens);
		WRITE_ONCE(rate_info->t_l0_last, t_last);

		return rt;
	}
	case PRIO_OFFLINE_L1: {
		__u64 tokens, t_last, byte_per_seconds;
		__u32 rt = 0;

		tokens           = READ_ONCE(rate_info->l1_slot);
		t_last           = READ_ONCE(rate_info->t_l1_last);
		byte_per_seconds = READ_ONCE(rate_info->l1_bps);

		rt = accept(ctx_wire_len(skb), &tokens, &t_last, byte_per_seconds);

		WRITE_ONCE(rate_info->l1_slot, tokens);
		WRITE_ONCE(rate_info->t_l1_last, t_last);

		return rt;
	}
	case PRIO_OFFLINE_L2: {
		__u64 tokens, t_last, byte_per_seconds;
		__u32 rt = 0;

		tokens           = READ_ONCE(rate_info->l2_slot);
		t_last           = READ_ONCE(rate_info->t_l2_last);
		byte_per_seconds = READ_ONCE(rate_info->l2_bps);

		rt = accept(ctx_wire_len(skb), &tokens, &t_last, byte_per_seconds);

		WRITE_ONCE(rate_info->l2_slot, tokens);
		WRITE_ONCE(rate_info->t_l2_last, t_last);

		return rt;
	}
	}
	return 0;
}

#ifdef FEAT_EDT
static __always_inline int edt(struct __sk_buff *skb, struct rate_info *info) {
	if (info->bps == 0) {
		return TC_ACT_OK;
	}
	__u64 delay, now, t, t_next;

	now = bpf_ktime_get_ns();
	t   = skb->tstamp;
	if (t < now)
		t = now;
	delay  = (__u64)ctx_wire_len(skb) * NSEC_PER_SEC / info->bps;
	t_next = READ_ONCE(info->t_last) + delay;
	if (t_next <= t) {
		WRITE_ONCE(info->t_last, t);
		return TC_ACT_OK;
	}

	if (t_next - now >= T_HORIZON_DROP)
		return TC_ACT_SHOT;

	WRITE_ONCE(info->t_last, t_next);
	skb->tstamp = t_next;
	return TC_ACT_OK;
}

static __always_inline int global_edt(struct __sk_buff *skb, struct global_rate_info *rate_info) {
	__u64 delay, now, t, t_next;

	// edt
	now = bpf_ktime_get_ns();
	t   = skb->tstamp;
	if (t < now)
		t = now;

	if (skb->priority == PRIO_ONLINE) {
		delay = (__u64)ctx_wire_len(skb) * NSEC_PER_SEC / rate_info->l0_bps;

		// if t_last is so big  ? ...
		t_next = READ_ONCE(rate_info->t_l0_last) + delay;
		if (t_next <= t) {
			WRITE_ONCE(rate_info->t_l0_last, t);
			return TC_ACT_OK;
		}
		if (t_next - now >= T_HORIZON_DROP) {
			return TC_ACT_SHOT;
		}

		WRITE_ONCE(rate_info->t_l0_last, t_next);
		skb->tstamp = t_next;
	} else if (skb->priority == PRIO_OFFLINE_L1) {
		delay  = (__u64)ctx_wire_len(skb) * NSEC_PER_SEC / rate_info->l1_bps;
		t_next = rate_info->t_l1_last + delay;
		if (t_next <= t) {
			WRITE_ONCE(rate_info->t_l1_last, t);
			return TC_ACT_OK;
		}
		if (t_next - now >= T_HORIZON_DROP) {
			return TC_ACT_SHOT;
		}
		WRITE_ONCE(rate_info->t_l1_last, t_next);
		skb->tstamp = t_next;
	} else if (skb->priority == PRIO_OFFLINE_L2) {
		delay  = (__u64)ctx_wire_len(skb) * NSEC_PER_SEC / rate_info->l2_bps;
		t_next = rate_info->t_l2_last + delay;
		if (t_next <= t) {
			WRITE_ONCE(rate_info->t_l2_last, t);
			return TC_ACT_OK;
		}
		if (t_next - now >= T_HORIZON_DROP) {
			return TC_ACT_SHOT;
		}
		WRITE_ONCE(rate_info->t_l2_last, t_next);
		skb->tstamp = t_next;
	}

	return TC_ACT_OK;
}
#endif // FEAT_EDT

static __always_inline void adjust_rate(const struct global_rate_cfg *cfg, struct global_rate_info *info, __u32 direction) {
	__u64 overflow;
	__u64 now;

	__u64 hw_max, l0_min, l1_min, l1_max, l2_min, l2_max;
	__u64 l0_cur, l1_cur, l2_cur;
	__u64 avg;

	now    = bpf_ktime_get_ns();
	hw_max = READ_ONCE(cfg->hw_min_bps) / MEGABYTE;
	l0_min = READ_ONCE(cfg->l0_min_bps) / MEGABYTE;
	l1_min = READ_ONCE(cfg->l1_min_bps) / MEGABYTE;
	l1_max = READ_ONCE(cfg->l1_max_bps) / MEGABYTE;
	l2_min = READ_ONCE(cfg->l2_min_bps) / MEGABYTE;
	l2_max = READ_ONCE(cfg->l2_max_bps) / MEGABYTE;

	l0_cur = READ_ONCE(info->l0_bps) / MEGABYTE;
	l1_cur = READ_ONCE(info->l1_bps) / MEGABYTE;
	l2_cur = READ_ONCE(info->l2_bps) / MEGABYTE;

	if ((now - READ_ONCE(info->t_last)) < NSEC_PER_SEC)
		return;

	WRITE_ONCE(info->t_last, now);

	avg = get_average_rate(direction) / MEGABYTE;

	if (avg > hw_max) {
		overflow = avg - hw_max;

		// suppress l2
		if (l2_cur > l2_min) {
			if (overflow >= (l2_cur - l2_min)) {
				overflow -= (l2_cur - l2_min);
				WRITE_ONCE(info->l2_bps, l2_min * MEGABYTE);
			} else {
				WRITE_ONCE(info->l2_bps, (l2_cur - overflow) * MEGABYTE);
				overflow = 0;
			}
		} else {
			WRITE_ONCE(info->l2_bps, l2_min * MEGABYTE);
		}

		// suppress l1
		if (overflow > 0) {
			if (l1_cur > l1_min) {
				if (overflow >= (l1_cur - l1_min)) {
					overflow -= (l1_cur - l1_min);
					WRITE_ONCE(info->l1_bps, l1_min * MEGABYTE);
				} else {
					WRITE_ONCE(info->l1_bps, (l1_cur - overflow) * MEGABYTE);
					overflow = 0;
				}
			} else {
				WRITE_ONCE(info->l1_bps, l1_min * MEGABYTE);
			}
		}

		// suppress online
		if (overflow > 0) {
			// rate_info->online_rate -= overflow;
			if (l0_cur > l0_min) {
				if (overflow >= (l0_cur - l0_min)) {
					WRITE_ONCE(info->l0_bps, l0_min * MEGABYTE);
				} else {
					WRITE_ONCE(info->l0_bps, (l0_cur - overflow) * MEGABYTE);
				}
			} else {
				WRITE_ONCE(info->l0_bps, l0_min * MEGABYTE);
			}
		}
	} else {
		overflow = hw_max - avg;

		if (overflow > 0) {
			// recover online
			if (hw_max > l0_cur) {
				if (overflow >= (hw_max - l0_cur)) {
					overflow -= (hw_max - l0_cur);
					WRITE_ONCE(info->l0_bps, hw_max * MEGABYTE); // never reach here...
				} else {
					WRITE_ONCE(info->l0_bps, (l0_cur + overflow) * MEGABYTE); // tx-max | 7899412000000  | 100000000      | 18446744073057000000
					overflow = 0;
				}
			} else {
				WRITE_ONCE(info->l0_bps, hw_max * MEGABYTE);
			}

			// recover l1
			if (overflow > 0) {
				if (l1_max > l1_cur) {
					if (overflow >= (l1_max - l1_cur)) {
						overflow -= (l1_max - l1_cur);
						WRITE_ONCE(info->l1_bps, l1_max * MEGABYTE);
					} else {
						WRITE_ONCE(info->l1_bps, (l1_cur + overflow) * MEGABYTE);
						overflow = 0;
					}
				} else {
					WRITE_ONCE(info->l1_bps, l1_max * MEGABYTE);
				}
			}

			// recover l2
			if (overflow > 0) {
				if (l2_max > l2_cur) {
					if (overflow >= (l2_max - l2_cur)) {
						WRITE_ONCE(info->l2_bps, l2_max * MEGABYTE);
					} else {
						WRITE_ONCE(info->l2_bps, (l2_cur + overflow) * MEGABYTE);
					}
				} else {
					WRITE_ONCE(info->l2_bps, l2_max * MEGABYTE);
				}
			}
		}
	}
}

SEC("tc/qos_cgroup")
int qos_cgroup(struct __sk_buff *skb) {
	// 1. look up ip in cgroup_rate_limit_cfg ( container )
	// 2. host network will not support per pod limit... ,just set class_id as priority
	// 3. for container, will add per pod rate limit

	void *data          = (void *)(long)skb->data;
	struct ethhdr *l2   = data;
	struct ip_addr addr = {0};

	void *data_end = (void *)(long)skb->data_end;
	if (data + sizeof(*l2) > data_end) {
		return DEFAULT_TC_ACT;
	}

	__u32 direction = get_direction(skb);

	switch (skb->protocol) {
	case bpf_htons(ETH_P_IP): {
		struct iphdr *l3;
		l3 = (struct iphdr *)(l2 + 1);
		if ((void *)(l3 + 1) > data_end) {
			return DEFAULT_TC_ACT;
		}
		if (direction == INGRESS_TRAFFIC) {
			addr.d1 = 0;
			addr.d2 = 0;
			addr.d3 = 0xffff0000;
			addr.d4 = (__u32)l3->daddr;
		} else {
			addr.d1 = 0;
			addr.d2 = 0;
			addr.d3 = 0xffff0000;
			addr.d4 = (__u32)l3->saddr;
		}

		break;
	}
	case bpf_htons(ETH_P_IPV6): {
		struct ipv6hdr *l3;
		l3 = (struct ipv6hdr *)(l2 + 1);
		if ((void *)(l3 + 1) > data_end) {
			return DEFAULT_TC_ACT;
		}

		if (direction == INGRESS_TRAFFIC) {
			addr.d1 = (__u32)l3->daddr.in6_u.u6_addr32[0];
			addr.d2 = (__u32)l3->daddr.in6_u.u6_addr32[1];
			addr.d3 = (__u32)l3->daddr.in6_u.u6_addr32[2];
			addr.d4 = (__u32)l3->daddr.in6_u.u6_addr32[3];
		} else {
			addr.d1 = (__u32)l3->saddr.in6_u.u6_addr32[0];
			addr.d2 = (__u32)l3->saddr.in6_u.u6_addr32[1];
			addr.d3 = (__u32)l3->saddr.in6_u.u6_addr32[2];
			addr.d4 = (__u32)l3->saddr.in6_u.u6_addr32[3];
		}

		break;
	}
	default:
		return DEFAULT_TC_ACT;
	}

	cal_rate(ctx_wire_len(skb), direction);

	const struct cgroup_info *pod_cgroup_info = NULL;

	pod_cgroup_info = bpf_map_lookup_elem(&pod_map, &addr);
	if (pod_cgroup_info == NULL) {
#if LINUX_VERSION_CODE >= KERNEL_VERSION(5, 10, 0)
 		// set classid as priority for host network pods
 		skb->priority = bpf_skb_cgroup_classid(skb);
#endif
	} else {
		skb->priority = pod_cgroup_info->class_id;

		struct cgroup_rate_id rate_id = {0};
		rate_id.inode                 = pod_cgroup_info->inode;
		rate_id.direction             = direction;

		struct rate_info *info = bpf_map_lookup_elem(&cgroup_rate_map, &rate_id);
		if (info != NULL && info->bps > 0) {
			int ret = TC_ACT_OK;

#ifdef FEAT_EDT
			if (direction == INGRESS_TRAFFIC) {
				ret = tb_rate_limit(skb, info);
			} else {
				ret = edt(skb, info);
			}
#else
			ret = tb_rate_limit(skb, info);
#endif
			if (ret != TC_ACT_OK) {
				return ret;
			}
		}
	}
	bpf_tail_call(skb, &qos_prog_map, PROG_TC_GLOBAL);

	return DEFAULT_TC_ACT;
}

SEC("tc/qos_global")
int qos_global(struct __sk_buff *skb) {
	struct global_rate_cfg *g_cfg   = NULL;
	struct global_rate_info *g_info = NULL;
	int ret                         = TC_ACT_OK;
	__u32 direction                 = get_direction(skb);

	// load current level rate info
	g_cfg = bpf_map_lookup_elem(&terway_global_cfg, &direction);
	if (g_cfg == NULL)
		return DEFAULT_TC_ACT;

	g_info = bpf_map_lookup_elem(&global_rate_map, &direction);
	if (g_info == NULL) {
		g_info = &(struct global_rate_info){
			.t_last    = bpf_ktime_get_ns(),
			.t_l0_last = 0,
			.l0_bps    = g_cfg->hw_min_bps,
			.t_l1_last = 0,
			.l1_bps    = g_cfg->l1_max_bps,
			.t_l2_last = 0,
			.l2_bps    = g_cfg->l2_max_bps,
		};
		bpf_map_update_elem(&global_rate_map, &direction, g_info, BPF_NOEXIST);
		return DEFAULT_TC_ACT;
	}

#ifdef FEAT_EDT
	// get priority and do the rate limit
	switch (direction) {
	case INGRESS_TRAFFIC:
		ret = global_tb_rate_limit(skb, g_info);
		break;
	case EGRESS_TRAFFIC:
		ret = global_edt(skb, g_info);
		break;
	}
#else
	ret = global_tb_rate_limit(skb, g_info);
#endif

	if (ret != TC_ACT_OK) {
		return ret;
	}
	adjust_rate(g_cfg, g_info, direction);

	return DEFAULT_TC_ACT;
}

SEC("tc/qos_prog_ingress")
int qos_prog_ingress(struct __sk_buff *skb) {
	mark_ingress(skb);

	bpf_tail_call(skb, &qos_prog_map, PROG_TC_CGROUP);

	return DEFAULT_TC_ACT;
};

SEC("tc/qos_prog_egress")
int qos_prog_egress(struct __sk_buff *skb) {
	mark_egress(skb);

	bpf_tail_call(skb, &qos_prog_map, PROG_TC_CGROUP);

	return DEFAULT_TC_ACT;
};

char _license[] SEC("license") = "GPL";
