// Package rfc2136 provides a provider that updates records on an
// authoritative name server with dynamic DNS updates (RFC 2136), the protocol
// behind nsupdate. It works with BIND, Knot DNS, PowerDNS, Technitium and any
// other server that accepts updates, without a third-party DNS service in
// between.
//
// Requests are signed with TSIG (RFC 8945) when a key is configured; the reply
// must then carry a valid signature too. Each update is one message that
// deletes the record set at the name and adds the new record, so the server
// applies both or neither.
//
// It is left out of the default build, because the DNS library it uses adds about 0.6 MB to the binary; the full build and
// the "rfc2136" build tag bring it.
//
//dnspatch:extra
package rfc2136

import "github.com/dnspatch/dnspatch/plugin"

// Name is the type name of the provider in the configuration file.
const Name = "rfc2136"

func init() {
	plugin.RegisterProvider(Name, func(cfg Config) (plugin.Provider, error) {
		return newProvider(cfg)
	})
}
