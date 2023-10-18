/*
 * Copyright (c) 2023, Alibaba Group;
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package bpf

import (
	"net/netip"
	"reflect"
	"testing"
)

func Test_ip2Addr(t *testing.T) {
	type args struct {
		ip netip.Addr
	}
	tests := []struct {
		name string
		args args
		want *addr
	}{
		{
			name: "",
			args: args{ip: netip.MustParseAddr("172.16.1.237")},
			want: &addr{
				D1: 0x00000000,
				D2: 0x00000000,
				D3: 0xffff0000,
				D4: 0xed0110ac,
			},
		},
		{
			name: "",
			// net.IP{0x24, 0x8, 0x40, 0x5, 0x3, 0x9c, 0x78, 0x1, 0x10, 0x1, 0xe5, 0xd, 0xbc, 0x3f, 0xe1, 0x16}
			args: args{ip: netip.MustParseAddr("2408:4005:39c:7801:1001:e50d:bc3f:e116")},
			want: &addr{
				D1: 0x05400824,
				D2: 0x01789C03,
				D3: 0x0de50110,
				D4: 0x16e13fbc,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ip2Addr(tt.args.ip); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ip2Addr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_NewMap(t *testing.T) {

}
