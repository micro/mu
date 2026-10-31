package hosts

import (
	"net"
	"testing"
)

func TestPrivateDestinations(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "10.0.0.1", "169.254.169.254", "0.0.0.1", "100.64.0.1", "198.18.0.1", "224.0.0.1", "168.63.129.16", "64:ff9b::7f00:1"} {
		if !PrivateIP(net.ParseIP(s)) {
			t.Errorf("allowed %s", s)
		}
	}
	if PrivateIP(net.ParseIP("93.184.216.34")) {
		t.Fatal("public address blocked")
	}
}
