package service

import (
	"os"
	"testing"
)

// The default is a single self-contained binary: no ports, no daemon, and
// nothing external able to register.
func TestDefaultRegistryIsInProcess(t *testing.T) {
	t.Setenv("MU_REGISTRY", "")
	if got := registrySpec(); got != "" {
		t.Errorf("registrySpec() = %q, want empty", got)
	}
	if got := advertiseAddress(); got != "127.0.0.1:0" {
		t.Errorf("advertiseAddress() = %q, want loopback", got)
	}
}

// With a networked registry, a service has to advertise an address other hosts
// can dial — loopback would register an address nobody else can reach.
func TestNetworkedRegistryAdvertisesRoutably(t *testing.T) {
	t.Setenv("MU_REGISTRY", "mdns")
	if got := advertiseAddress(); got == "127.0.0.1:0" {
		t.Error("advertising loopback with a networked registry; nothing could reach it")
	}
}

func TestAdvertiseOverride(t *testing.T) {
	t.Setenv("MU_REGISTRY", "mdns")
	t.Setenv("MU_ADVERTISE", "10.0.0.5:9000")
	if got := advertiseAddress(); got != "10.0.0.5:9000" {
		t.Errorf("advertiseAddress() = %q, want the override", got)
	}
}

func TestRegistrySpecIsCaseInsensitive(t *testing.T) {
	t.Setenv("MU_REGISTRY", "  MDNS  ")
	if got := registrySpec(); got != "mdns" {
		t.Errorf("registrySpec() = %q, want mdns", got)
	}
}

// Services() reports what the registry knows, so a service registered by
// another process appears without Mu importing it. Locally hosted services are
// always included even if the registry is unhappy.
func TestServicesIncludesLocallyHosted(t *testing.T) {
	if err := Register("disco-probe", new(EchoSrv)); err != nil {
		t.Fatalf("register: %v", err)
	}
	found := false
	for _, s := range Services() {
		if s == "disco-probe" {
			found = true
		}
	}
	if !found {
		t.Error("locally hosted service missing from Services()")
	}
}

func TestServicesAreSortedAndUnique(t *testing.T) {
	_ = os.Setenv("MU_REGISTRY", "")
	got := Services()
	seen := map[string]bool{}
	for i, s := range got {
		if seen[s] {
			t.Errorf("duplicate service %q", s)
		}
		seen[s] = true
		if i > 0 && got[i-1] > s {
			t.Errorf("not sorted: %q before %q", got[i-1], s)
		}
	}
}
