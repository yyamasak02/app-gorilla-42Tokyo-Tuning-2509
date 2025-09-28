package service

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"backend/internal/repository"
	"backend/internal/service/utils"

	"go.opentelemetry.io/otel"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")
	ErrInternalServer  = errors.New("internal server error")
)

type AuthService struct {
	store             *repository.Store
	cachedSessionRepo *repository.CachedSessionRepository
}

func NewAuthService(store *repository.Store) *AuthService {
	return &AuthService{store: store}
}

func NewAuthServiceWithCache(store *repository.Store, cachedSessionRepo *repository.CachedSessionRepository) *AuthService {
	return &AuthService{
		store:             store,
		cachedSessionRepo: cachedSessionRepo,
	}
}

func (s *AuthService) Login(ctx context.Context, userName, password string) (string, time.Time, error) {
	ctx, span := otel.Tracer("service.auth").Start(ctx, "AuthService.Login")
	defer span.End()

	var sessionID string
	var expiresAt time.Time
	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		user, err := s.store.UserRepo.FindByUserName(ctx, userName)
		if err != nil {
			log.Printf("[Login] ユーザー検索失敗(userName: %s): %v", userName, err)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrUserNotFound
			}
			return ErrInternalServer
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
		if err != nil {
			log.Printf("[Login] パスワード検証失敗: %v", err)
			span.RecordError(err)
			return ErrInvalidPassword
		}

		sessionDuration := 24 * time.Hour
		// キャッシュ付きセッションリポジトリが利用可能な場合はそれを使用
		if s.cachedSessionRepo != nil {
			sessionID, expiresAt, err = s.cachedSessionRepo.Create(ctx, user.UserID, sessionDuration)
		} else {
			sessionID, expiresAt, err = s.store.SessionRepo.Create(ctx, user.UserID, sessionDuration)
		}
		if err != nil {
			log.Printf("[Login] セッション生成失敗: %v", err)
			return ErrInternalServer
		}
		return nil
	})
	if err != nil {
		return "", time.Time{}, err
	}
	log.Printf("Login successful for UserName '%s', session created.", userName)
	return sessionID, expiresAt, nil
}