#include "common.h"
#include <bpf_endian.h>
#include <bpf_helpers.h>
#include "qos_tc.h"

// cal_rate cal package transferred
static __always_inline void cal_rate(__u64 len)
{
    __u64 now = bpf_ktime_get_ns();
    const __u32 meta_index = 0;
    struct net_stat *meta;

    __u64 index;
    __u64 t_last, last_bytes;
    __u64 total_bytes;
    __u64 cur_rate;

    // use index 0 for the global rate cal
    meta = bpf_map_lookup_elem(&terway_net_stat, &meta_index);
    if (meta == NULL)
    {
        return;
    }

    index = READ_ONCE(meta->index);
    t_last = READ_ONCE(meta->ts);
    last_bytes = READ_ONCE(meta->val);
    if (index == 0)
    {
        // init it
        WRITE_ONCE(meta->index, 1);
        WRITE_ONCE(meta->ts, now);
        WRITE_ONCE(meta->val, 0);
        index++;
    }
    else if (index >= 11) // may be mod ...
    {
        index = 1;
    }

    total_bytes = last_bytes + len;

    // aggriate every 100ms for actual bps
    // as we calculate all packag, this should always be valid...
    if ((now - t_last) < 100 * NSEC_PER_MSEC)
    {
        WRITE_ONCE(meta->val, total_bytes);
        return;
    }

    // always reset meta
    WRITE_ONCE(meta->ts, now);
    WRITE_ONCE(meta->val, 0);

    if ((now - t_last) > 300 * NSEC_PER_MSEC)
    {
        // the rate has no meaning
        cur_rate = 0;
    }
    else
    {
        cur_rate = total_bytes * NSEC_PER_SEC / (now - t_last);
    }

    // set current bps , update by index
    struct net_stat *cur;
    cur = bpf_map_lookup_elem(&terway_net_stat, &index);
    if (cur == NULL)
    {
        return;
    }
    WRITE_ONCE(cur->val, cur_rate);
    WRITE_ONCE(cur->ts, now);

    // move index
    WRITE_ONCE(meta->index, index + 1);
}

static __always_inline __u64 get_average_rate()
{
    __u32 i;
    __u64 now = bpf_ktime_get_ns();
    __u64 cur_rate = 0;
    __u32 index = 1;

    for (i = 1; i < 11; i++)
    {
        struct net_stat *info;
        index = i;

        info = bpf_map_lookup_elem(&terway_net_stat, &index);
        if (info == NULL)
            continue;

        if ((now - READ_ONCE(info->ts)) < 1200 * NSEC_PER_MSEC)
            cur_rate += READ_ONCE(info->val);
    }
    return cur_rate / 10;
}

static __always_inline int edt(struct __sk_buff *skb, struct edt_info *info)
{
    __u64 delay, now, t, t_next;

    now = bpf_ktime_get_ns();
    t = skb->tstamp;
    if (t < now)
        t = now;
    delay = (__u64)skb->wire_len * NSEC_PER_SEC / info->bps;
    t_next = READ_ONCE(info->t_last) + delay;
    if (t_next <= t)
    {
        WRITE_ONCE(info->t_last, t);
        return TC_ACT_OK;
    }

    if (t_next - now >= T_HORIZON_DROP)
        return TC_ACT_SHOT;
    WRITE_ONCE(info->t_last, t_next);
    skb->tstamp = t_next;
    return TC_ACT_OK;
}

static __always_inline int global_edt(struct __sk_buff *skb, struct global_edt_info *rate_info)
{
    __u64 delay, now, t, t_next;

    // edt
    now = bpf_ktime_get_ns();
    t = skb->tstamp;
    if (t < now)
        t = now;

    if (skb->priority == PRIO_ONLINE)
    {
        delay = (__u64)skb->wire_len * NSEC_PER_SEC / rate_info->l0_bps;
        t_next = READ_ONCE(rate_info->t_l0_last) + delay;
        if (t_next <= t)
        {
            WRITE_ONCE(rate_info->t_l0_last, t);
            return TC_ACT_OK;
        }
        if (t_next - now >= T_HORIZON_DROP)
            return TC_ACT_SHOT;
        WRITE_ONCE(rate_info->t_l0_last, t_next);
        skb->tstamp = t_next;
    }
    else if (skb->priority == PRIO_OFFLINE_L1)
    {
        delay = (__u64)skb->wire_len * NSEC_PER_SEC / rate_info->l1_bps;
        t_next = rate_info->t_l1_last + delay;
        if (t_next <= t)
        {
            WRITE_ONCE(rate_info->t_l1_last, t);
            return TC_ACT_OK;
        }
        if (t_next - now >= T_HORIZON_DROP)
            return TC_ACT_SHOT;
        WRITE_ONCE(rate_info->t_l1_last, t_next);
        skb->tstamp = t_next;
    }
    else if (skb->priority == PRIO_OFFLINE_L2)
    {
        delay = (__u64)skb->wire_len * NSEC_PER_SEC / rate_info->l2_bps;
        t_next = rate_info->t_l2_last + delay;
        if (t_next <= t)
        {
            WRITE_ONCE(rate_info->t_l2_last, t);
            return TC_ACT_OK;
        }
        if (t_next - now >= T_HORIZON_DROP)
            return TC_ACT_SHOT;
        WRITE_ONCE(rate_info->t_l2_last, t_next);
        skb->tstamp = t_next;
    }

    return TC_ACT_OK;
}

