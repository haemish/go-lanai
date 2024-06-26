// Copyright 2023 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package filter_tests

import (
	"context"
	"github.com/cisco-open/go-lanai/pkg/data"
	"github.com/cisco-open/go-lanai/pkg/data/types"
	"github.com/cisco-open/go-lanai/pkg/data/types/pqx"
	"github.com/cisco-open/go-lanai/pkg/opa"
	opadata "github.com/cisco-open/go-lanai/pkg/opa/data"
	"github.com/cisco-open/go-lanai/pkg/opa/data/filter_tests/testdata"
	opatest "github.com/cisco-open/go-lanai/pkg/opa/test"
	"github.com/cisco-open/go-lanai/pkg/tenancy"
	"github.com/cisco-open/go-lanai/test"
	"github.com/cisco-open/go-lanai/test/apptest"
	"github.com/cisco-open/go-lanai/test/dbtest"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
	"gorm.io/gorm"
	"testing"
	"time"
)

/*************************
	Test Setup
 *************************/

//func TestMain(m *testing.M) {
//	suitetest.RunTests(m,
//		dbtest.EnableDBRecordMode(),
//	)
//}

/*************************
	Test
 *************************/

func TestOPAFilterWithCustomConfigPostgresql(t *testing.T) {
	di := &TestDI{}
	test.RunTest(context.Background(), t,
		apptest.Bootstrap(),
		apptest.WithTimeout(10*time.Minute),
		dbtest.WithDBPlayback("testdb", dbtest.DBPort(5432), dbtest.DBCredentials("postgres", "")),
		opatest.WithBundles(opatest.DefaultBundleFS, testdata.ModelCBundleFS),
		apptest.WithModules(tenancy.Module),
		apptest.WithConfigFS(testdata.ConfigFS),
		apptest.WithFxOptions(
			fx.Provide(testdata.ProvideMockedTenancyAccessor),
		),
		apptest.WithDI(di),
		test.SubTestSetup(SetupTestPrepareModelC(&di.DI)),
		test.GomegaSubTest(SubTestModelCCreate(di), "TestModelCreate"),
		test.GomegaSubTest(SubTestModelCList(di), "TestModelList"),
		test.GomegaSubTest(SubTestModelCGet(di), "TestModelGet"),
		test.GomegaSubTest(SubTestModelCUpdate(di), "TestModelUpdate"),
		test.GomegaSubTest(SubTestModelCDelete(di), "TestModelDelete"),
		test.GomegaSubTest(SubTestModelCSave(di), "TestModelSave"),
	)
}

func TestOPAFilterWithCustomConfig(t *testing.T) {
	di := &TestDI{}
	test.RunTest(context.Background(), t,
		apptest.Bootstrap(),
		apptest.WithTimeout(10*time.Minute),
		dbtest.WithDBPlayback("testdb"),
		opatest.WithBundles(opatest.DefaultBundleFS, testdata.ModelCBundleFS),
		apptest.WithModules(tenancy.Module),
		apptest.WithConfigFS(testdata.ConfigFS),
		apptest.WithFxOptions(
			fx.Provide(testdata.ProvideMockedTenancyAccessor),
		),
		apptest.WithDI(di),
		test.SubTestSetup(SetupTestPrepareModelC(&di.DI)),
		test.GomegaSubTest(SubTestModelCCreate(di), "TestModelCreate"),
		test.GomegaSubTest(SubTestModelCList(di), "TestModelList"),
		test.GomegaSubTest(SubTestModelCGet(di), "TestModelGet"),
		test.GomegaSubTest(SubTestModelCUpdate(di), "TestModelUpdate"),
		test.GomegaSubTest(SubTestModelCDelete(di), "TestModelDelete"),
		test.GomegaSubTest(SubTestModelCSave(di), "TestModelSave"),
	)
}

/*************************
	Sub Tests
 *************************/

func SetupTestPrepareModelC(di *dbtest.DI) test.SetupFunc {
	var models []*ModelC
	closure := func(ctx context.Context, db *gorm.DB) {
		resetIdLookup()
		const more = 9
		extra := make([]*ModelC, 0, len(models)*more)
		for _, m := range models {
			key := LookupKey{Tenant: m.TenantID, Owner: m.OwnerID}
			prepareIdLookup(m.ID, key)
			for i := 0; i < more; i++ {
				newM := *m
				newM.ID = uuid.New()
				prepareIdLookup(newM.ID, key)
				extra = append(extra, &newM)
			}
		}
		db.WithContext(ctx).CreateInBatches(extra, 50)
	}
	// We use special DB scope to prepare data, to by-pass policy filtering
	return dbtest.PrepareDataWithScope(di,
		dbtest.SetupWithGormScopes(opadata.SkipFiltering()),
		dbtest.SetupUsingSQLFile(testdata.ModelDataFS, "create_table_c.sql"),
		dbtest.SetupTruncateTables(ModelC{}.TableName()),
		dbtest.SetupUsingModelSeedFile(testdata.ModelDataFS, &models, "model_c.yml", closure),
	)
}

