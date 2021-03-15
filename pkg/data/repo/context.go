package repo

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/data"
)

var (
	ErrorInvalidCrudModel = data.NewDataError(data.ErrorCodeInvalidCrudModel, "invalid model for CrudRepository")
	ErrorUnsupportedOptions = data.NewDataError(data.ErrorCodeUnsupportedOptions, "unsupported CrudOption")
	ErrorUnsupportedCondition = data.NewDataError(data.ErrorCodeUnsupportedCondition, "unsupported CrudCondition")
)

// Factory usually used in repository creation.
type Factory interface {
	// NewCRUD create a implementation specific CrudRepository.
	// "model" represent the model this repository works on. It could be Struct or *Struct
	// It panic if model is not a valid model definition
	// accepted options depends on implementation. for gorm, *gorm.Session can be supplied
	NewCRUD(model interface{}, options...interface{}) CrudRepository
}

// CrudCondition is typically used for generic CRUD repository
// supported condition depends on operation and underlying implementation:
// 	- map[string]interface{} (should be generally supported)
//		e.g. {"col1": "val1", "col2": 10} -> "WHERE col1 = "val1" AND col2 = 10"
//	- Struct (supported by gorm)
//		e.g. &User{FirstName: "john", Age: 20} -> "WHERE first_name = "john" AND age = 20"
//  - raw condition generated by WhereCondition
//  - valid gorm clause.
// 		e.g. clause.Where
// If given condition is not supported, an error with code data.ErrorCodeUnsupportedCondition will be return
//  - TODO 1: more features leveraging "gorm" lib. Ref: https://gorm.io/docs/query.html#Conditions
//  - TODO 2: more detailed documentation of already supported types
type CrudCondition interface {}

// CrudOption is typically used for generic CRUD repository
// supported options depends on operation and underlying implementation
//  - OmitOption for read/write
//  - JoinsOption for read
//  - PreloadOption for read
//  - SelectOption for read/write
// 	- SortOption for read
//  - PageOption for read
// 	- ...
// If given condition is not supported, an error with code data.ErrorCodeUnsupportedOptions will be return
// TODO Provide more supporting features
type CrudOption interface {}

type CrudRepository interface {

	// FindById fetch model by primary key and scan it into provided interface.
	// "dest" is usually a pointer of model
	FindById(ctx context.Context, dest interface{}, id interface{}, options...CrudOption) error

	// FindAll fetch all model scan it into provided slice.
	// "dest" is usually a pointer of model slice
	FindAll(ctx context.Context, dest interface{}, options...CrudOption) error

	// FindAll fetch all model with given condition and scan result into provided value.
	// "dest" can be
	// 	- a pointer of model
	// 	- a pointer of model slice
	//	- any other result type supported
	// if the "query" result doesn't agree with the provided interface, error will return.
	// e.g. dest is *User instead of []*User, but query found multiple users
	FindBy(ctx context.Context, dest interface{}, condition CrudCondition, options...CrudOption) error

	// CountAll counts all
	CountAll(ctx context.Context) (int, error)

	// CountBy counts based on conditions.
	CountBy(ctx context.Context, condition CrudCondition) (int, error)

	// Save create or update model or model array.
	Save(ctx context.Context, v interface{}, options...CrudOption) error

	// Create create model or model array. returns error if model already exists
	Create(ctx context.Context, v interface{}, options...CrudOption) error

	// Update update model, only non-zero fields of "v" are updated
	// "v" can be struct or map[string]interface{},
	// could support SelectOption and OmitOption depends on implementation
	Update(ctx context.Context, v interface{}, options...CrudOption) error

	// Delete delete given model or model array
	// returns error if such deletion violate any existing foreign key constraints
	Delete(ctx context.Context, v interface{}) error

	// DeleteBy delete models matching given condition.
	// returns error if such deletion violate any existing foreign key constraints
	DeleteBy(ctx context.Context, condition CrudCondition) error

	// Truncate attempt to truncate the table associated the repository
	// returns error if such truncattion violate any existing foreign key constraints
	Truncate(ctx context.Context) error
}


