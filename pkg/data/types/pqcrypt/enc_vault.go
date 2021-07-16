package pqcrypt

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/vault"
	"encoding/json"
	"fmt"
)

// vaultEncryptor implements Encryptor and KeyOperations
type vaultEncryptor struct {
	transit vault.TransitEngine
	props   *KeyProperties
}

func newVaultEncryptor(client *vault.Client, props *KeyProperties) Encryptor {
	return &vaultEncryptor{
		transit: vault.NewTransitEngine(client, func(opt *vault.KeyOption) {
			opt.KeyType = props.Type
			opt.Exportable = props.Exportable
			opt.AllowPlaintextBackup = props.AllowPlaintextBackup
		}),
		props: props,
	}
}

func (enc *vaultEncryptor) Encrypt(ctx context.Context, kid string, v interface{}) (raw *EncryptedRaw, err error) {
	raw = &EncryptedRaw{
		Ver:   V2,
		KeyID: normalizeKeyID(kid),
		Alg:   AlgVault,
	}
	switch {
	case raw.KeyID == "":
		return nil, newEncryptionError("KeyID is required for algorithm %v", raw.Alg)
	}

	if v == nil {
		raw.Raw = "" // special rule encrypted "" <-> nil
		return raw, nil
	}

	jsonVal, e := json.Marshal(v)
	if e != nil {
		return nil, newEncryptionError("failed to marshal data - %v", e)
	}
	cipher, e := enc.transit.Encrypt(ctx, raw.KeyID, jsonVal)
	if e != nil {
		return nil, newEncryptionError("encryption engine - %v", e)
	}
	raw.Raw = string(cipher)
	return
}

func (enc *vaultEncryptor) Decrypt(ctx context.Context, raw *EncryptedRaw, dest interface{}) error {
	switch {
	case raw == nil:
		return newDecryptionError("raw data is nil")
	case raw.Alg != AlgVault:
		return ErrUnsupportedAlgorithm
	case raw.KeyID == "":
		return newDecryptionError("KeyID is required for algorithm %v", raw.Alg)
	}

	switch raw.Ver {
	case V1, V2:
		var cipher []byte
		switch v := raw.Raw.(type) {
		case []byte:
			cipher = v
		case string:
			cipher = []byte(v)
		default:
			return newDecryptionError("invalid ciphertext, expected string, but got %T", raw.Raw)
		}

		if len(cipher) == 0 {
			// special rule encrypted "" <-> nil
			return tryAssign(nil, dest)
		}

		plain, e := enc.transit.Decrypt(ctx, normalizeKeyID(raw.KeyID), cipher)
		if e != nil {
			return newDecryptionError("encryption engine - %v", e)
		}

		if e := json.Unmarshal(plain, dest); e != nil {
			return newDecryptionError("failed to unmarshal decrypted data - %v", e)
		}
	default:
		return ErrUnsupportedVersion
	}
	return nil
}

func (enc *vaultEncryptor) KeyOperations() KeyOperations {
	return enc
}

/* KeyOperations */

func (enc *vaultEncryptor) Create(ctx context.Context, kid string, _ ...KeyOptions) error {
	kid = normalizeKeyID(kid)
	if kid == "" {
		return fmt.Errorf("invalid key ID")
	}
	return enc.transit.PrepareKey(ctx, kid)
}