static __always_inline void adjust_rate(const struct global_rate_cfg *cfg, struct global_edt_info *g_edt_info)
{
    __u64 overflow;
    __u64 now;

    __u64 hw_max, l0_min, l1_min, l1_max, l2_min, l2_max;
    __u64 l0_cur, l1_cur, l2_cur;
    __u64 avg;

    now = bpf_ktime_get_ns();
    hw_max = READ_ONCE(cfg->hw_min_bps) / MEGABYTE;
    l0_min = READ_ONCE(cfg->l0_min_bps) / MEGABYTE;
    l1_min = READ_ONCE(cfg->l1_min_bps) / MEGABYTE;
    l1_max = READ_ONCE(cfg->l1_max_bps) / MEGABYTE;
    l2_min = READ_ONCE(cfg->l2_min_bps) / MEGABYTE;
    l2_max = READ_ONCE(cfg->l2_max_bps) / MEGABYTE;

    l0_cur = READ_ONCE(g_edt_info->l0_bps) / MEGABYTE;
    l1_cur = READ_ONCE(g_edt_info->l1_bps) / MEGABYTE;
    l2_cur = READ_ONCE(g_edt_info->l2_bps) / MEGABYTE;

    if ((now - READ_ONCE(g_edt_info->t_last)) < NSEC_PER_SEC)
        return;

    WRITE_ONCE(g_edt_info->t_last, now);

    avg = get_average_rate() / MEGABYTE;

    if (avg > hw_max)
    {
        overflow = avg - hw_max;

        // suppress l2
        if (l2_cur > l2_min)
        {
            if (overflow >= (l2_cur - l2_min))
            {
                overflow -= (l2_cur - l2_min);
                WRITE_ONCE(g_edt_info->l2_bps, l2_min * MEGABYTE);
            }
            else
            {
                WRITE_ONCE(g_edt_info->l2_bps, (l2_cur - overflow) * MEGABYTE);
                overflow = 0;
            }
        }

        // suppress l1
        if (overflow > 0)
        {
            if (l1_cur > l1_min)
            {
                if (overflow >= (l1_cur - l1_min))
                {
                    overflow -= (l1_cur - l1_min);
                    WRITE_ONCE(g_edt_info->l1_bps, l1_min * MEGABYTE);
                }
                else
                {
                    WRITE_ONCE(g_edt_info->l1_bps, (l1_cur - overflow) * MEGABYTE);
                    overflow = 0;
                }
            }
        }

        // suppress online
        if (overflow > 0)
        {
            // rate_info->online_rate -= overflow;
            if (l0_cur > l0_min)
            {
                if (overflow >= (l0_cur - l0_min))
                {
                    WRITE_ONCE(g_edt_info->l0_bps, l0_min * MEGABYTE);
                }
                else
                {
                    WRITE_ONCE(g_edt_info->l0_bps, (l0_cur - overflow) * MEGABYTE);
                }
            }
        }
    }
    else
    {
        overflow = hw_max - avg;

        if (overflow > 0)
        {
            // recover online
            if (hw_max > l0_cur)
            {
                if (overflow >= (hw_max - l0_cur))
                {
                    overflow -= (hw_max - l0_cur);
                    WRITE_ONCE(g_edt_info->l0_bps, hw_max * MEGABYTE); // never reach here...
                }
                else
                {
                    WRITE_ONCE(g_edt_info->l0_bps, (l0_cur + overflow) * MEGABYTE); // tx-max | 7899412000000  | 100000000      | 18446744073057000000
                    overflow = 0;
                }
            }

            // recover l1
            if (overflow > 0)
            {
                if (l1_max > l1_cur)
                {
                    if (overflow >= (l1_max - l1_cur))
                    {
                        overflow -= (l1_max - l1_cur);
                        WRITE_ONCE(g_edt_info->l1_bps, l1_max * MEGABYTE);
                    }
                    else
                    {
                        WRITE_ONCE(g_edt_info->l1_bps, (l1_cur + overflow) * MEGABYTE);
                        overflow = 0;
                    }
                }
            }

            // recover l2
            if (overflow > 0)
            {
                if (l2_max > l2_cur)
                {
                    if (overflow >= (l2_max - l2_cur))
                    {
                        WRITE_ONCE(g_edt_info->l2_bps, l2_max * MEGABYTE);
                    }
                    else
                    {
                        WRITE_ONCE(g_edt_info->l2_bps, (l2_cur + overflow) * MEGABYTE);
                    }
                }
            }
        }
    }
}