func SubTestModelCCreate(di *TestDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		var model ModelC
		var rs *gorm.DB
		model = ModelC{
			Value:      "test created",
			TenantName: "Tenant A-1",
			OwnerName:  "user1",
			TenantID:   testdata.MockedTenantIdA1,
			TenantPath: pqx.UUIDArray{testdata.MockedRootTenantId, testdata.MockedTenantIdA, testdata.MockedTenantIdA1},
			OwnerID:    testdata.MockedUserId1,
		}
		// user1 - selected tenant branch - with permission - default rule
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdA1), testdata.ExtraPermsSecurityOptions("MANAGE"))
		model.ID = uuid.New()
		rs = di.DB.WithContext(ctx).Create(&model)
		assertDBResult(ctx, g, rs, "create model of selected tenant with strict/default rule", nil, 1)

		// user1 - other tenant branch - with permission - default rule
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdB), testdata.ExtraPermsSecurityOptions("MANAGE"))
		model.ID = uuid.New()
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagCreate, "res.test.allow_create")).
			Create(&model)
		assertDBResult(ctx, g, rs, "create model of non-selected tenant with strict/default rule", opa.ErrAccessDenied, 0)

		// user1 - other tenant branch - relaxed rule - with proper permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdB), testdata.ExtraPermsSecurityOptions("MANAGE_GLOBAL"))
		model.ID = uuid.New()
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagCreate, "res.test.allow_create_alt")).
			Create(&model)
		assertDBResult(ctx, g, rs, "create model of non-selected tenant with relaxed rule", nil, 1)

		// user1 - other tenant branch - no permission - relaxed rule with exception
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdB))
		model.ID = uuid.New()
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagCreate, "res.test.allow_create_alt")).
			Scopes(opadata.FilterWithExtraData("exception", "skip")).
			Create(&model)
		assertDBResult(ctx, g, rs, "create model of non-selected tenant with exception", nil, 1)
	}
}

func SubTestModelCList(di *TestDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		var models []*ModelC
		var rs *gorm.DB
		// user1 - relaxed rule (default, allow_read_alt, all owned models)
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(), testdata.ExtraPermsSecurityOptions("VIEW_GLOBAL"))
		rs = di.DB.WithContext(ctx).Model(&ModelC{}).Find(&models)
		assertDBResult(ctx, g, rs, "list models using relaxed rule", nil, 50)
		g.Expect(models).To(HaveLen(50), "user1 should see %d models", 50)
		assertOwnership(g, testdata.MockedUserId1, "list models using relaxed rule", models...)

		// user1 - strict rule (tenancy)
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions())
		rs = di.DB.WithContext(ctx).Model(&ModelC{}).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagRead, "res.test.filter_read")).
			Find(&models)
		assertDBResult(ctx, g, rs, "list models using strict rule", nil, 10)
		g.Expect(models).To(HaveLen(10), "user1 should see %d models", 10)
		assertOwnership(g, testdata.MockedUserId1, "list models using strict rule", models...)

		// user1 - relaxed rule (exceptions)
		rs = di.DB.WithContext(ctx).Model(&ModelC{}).
			Scopes(opadata.FilterWithExtraData("exception", "skip")).
			Find(&models)
		assertDBResult(ctx, g, rs, "list models using relaxed rule and exceptions", nil, 90)
		g.Expect(models).To(HaveLen(90), "user1 should see %d models", 90)
	}
}

func SubTestModelCGet(di *TestDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		var id uuid.UUID
		var rs *gorm.DB
		// user1 - relaxed rule (default, allow_read_alt, all owned models) - owner, non-selected tenant, but have permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdB), testdata.ExtraPermsSecurityOptions("VIEW_GLOBAL"))
		id = findID(testdata.MockedUserId1, testdata.MockedTenantIdA2)
		rs = di.DB.WithContext(ctx).Take(new(ModelC), id)
		assertDBResult(ctx, g, rs, "owner get model with relaxed rule", nil, 1)

		// user1 - strict rule - owner, non-selected tenant
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdB))
		id = findID(testdata.MockedUserId1, testdata.MockedTenantIdA2)
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagRead, "res.test.filter_read")).
			Take(new(ModelC), id)
		assertDBResult(ctx, g, rs, "owner get model from different tenant with strict rule", data.ErrorRecordNotFound, 0)

		// user1 - strict rule - owner, selected tenant
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdA))
		id = findID(testdata.MockedUserId1, testdata.MockedTenantIdA2)
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagRead, "res.test.filter_read")).
			Take(new(ModelC), id)
		assertDBResult(ctx, g, rs, "owner get model from same tenant with strict rule", nil, 1)

		// user1 - relaxed rule (exception) - non-owner, diff tenant, no permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdB))
		id = findID(testdata.MockedUserId2, testdata.MockedTenantIdA2)
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterWithExtraData("exception", "skip")).
			Take(new(ModelC), id)
		assertDBResult(ctx, g, rs, "get model with permission", nil, 1)
	}
}

