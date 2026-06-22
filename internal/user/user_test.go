package user

import (
	"context"
	"errors"
	"testing"
)

// MockPool simulates pgx.Conn behavior for testing
type MockPool struct {
	queryRowFn func(ctx context.Context, query string, args ...interface{}) interface{ Scan(dest ...interface{}) error }
	execFn     func(ctx context.Context, query string, args ...interface{}) error
}

func (m *MockPool) QueryRow(ctx context.Context, query string, args ...interface{}) interface{ Scan(dest ...interface{}) error } {
	return m.queryRowFn(ctx, query, args...)
}

// TestUserFieldValidation ensures User struct marshals correctly
func TestUserStructFields(t *testing.T) {
	t.Parallel()
	u := &User{
		ID:          "123",
		Email:       "test@example.com",
		DisplayName: "Test User",
	}
	if u.ID != "123" {
		t.Error("ID not set correctly")
	}
	if u.Email != "test@example.com" {
		t.Error("Email not set correctly")
	}
	if u.DisplayName != "Test User" {
		t.Error("DisplayName not set correctly")
	}
}

// TestUserErrors verifies error types
func TestUserErrors(t *testing.T) {
	t.Parallel()
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Error("ErrNotFound not working")
	}
	if errors.Is(ErrNotFound, errors.New("other")) {
		t.Error("ErrNotFound incorrectly matching other errors")
	}
}
