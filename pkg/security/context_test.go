package security

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/tenancy"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/utils"
	"cto-github.cisco.com/NFV-BU/go-lanai/test"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/apptest"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/mocks"
	"errors"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	"go.uber.org/fx"
	"testing"
	"time"
)

var (
	MockedRootTenantId = uuid.MustParse("23967dfe-d90f-4e1b-9406-e2df6685f232")
	MockedTenantIdA    = uuid.MustParse("d8423acc-28cb-4209-95d6-089de7fb27ef")
)

// Uncomment this function to generate a new copyist sql file to test against - needed when expected db sql commands change

//func TestMain(m *testing.M) {
//	suitetest.RunTests(m,
//		dbtest.EnableDBRecordMode(),
//	)
//}

type contextTestDI struct {
	fx.In
	TA tenancy.Accessor
}

func provideMockedTenancyAccessor() tenancy.Accessor {
	tenancyRelationship := []mocks.TenancyRelation{
		{Parent: MockedRootTenantId, Child: MockedTenantIdA},
	}
	return mocks.NewMockTenancyAccessor(tenancyRelationship, MockedRootTenantId)
}

func TestContext(t *testing.T) {
	di := &contextTestDI{}
	test.RunTest(context.Background(), t,
		apptest.Bootstrap(),
		apptest.WithModules(tenancy.Module),
		apptest.WithTimeout(time.Minute),
		apptest.WithFxOptions(
			fx.Provide(provideMockedTenancyAccessor),
		),
		apptest.WithDI(di),
		test.GomegaSubTest(SubTestHasErrorAccessingTenant(di), "SubTestHasErrorAccessingTenant"),
	)
}

func SubTestHasErrorAccessingTenant(di *contextTestDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		tests := []struct {
			name              string
			tenantId          string
			permission        string
			hasDescendant     bool
			assignedTenantIds utils.StringSet
			expectedErr       error
		}{
			{
				name:          "test invalid tenant id",
				tenantId:      uuid.New().String(),
				permission:    SpecialPermissionAPIAdmin,
				hasDescendant: false,
				expectedErr:   ErrorInvalidTenantId,
			},
			{
				name:          "test has access to all",
				tenantId:      MockedRootTenantId.String(),
				permission:    SpecialPermissionAccessAllTenant,
				hasDescendant: false,
				expectedErr:   nil,
			},
			{
				name:              "test has access to tenant",
				tenantId:          MockedTenantIdA.String(),
				permission:        SpecialPermissionAPIAdmin,
				hasDescendant:     true,
				assignedTenantIds: utils.NewStringSet(MockedTenantIdA.String()),
				expectedErr:       nil,
			},
			{
				name:              "test no access",
				tenantId:          MockedTenantIdA.String(),
				permission:        SpecialPermissionAPIAdmin,
				hasDescendant:     true,
				assignedTenantIds: utils.NewStringSet(),
				expectedErr:       ErrorTenantAccessDenied,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockedAuth := &MockedAccountAuth{
					permissions: map[string]interface{}{tt.permission: struct{}{}},
					details: &MockedUserDetails{
						userId:            uuid.New().String(),
						username:          "test user",
						assignedTenantIds: tt.assignedTenantIds,
					},
				}
				ctx = context.WithValue(ctx, ContextKeySecurity, mockedAuth)
				err := HasErrorAccessingTenant(ctx, tt.tenantId)
				g.Expect(errors.Is(err, tt.expectedErr)).To(gomega.BeTrue())
			})
		}
	}
}