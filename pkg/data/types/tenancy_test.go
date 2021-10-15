package types

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/tenancy"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/utils"
	"cto-github.cisco.com/NFV-BU/go-lanai/test"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/apptest"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/dbtest"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/mocks"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/sectest"
	"fmt"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
	"gorm.io/gorm"
	"testing"
)

var (
	MockedRootTenantId = uuid.MustParse("23967dfe-d90f-4e1b-9406-e2df6685f232")
	MockedTenantIdA    = uuid.MustParse("d8423acc-28cb-4209-95d6-089de7fb27ef")
	MockedTenantIdB    = uuid.MustParse("37b7181a-0892-4706-8f26-60d286b63f14")
	MockedTenantIdA1   = uuid.MustParse("be91531e-ca96-46eb-aea6-b7e0e2a50e21")
	MockedTenantIdA2   = uuid.MustParse("b50c18d9-1741-49bd-8536-30943dfffb45")
	MockedTenantIdB1   = uuid.MustParse("1513b015-6a7d-4de3-8b4f-cbb090ac126d")
	MockedTenantIdB2   = uuid.MustParse("b21445de-9192-45de-acd7-91745ab4cc13")
	MockedModelIDs     = map[uuid.UUID]uuid.UUID{
		MockedRootTenantId: uuid.MustParse("23202b9c-9752-46fa-89ae-9c76277e9bab"),
		MockedTenantIdA:    uuid.MustParse("c60a624a-271a-4a95-96db-9cb7f395f10f"),
		MockedTenantIdB:    uuid.MustParse("435a81e9-1e39-4b66-9211-7cdeea0cda8f"),
		MockedTenantIdA1:   uuid.MustParse("ff9887fb-6809-46f3-b8f1-2de9f4054e36"),
		MockedTenantIdA2:   uuid.MustParse("d1225359-c075-4b0f-ad61-c6e5318f6056"),
		MockedTenantIdB1:   uuid.MustParse("c7547b63-6631-43e0-815d-03bf5e2728a1"),
		MockedTenantIdB2:   uuid.MustParse("2739ee86-d9fb-48f9-9ff9-29f78bfc96c4"),
	}
)

type loadModelFunc func(ctx context.Context, db *gorm.DB, tenantId uuid.UUID, g *gomega.WithT) *TenancyModel

/*************************
	Test
 *************************/

//func TestMain(m *testing.M) {
//	suitetest.RunTests(m,
//		dbtest.EnableDBRecordMode(),
//	)
//}

type testDI struct {
	fx.In
	DB *gorm.DB
}

func TestTenancyEnforcement(t *testing.T) {
	di := &testDI{}
	test.RunTest(context.Background(), t,
		apptest.Bootstrap(),
		dbtest.WithDBPlayback("testdb"),
		apptest.WithModules(tenancy.Module),
		apptest.WithProperties(
			"data.logging.level: debug",
			"log.levels.data: debug",
			"log.levels.bootstrap: warn",
		),
		apptest.WithFxOptions(
			fx.Provide(provideMockedTenancyAccessor),
		),
		apptest.WithDI(di),
		test.SubTestSetup(SetupTestCreateTenancyModels(di)),
		test.GomegaSubTest(SubTestTenancySave(di, loadModelForTenantId), "TestSaveLoadedModel"),
		test.GomegaSubTest(SubTestTenancySave(di, synthesizeModelForTenantId), "TestSaveSynthesizedModel"),
		test.GomegaSubTest(SubTestTenancyUpdates(di), "TestUpdates"),
		test.GomegaSubTest(SubTestTenancyUpdatesWithoutAccess(di), "TestUpdatesWithoutAccess"),
		test.GomegaSubTest(SubTestTenancyDelete(di), "TestDelete"),
	)
}

/*************************
	Sub Tests
 *************************/

