package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/alexedwards/argon2id"
	"github.com/felipedsvit/erreia/internal/user"
)

// fakeUserStore is a goroutine-safe in-memory UserStore used by the unit
// tests in this file. It mimics the surface area of *user.Repo that the
// auth service actually uses, so we never need a real Postgres.
type fakeUserStore struct {
	mu    sync.Mutex
	byID  map[string]*user.User
	byEml map[string]string // email -> id
	hash  map[string]string // id -> password hash
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		byID:  make(map[string]*user.User),
		byEml: make(map[string]string),
		hash:  make(map[string]string),
	}
}

func (f *fakeUserStore) Create(_ context.Context, email, displayName, passwordHash string) (*user.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.byEml[email]; exists {
		return nil, user.ErrEmailTaken
	}
	u := &user.User{
		ID:          "u_" + email,
		Email:       email,
		DisplayName: displayName,
	}
	f.byID[u.ID] = u
	f.byEml[email] = u.ID
	f.hash[u.ID] = passwordHash
	return u, nil
}

func (f *fakeUserStore) GetByEmail(_ context.Context, email string) (*user.User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.byEml[email]
	if !ok {
		return nil, "", user.ErrNotFound
	}
	return f.byID[id], f.hash[id], nil
}

func newTestService() (*Service, *fakeUserStore) {
	fu := newFakeUserStore()
	return &Service{users: fu}, fu
}

// TestRegisterAndAuthenticateHappyPath is the end-to-end happy path.
func TestRegisterAndAuthenticateHappyPath(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService()

	u, err := svc.Register(context.Background(), "alice@example.com", "Alice", "Hunter22Hunter")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.ID == "" || u.Email != "alice@example.com" {
		t.Fatalf("unexpected user: %+v", u)
	}

	got, err := svc.Authenticate(context.Background(), "alice@example.com", "Hunter22Hunter")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got.ID != u.ID {
		t.Fatalf("ids differ: %s vs %s", got.ID, u.ID)
	}
}

// TestAuthenticateWrongPassword covers the credential mismatch path.
func TestAuthenticateWrongPassword(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService()
	_, _ = svc.Register(context.Background(), "bob@example.com", "Bob", "Correct1")

	_, err := svc.Authenticate(context.Background(), "bob@example.com", "wrong password")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	if !errorIs(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

// TestAuthenticateUnknownEmail covers the not-found path.
func TestAuthenticateUnknownEmail(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService()
	_, err := svc.Authenticate(context.Background(), "ghost@example.com", "x")
	if !errorIs(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

// TestRegisterShortPassword enforces the minimum length policy.
func TestRegisterShortPassword(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService()
	if _, err := svc.Register(context.Background(), "a@example.com", "A", "short"); err == nil {
		t.Fatal("expected error for short password")
	}
}

// TestRegisterRequiresUppercase enforces the character-class policy.
func TestRegisterRequiresUppercase(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService()
	_, err := svc.Register(context.Background(), "u@example.com", "U", "alllower1x")
	if err == nil {
		t.Fatal("expected error for password without uppercase")
	}
}

// TestRegisterRequiresLowercase enforces the character-class policy.
func TestRegisterRequiresLowercase(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService()
	_, err := svc.Register(context.Background(), "l@example.com", "L", "ALLUPPER1X")
	if err == nil {
		t.Fatal("expected error for password without lowercase")
	}
}

// TestRegisterRequiresDigit enforces the character-class policy.
func TestRegisterRequiresDigit(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService()
	_, err := svc.Register(context.Background(), "d@example.com", "D", "NoDigitsHere")
	if err == nil {
		t.Fatal("expected error for password without digit")
	}
}

// TestRegisterDuplicateEmail enforces the uniqueness constraint.
func TestRegisterDuplicateEmail(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService()
	if _, err := svc.Register(context.Background(), "a@example.com", "A", "LongEnough1pw"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Register(context.Background(), "a@example.com", "A2", "AnotherGood1pw")
	if !errorIs(err, user.ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

// TestPasswordHashIsArgon2id sanity-checks that the stored hash is the
// expected algorithm/parameters by re-verifying it through argon2id.
func TestPasswordHashIsArgon2id(t *testing.T) {
	t.Parallel()
	svc, fu := newTestService()
	_, _ = svc.Register(context.Background(), "c@example.com", "C", "Supersecret123")

	_, hash, err := fu.GetByEmail(context.Background(), "c@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash is not argon2id: %q", hash)
	}
	ok, err := argon2id.ComparePasswordAndHash("Supersecret123", hash)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("hash did not verify")
	}
}

func errorIs(err, target error) bool {
	return errors.Is(err, target)
}
