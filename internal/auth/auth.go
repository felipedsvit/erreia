package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"unicode"

	"github.com/alexedwards/argon2id"

	"github.com/felipedsvit/erreia/internal/user"
)

// UserStore is the slice of *user.Repo that auth needs. Defined as an
// interface so the service can be unit-tested without a database.
type UserStore interface {
	Create(ctx context.Context, email, displayName, passwordHash string) (*user.User, error)
	GetByEmail(ctx context.Context, email string) (*user.User, string, error)
}

type Service struct {
	users UserStore
}

func NewService(users UserStore) *Service { return &Service{users: users} }

var params = &argon2id.Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// Compile-time guarantee that *user.Repo satisfies UserStore.
var _ UserStore = (*user.Repo)(nil)

func (s *Service) Register(ctx context.Context, email, displayName, password string) (*user.User, error) {
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	hash, err := argon2id.CreateHash(password, params)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	u, err := s.users.Create(ctx, email, displayName, hash)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) Authenticate(ctx context.Context, email, password string) (*user.User, error) {
	u, hash, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	ok, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return nil, fmt.Errorf("compare hash: %w", err)
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if len(password) > 128 {
		return fmt.Errorf("password must not exceed 128 characters")
	}
	var hasUpper, hasLower, hasDigit bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		}
	}
	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}
	return nil
}

var ErrInvalidCredentials = errors.New("invalid email or password")

// IsAuthenticated is a tiny helper for handlers that want a quick check.
func IsAuthenticated(r *http.Request, uidFn func(*http.Request) string) bool {
	return uidFn(r) != ""
}
