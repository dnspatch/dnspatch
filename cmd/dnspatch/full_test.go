//go:build full

package main

import (
	"testing"

	"github.com/dnspatch/dnspatch/plugin"
)

// The full build has every plugin of the source tree, also with dnspatch_none.
func TestFullBuildHasEveryPlugin(t *testing.T) {
	known := plugin.Default.Known()
	if len(known) == 0 {
		t.Fatal("no plugin is declared")
	}

	for _, k := range known {
		if !plugin.Default.Registered(k.Kind, k.Name) {
			t.Errorf("%s %q is missing from the full build", k.Kind, k.Name)
		}
	}
}
