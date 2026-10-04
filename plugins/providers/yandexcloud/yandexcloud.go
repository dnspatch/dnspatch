// Package yandexcloud provides a provider for zones hosted on Yandex Cloud
// DNS, managed through its public REST API
// (https://yandex.cloud/en/docs/dns/api-ref/).
//
// The API has no static key: every call carries an IAM token of a service
// account, valid for at most 12 hours. The provider signs a short-lived JWT
// with the account's authorized key, exchanges it for an IAM token, caches
// the token and renews it when it is close to expiry or a call comes back
// 401.
//
// It is left out of the default build, because its JWT signing code adds about 0.2 MB to the binary; the full build and
// the "yandexcloud" build tag bring it.
//
//dnspatch:extra
package yandexcloud

import "github.com/dnspatch/dnspatch/plugin"

// Name is the type name of the provider in the configuration file.
const Name = "yandexcloud"

func init() {
	plugin.RegisterProvider(Name, func(cfg Config) (plugin.Provider, error) {
		return newProvider(cfg, nil)
	})
}
