//go:build !full && !dnspatch_none

package main

import (
	"strings"
	"testing"

	"github.com/dnspatch/dnspatch/plugin"
)

// A notifier backend that a build does not carry must fail loudly and say so,
// which is what keeps the plain build small and a config that uses a backend
// from being silently ignored. It holds for any build, whichever notifier tags
// it was made with, so it needs no list of them.
func TestANotifierMissingFromTheBuildSaysSo(t *testing.T) {
	for _, k := range plugin.Default.Known() {
		if k.Kind != plugin.KindNotifier || plugin.Default.Registered(k.Kind, k.Name) {
			continue
		}

		_, err := plugin.Default.BuildNotifier(k.Name, map[string]any{})
		if err == nil || !strings.Contains(err.Error(), "not compiled into this build") {
			t.Errorf("BuildNotifier(%s) error = %v, want it to say the backend is not compiled in", k.Name, err)
		}
	}
}

// The plain build has every plugin that is not marked extra, which is what the
// dnspatch image ships. The extra ones are left to their own tags, so a build
// made with one of them is not an error here.
func TestPlainBuildHasEveryPluginOfTheDefaultBuild(t *testing.T) {
	for _, k := range plugin.Default.Known() {
		if !k.Extra && !plugin.Default.Registered(k.Kind, k.Name) {
			t.Errorf("%s %q is missing from the plain build", k.Kind, k.Name)
		}
	}
}

// The plugins that pull in a large dependency are what separates the plain
// build from the full one; this is the list that the documentation and the
// release notes promise.
func TestTheHeavyPluginsAreExtra(t *testing.T) {
	want := map[string]bool{"rfc2136": true, "yandexcloud": true, "namecheap": true, "redis": true, "rabbitmq": true, "mqtt": true}

	for _, k := range plugin.Default.Known() {
		if k.Extra != want[k.Name] {
			t.Errorf("%s %q extra = %v, want %v", k.Kind, k.Name, k.Extra, want[k.Name])
		}
	}
}
