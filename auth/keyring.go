package auth

import (
	"context"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"filippo.io/xaes256gcm"
	"github.com/busthorne/keyring"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/books"
	"github.com/busthorne/simp/config"
)

var (
	rings        = map[string]Keyring{}
	keyringCache = map[string]keyring.Item{}
)

func ClearCache() {
	keyringCache = map[string]keyring.Item{}
}

// NewKeyring opens a keyring for a given provider.
//
// The providers are only able to access their own keys, so as not to risk
// conflicts between providers.
func NewKeyring(auth config.Auth, provider *config.Provider) (keyring.Keyring, error) {
	namespace := ""
	if provider != nil {
		namespace = provider.Driver + "." + provider.Name
	}
	if r, ok := rings[auth.Name]; ok {
		r.namespace = namespace
		return &r, nil
	}
	ring, err := keyring.Open(keyring.Config{
		AllowedBackends:                []keyring.BackendType{keyring.BackendType(auth.Backend)},
		ServiceName:                    "simp",
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
		return nil, fmt.Errorf("failed to open keyring %s: %w", auth.Name, err)
	}
	const masterKey = "master_key"
	secretItem, err := ring.Get(masterKey)
	switch err {
	case nil:
	case keyring.ErrKeyNotFound:
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, err
		}
		secretItem = keyring.Item{
			Key:  masterKey,
			Data: secret,
		}
		if err := ring.Set(secretItem); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("failed read master key from %q keyring: %w", auth.Name, err)
	}
	aead, err := xaes256gcm.NewWithManualNonces(secretItem.Data)
	if err != nil {
		return nil, err
	}
	k := Keyring{auth.Name, namespace, aead}
	rings[auth.Name] = k
	return &k, nil
}

// Keyring provides a per-provider view of a keyring.
type Keyring struct {
	ring      string
	namespace string
	aead      cipher.AEAD
}

func (k *Keyring) Get(key string) (item keyring.Item, err error) {
	if item, ok := keyringCache[k.ns(key)]; ok {
		return item, nil
	}
	cv, err := books.Session().KeyringGet(context.Background(), books.KeyringGetParams{
		Ring: k.ring,
		Ns:   k.namespace,
		Key:  key,
	})
	if err != nil {
		return item, err
	}
	item.Key = key
	item.Data, err = k.decrypt(cv)
	if err != nil {
		return item, err
	}
	keyringCache[k.ns(key)] = item
	return item, nil
}

func (k *Keyring) GetMetadata(key string) (keyring.Metadata, error) {
	return keyring.Metadata{}, simp.ErrNotImplemented
}

func (k *Keyring) Set(item keyring.Item) error {
	v, err := k.encrypt(item.Data)
	if err != nil {
		return err
	}
	err = books.Session().KeyringSet(context.Background(), books.KeyringSetParams{
		Ring:  k.ring,
		Ns:    k.namespace,
		Key:   item.Key,
		Value: v,
	})
	if err != nil {
		return err
	}
	keyringCache[k.ns(item.Key)] = item
	return nil
}

func (k *Keyring) Remove(key string) error {
	delete(keyringCache, k.ns(key))
	return books.Session().KeyringDelete(context.Background(), books.KeyringDeleteParams{
		Ring: k.ring,
		Ns:   k.namespace,
		Key:  key,
	})
}

func (k *Keyring) Keys() ([]string, error) {
	return books.Session().KeyringList(context.Background(), books.KeyringListParams{
		Ring: k.ring,
		Ns:   k.namespace,
	})
}

func (k *Keyring) ns(key string) string {
	return k.namespace + "/" + key
}

func (k *Keyring) encrypt(plaintext []byte) ([]byte, error) {
	if k.aead == nil {
		return nil, errors.New("encryption key is not set")
	}
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	seal := k.aead.Seal(nil, nonce, plaintext, nil)
	return append(nonce, seal...), nil
}

func (k *Keyring) decrypt(ciphertext []byte) ([]byte, error) {
	if k.aead == nil {
		return nil, errors.New("encryption key is not set")
	}
	if len(ciphertext) < 24 {
		return nil, errors.New("bad ciphertext")
	}
	nonce, seal := ciphertext[:24], ciphertext[24:]
	return k.aead.Open(nil, nonce, seal, nil)
}
