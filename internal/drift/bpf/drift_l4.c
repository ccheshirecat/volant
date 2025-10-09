#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/pkt_cls.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <stddef.h>

#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tc.h>

struct portmap_key {
	__u8 proto;
	__u8 pad;
	__be16 port;
};

struct portmap_value {
	__be32 dst_ip;
	__be16 dst_port;
	__u16 pad;
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 4096);
	__type(key, struct portmap_key);
	__type(value, struct portmap_value);
} portmap SEC(".maps");

static __always_inline int rewrite_tcp(struct __sk_buff *skb, struct iphdr *iph, struct tcphdr *tcph, __be32 new_ip, __be16 new_port, __u32 l3_off, __u32 l4_off)
{
	__be16 old_port = tcph->dest;
	__be32 old_ip = iph->daddr;

	if (bpf_l4_csum_replace(skb, l4_off + offsetof(struct tcphdr, check), old_port, new_port, sizeof(new_port)))
		return TC_ACT_OK;

	if (bpf_l4_csum_replace(skb, l4_off + offsetof(struct tcphdr, check), old_ip, new_ip, sizeof(new_ip) | BPF_F_PSEUDO_HDR))
		return TC_ACT_OK;

	if (bpf_l3_csum_replace(skb, l3_off + offsetof(struct iphdr, check), old_ip, new_ip, sizeof(new_ip)))
		return TC_ACT_OK;

	if (bpf_skb_store_bytes(skb, l4_off + offsetof(struct tcphdr, dest), &new_port, sizeof(new_port), 0))
		return TC_ACT_OK;
	if (bpf_skb_store_bytes(skb, l3_off + offsetof(struct iphdr, daddr), &new_ip, sizeof(new_ip), 0))
		return TC_ACT_OK;

	return TC_ACT_OK;
}

static __always_inline int rewrite_udp(struct __sk_buff *skb, struct iphdr *iph, struct udphdr *udph, __be32 new_ip, __be16 new_port, __u32 l3_off, __u32 l4_off)
{
	__be16 old_port = udph->dest;
	__be32 old_ip = iph->daddr;

	if (udph->check) {
		if (bpf_l4_csum_replace(skb, l4_off + offsetof(struct udphdr, check), old_port, new_port, sizeof(new_port)))
			return TC_ACT_OK;
		if (bpf_l4_csum_replace(skb, l4_off + offsetof(struct udphdr, check), old_ip, new_ip, sizeof(new_ip) | BPF_F_PSEUDO_HDR))
			return TC_ACT_OK;
	}

	if (bpf_l3_csum_replace(skb, l3_off + offsetof(struct iphdr, check), old_ip, new_ip, sizeof(new_ip)))
		return TC_ACT_OK;

	if (bpf_skb_store_bytes(skb, l4_off + offsetof(struct udphdr, dest), &new_port, sizeof(new_port), 0))
		return TC_ACT_OK;
	if (bpf_skb_store_bytes(skb, l3_off + offsetof(struct iphdr, daddr), &new_ip, sizeof(new_ip), 0))
		return TC_ACT_OK;

	return TC_ACT_OK;
}

SEC("tc")
int drift_l4_ingress(struct __sk_buff *skb)
{
	void *data = (void *)(long)skb->data;
	void *data_end = (void *)(long)skb->data_end;

	struct ethhdr *eth = data;
	if ((void *)(eth + 1) > data_end)
		return TC_ACT_OK;

	if (eth->h_proto != bpf_htons(ETH_P_IP))
		return TC_ACT_OK;

	struct iphdr *iph = (struct iphdr *)(eth + 1);
	if ((void *)(iph + 1) > data_end)
		return TC_ACT_OK;

	if (iph->ihl < 5)
		return TC_ACT_OK;

	__u8 proto = iph->protocol;
	__u32 ihl_bytes = iph->ihl * 4;
	void *l4 = (void *)iph + ihl_bytes;
	if (l4 + sizeof(__be16) > data_end)
		return TC_ACT_OK;

	__u32 l3_off = (void *)iph - data;
	__u32 l4_off = (void *)l4 - data;

	if (proto == IPPROTO_TCP) {
		struct tcphdr *tcph = l4;
		if ((void *)(tcph + 1) > data_end)
			return TC_ACT_OK;
		struct portmap_key key = {
			.proto = proto,
			.port = tcph->dest,
		};
		struct portmap_value *value = bpf_map_lookup_elem(&portmap, &key);
		if (!value)
			return TC_ACT_OK;
		return rewrite_tcp(skb, iph, tcph, value->dst_ip, value->dst_port, l3_off, l4_off);
	}

	if (proto == IPPROTO_UDP) {
		struct udphdr *udph = l4;
		if ((void *)(udph + 1) > data_end)
			return TC_ACT_OK;
		struct portmap_key key = {
			.proto = proto,
			.port = udph->dest,
		};
		struct portmap_value *value = bpf_map_lookup_elem(&portmap, &key);
		if (!value)
			return TC_ACT_OK;
		return rewrite_udp(skb, iph, udph, value->dst_ip, value->dst_port, l3_off, l4_off);
	}

	return TC_ACT_OK;
}

char LICENSE[] SEC("license") = "Dual BSD/GPL";
