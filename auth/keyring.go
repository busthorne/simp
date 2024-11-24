package auth

import (
	"strings"

	"github.com/busthorne/keyring"
	"github.com/busthorne/simp/config"
)

var rings = map[string]keyring.Keyring{}

// NewKeyring opens a keyring for a given provider.
//
// The providers are only able to access their own keys, so as not to risk
// conflicts between providers.
func NewKeyring(auth config.Auth, provider *config.Provider) (keyring.Keyring, error) {
	id := "simp_" + auth.Name
	ring, ok := rings[id]
	if !ok {
		r, err := keyring.Open(keyring.Config{
			AllowedBackends:                []keyring.BackendType{keyring.BackendType(auth.Backend)},
			ServiceName:                    "simp_" + auth.Name,
			KeyCtlScope:                    "user",
			KeychainAccessibleWhenUnlocked: true,
			KeychainTrustApplication:       true,
			KeychainSynchronizable:         auth.KeychainSynchronizable,
			FileDir:                        auth.FileDir,
			KWalletAppID:                   auth.KWalletAppID,
			KWalletFolder:                  auth.KWalletFolder,
			LibSecretCollectionName:        auth.LibSecretCollectionName,
			PassDir:                        auth.PassDir,
			PassCmd:                        auth.PassCmd,
		})
		if err != nil {
			return nil, err
		}
		ring = r
		rings[id] = r
	}
	k := &Keyring{ring, "", make(keyringCache)}
	if provider != nil {
		k.prefix = provider.Driver + "/" + provider.Name + "/"
	}
	return k, nil
}

type keyringCache = map[string]keyring.Item

// Keyring provides a per-provider view of a keyring.
type Keyring struct {
	ring   keyring.Keyring
	prefix string

	cache keyringCache
}

func (k *Keyring) rewrite(key string) string {
	return strings.TrimPrefix(key, k.prefix)
}

func (k *Keyring) Get(key string) (keyring.Item, error) {
	if item, ok := k.cache[key]; ok {
		return item, nil
	}
	item, err := k.ring.Get(k.prefix + key)
	if err != nil {
		return keyring.Item{}, err
	}
	item.Key = k.rewrite(item.Key)
	k.cache[key] = item
	return item, nil
}

func (k *Keyring) GetMetadata(key string) (keyring.Metadata, error) {
	meta, err := k.ring.GetMetadata(k.prefix + key)
	if err != nil {
		return keyring.Metadata{}, err
	}
	meta.Key = k.rewrite(meta.Key)
	return meta, nil
}

func (k *Keyring) Set(item keyring.Item) error {
	item.Key = k.prefix + item.Key
	err := k.ring.Set(item)
	if err != nil {
		return err
	}
	k.cache[item.Key] = item
	return nil
}

func (k *Keyring) Remove(key string) error {
	delete(k.cache, key)
	return k.ring.Remove(k.prefix + key)
}

func (k *Keyring) Keys() ([]string, error) {
	real, err := k.ring.Keys()
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for _, key := range real {
		if strings.HasPrefix(key, k.prefix) {
			keys = append(keys, k.rewrite(key))
		}
	}
	return keys, nil
}
