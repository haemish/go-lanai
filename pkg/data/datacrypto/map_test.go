package datacrypto

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/utils"
	"cto-github.cisco.com/NFV-BU/go-lanai/test"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/apptest"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/dbtest"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/suitetest"
	"fmt"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
	"gorm.io/gorm"
	"testing"
)

const (
	testModelNameV1PlainMap        = "v1_plain_map"
	testModelNameV2PlainMap        = "v2_plain_map"
	testModelNameV2InvalidPlainMap = "v2_invalid_plain_map"
	testModelNameV2InvalidVaultMap = "v2_invalid_mock_map"
)

/*************************
	Models
 *************************/

type EncryptedModel struct {
	ID    int    `gorm:"primaryKey;type:serial;"`
	Name  string `gorm:"uniqueIndex;not null;"`
	Value *EncryptedMap
}

func (EncryptedModel) TableName() string {
	return "data_encryption_test"
}

/*************************
	Test Cases
 *************************/

func TestMain(m *testing.M) {
	suitetest.RunTests(m,
		dbtest.EnableDBRecordMode(),
	)
}

type dbDI struct {
	fx.In
	DB *gorm.DB
}

func TestEncryptedMap(t *testing.T) {
	v := map[string]interface{}{
		"key1": "value1",
		"key2": 2.0,
	}
	di := dbDI{}
	test.RunTest(context.Background(), t,
		apptest.Bootstrap(),
		dbtest.WithDBPlayback("testdb"),
		apptest.WithDI(&di),
		test.GomegaSubTest(SubTestMapSuccessfulSqlScan(&di, testModelNameV1PlainMap, expectMap(V1, AlgPlain, v)), "SuccessfulSqlScanWithV1PlainText"),
		test.GomegaSubTest(SubTestMapSuccessfulSqlScan(&di, testModelNameV2PlainMap, expectMap(V2, AlgPlain, v)), "SuccessfulSqlScanWithV2PlainText"),
		test.GomegaSubTest(SubTestMapSuccessfulSqlValue(&di, V1, AlgPlain, v), "SuccessfulSqlValueWithV1PlainText"),
		test.GomegaSubTest(SubTestMapSuccessfulSqlValue(&di, V2, AlgPlain, v), "SuccessfulSqlValueWithV2PlainText"),
		test.GomegaSubTest(SubTestMapFailedSqlScan(&di, testModelNameV2InvalidPlainMap), "FailedSqlScanWithV2PlainText"),
		test.GomegaSubTest(SubTestMapFailedSqlScan(&di, testModelNameV2InvalidVaultMap), "FailedSqlScanWithV2PlainText"),
		test.GomegaSubTest(SubTestMapFailedSqlValue(&di), "FailedSqlValueWithInvalidKeyIDAndAlg"),
		// TODO, failed cases
		// TODO, AlgVault cases
	)
}

/*************************
	Sub-Test Cases
 *************************/

func SubTestMapSuccessfulSqlScan(di *dbDI, name string, expected *testSpecs) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		m := EncryptedModel{}
		r := di.DB.WithContext(ctx).
			Where("Name = ?", name).
			Take(&m)
		g.Expect(r.Error).To(Succeed(), "db select shouldn't return error")
		g.Expect(m.Value).To(Not(BeNil()), "encrypted field shouldn't be nil")
		g.Expect(m.Value.Ver).To(BeIdenticalTo(expected.ver), "encrypted field's data should have correct Ver")
		g.Expect(m.Value.KeyID).To(Not(Equal(uuid.UUID{})), "encrypted field's data should have valid KeyID")
		g.Expect(m.Value.Alg).To(Equal(AlgPlain), "encrypted field's data should have correct Alg")
		if expected.data != nil {
			g.Expect(m.Value.Data).To(Equal(expected.data), "encrypted field's data should have correct Data")
		} else {
			g.Expect(m.Value.Data).To(BeNil(), "encrypted field's data should have correct Data")
		}
	}
}

func SubTestMapFailedSqlScan(di *dbDI, name string) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		m := EncryptedModel{}
		r := di.DB.WithContext(ctx).
			Where("Name = ?", name).
			Take(&m)
		g.Expect(r.Error).To(Not(Succeed()), "db select should return error")
	}
}

func SubTestMapSuccessfulSqlValue(di *dbDI, ver Version, alg Algorithm, v map[string]interface{}) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		kid := uuid.New()
		m := EncryptedModel{
			Name:  fmt.Sprintf("temp_%s_%s", alg, utils.RandomString(8)),
			Value: newEncryptedMap(ver, kid, alg, v),
		}

		r := di.DB.WithContext(ctx).Save(&m)
		g.Expect(r.Error).To(Succeed(), "db save shouldn't return error")
		defer func() {
			di.DB.Delete(&EncryptedModel{}, m.ID)
		}()
		g.Expect(m.Value.Ver).To(BeIdenticalTo(V2), "encrypted field's data Ver should be correct")

		// fetch back
		decrypted := EncryptedModel{}
		r = di.DB.WithContext(ctx).Take(&decrypted, m.ID)
		g.Expect(r.Error).To(Succeed(), "db select shouldn't return error")

		g.Expect(decrypted.Value).To(Not(BeNil()), "decrypted field shouldn't be nil")
		g.Expect(decrypted.Value.Ver).To(BeIdenticalTo(V2), "decrypted field's data should have correct Ver")
		g.Expect(decrypted.Value.KeyID).To(Equal(kid), "decrypted field's data should have correct KeyID")
		g.Expect(decrypted.Value.Alg).To(Equal(alg), "decrypted field's data should have correct Alg")
		if v != nil {
			g.Expect(decrypted.Value.Data).To(Equal(v), "decrypted field's data should have correct Data")
		} else {
			g.Expect(decrypted.Value.Data).To(BeNil(), "decrypted field's data should have correct Data")
		}
	}
}

func SubTestMapFailedSqlValue(di *dbDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		m := EncryptedModel{
			Name:  fmt.Sprintf("temp_invalid_%s", utils.RandomString(8)),
			Value: newEncryptedMap(V2, uuid.UUID{}, AlgVault, nil),
		}
		r := di.DB.WithContext(ctx).Save(&m)
		g.Expect(r.Error).To(Not(Succeed()), "db select should return error")
	}
}

/*************************
	Helper
 *************************/

type testSpecs struct {
	ver Version
	alg Algorithm
	data interface{}
}

func expectMap(ver Version, alg Algorithm, data map[string]interface{}) *testSpecs {
	return &testSpecs{
		ver:  ver,
		alg:  alg,
		data: data,
	}
}