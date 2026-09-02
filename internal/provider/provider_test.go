package provider

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

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

func TestValidateRunsSyntheticRequest(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	result := Validate(context.Background(), Manifest{
		Name: "symbols", Command: []string{shell, "-c", "cat >/dev/null; printf '{}'"}, Capabilities: []string{CapabilitySymbols},
	})
	if result.Status != "ok" || len(result.Checks) != 3 {
		t.Fatalf("validation = %+v", result)
	}
}

func TestValidateRejectsInvalidResponse(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	result := Validate(context.Background(), Manifest{
		Name: "calls", Command: []string{shell, "-c", "cat >/dev/null; printf broken"}, Capabilities: []string{CapabilityCalls},
	})
	if result.Status != "failed" || !strings.Contains(result.Checks[len(result.Checks)-1].Message, "invalid JSON") {
		t.Fatalf("validation = %+v", result)
	}
}

func TestValidateRejectsMissingDependency(t *testing.T) {
	result := Validate(context.Background(), Manifest{
		Name: "missing", Command: []string{"sh", "-c", "printf '{}'"}, Capabilities: []string{CapabilitySymbols}, Requires: []string{"changes-command-that-does-not-exist"},
	})
	if result.Status != "failed" || !strings.Contains(result.Checks[len(result.Checks)-1].Message, "unavailable") {
		t.Fatalf("validation = %+v", result)
	}
}
