package linker

import "testing"

// Root-owned paths are trusted like the user's own (root can change anything
// anyway), so links into system locations such as /usr/share keep working.
func TestUntrusted_RootOwnedTrusted(t *testing.T) {
	if reason := untrusted("/", true); reason != "" {
		t.Errorf("untrusted(/) = %q, want trusted", reason)
	}
}
