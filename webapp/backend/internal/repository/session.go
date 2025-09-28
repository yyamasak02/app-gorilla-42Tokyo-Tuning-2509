package repository

import (
	"context"
	"errors"

	"github.com/patrickmn/go-cache"
)

type SessionRepository struct {
	db    DBTX
	Cache *cache.Cache
}

func NewSessionRepository(db DBTX, cache *cache.Cache) *SessionRepository {
	return &SessionRepository{db: db, Cache: cache}
}

// セッションIDからユーザーIDを取得
func (r *SessionRepository) FindUserBySessionID(ctx context.Context, sessionID string) (int, error) {
	if userID, found := r.Cache.Get(sessionID); found {
		return userID.(int), nil
	}
	return 0, errors.New("invalid session")
}