SEC("tc/qos_prog")
int qos_prog(struct __sk_buff *skb)
{
    bpf_tail_call(skb, &qos_prog_map, PROG_TC_CLASSIFY);

    return TC_ACT_OK;
};

int __section("tc/qos_cgroup") qos_cgroup(struct __sk_buff *skb)
{
    // 1. look up ip in cgroup_rate_limit_cfg ( container )
    // 2. host network will not support per pod limit... ,just set class_id as priority
    // 3. for container, will add per pod rate limit

    void *data = (void *)(long)skb->data;
    struct ethhdr *l2 = data;
    struct ip_addr saddr = {0};

    void *data_end = (void *)(long)skb->data_end;
    if (data + sizeof(*l2) > data_end)
    {
        return TC_ACT_OK;
    }

    switch (skb->protocol)
    {
    case bpf_htons(ETH_P_IP):
    {
        struct iphdr *l3;
        l3 = (struct iphdr *)(l2 + 1);
        if ((void *)(l3 + 1) > data_end)
        {
            return TC_ACT_OK;
        }

        saddr.d1 = 0;
        saddr.d2 = 0;
        saddr.d3 = 0xffff0000;
        saddr.d4 = (__u32)l3->saddr;
        break;
    }
    case bpf_htons(ETH_P_IPV6):
    {
        struct ipv6hdr *l3;
        l3 = (struct ipv6hdr *)(l2 + 1);
        if ((void *)(l3 + 1) > data_end)
        {
            return TC_ACT_OK;
        }

        saddr.d1 = (__u32)l3->saddr.in6_u.u6_addr32[0];
        saddr.d2 = (__u32)l3->saddr.in6_u.u6_addr32[1];
        saddr.d3 = (__u32)l3->saddr.in6_u.u6_addr32[2];
        saddr.d4 = (__u32)l3->saddr.in6_u.u6_addr32[3];
        break;
    }
    default:
        return TC_ACT_OK;
    }

    cal_rate(skb->wire_len);

    const struct cgroup_info *pod_cgroup_info = NULL;

    pod_cgroup_info = bpf_map_lookup_elem(&pod_map, &saddr);
    if (pod_cgroup_info == NULL)
    {
        // set classid as priority for host network pods
        skb->priority = bpf_skb_cgroup_classid(skb);
    }
    else
    {
        skb->priority = pod_cgroup_info->class_id;

        // edt for each pod...
        struct cgroup_rate_id rate_id = {0};
        rate_id.inode = pod_cgroup_info->inode;
        rate_id.direction = EGRESS_TRAFFIC;

        struct edt_info *cgroup_rate;

        cgroup_rate = bpf_map_lookup_elem(&cgroup_rate_map, &rate_id);
        if (cgroup_rate != NULL && cgroup_rate->bps > 0)
        {
            int ret = TC_ACT_OK;
            ret = edt(skb, cgroup_rate);
            if (ret != TC_ACT_OK)
            {
                return ret;
            }
        }
    }
    bpf_tail_call(skb, &qos_prog_map, PROG_TC_RATE_LIMIT);

    return TC_ACT_OK;
}

int __section("tc/qos_global") qos_global(struct __sk_buff *skb)
{
    struct global_rate_cfg *g_cfg = NULL;
    struct global_edt_info *g_edt_info = NULL;
    unsigned int index = EGRESS_TRAFFIC;
    int ret = TC_ACT_OK;

    g_cfg = bpf_map_lookup_elem(&terway_global_cfg, &index);
    if (g_cfg == NULL)
        return TC_ACT_OK;

    g_edt_info = bpf_map_lookup_elem(&global_rate_map, &index);
    if (g_edt_info == NULL)
        return TC_ACT_OK;

    if (READ_ONCE(g_edt_info->t_last) == 0)
    {
        WRITE_ONCE(g_edt_info->t_last, bpf_ktime_get_ns());

        WRITE_ONCE(g_edt_info->t_l0_last, 0);
        WRITE_ONCE(g_edt_info->l0_bps, g_cfg->hw_min_bps);

        WRITE_ONCE(g_edt_info->t_l1_last, 0);
        WRITE_ONCE(g_edt_info->l1_bps, g_cfg->l1_max_bps);

        WRITE_ONCE(g_edt_info->t_l2_last, 0);
        WRITE_ONCE(g_edt_info->l2_bps, g_cfg->l2_max_bps);
    }

    ret = global_edt(skb, g_edt_info);
    if (ret != TC_ACT_OK)
    {
        return ret;
    }
    adjust_rate(g_cfg, g_edt_info);

    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
