package provider

import (
	"strings"
	"testing"

	providerlib "github.com/roshbhatia/go-utils/provider"
)

func TestRenderValidationsMarksSuccessfulChecks(t *testing.T) {
	t.Parallel()
	output, err := RenderValidations([]Validation{{
		Manifest: manifest("symbols", ActionSymbols, []string{"symbols"}),
		Checks: []providerlib.Check{{
			Kind: "manifest", Target: "symbols", Status: providerlib.CheckOK,
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "+ symbols") || !strings.Contains(output, "+ manifest") {
		t.Fatalf("output = %q", output)
	}
}
