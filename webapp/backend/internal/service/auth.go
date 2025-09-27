package service

import (
	"context"
	"database/sql"
	"errors"
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
		user, err := s.store.UserRepo.FindByUserName(ctx, userName)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrUserNotFound
			}
			return ErrInternalServer
		}

		err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
		if err != nil {
			span.RecordError(err)
			return ErrInvalidPassword
		}

		// セッション作成は認証成功後なので、タイムアウトを短く設定
		sessionDuration := 24 * time.Hour
		sessionID, expiresAt, err = s.store.SessionRepo.Create(ctx, user.UserID, sessionDuration)
		if err != nil {
			return ErrInternalServer
		}
		return nil
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return sessionID, expiresAt, nil
}
