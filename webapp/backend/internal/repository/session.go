package repository

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SessionRepository struct {
	db DBTX
}

func NewSessionRepository(db DBTX) *SessionRepository {
	return &SessionRepository{db: db}
}

// キャッシュ付きセッションリポジトリ
type CachedSessionRepository struct {
	db       DBTX
	cache    map[string]int // sessionID -> userID
	expires  map[string]time.Time // sessionID -> expiresAt
	mutex    sync.RWMutex
}

func NewCachedSessionRepository(db DBTX) *CachedSessionRepository {
	return &CachedSessionRepository{
		db:      db,
		cache:   make(map[string]int),
		expires: make(map[string]time.Time),
	}
}

// セッションを作成し、セッションIDと有効期限を返す
func (r *SessionRepository) Create(ctx context.Context, userBusinessID int, duration time.Duration) (string, time.Time, error) {
	sessionUUID, err := uuid.NewRandom()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(duration)
	sessionIDStr := sessionUUID.String()

	query := "INSERT INTO user_sessions (session_uuid, user_id, expires_at) VALUES (?, ?, ?)"
	_, err = r.db.ExecContext(ctx, query, sessionIDStr, userBusinessID, expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return sessionIDStr, expiresAt, nil
}

// セッションIDからユーザーIDを取得
func (r *SessionRepository) FindUserBySessionID(ctx context.Context, sessionID string) (int, error) {
	var userID int
	query := `
		SELECT 
			s.user_id
		FROM 
			user_sessions s
		WHERE s.session_uuid = ? AND s.expires_at > ?`
	err := r.db.GetContext(ctx, &userID, query, sessionID, time.Now())
	if err != nil {
		return 0, err
	}
	return userID, nil
}

// キャッシュ付きセッション作成
func (r *CachedSessionRepository) Create(ctx context.Context, userBusinessID int, duration time.Duration) (string, time.Time, error) {
	sessionUUID, err := uuid.NewRandom()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(duration)
	sessionIDStr := sessionUUID.String()

	query := "INSERT INTO user_sessions (session_uuid, user_id, expires_at) VALUES (?, ?, ?)"
	_, err = r.db.ExecContext(ctx, query, sessionIDStr, userBusinessID, expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}

	// キャッシュに保存
	r.mutex.Lock()
	r.cache[sessionIDStr] = userBusinessID
	r.expires[sessionIDStr] = expiresAt
	r.mutex.Unlock()
	
	return sessionIDStr, expiresAt, nil
}

// キャッシュ付きセッション検索
func (r *CachedSessionRepository) FindUserBySessionID(ctx context.Context, sessionID string) (int, error) {
	// 1. キャッシュから検索
	r.mutex.RLock()
	expiresAt, exists := r.expires[sessionID]
	if exists {
		if time.Now().Before(expiresAt) {
			userID := r.cache[sessionID]
			r.mutex.RUnlock()
			return userID, nil
		} else {
			// 期限切れの場合はキャッシュから削除
			r.mutex.RUnlock()
			r.mutex.Lock()
			delete(r.cache, sessionID)
			delete(r.expires, sessionID)
			r.mutex.Unlock()
		}
	} else {
		r.mutex.RUnlock()
	}

	// 2. キャッシュにない場合はDBから取得
	var userID int
	query := `
		SELECT 
			s.user_id
		FROM 
			user_sessions s
		WHERE s.session_uuid = ? AND s.expires_at > ?`
	err := r.db.GetContext(ctx, &userID, query, sessionID, time.Now())
	if err != nil {
		return 0, err
	}

	// 3. セッションの有効期限を取得
	var sessionExpiresAt time.Time
	expireQuery := `SELECT expires_at FROM user_sessions WHERE session_uuid = ?`
	err = r.db.GetContext(ctx, &sessionExpiresAt, expireQuery, sessionID)
	if err != nil {
		return 0, err
	}

	// 4. キャッシュに保存
	r.mutex.Lock()
	r.cache[sessionID] = userID
	r.expires[sessionID] = sessionExpiresAt
	r.mutex.Unlock()

	return userID, nil
}

// セッション無効化（キャッシュからも削除）
func (r *CachedSessionRepository) InvalidateSession(ctx context.Context, sessionID string) error {
	// キャッシュから削除
	r.mutex.Lock()
	delete(r.cache, sessionID)
	delete(r.expires, sessionID)
	r.mutex.Unlock()
	
	// DBからも削除
	query := "DELETE FROM user_sessions WHERE session_uuid = ?"
	_, err := r.db.ExecContext(ctx, query, sessionID)
	return err
}
