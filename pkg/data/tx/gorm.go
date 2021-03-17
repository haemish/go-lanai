package tx

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/data"
	"database/sql"
	"gorm.io/gorm"
)

type GormTxManager interface {
	TxManager
	WithDB(*gorm.DB) GormTxManager
}

var (
	ctxKeyGorm = gormCtxKey{}
)

type gormCtxKey struct{}

type gormTxContext struct {
	txContext
	db *gorm.DB
}

func (c gormTxContext) Value(key interface{}) interface{} {
	if k, ok := key.(gormCtxKey); ok && k == ctxKeyGorm {
		return c.db
	}
	return c.Context.Value(key)
}

func GormTxWithContext(ctx context.Context) (tx *gorm.DB) {
	if db, ok := ctx.Value(ctxKeyGorm).(*gorm.DB); ok {
		return db.WithContext(ctx)
	}
	return nil
}

// gormTxManager implements TxManager, ManualTxManager and GormTxManager
type gormTxManager struct {
	db *gorm.DB
}

func newGormTxManager(db *gorm.DB) *gormTxManager {
	return &gormTxManager{
		db: db,
	}
}

func (m gormTxManager) WithDB(db *gorm.DB) GormTxManager {
	return &gormTxManager{
		db: db,
	}
}

func (m gormTxManager) Transaction(ctx context.Context, tx TxFunc, opts ...*sql.TxOptions) error {
	return m.db.Transaction(func(txDb *gorm.DB) error {
		c := gormTxContext{
			txContext: txContext{Context: ctx},
			db: txDb,
		}
		return tx(c)
	}, opts...)
}

func (m gormTxManager) Begin(ctx context.Context, opts ...*sql.TxOptions) (context.Context, error) {
	tx := m.db.Begin(opts...)
	if tx.Error != nil {
		return ctx, tx.Error
	}
	return gormTxContext{
		txContext: txContext{Context: ctx},
		db: tx,
	}, nil
}

func (m gormTxManager) Rollback(ctx context.Context) (context.Context, error) {
	if tx, ok := ctx.Value(ctxKeyGorm).(*gorm.DB); ok {
		tx.Rollback()
		if tx.Error != nil {
			return ctx, tx.Error
		}
	}

	if orig, ok := ctx.Value(ctxKeyBeginCtx).(context.Context); ok {
		return orig, nil
	}
	return ctx, data.NewDataError(data.ErrorCodeInvalidTransaction, "SavePoint failed. did you pass along the context provided by Begin(...)?")
}

func (m gormTxManager) Commit(ctx context.Context) (context.Context, error) {
	if tx, ok := ctx.Value(ctxKeyGorm).(*gorm.DB); ok {
		tx.Commit()
		if tx.Error != nil {
			return ctx, tx.Error
		}
	}

	if orig, ok := ctx.Value(ctxKeyBeginCtx).(context.Context); ok {
		return orig, nil
	}
	return ctx, data.NewDataError(data.ErrorCodeInvalidTransaction, "SavePoint failed. did you pass along the context provided by Begin(...)?")
}

func (m gormTxManager) SavePoint(ctx context.Context, name string) (context.Context, error) {
	if tx, ok := ctx.Value(ctxKeyGorm).(*gorm.DB); ok {
		tx.SavePoint(name)
		if tx.Error != nil {
			return ctx, tx.Error
		}
	}

	if _, ok := ctx.Value(ctxKeyBeginCtx).(context.Context); ok {
		return ctx, nil
	}
	return ctx, data.NewDataError(data.ErrorCodeInvalidTransaction, "SavePoint failed. did you pass along the context provided by Begin(...)?")
}

func (m gormTxManager) RollbackTo(ctx context.Context, name string) (context.Context, error) {
	if tx, ok := ctx.Value(ctxKeyGorm).(*gorm.DB); ok {
		tx.RollbackTo(name)
		if tx.Error != nil {
			return ctx, tx.Error
		}
	}

	if _, ok := ctx.Value(ctxKeyBeginCtx).(context.Context); ok {
		return ctx, nil
	}
	return ctx, data.NewDataError(data.ErrorCodeInvalidTransaction, "SavePoint failed. did you pass along the context provided by Begin(...)?")
}

