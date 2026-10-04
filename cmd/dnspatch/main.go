// Command dnspatch is a dynamic DNS daemon: it watches the public IP address
// and patches DNS records when it changes.
//
// A plain "go build" gives the lightweight build: every retriever and provider
// except the heavy ones (rfc2136, yandexcloud, namecheap), the ping_url hook,
// and no notifiers, so a config that publishes to a [notify.<name>] notifier is
// rejected. What goes into a build is chosen with build tags:
//
//	full           every plugin
//	dnspatch_none  no retriever and no provider, except the ones named below
//	<plugin>       one plugin, by its type name: redis, rfc2136, cloudflare, ipify, ...
//
// The tag of a plugin comes from plugins/all, which cmd/genplugins generates,
// and docs/deployment/building.md has the table. A plugin's tag adds it to the
// lightweight build, or, with dnspatch_none, to nothing. The release binaries
// and images come in two flavours: this lightweight one and the -full one,
// built with -tags full.
package main

import (
	"github.com/dnspatch/dnspatch/app"
	_ "github.com/dnspatch/dnspatch/plugins/all"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	app.Main(app.WithVersion(version))
}