func SubTestModelCUpdate(di *TestDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		const NewValue = `Updated`
		var id uuid.UUID
		var rs *gorm.DB
		// user2 - disabled (default) - not owner, not member, no permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User2SecurityOptions())
		id = findIDByOwner(testdata.MockedUserId1)
		rs = di.DB.WithContext(ctx).Model(&ModelC{ID: id}).Updates(&ModelA{Value: NewValue})
		assertDBResult(ctx, g, rs, "update model with disabled filter", nil, 1)
		assertPostOpModel[ModelC](ctx, g, di.DB, id, "update with disabled filter", "Value", NewValue)

		// user2 - enabled (default to filter_write) - not owner, not member, no permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User2SecurityOptions())
		id = findIDByOwner(testdata.MockedUserId1)
		rs = di.DB.WithContext(ctx).Model(&ModelC{ID: id}).
			Scopes(opadata.FilterByPolicies(opadata.DBOperationFlagUpdate)).
			Updates(&ModelC{Value: NewValue})
		assertDBResult(ctx, g, rs, "update model with enabled filter", nil, 0)

		// user1 - enabled (allow_write_alt) - owner, correct permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdA), testdata.ExtraPermsSecurityOptions("MANAGE_GLOBAL"))
		id = findIDByOwner(testdata.MockedUserId1)
		rs = di.DB.WithContext(ctx).Model(&ModelC{ID: id}).
			Scopes(opadata.FilterByPolicies(opadata.DBOperationFlagUpdate)).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagUpdate, "res.test.allow_write_alt")).
			Updates(&ModelC{Value: NewValue})
		assertDBResult(ctx, g, rs, "update model with alternative rule", nil, 1)
		assertPostOpModel[ModelC](ctx, g, di.DB, id, "update model with alternative rule", "Value", NewValue)

		// user1 - enabled (allow_write_alt) - non-owner, no permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdA))
		id = findIDByOwner(testdata.MockedUserId2)
		rs = di.DB.WithContext(ctx).Model(&ModelC{ID: id}).
			Scopes(opadata.FilterByPolicies(opadata.DBOperationFlagUpdate)).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagUpdate, "res.test.allow_write_alt")).
			Updates(&ModelC{Value: NewValue})
		assertDBResult(ctx, g, rs, "update model with alternative rule", nil, 0)

		// user2 - enabled (allow_write_alt, exception) -  not owner, not member, no permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdA))
		id = findIDByOwner(testdata.MockedUserId2)
		rs = di.DB.WithContext(ctx).Model(&ModelC{ID: id}).
			Scopes(opadata.FilterByPolicies(opadata.DBOperationFlagUpdate)).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagUpdate, "res.test.allow_write_alt")).
			Scopes(opadata.FilterWithExtraData("exception", "skip")).
			Updates(&ModelC{Value: NewValue})
		assertDBResult(ctx, g, rs, "update model with alternative rule and exception", nil, 1)
		assertPostOpModel[ModelC](ctx, g, di.DB, id, "update model with alternative rule and exception", "Value", NewValue)
	}
}

func SubTestModelCDelete(di *TestDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		var id uuid.UUID
		var rs *gorm.DB
		// user1 - default rule (filter_delete) - not owner, not member, but have permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(), testdata.ExtraPermsSecurityOptions("MANAGE"))
		id = findID(testdata.MockedUserId2, testdata.MockedTenantIdB1)
		rs = di.DB.WithContext(ctx).Delete(&ModelC{ID: id})
		assertDBResult(ctx, g, rs, "delete model of other tenant with permission", nil, 1)
		assertPostOpModel[ModelC](ctx, g, di.DB, id, "delete model of other tenant with permission")

		// user1 - alternative rule (allow_delete_alt) - not owner, not member, but have permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(), testdata.ExtraPermsSecurityOptions("MANAGE"))
		id = findID(testdata.MockedUserId2, testdata.MockedTenantIdB1)
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagDelete, "res.test.allow_delete_alt")).
			Delete(&ModelC{ID: id})
		assertDBResult(ctx, g, rs, "delete model of other tenant", nil, 0)
		assertPostOpModel[ModelC](ctx, g, di.DB, id, "delete model of other tenant", "exists")

		// user1 - alternative rule (allow_delete_alt, exception) - not owner, not member, but have permission
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(), testdata.ExtraPermsSecurityOptions("MANAGE"))
		id = findID(testdata.MockedUserId2, testdata.MockedTenantIdB1)
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagDelete, "res.test.allow_delete_alt")).
			Scopes(opadata.FilterWithExtraData("exception", "skip")).
			Delete(&ModelC{ID: id})
		assertDBResult(ctx, g, rs, "delete model of other tenant with exception", nil, 1)
		assertPostOpModel[ModelC](ctx, g, di.DB, id, "delete model of other tenant with exception")
	}
}

