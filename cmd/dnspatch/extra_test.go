//go:build rfc2136 && !full && !dnspatch_none

package main

import (
	"testing"

	"github.com/dnspatch/dnspatch/plugin"
)

// The tag of a plugin that the plain build leaves out adds that one plugin to
// the plain build, and nothing else.
func TestAnExtraTagAddsOnlyThatPlugin(t *testing.T) {
	for _, k := range plugin.Default.Known() {
		want := !k.Extra || k.Name == "rfc2136"

		if got := plugin.Default.Registered(k.Kind, k.Name); got != want {
			t.Errorf("%s %q registered = %v, want %v", k.Kind, k.Name, got, want)
		}
	}
}