func SetupTestCreateTenancyModels(di *testDI) test.SetupFunc {
	return func(ctx context.Context, t *testing.T) (context.Context, error) {
		g := gomega.NewWithT(t)
		prepareTable(di.DB, g)
		table := TenancyModel{}.TableName()
		r := di.DB.Exec(fmt.Sprintf(`TRUNCATE TABLE "%s" RESTRICT`, table))
		g.Expect(r.Error).To(Succeed(), "truncating table of %s should return no error", table)

		reqA := []*TenancyModel{
			newModelWithTenantId(MockedTenantIdA, "Tenant A"),
			newModelWithTenantId(MockedTenantIdA1, "Tenant A-1"),
			newModelWithTenantId(MockedTenantIdA2, "Tenant A-2"),
		}
		reqRoot := []*TenancyModel{
			newModelWithTenantId(MockedTenantIdB, "Tenant B"),
			newModelWithTenantId(MockedTenantIdB1, "Tenant B-1"),
			newModelWithTenantId(MockedTenantIdB2, "Tenant B-2"),
			newModelWithTenantId(MockedRootTenantId, "Root Tenant"),
		}

		// mock security with access to Tenant A only
		secCtx := mockedSecurityWithTenantAccess(ctx, MockedTenantIdA)
		for _, m := range reqA {
			r := di.DB.WithContext(secCtx).Create(m)
			g.Expect(r.Error).To(Succeed(), "creation of model belonging to %s should return no error", m.TenantName)
			g.Expect(m.TenantPath).To(Not(BeEmpty()), "creation of model belonging to %s should populate tenant path", m.TenantName)
		}
		for _, m := range reqRoot {
			r := di.DB.WithContext(secCtx).Create(m)
			g.Expect(r.Error).To(Not(Succeed()), "creation of model belonging to %s should return error due to insufficient access", m.TenantName)
		}

		// mock with access to Root tenant and try previously failed creation again
		secCtx = mockedSecurityWithTenantAccess(ctx, MockedRootTenantId)
		for _, m := range reqRoot {
			r := di.DB.WithContext(secCtx).Create(m)
			g.Expect(r.Error).To(Succeed(), "creation of model belonging to %s should return no error", m.TenantName)
			g.Expect(m.TenantPath).To(Not(BeEmpty()), "creation of model belonging to %s should populate tenant path", m.TenantName)
		}
		return ctx, nil
	}
}

func SubTestTenancySave(di *testDI, loadFn loadModelFunc) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		secCtx := mockedSecurityWithTenantAccess(ctx, MockedTenantIdA, MockedTenantIdA1, MockedTenantIdA2)
		// Save without changing TenantID
		m := loadFn(ctx, di.DB, MockedTenantIdA, g)
		cpy := *m
		cpy.Value = "Updated"
		r := di.DB.WithContext(secCtx).Save(&cpy)
		g.Expect(r.Error).To(Succeed(), "save model belonging to %s should return no error", m.TenantName)
		g.Expect(r.RowsAffected).To(BeEquivalentTo(1), "save model belonging to %s should change 1 row", m.TenantName)
		g.Expect(cpy.TenantPath).To(HaveLen(2), "save model belonging to %s should have correct tenant path", m.TenantName)

		// Save with changed TenantID
		m = loadFn(ctx, di.DB, MockedTenantIdA, g)
		cpy = *m
		cpy.Value = "Updated"
		cpy.TenantID = MockedTenantIdA1 // move to sub tenant
		r = di.DB.WithContext(secCtx).Save(&cpy)
		g.Expect(r.Error).To(Succeed(), "save model belonging to %s should return no error", m.TenantName)
		g.Expect(r.RowsAffected).To(BeEquivalentTo(1), "save model belonging to %s should change 1 row", m.TenantName)
		g.Expect(cpy.TenantPath).To(HaveLen(3), "save model belonging to %s should have correct tenant path", m.TenantName)

		secCtx = mockedSecurityWithTenantAccess(ctx, MockedTenantIdB)
		// insufficient access Save without changing TenantID
		m = loadFn(ctx, di.DB, MockedTenantIdA, g)
		cpy = *m
		cpy.Value = "Updated"
		r = di.DB.WithContext(secCtx).Save(&cpy)
		g.Expect(r.Error).To(Not(Succeed()), "save model belonging to %s should fail due to insufficient access", m.TenantName)

		// insufficient access Save after TenantID changed (model moved to an inaccessible tenant)
		m = loadFn(ctx, di.DB, MockedTenantIdB, g)
		cpy = *m
		cpy.Value = "Updated"
		cpy.TenantID = MockedTenantIdA1 // move to sub tenant
		r = di.DB.WithContext(secCtx).Save(&cpy)
		g.Expect(r.Error).To(Not(Succeed()), "save model belonging to %s should fail due to insufficient access", m.TenantName)
	}
}

