
#ifndef __LITTLE_ENDIAN_BITFIELD
#define __LITTLE_ENDIAN_BITFIELD
#endif

#include "compiler.h"
#include <linux/ipv6.h>
#include <linux/ip.h>
#include <linux/if_ether.h>
#include <linux/bpf.h>

#ifndef TC_ACT_OK
# define TC_ACT_OK		0
#endif

#ifndef TC_ACT_SHOT
# define TC_ACT_SHOT		2
#endif

#ifndef TC_ACT_PIPE
# define TC_ACT_PIPE		3
#endif
