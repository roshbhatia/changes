package source

import "testing"

func TestContributorsRequireDeclaredAssistance(t *testing.T) {
	for _, test := range []struct {
		trailer string
		ai      bool
	}{
		{"Co-authored-by: Claude <human@example.invalid>", false},
		{"Co-authored-by: Claude <noreply@anthropic.com>", true},
		{"Assisted-by: Codex", true},
		{"Assisted-by: Human", false},
		{"AI-assisted-by: Custom local agent", true},
	} {
		people := Contributors("Human", "human@example.invalid", test.trailer)
		if len(people) != 2 || people[1].AI != test.ai || people[0].AI {
			t.Fatalf("%q: %#v", test.trailer, people)
		}
	}
}
