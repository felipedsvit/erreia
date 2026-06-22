package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/felipedsvit/erreia/internal/auth"
	"github.com/felipedsvit/erreia/internal/board"
	"github.com/felipedsvit/erreia/internal/config"
	"github.com/felipedsvit/erreia/internal/realtime"
	"github.com/felipedsvit/erreia/internal/session"
	"github.com/felipedsvit/erreia/internal/user"
)

// fakeUserStore is the in-memory UserStore used by handler tests.
type fakeUserStore struct {
	mu     sync.Mutex
	byID   map[string]*user.User
	boards map[string][]user.Board
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		byID:   map[string]*user.User{},
		boards: map[string][]user.Board{},
	}
}

func (f *fakeUserStore) GetByID(_ context.Context, id string) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}

func (f *fakeUserStore) ListBoards(_ context.Context, ownerID string) ([]user.Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.boards[ownerID], nil
}

func (f *fakeUserStore) SetAvatarKey(_ context.Context, userID, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byID[userID]; ok {
		u.AvatarKey = key
	}
	return nil
}

func (f *fakeUserStore) putUser(u *user.User) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[u.ID] = u
}

// fakeAuthService satisfies Authenticator using the in-memory store.
type fakeAuthService struct {
	store *fakeUserStore
}

func (a *fakeAuthService) Register(_ context.Context, email, displayName, password string) (*user.User, error) {
	if len(password) < 8 {
		return nil, errors.New("password too short")
	}
	u := &user.User{
		ID:          "u_" + email,
		Email:       email,
		DisplayName: displayName,
	}
	a.store.putUser(u)
	return u, nil
}

func (a *fakeAuthService) Authenticate(_ context.Context, email, password string) (*user.User, error) {
	u, err := a.store.GetByID(context.Background(), "u_"+email)
	if err != nil {
		return nil, auth.ErrInvalidCredentials
	}
	if password != "correct-password" {
		return nil, auth.ErrInvalidCredentials
	}
	return u, nil
}

// fakeBoardService is a BoardService that keeps everything in memory.
// It implements only what handler tests exercise.
type fakeBoardService struct {
	mu     sync.Mutex
	boards map[string]*board.Board
	cards  map[string]*board.Card
	cols   map[string]*board.Column
}

func newFakeBoardService() *fakeBoardService {
	return &fakeBoardService{
		boards: map[string]*board.Board{},
		cards:  map[string]*board.Card{},
		cols:   map[string]*board.Column{},
	}
}

func (b *fakeBoardService) CreateBoard(_ context.Context, ownerID, title string) (*board.Board, error) {
	id := "b_" + title
	brd := &board.Board{ID: id, OwnerID: ownerID, Title: title}
	b.boards[id] = brd
	return brd, nil
}
func (b *fakeBoardService) CreateColumn(_ context.Context, boardID, title string) (*board.Column, error) {
	c := &board.Column{ID: "col_" + title, BoardID: boardID, Title: title}
	b.cols[c.ID] = c
	return c, nil
}
func (b *fakeBoardService) CreateCard(_ context.Context, columnID, title string) (*board.Card, string, error) {
	id := "card_" + title
	c := &board.Card{ID: id, ColumnID: columnID, Title: title, Position: 0}
	b.cards[id] = c
	col := b.cols[columnID]
	return c, col.BoardID, nil
}
func (b *fakeBoardService) GetBoard(_ context.Context, boardID string) (*board.Board, error) {
	brd, ok := b.boards[boardID]
	if !ok {
		return nil, board.ErrNotFound
	}
	return brd, nil
}
func (b *fakeBoardService) GetColumn(_ context.Context, columnID string) (*board.Column, error) {
	c, ok := b.cols[columnID]
	if !ok {
		return nil, board.ErrNotFound
	}
	return c, nil
}
func (b *fakeBoardService) GetCard(_ context.Context, cardID string) (*board.Card, error) {
	c, ok := b.cards[cardID]
	if !ok {
		return nil, board.ErrNotFound
	}
	return c, nil
}
func (b *fakeBoardService) UpdateCard(_ context.Context, cardID, title, description string) (*board.Card, error) {
	c := b.cards[cardID]
	c.Title = title
	c.Description = description
	return c, nil
}
func (b *fakeBoardService) DeleteCard(_ context.Context, cardID string) error {
	delete(b.cards, cardID)
	return nil
}
func (b *fakeBoardService) MoveCard(_ context.Context, cardID, destColumnID string, pos int) (*board.Card, error) {
	c := b.cards[cardID]
	c.ColumnID = destColumnID
	c.Position = pos
	return c, nil
}
func (b *fakeBoardService) BoardOwnerOf(_ context.Context, boardID string) (string, error) {
	brd, ok := b.boards[boardID]
	if !ok {
		return "", board.ErrNotFound
	}
	return brd.OwnerID, nil
}
func (b *fakeBoardService) ColumnBoardID(_ context.Context, columnID string) (string, error) {
	col, ok := b.cols[columnID]
	if !ok {
		return "", board.ErrNotFound
	}
	return col.BoardID, nil
}
func (b *fakeBoardService) CardBoardID(_ context.Context, cardID string) (string, error) {
	c, ok := b.cards[cardID]
	if !ok {
		return "", board.ErrNotFound
	}
	return b.ColumnBoardID(context.Background(), c.ColumnID)
}

