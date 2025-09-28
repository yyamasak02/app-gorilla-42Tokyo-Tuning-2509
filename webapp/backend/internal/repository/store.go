package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
)

type Store struct {
	db          DBTX
	UserRepo    *UserRepository
	SessionRepo *SessionRepository
	ProductRepo *ProductRepository
	OrderRepo   *OrderRepository
}

func NewStore(db DBTX, cache *cache.Cache) *Store {
	return &Store{
		db:          db,
		UserRepo:    NewUserRepository(db),
		SessionRepo: NewSessionRepository(db, cache),
		ProductRepo: NewProductRepository(db),
		OrderRepo:   NewOrderRepository(db),
	}
}

func (s *Store) ExecTx(ctx context.Context, fn func(txStore *Store) error) error {
	db, ok := s.db.(*sqlx.DB)
	if !ok {
		return fn(s)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	txStore := NewStore(tx, s.SessionRepo.Cache)
	if err := fn(txStore); err != nil {
		return err
	}

	return tx.Commit()
}
