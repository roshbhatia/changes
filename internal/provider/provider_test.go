package provider

import "testing"

func TestSupportsAdvertisedCapability(t *testing.T) {
	t.Parallel()
	configured := Manifest{Capabilities: []string{CapabilitySymbols}}
	if !configured.Supports(CapabilitySymbols) || configured.Supports(CapabilityCalls) {
		t.Fatalf("unexpected capabilities: %v", configured.Capabilities)
	}
}

func TestResultPathIncludesProviderAndRequest(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	one, err := resultPath(Manifest{Name: "symbols", Command: []string{"one"}}, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	two, err := resultPath(Manifest{Name: "symbols", Command: []string{"one"}}, []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if one == two {
		t.Fatal("different requests shared one cache entry")
	}
}