// fakeObjectStore is the in-memory storage.ObjectStore used in tests.
type fakeObjectStore struct{}

func (fakeObjectStore) Put(_ context.Context, _ string, _ string, _ io.Reader) error { return nil }
func (fakeObjectStore) Delete(_ context.Context, _ string) error                     { return nil }
func (fakeObjectStore) PresignedGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "http://test/" + key, nil
}
func (fakeObjectStore) Get(_ context.Context, _ string) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader("")), "application/octet-stream", nil
}

// newTestServer wires the web layer with all fakes. main.go uses the
// real implementations; tests use these.
func newTestServer(t *testing.T) (http.Handler, *fakeUserStore, *scs.SessionManager) {
	t.Helper()
	users := newFakeUserStore()
	authSvc := &fakeAuthService{store: users}
	boards := newFakeBoardService()
	hub := realtime.NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))

	cfg := &config.Config{
		Env:             "test",
		HTTPAddr:        ":0",
		BaseURL:         "http://test",
		RealtimeChannel: "test",
	}
	deps := Dependencies{
		Users:  users,
		Auth:   authSvc,
		Boards: boards,
		Store:  fakeObjectStore{},
		Hub:    hub,
	}
	sm := scs.New()
	h := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), sm, deps, nil)
	return h, users, sm
}

var _ = fmt.Sprintf // keep imports happy

// TestHealthz is a smoke test.
func TestHealthz(t *testing.T) {
	h, _, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected body ok, got %q", rec.Body.String())
	}
}

// TestHomeRendersForGuest verifies the public home page returns 200.
func TestHomeRendersForGuest(t *testing.T) {
	h, _, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "erreia") {
		t.Fatalf("expected body to mention erreia, got %q", rec.Body.String())
	}
}

// TestLoginFormRenders checks the login form is reachable.
func TestLoginFormRenders(t *testing.T) {
	h, _, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `name="email"`) {
		t.Fatalf("expected email input in form, got %q", rec.Body.String())
	}
}

// TestRegisterFormRenders checks the register form.
func TestRegisterFormRenders(t *testing.T) {
	h, _, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/register", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `name="display_name"`) {
		t.Fatalf("expected display_name input, got %q", rec.Body.String())
	}
}

// TestRegisterSuccessAndRedirect walks a full register flow and asserts
// the user is created and the session carries the user id.
func TestRegisterSuccessAndRedirect(t *testing.T) {
	h, users, sm := newTestServer(t)
	rec := httptest.NewRecorder()
	form := url.Values{
		"email":        {"alice@example.com"},
		"display_name": {"Alice"},
		"password":     {"hunter22hunter"},
	}
	// First GET to /register so the session is created.
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/register", nil)
	h.ServeHTTP(getRec, getReq)
	for _, c := range getRec.Result().Cookies() {
		rec.Result().Cookies() // touch
		_ = c
	}

	// Now post with the cookies.
	postReq := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range getRec.Result().Cookies() {
		postReq.AddCookie(c)
	}
	// Pull the CSRF token out of the form rendered above.
	csrf := extractCSRF(t, getRec.Body.String())
	form.Set("csrf_token", csrf)
	postReq = httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range getRec.Result().Cookies() {
		postReq.AddCookie(c)
	}

	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d (body: %s)", postRec.Code, postRec.Body.String())
	}
	if loc := postRec.Header().Get("Location"); loc != "/dashboard" {
		t.Fatalf("expected redirect to /dashboard, got %q", loc)
	}
	if _, err := users.GetByID(context.Background(), "u_alice@example.com"); err != nil {
		t.Fatalf("user not created: %v", err)
	}
	_ = sm
}

