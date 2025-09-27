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
	store *repository.Store
}

func NewAuthService(store *repository.Store) *AuthService {
	return &AuthService{store: store}
}

func (s *AuthService) Login(ctx context.Context, userName, password string) (string, time.Time, error) {
	ctx, span := otel.Tracer("service.auth").Start(ctx, "AuthService.Login")
	defer span.End()

	var sessionID string
	var expiresAt time.Time
	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		// ユーザー検索のタイマー
		start := time.Now()
		user, err := s.store.UserRepo.FindByUserName(ctx, userName)
		userSearchTime := time.Since(start)
		log.Printf("[Login] ユーザー検索時間: %v", userSearchTime)
		
		if err != nil {
			log.Printf("[Login] ユーザー検索失敗(userName: %s): %v", userName, err)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrUserNotFound
			}
			return ErrInternalServer
		}

		// パスワード検証のタイマー
		start = time.Now()
		err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
		passwordVerifyTime := time.Since(start)
		log.Printf("[Login] パスワード検証時間: %v", passwordVerifyTime)
		
		if err != nil {
			log.Printf("[Login] パスワード検証失敗: %v", err)
			span.RecordError(err)
			return ErrInvalidPassword
		}

		// セッション作成のタイマー
		start = time.Now()
		sessionDuration := 24 * time.Hour
		sessionID, expiresAt, err = s.store.SessionRepo.Create(ctx, user.UserID, sessionDuration)
		sessionCreateTime := time.Since(start)
		log.Printf("[Login] セッション作成時間: %v", sessionCreateTime)
		
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
