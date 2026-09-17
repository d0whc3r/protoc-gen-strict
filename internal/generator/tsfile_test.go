package generator

import (
	"slices"
	"testing"
)

// TestDocLines covers the service and method docs: a proto declaration without
// a leading comment used to print an empty `/** */` and a blank ` *` line.
func TestDocLines(t *testing.T) {
	f := &tsFile{}
	f.doc("  ", docLines(""))                               // an undocumented method
	f.doc("", docLines("", "The same service descriptor.")) // an undocumented service

	want := []string{"/** The same service descriptor. */"}
	if !slices.Equal(f.lines, want) {
		t.Errorf("lines = %q, want %q", f.lines, want)
	}
}