func SubTestModelCSave(di *TestDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		const NewValue = `Saved`
		newModelTmpl := ModelC{
			Value:      "test created",
			TenantName: "Tenant A-1",
			OwnerName:  "user1",
			TenantID:   testdata.MockedTenantIdA1,
			TenantPath: pqx.UUIDArray{testdata.MockedRootTenantId, testdata.MockedTenantIdA, testdata.MockedTenantIdA1},
			OwnerID:    testdata.MockedUserId1,
		}

		var id uuid.UUID
		var model *ModelC
		var rs *gorm.DB
		// user1 - create:strict, update:relaxed (allow same tenant create, diff tenant update) - same tenant update
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdA), testdata.ExtraPermsSecurityOptions("MANAGE_GLOBAL"))
		id = findID(testdata.MockedUserId1, testdata.MockedTenantIdA2)
		model = mustLoadModel[ModelC](ctx, g, di.DB, id)
		model.Value = NewValue
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterByPolicies(opadata.DBOperationFlagCreate, opadata.DBOperationFlagUpdate)).
			Scopes(opadata.FilterWithQueries(
				opadata.DBOperationFlagUpdate, "res.test.allow_write_alt",
				opadata.DBOperationFlagCreate, "res.test.allow_create",
			)).
			Save(model)
		assertDBResult(ctx, g, rs, "save existing model in same tenant as owner", nil, 1)
		assertPostOpModel[ModelC](ctx, g, di.DB, id, "save existing model in same tenant as owner", "Value", NewValue)

		// user1 - create:strict, update:relaxed (allow same tenant create, diff tenant update) - same tenant create
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdA), testdata.ExtraPermsSecurityOptions("MANAGE"))
		m := newModelTmpl
		model = &m
		model.Value = NewValue
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterByPolicies(opadata.DBOperationFlagCreate, opadata.DBOperationFlagUpdate)).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagUpdate, "res.test.allow_write_alt")).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagCreate, "res.test.allow_create")).
			Save(model)
		assertDBResult(ctx, g, rs, "save new model in same tenant as owner", nil, 1)
		assertPostOpModel[ModelC](ctx, g, di.DB, id, "save new model in same tenant as owner", "Value", NewValue)

		// user1 - create:strict, update:relaxed - different tenant update without permission (exception)
		ctx = testdata.ContextWithSecurityMock(ctx, testdata.User1SecurityOptions(testdata.MockedTenantIdB))
		id = findID(testdata.MockedUserId1, testdata.MockedTenantIdA2)
		model = mustLoadModel[ModelC](ctx, g, di.DB, id)
		model.Value = NewValue
		rs = di.DB.WithContext(ctx).
			Scopes(opadata.FilterByPolicies(opadata.DBOperationFlagCreate, opadata.DBOperationFlagUpdate)).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagUpdate, "res.test.allow_write_alt")).
			Scopes(opadata.FilterWithQueries(opadata.DBOperationFlagCreate, "res.test.allow_create")).
			Scopes(opadata.FilterWithExtraData("exception", "skip")).
			Save(model)
		assertDBResult(ctx, g, rs, "save existing model without permission but exempted", nil, 1)
		assertPostOpModel[ModelC](ctx, g, di.DB, id, "save existing model without permission but exempted", "Value", NewValue)
	}
}

/*************************
	Helpers
 *************************/

/*************************
	Models
 *************************/

type ModelC struct {
	ID                    uuid.UUID `gorm:"primaryKey;type:uuid;default:gen_random_uuid();"`
	Value                 string
	TenantName            string
	OwnerName             string
	TenantID              uuid.UUID     `gorm:"type:KeyID;not null" opa:"field:tenant_id"`
	TenantPath            pqx.UUIDArray `gorm:"type:uuid[];index:,type:gin;not null" opa:"field:tenant_path"`
	OwnerID               uuid.UUID     `gorm:"type:KeyID;not null" opa:"field:owner_id"`
	opadata.FilteredModel `opa:"type:model, package:res.test, read:allow_read_alt, update:-"`
	types.Audit
}

func (ModelC) TableName() string {
	return "test_opa_model_c"
}