func SubTestTenancyUpdates(di *testDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		secCtx := mockedSecurityWithTenantAccess(ctx, MockedTenantIdA1, MockedTenantIdA2)
		// Updates using Map without changing tenant ID
		id := MockedModelIDs[MockedTenantIdA1]
		r := di.DB.WithContext(secCtx).Model(&TenancyModel{ID: id}).
			Updates(map[string]interface{}{"Value": "Updated"})
		m := loadModelWithId(ctx, di.DB, id, g)
		g.Expect(r.Error).To(Succeed(), "update model belonging to %s should return no error", m.TenantName)
		g.Expect(r.RowsAffected).To(BeEquivalentTo(1), "update model belonging to %s should change 1 row", m.TenantName)
		g.Expect(m.Value).To(Equal("Updated"), "updated model belonging to %s should have correct Value", m.TenantName)
		g.Expect(m.TenantPath).To(HaveLen(3), "updated model belonging to %s should have correct tenant path", m.TenantName)

		// Updates using Struct without changing tenant ID
		id = MockedModelIDs[MockedTenantIdA2]
		r = di.DB.WithContext(secCtx).Model(&TenancyModel{ID: id}).
			Updates(&TenancyModel{Value: "Updated"})
		m = loadModelWithId(ctx, di.DB, id, g)
		g.Expect(r.Error).To(Succeed(), "update model belonging to %s should return no error", m.TenantName)
		g.Expect(r.RowsAffected).To(BeEquivalentTo(1), "update model belonging to %s should change 1 row", m.TenantName)
		g.Expect(m.Value).To(Equal("Updated"), "updated model belonging to %s should have correct Value", m.TenantName)
		g.Expect(m.TenantPath).To(HaveLen(3), "updated model belonging to %s should have correct tenant path", m.TenantName)

		// Updates using Map with changed TenantID (move to another tenant)
		secCtx = mockedSecurityWithTenantAccess(ctx, MockedTenantIdA, MockedTenantIdB1, MockedTenantIdB2)
		id = MockedModelIDs[MockedTenantIdB1]
		r = di.DB.WithContext(secCtx).Model(&TenancyModel{ID: id}).
			Updates(map[string]interface{}{"tenant_id": MockedTenantIdA, "Value": "Updated"})
		m = loadModelWithId(ctx, di.DB, id, g)
		g.Expect(r.Error).To(Succeed(), "update model belonging to %s should return no error", m.TenantName)
		g.Expect(r.RowsAffected).To(BeEquivalentTo(1), "update model belonging to %s should change 1 row", m.TenantName)
		g.Expect(m.Value).To(Equal("Updated"), "updated model belonging to %s should have correct Value", m.TenantName)
		g.Expect(m.TenantPath).To(HaveLen(2), "updated model belonging to %s should have correct tenant path", m.TenantName)

		// Updates using Struct with changed TenantID (move to another tenant)
		id = MockedModelIDs[MockedTenantIdB2]
		r = di.DB.WithContext(secCtx).Model(&TenancyModel{ID: id}).
			Updates(&TenancyModel{Tenancy: Tenancy{TenantID: MockedTenantIdA}, Value: "Updated"})
		m = loadModelWithId(ctx, di.DB, id, g)
		g.Expect(r.Error).To(Succeed(), "update model belonging to %s should return no error", m.TenantName)
		g.Expect(r.RowsAffected).To(BeEquivalentTo(1), "update model belonging to %s should change 1 row", m.TenantName)
		g.Expect(m.Value).To(Equal("Updated"), "updated model belonging to %s should have correct Value", m.TenantName)
		g.Expect(m.TenantPath).To(HaveLen(2), "updated model belonging to %s should have correct tenant path", m.TenantName)
	}
}

func SubTestTenancyUpdatesWithoutAccess(di *testDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		secCtx := mockedSecurityWithTenantAccess(ctx, MockedTenantIdB)
		// Updates using Map without changing tenant ID
		id := MockedModelIDs[MockedTenantIdA1]
		r := di.DB.WithContext(secCtx).Model(&TenancyModel{ID: id}).
			Updates(map[string]interface{}{"Value": "Updated"})
		g.Expect(r.RowsAffected).To(BeEquivalentTo(0), "update model belonging to %s should update 0 rows due to insufficient access", MockedTenantIdA1)

		// Updates using Struct without changing tenant ID
		secCtx = mockedSecurityWithTenantAccess(ctx, MockedTenantIdB, MockedTenantIdB1, MockedTenantIdB2)
		id = MockedModelIDs[MockedTenantIdA2]
		r = di.DB.WithContext(secCtx).Model(&TenancyModel{ID: id}).
			Updates(&TenancyModel{Value: "Updated"})
		g.Expect(r.RowsAffected).To(BeEquivalentTo(0), "update model belonging to %s should update 0 rows due to insufficient access", MockedTenantIdA1)

		// Updates using Map with changed TenantID (move to another tenant)
		secCtx = mockedSecurityWithTenantAccess(ctx, MockedTenantIdB1, MockedTenantIdB2)
		id = MockedModelIDs[MockedTenantIdB1]
		r = di.DB.WithContext(secCtx).Model(&TenancyModel{ID: id}).
			Updates(map[string]interface{}{"tenant_id": MockedTenantIdA, "Value": "Updated"})
		g.Expect(r.RowsAffected).To(BeEquivalentTo(0), "update model belonging to %s should update 0 rows due to insufficient access", MockedTenantIdA1)

		// Updates using Struct with changed TenantID (move to another tenant)
		id = MockedModelIDs[MockedTenantIdB2]
		r = di.DB.WithContext(secCtx).Model(&TenancyModel{ID: id}).
			Updates(&TenancyModel{Tenancy: Tenancy{TenantID: MockedTenantIdA}, Value: "Updated"})
		g.Expect(r.RowsAffected).To(BeEquivalentTo(0), "update model belonging to %s should update 0 rows due to insufficient access", MockedTenantIdA1)

	}
}

