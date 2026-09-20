package assets

import (
	"testing"

	"goGame/internal/content"
)

// The shipped campaign must satisfy every content validation rule:
// referential integrity, ending rules, and reachability from title.
func TestEmbeddedContentIsValid(t *testing.T) {
	if _, err := content.Load(FS()); err != nil {
		t.Fatalf("embedded content failed validation: %v", err)
	}
}
