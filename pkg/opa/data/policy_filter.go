package opadata

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/data"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/data/types"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/opa"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

/****************************
	Func
 ****************************/

/****************************
	Types
 ****************************/

// PolicyFilter implements
// - schema.GormDataTypeInterface
// - schema.QueryClausesInterface
// - schema.UpdateClausesInterface
// - schema.DeleteClausesInterface
// - schema.CreateClausesInterface
// this data type adds "WHERE" clause for tenancy filtering
type PolicyFilter struct {}

// QueryClauses implements schema.QueryClausesInterface,
func (pf PolicyFilter) QueryClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{newPolicyFilterClause(f, PolicyFlagRead)}
}

// UpdateClauses implements schema.UpdateClausesInterface,
func (pf PolicyFilter) UpdateClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{newPolicyFilterClause(f, PolicyFlagUpdate)}
}

// DeleteClauses implements schema.DeleteClausesInterface,
func (pf PolicyFilter) DeleteClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{newPolicyFilterClause(f, PolicyFlagDelete)}
}

// CreateClauses implements schema.CreateClausesInterface,
func (pf PolicyFilter) CreateClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{newPolicyFilterClause(f, PolicyFlagCreate)}
}

/***************************
	Read, Update, Delete
 ***************************/

// policyFilterClause implements clause.Interface and gorm.StatementModifier, where gorm.StatementModifier do the real work.
// See gorm.DeletedAt for impl. reference
type policyFilterClause struct {
	types.NoopStatementModifier
	metadata
	Flag PolicyFlag
}

func newPolicyFilterClause(f *schema.Field, flag PolicyFlag) clause.Interface {
	meta, e := loadMetadata(f.Schema)
	if e != nil {
		panic(e)
	}
	switch flag {
	case PolicyFlagCreate:
		return newCreatePolicyFilterClause(meta)
	default:
		return &policyFilterClause{
			metadata: *meta,
			Flag:     flag,
		}
	}
}

func (c policyFilterClause) ModifyStatement(stmt *gorm.Statement) {
	if shouldSkip(stmt.Context, c.Flag, c.Mode) {
		return
	}

	rs, e := opa.FilterResource(stmt.Context, c.ResType, flagToResOp(c.Flag), c.opaFilterOptions(stmt))
	if e != nil {
		switch {
		case errors.Is(e, opa.QueriesNotResolvedError):
			stmt.Error = data.NewRecordNotFoundError("record not found")
		default:
			stmt.Error = data.NewInternalError(fmt.Sprintf(`OPA filtering failed with error: %v`, e), e)
		}
		return
	}
	exprs := rs.Result.([]clause.Expression)
	if len(exprs) == 0 {
		return
	}

	// special fix for db.Model(&targetModel{}).Where(&targetModel{f1:v1}).Or(&targetModel{f2:v2})...
	// Ref:	https://github.com/go-gorm/gorm/issues/3627
	//		https://github.com/go-gorm/gorm/commit/9b2181199d88ed6f74650d73fa9d20264dd134c0#diff-e3e9193af67f3a706b3fe042a9f121d3609721da110f6a585cdb1d1660fd5a3c
	types.FixWhereClausesForStatementModifier(stmt)

	if len(exprs) == 1 {
		stmt.AddClause(clause.Where{Exprs: exprs})
	} else {
		stmt.AddClause(clause.Where{Exprs: []clause.Expression{clause.Or(exprs...)}})
	}
}

func (c policyFilterClause) opaFilterOptions(stmt *gorm.Statement) opa.ResourceFilterOptions {
	unknowns := make([]string, 0, len(c.Fields))
	for k := range c.Fields {
		unknown := fmt.Sprintf(`%s.%s.%s`, opa.InputPrefixRoot, opa.InputPrefixResource, k)
		unknowns = append(unknowns, unknown)
	}
	return func(rf *opa.ResourceFilter) {
		rf.Policy = c.Policy
		rf.Unknowns = unknowns
		rf.QueryMapper = NewGormPartialQueryMapper(&GormMapperConfig{
			Fields:    c.Fields,
			Statement: stmt,
		})
	}
}

/***************************
	Create
 ***************************/

// createPolicyFilterClause is a special policyFilterClause that TODO
type createPolicyFilterClause struct {
	policyFilterClause
}

func newCreatePolicyFilterClause(meta *metadata) *createPolicyFilterClause {
	return &createPolicyFilterClause{
		policyFilterClause{
			metadata:              *meta,
			Flag:                  PolicyFlagCreate,
		},
	}
}

func (c createPolicyFilterClause) ModifyStatement(stmt *gorm.Statement) {
	if shouldSkip(stmt.Context, PolicyFlagCreate, c.Mode) {
		return
	}

	m, e := resolveTargetModel(stmt, &c.metadata)
	if e != nil {
		_ = stmt.Statement.AddError(e)
		return
	}

	if e := c.checkPolicy(stmt.Context, &m, opa.OpCreate); e != nil {
		_ = stmt.Statement.AddError(e)
		return
	}
}

func (c createPolicyFilterClause) checkPolicy(ctx context.Context, m *targetModel, op opa.ResourceOperation) error {
	input := map[string]interface{}{}
	switch {
	case m.val.IsValid():
		// create by model struct
		for k, tagged := range m.meta.Fields {
			v := m.val.FieldByIndex(tagged.StructField.Index).Interface()
			input[k] = v
		}
	case m.valueMap != nil:
		// create by value map
		for k, tagged := range m.meta.Fields {
			v, ok := m.valueMap[tagged.Name]
			if !ok || v == nil {
				v, ok = m.valueMap[tagged.DBName]
			}
			if ok {
				input[k] = v
			}
		}
	default:
		return opa.AccessDeniedError.WithMessage(`Cannot resolve values for model creation`)
	}

	return opa.AllowResource(ctx, m.meta.ResType, op, func(res *opa.Resource) {
		res.ExtraData = input
	})
}

/***********************
	Helpers
 ***********************/

func flagToResOp(flag PolicyFlag) opa.ResourceOperation {
	switch flag {
	case PolicyFlagRead:
		return opa.OpRead
	case PolicyFlagUpdate:
		return opa.OpWrite
	case PolicyFlagDelete:
		return opa.OpDelete
	default:
		return opa.OpCreate
	}
}

//func extractFilterTag(f *schema.Field) string {
//	if tag, ok := f.Tag.Lookup(types.TagFilter); ok {
//		return strings.ToLower(strings.TrimSpace(tag))
//	}
//	// TODO Fix this: check if tag is available on embedded struct
//	sf, ok := reflectutils.FindStructField(f.Schema.ModelType, func(t reflect.StructField) bool {
//		return t.Anonymous && (t.Type.AssignableTo(typeTenancy) || t.Type.AssignableTo(typeTenancyPtr))
//	})
//	if ok {
//		return sf.Tag.Get(types.TagFilter)
//	}
//	return ""
//}
//
//func determineFilteringMode(f *schema.Field) (mode policyMode) {
//	// TODO determine mode
//	mode = 0
//	tag := extractFilterTag(f)
//	switch tag {
//	case "":
//		mode = defaultPolicyMode
//	case "-":
//	default:
//		if strings.ContainsRune(tag, 'r') {
//			mode = mode | policyMode(PolicyFlagRead)
//		}
//		if strings.ContainsRune(tag, 'w') {
//			mode = mode | policyMode(PolicyFlagUpdate)
//		}
//	}
//	return
//}





