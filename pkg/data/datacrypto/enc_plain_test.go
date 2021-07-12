package datacrypto

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/test"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"testing"
)

/*************************
	Test Cases
 *************************/

func TestPlainTextEncryptor(t *testing.T) {
	enc := plainTextEncryptor{}
	mapValue := map[string]interface{}{
		"key1": "value1",
		"key2": 2.0,
	}
	strValue := "this is a string"
	arrValue := []interface{}{"value1", 2.0}
	test.RunTest(context.Background(), t,
		test.GomegaSubTest(SubTestPlainTextEncryptor(enc, V1, mapValue), "PlainTextMapV1"),
		test.GomegaSubTest(SubTestPlainTextEncryptor(enc, V2, mapValue), "PlainTextMapV2"),
		test.GomegaSubTest(SubTestPlainTextEncryptor(enc, V1, strValue), "PlainTextStringV1"),
		test.GomegaSubTest(SubTestPlainTextEncryptor(enc, V2, strValue), "PlainTextStringV2"),
		test.GomegaSubTest(SubTestPlainTextEncryptor(enc, V1, arrValue), "PlainTextSliceV1"),
		test.GomegaSubTest(SubTestPlainTextEncryptor(enc, V2, arrValue), "PlainTextSliceV2"),
		test.GomegaSubTest(SubTestPlainTextEncryptor(enc, V1, nil), "PlainTextNilV1"),
		test.GomegaSubTest(SubTestPlainTextEncryptor(enc, V2, nil), "PlainTextNilV2"),
	)
}

func TestPlainTextFailedEncrypt(t *testing.T) {
	enc := plainTextEncryptor{}
	test.RunTest(context.Background(), t,
		test.GomegaSubTest(SubTestPlainTextFailedEncryption(enc, Version(-1), AlgPlain, ErrUnsupportedVersion), "InvalidVersion"),
		test.GomegaSubTest(SubTestPlainTextFailedEncryption(enc, V1, AlgVault, ErrUnsupportedAlgorithm), "V1UnsupportedAlg"),
		test.GomegaSubTest(SubTestPlainTextFailedEncryption(enc, V2, AlgVault, ErrUnsupportedAlgorithm), "V2UnsupportedAlg"),
	)
}

func TestPlainTextFailedDecrypt(t *testing.T) {
	enc := plainTextEncryptor{}
	m := map[string]interface{}{}
	s := ""
	test.RunTest(context.Background(), t,
		test.GomegaSubTest(SubTestPlainTextFailedDecryption(enc, Version(-1), AlgPlain, ErrUnsupportedVersion), "InvalidVersion"),
		test.GomegaSubTest(SubTestPlainTextFailedDecryption(enc, V1, AlgVault, ErrUnsupportedAlgorithm), "V1UnsupportedAlg"),
		test.GomegaSubTest(SubTestPlainTextFailedDecryption(enc, V2, AlgVault, ErrUnsupportedAlgorithm), "V2UnsupportedAlg"),
		test.GomegaSubTest(SubTestPlainTextTypeMismatch(enc, m), "AssignmentNonPointer"),
		test.GomegaSubTest(SubTestPlainTextTypeMismatch(enc, &s), "AssignmentTypeMismatch"),
	)
}

/*************************
	Sub-Test Cases
 *************************/

func SubTestPlainTextEncryptor(enc Encryptor, ver Version, v interface{}) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		kid := uuid.New()
		raw := EncryptedRaw{
			Ver:   ver,
			KeyID: kid,
			Alg:   AlgPlain,
		}

		// encrypt
		e := enc.Encrypt(ctx, v, &raw)
		g.Expect(e).To(Succeed(), "Encrypt shouldn't return error")
		g.Expect(raw.Ver).To(BeIdenticalTo(V2), "encrypted data should be V2")
		if v != nil {
			g.Expect(raw.Raw).To(Equal(v), "encrypted raw should be correct")
		} else {
			g.Expect(raw.Raw).To(BeNil(), "encrypted raw should be correct")
		}

		// serialize
		bytes, e := json.Marshal(raw)
		g.Expect(e).To(Succeed(), "JSON marshal of raw data shouldn't return error")

		// deserialize
		parsed := EncryptedRaw{}
		e = json.Unmarshal(bytes, &parsed)
		g.Expect(e).To(Succeed(), "JSON unmarshal of raw data shouldn't return error")
		g.Expect(parsed.Ver).To(BeIdenticalTo(V2), "unmarshalled data should be V2")
		g.Expect(parsed.KeyID).To(Equal(kid), "unmarshalled KeyID should be correct")
		g.Expect(parsed.Alg).To(BeIdenticalTo(AlgPlain), "unmarshalled Alg should be correct")
		if v != nil {
			g.Expect(parsed.Raw).To(BeEquivalentTo(v), "unmarshalled Raw should be correct")
		} else {
			g.Expect(parsed.Raw).To(BeNil(), "unmarshalled Raw should be correct")
		}

		// decrypt
		decrypted := interface{}(nil)
		e = enc.Decrypt(ctx, &parsed, &decrypted)
		g.Expect(e).To(Succeed(), "decrypted of raw data shouldn't return error")
		if v != nil {
			g.Expect(decrypted).To(BeEquivalentTo(v), "decrypted value should be correct")
		} else {
			g.Expect(decrypted).To(BeNil(), "decrypted value should be correct")
		}
	}
}

func SubTestPlainTextFailedEncryption(enc Encryptor, ver Version, alg Algorithm, expectedErr error) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *WithT) {
		// encrypt with nil values
		e := enc.Encrypt(ctx, nil, nil)
		g.Expect(e).To(Not(Succeed()), "Encrypt should return error")

		kid := uuid.New()
		raw := EncryptedRaw{
			Ver:   ver,
			KeyID: kid,
			Alg:   alg,
		}

		// encrypt
		any := map[string]string{}
		e = enc.Encrypt(ctx, any, &raw)
		g.Expect(e).To(Not(Succeed()), "Encrypt should return error")
		g.Expect(e).To(BeIdenticalTo(expectedErr), "Encrypt should return correct error")
	}
}

func SubTestPlainTextFailedDecryption(enc Encryptor, ver Version, alg Algorithm, expectedErr error) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *WithT) {
		// decrypt with nil value
		e := enc.Decrypt(ctx, nil, nil)
		g.Expect(e).To(Not(Succeed()), "Decrypt of raw data should return error")

		kid := uuid.New()
		raw := EncryptedRaw{
			Ver:   ver,
			KeyID: kid,
			Alg:   alg,
			Raw:   map[string]interface{}{},
		}

		// decrypt
		decrypted := interface{}(nil)
		e = enc.Decrypt(ctx, &raw, &decrypted)
		g.Expect(e).To(Not(Succeed()), "Decrypt of raw data should return error")
		g.Expect(e).To(BeIdenticalTo(expectedErr), "Encrypt should return correct error")
	}
}

func SubTestPlainTextTypeMismatch(enc Encryptor, v interface{}) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *WithT) {
		encryptor = plainTextEncryptor{}
		kid := uuid.New()
		raw := EncryptedRaw{
			Ver:   V2,
			KeyID: kid,
			Alg:   AlgPlain,
			Raw:   map[string]interface{}{},
		}

		// decrypt
		e := enc.Decrypt(ctx, &raw, v)
		g.Expect(e).To(Not(Succeed()), "Decrypt of raw data should return error")
	}
}