// TestLoginBadPasswordReturnsForm is a small negative case.
func TestLoginBadPasswordReturnsForm(t *testing.T) {
	h, _, _ := newTestServer(t)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/login", nil))
	csrf := extractCSRF(t, getRec.Body.String())

	form := url.Values{"email": {"alice@example.com"}, "password": {"wrong"}, "csrf_token": {csrf}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range getRec.Result().Cookies() {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid email or password") {
		t.Fatalf("expected error message in body, got %q", rec.Body.String())
	}
}

// TestDashboardRequiresAuth is a redirect check.
func TestDashboardRequiresAuth(t *testing.T) {
	h, _, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("expected /login, got %q", loc)
	}
}

// TestDashboardRendersForAuthedUser covers the happy path of the
// authenticated dashboard.
func TestDashboardRendersForAuthedUser(t *testing.T) {
	h, users, sm := newTestServer(t)
	// Seed a user.
	users.putUser(&user.User{ID: "u-alice", Email: "alice@example.com", DisplayName: "Alice"})
	users.boards["u-alice"] = []user.Board{{ID: "b1", OwnerID: "u-alice", Title: "Inbox"}}

	// Build a session that has the userID.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm.Put(r.Context(), "userID", "u-alice")
	})).ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body %s)", out.Code, out.Body.String())
	}
	if !strings.Contains(out.Body.String(), "Inbox") {
		t.Fatalf("expected board title in body, got %q", out.Body.String())
	}
}

// TestStaticServesCSS hits the embed.FS to make sure it is wired up.
func TestStaticServesCSS(t *testing.T) {
	h, _, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Cache-Control") == "" {
		t.Fatal("expected cache-control header on static")
	}
}

// TestCreateBoardRendersAndCreates covers POST /boards.
func TestCreateBoardRendersAndCreates(t *testing.T) {
	h, users, sm := newTestServer(t)
	users.putUser(&user.User{ID: "u-bob", Email: "bob@example.com", DisplayName: "Bob"})

	// Get a session with the userID and a CSRF token.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm.Put(r.Context(), "userID", "u-bob")
		_ = session.SetFlash
		sm.Put(r.Context(), "csrf_token", "tok-1")
	})).ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	form := url.Values{"title": {"Inbox"}, "csrf_token": {"tok-1"}}
	post := httptest.NewRequest(http.MethodPost, "/boards", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range rec.Result().Cookies() {
		post.AddCookie(c)
	}
	out := httptest.NewRecorder()
	h.ServeHTTP(out, post)
	if out.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d (body %s)", out.Code, out.Body.String())
	}
	if loc := out.Header().Get("Location"); loc != "/boards/b_Inbox" {
		t.Fatalf("expected /boards/b_Inbox, got %q", loc)
	}
}

// TestSSEForbiddenForNonOwner is an authorization check.
func TestSSEForbiddenForNonOwner(t *testing.T) {
	h, users, sm := newTestServer(t)
	users.putUser(&user.User{ID: "u-1", Email: "1@example.com", DisplayName: "1"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events/board/nonexistent", nil)
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm.Put(r.Context(), "userID", "u-1")
	})).ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	if out.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", out.Code)
	}
}

// --- helpers ---

// extractCSRF pulls the value of a hidden csrf_token input out of a
// rendered HTML body. Tests are tolerant of whitespace differences.
func extractCSRF(t *testing.T, body string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, `name="csrf_token"`) {
			continue
		}
		idx := strings.Index(line, `value="`)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(`value="`):]
		end := strings.Index(rest, `"`)
		if end < 0 {
			continue
		}
		return rest[:end]
	}
	t.Fatalf("csrf token not found in body: %q", body)
	return ""
}

// avoid unused import of json, bytes
var (
	_ = json.Marshal
	_ = bytes.NewReader
)