func SubTestTenancyDelete(di *testDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {

	}
}

/*************************
	Helpers
 *************************/

func populateDefaults(d *sectest.SecurityDetailsMock) {
	d.Username = "any-username"
	d.UserId = "any-user-id"
	d.TenantExternalId = "any-tenant-ext-id"
	d.Permissions = utils.NewStringSet(security.SpecialPermissionSwitchTenant)
}

func mockedSecurityWithTenantAccess(parent context.Context, tenantId ...uuid.UUID) context.Context {
	return sectest.WithMockedSecurity(parent, func(m *sectest.SecurityDetailsMock) {
		populateDefaults(m)
		tidStrs := make([]string, len(tenantId))
		for i, id := range tenantId {
			tidStrs[i] = id.String()
		}
		m.Tenants = utils.NewStringSet(tidStrs...)
		m.TenantId = tidStrs[0]
	})
}

func newModelWithTenantId(tenantId uuid.UUID, value string) *TenancyModel {
	id, ok := MockedModelIDs[tenantId]
	if !ok {
		id = uuid.New()
	}
	return &TenancyModel{
		ID:         id,
		TenantName: value,
		Value:      value,
		Tenancy: Tenancy{
			TenantID: tenantId,
		},
	}
}

func loadModelWithId(ctx context.Context, db *gorm.DB, id uuid.UUID, g *gomega.WithT) *TenancyModel {
	m := TenancyModel{}
	r := db.WithContext(ctx).Take(&m, id)
	g.Expect(r.Error).To(Succeed(), "load model with ID [%v] should return no error", id)
	return &m
}

func loadModelForTenantId(ctx context.Context, db *gorm.DB, tenantId uuid.UUID, g *gomega.WithT) *TenancyModel {
	return loadModelWithId(ctx, db, MockedModelIDs[tenantId], g)
}

func synthesizeModelForTenantId(_ context.Context, _ *gorm.DB, tenantId uuid.UUID, _ *gomega.WithT) *TenancyModel {
	return newModelWithTenantId(tenantId, "")
}

/*************************
	Mocks
 *************************/

func provideMockedTenancyAccessor() tenancy.Accessor {
	tenancyRelationship := []mocks.TenancyRelation{
		{Parent: MockedRootTenantId, Child: MockedTenantIdA},
		{Parent: MockedRootTenantId, Child: MockedTenantIdB},
		{Parent: MockedTenantIdA, Child: MockedTenantIdA1},
		{Parent: MockedTenantIdA, Child: MockedTenantIdA2},
		{Parent: MockedTenantIdB, Child: MockedTenantIdB1},
		{Parent: MockedTenantIdB, Child: MockedTenantIdB2},
	}
	return mocks.NewMockTenancyAccessor(tenancyRelationship, MockedRootTenantId)
}

const tableSQL = `
CREATE TABLE IF NOT EXISTS public.test_tenancy (
	id UUID NOT NULL DEFAULT gen_random_uuid(),
	"tenant_name" STRING NOT NULL,
	"value" STRING NOT NULL,
	tenant_id UUID NULL,
	tenant_path UUID[] NULL,
	created_at TIMESTAMPTZ NULL,
	updated_at TIMESTAMPTZ NULL,
	created_by UUID NULL,
	updated_by UUID NULL,
	deleted_at TIMESTAMPTZ NULL,
	CONSTRAINT "primary" PRIMARY KEY (id ASC)
);`

const idxSQL = `CREATE INDEX IF NOT EXISTS idx_tenant_path ON public.test_tenancy USING GIN (tenant_path);`

func prepareTable(db *gorm.DB, g *gomega.WithT) {
	r := db.Exec(tableSQL)
	g.Expect(r.Error).To(Succeed(), "create table if not exists shouldn't fail")
	r = db.Exec(idxSQL)
	g.Expect(r.Error).To(Succeed(), "create index if not exists shouldn't fail")
}

type TenancyModel struct {
	ID         uuid.UUID `gorm:"primaryKey;type:uuid;default:gen_random_uuid();"`
	TenantName string
	Value      string
	Tenancy
	Audit
	SoftDelete
}

func (TenancyModel) TableName() string {
	return "test_tenancy"
}
