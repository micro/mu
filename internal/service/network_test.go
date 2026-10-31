package service

import "testing"

func TestNetworkRegistryCannotExposeRPC(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MU_REGISTRY", "mdns")
	t.Setenv("MU_ADVERTISE", ":9000")
	Init()
	if advertiseAddress() != "127.0.0.1:0" {
		t.Fatal("network-facing address advertised")
	}
	if tr.String() != "memory" || reg.String() != "memory" {
		t.Fatal("unauthenticated network transport enabled")
	}
}
