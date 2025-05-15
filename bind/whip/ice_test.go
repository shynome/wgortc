package whip

import (
	"testing"
)

func TestICEParse(t *testing.T) {
	cases := [][]string{
		{"stun://stun.relay.metered.ca:80", "stun:stun.relay.metered.ca:80"},
		{"stun:stun.relay.metered.ca:80", "stun:stun.relay.metered.ca:80"},
	}
	for _, c := range cases {
		ice := ParseICEServer(c[0])
		s := ice.URLs[0]
		if s != c[1] {
			t.Error("解析出错, 预期为: ", c[1], "实际为:", s)
			return
		}
	}
}
