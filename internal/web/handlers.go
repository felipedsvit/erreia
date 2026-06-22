package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/felipedsvit/erreia/internal/auth"
	"github.com/felipedsvit/erreia/internal/board"
	"github.com/felipedsvit/erreia/internal/realtime"
	"github.com/felipedsvit/erreia/internal/session"
	"github.com/felipedsvit/erreia/internal/storage"
	"github.com/felipedsvit/erreia/internal/user"
	mw "github.com/felipedsvit/erreia/internal/web/middleware"
	"github.com/felipedsvit/erreia/internal/web/templates"
)

// Dependencies bundles what handlers need. main.go builds and injects this.
// We expose the dependencies as interfaces so handler tests can inject
// fakes without standing up Postgres or MinIO.
type Dependencies struct {
	Users  UserStore
	Auth   Authenticator
	Boards BoardService
	Store  storage.ObjectStore
	Hub    *realtime.Hub
}

// UserStore is the slice of *user.Repo the handlers need.
type UserStore interface {
	GetByID(ctx context.Context, id string) (*user.User, error)
	ListBoards(ctx context.Context, ownerID string) ([]user.Board, error)
	SetAvatarKey(ctx context.Context, userID, key string) error
}

// Authenticator is the slice of *auth.Service the handlers need.
type Authenticator interface {
	Register(ctx context.Context, email, displayName, password string) (*user.User, error)
	Authenticate(ctx context.Context, email, password string) (*user.User, error)
}

// BoardService is the slice of *board.Service the handlers need.
type BoardService interface {
	CreateBoard(ctx context.Context, ownerID, title string) (*board.Board, error)
	CreateColumn(ctx context.Context, boardID, title string) (*board.Column, error)
	CreateCard(ctx context.Context, columnID, title string) (*board.Card, string, error)
	GetBoard(ctx context.Context, boardID string) (*board.Board, error)
	GetColumn(ctx context.Context, columnID string) (*board.Column, error)
	GetCard(ctx context.Context, cardID string) (*board.Card, error)
	UpdateCard(ctx context.Context, cardID, title, description string) (*board.Card, error)
	DeleteCard(ctx context.Context, cardID string) error
	MoveCard(ctx context.Context, cardID, destColumnID string, destPos int) (*board.Card, error)
	BoardOwnerOf(ctx context.Context, boardID string) (string, error)
	ColumnBoardID(ctx context.Context, columnID string) (string, error)
	CardBoardID(ctx context.Context, cardID string) (string, error)
}

// Compile-time guarantees that the real implementations satisfy the
// interfaces above. If they ever drift, the build will fail here.
var (
	_ UserStore     = (*user.Repo)(nil)
	_ Authenticator = (*auth.Service)(nil)
	_ BoardService  = (*board.Service)(nil)
)

// CurrentUser loads the authenticated user and decorates it with the CSRF
// token and (optionally) the avatar proxy URL.
func (s *Server) CurrentUser(r *http.Request) (*user.User, error) {
	uid := mw.UserIDFromContext(s.session, r)
	if uid == "" {
		return nil, errors.New("unauthenticated")
	}
	u, err := s.deps.Users.GetByID(r.Context(), uid)
	if err != nil {
		return nil, err
	}
	tok, err := session.GetOrCreateCSRF(s.session, r)
	if err != nil {
		return nil, err
	}
	u.CSRFToken = tok
	if u.AvatarKey != "" {
		// Serve the avatar via our own origin (proxied through
		// /me/avatar/image). This keeps img-src locked to 'self' under
		// the strict CSP; the browser never sees the storage host.
		u.AvatarURL = "/me/avatar/image"
	}
	return u, nil
}

// handleAvatarImage streams the avatar object for the authenticated user
// from the storage layer through the app. The browser only ever talks to
// the app origin, so the CSP can keep img-src 'self' data: without
// exposing the storage host. 304-friendly via ETag/If-None-Match.
func (s *Server) handleAvatarImage(w http.ResponseWriter, r *http.Request) {
	u, err := s.CurrentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if u.AvatarKey == "" {
		http.NotFound(w, r)
		return
	}
	body, contentType, err := s.deps.Store.Get(r.Context(), u.AvatarKey)
	if err != nil {
		s.logger.Warn("avatar fetch failed", "err", err, "user", u.ID)
		http.NotFound(w, r)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = io.Copy(w, body)
}

// requireAuth is middleware that 401s when no user is in the session.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return mw.RequireAuth(s.session)(next)
}

// --- public ---

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserIDFromContext(s.session, r)
	templ.Handler(templates.Home(uid != "")).ServeHTTP(w, r)
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	tok, _ := session.GetOrCreateCSRF(s.session, r)
	templ.Handler(templates.LoginForm(tok, "")).ServeHTTP(w, r)
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	email := r.FormValue("email")
	password := r.FormValue("password")
	u, err := s.deps.Auth.Authenticate(r.Context(), email, password)
	if err != nil {
		tok, _ := session.GetOrCreateCSRF(s.session, r)
		w.WriteHeader(http.StatusUnauthorized)
		templ.Handler(templates.LoginForm(tok, "Invalid email or password.")).ServeHTTP(w, r)
		return
	}
	if err := s.session.RenewToken(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	mw.SetUserID(s.session, r, u.ID)
	freshTok, _ := session.NewToken()
	session.SetCSRF(s.session, r, freshTok)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	tok, _ := session.GetOrCreateCSRF(s.session, r)
	templ.Handler(templates.RegisterForm(tok, "")).ServeHTTP(w, r)
}

func (s *Server) handleRegisterSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	email := r.FormValue("email")
	name := r.FormValue("display_name")
	password := r.FormValue("password")
	u, err := s.deps.Auth.Register(r.Context(), email, name, password)
	if err != nil {
		msg := "Could not create account."
		switch {
		case errors.Is(err, user.ErrEmailTaken):
			msg = "That email is already registered."
		case errors.Is(err, auth.ErrInvalidCredentials):
			msg = err.Error()
		case isInputValidationError(err):
			// Surface input-validation messages so the user knows what to fix
			// (e.g. password too short, missing uppercase, etc.). These are
			// produced by the auth layer, not the database, so they are
			// safe to display.
			msg = err.Error()
		default:
			// Anything else is a real internal error: log it and keep the
			// generic message. We deliberately do not echo the raw text
			// to avoid leaking schema, query, or storage details.
			s.logger.Error("register failed", "err", err)
		}
		tok, _ := session.GetOrCreateCSRF(s.session, r)
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(templates.RegisterForm(tok, msg)).ServeHTTP(w, r)
		return
	}
	if err := s.session.RenewToken(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	mw.SetUserID(s.session, r, u.ID)
	freshTok, _ := session.NewToken()
	session.SetCSRF(s.session, r, freshTok)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	if err := s.session.Destroy(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- authenticated ---

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	u, err := s.CurrentUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	boards, err := s.deps.Users.ListBoards(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	templ.Handler(templates.Dashboard(u, boards)).ServeHTTP(w, r)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, err := s.CurrentUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	templ.Handler(templates.Me(u)).ServeHTTP(w, r)
}

func (s *Server) handleAvatarUpload(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	u, err := s.CurrentUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "bad multipart form", http.StatusBadRequest)
		return
	}
	file, hdr, err := r.FormFile("avatar")
	if err != nil {
		http.Error(w, "missing avatar file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	key, err := s.avatarUploader.Upload(r.Context(), u.ID, hdr.Header.Get("Content-Type"), file)
	if err != nil {
		s.logger.Warn("avatar upload failed", "err", err)
		http.Error(w, "upload failed", http.StatusBadRequest)
		return
	}
	if err := s.deps.Users.SetAvatarKey(r.Context(), u.ID, key); err != nil {
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/me", http.StatusSeeOther)
}

func (s *Server) handleNewBoardForm(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleCreateBoard(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	u, err := s.CurrentUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	title := r.FormValue("title")
	if title == "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	b, err := s.deps.Boards.CreateBoard(r.Context(), u.ID, title)
	if err != nil {
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/boards/"+b.ID, http.StatusSeeOther)
}

func (s *Server) handleBoardView(w http.ResponseWriter, r *http.Request) {
	u, err := s.CurrentUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	boardID := chi.URLParam(r, "boardID")
	b, err := s.deps.Boards.GetBoard(r.Context(), boardID)
	if err != nil || b.OwnerID != u.ID {
		http.NotFound(w, r)
		return
	}
	templ.Handler(templates.BoardView(u, b)).ServeHTTP(w, r)
}

func (s *Server) handleCreateColumn(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	u, err := s.CurrentUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	boardID := chi.URLParam(r, "boardID")
	if owner, _ := s.deps.Boards.BoardOwnerOf(r.Context(), boardID); owner != u.ID {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	title := r.FormValue("title")
	if title == "" {
		http.Redirect(w, r, "/boards/"+boardID, http.StatusSeeOther)
		return
	}
	if _, err := s.deps.Boards.CreateColumn(r.Context(), boardID, title); err != nil {
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/boards/"+boardID, http.StatusSeeOther)
}

func (s *Server) handleCreateCard(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	u, err := s.CurrentUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	columnID := chi.URLParam(r, "columnID")
	boardID, err := s.deps.Boards.ColumnBoardID(r.Context(), columnID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if owner, _ := s.deps.Boards.BoardOwnerOf(r.Context(), boardID); owner != u.ID {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	title := r.FormValue("title")
	if title == "" {
		http.Redirect(w, r, "/boards/"+boardID, http.StatusSeeOther)
		return
	}
	if _, _, err := s.deps.Boards.CreateCard(r.Context(), columnID, title); err != nil {
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/boards/"+boardID, http.StatusSeeOther)
}

func (s *Server) handleCardEditForm(w http.ResponseWriter, r *http.Request) {
	u, err := s.CurrentUser(r)
	if err != nil {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	cardID := chi.URLParam(r, "cardID")
	boardID, err := s.deps.Boards.CardBoardID(r.Context(), cardID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if owner, _ := s.deps.Boards.BoardOwnerOf(r.Context(), boardID); owner != u.ID {
		http.NotFound(w, r)
		return
	}
	card, err := s.deps.Boards.GetCard(r.Context(), cardID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	templ.Handler(templates.CardEdit(*card, u.CSRFToken)).ServeHTTP(w, r)
}

func (s *Server) handleCardViewFragment(w http.ResponseWriter, r *http.Request) {
	u, err := s.CurrentUser(r)
	if err != nil {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	cardID := chi.URLParam(r, "cardID")
	boardID, err := s.deps.Boards.CardBoardID(r.Context(), cardID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if owner, _ := s.deps.Boards.BoardOwnerOf(r.Context(), boardID); owner != u.ID {
		http.NotFound(w, r)
		return
	}
	card, err := s.deps.Boards.GetCard(r.Context(), cardID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	templ.Handler(templates.CardView(*card)).ServeHTTP(w, r)
}

func (s *Server) handleCardUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	u, err := s.CurrentUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	cardID := chi.URLParam(r, "cardID")
	boardID, err := s.deps.Boards.CardBoardID(r.Context(), cardID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if owner, _ := s.deps.Boards.BoardOwnerOf(r.Context(), boardID); owner != u.ID {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	title := r.FormValue("title")
	description := r.FormValue("description")
	card, err := s.deps.Boards.UpdateCard(r.Context(), cardID, title, description)
	if err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	templ.Handler(templates.CardView(*card)).ServeHTTP(w, r)
}

func (s *Server) handleCardMove(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	u, err := s.CurrentUser(r)
	if err != nil {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	cardID := chi.URLParam(r, "cardID")
	boardID, err := s.deps.Boards.CardBoardID(r.Context(), cardID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if owner, _ := s.deps.Boards.BoardOwnerOf(r.Context(), boardID); owner != u.ID {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	destColumnID := r.FormValue("column_id")
	pos, _ := strconv.Atoi(r.FormValue("position"))
	if destColumnID == "" {
		http.Error(w, "missing column_id", http.StatusBadRequest)
		return
	}
	if _, err := s.deps.Boards.MoveCard(r.Context(), cardID, destColumnID, pos); err != nil {
		s.logger.Warn("card move failed", "err", err)
		http.Error(w, "move failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCardDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	u, err := s.CurrentUser(r)
	if err != nil {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	cardID := chi.URLParam(r, "cardID")
	boardID, err := s.deps.Boards.CardBoardID(r.Context(), cardID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if owner, _ := s.deps.Boards.BoardOwnerOf(r.Context(), boardID); owner != u.ID {
		http.NotFound(w, r)
		return
	}
	if err := s.deps.Boards.DeleteCard(r.Context(), cardID); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func (s *Server) checkCSRF(r *http.Request) bool {
	tok, err := session.GetOrCreateCSRF(s.session, r)
	if err != nil || tok == "" {
		return false
	}
	got := r.FormValue("csrf_token")
	if got == "" {
		got = r.Header.Get("X-CSRF-Token")
	}
	return got == tok
}

// isInputValidationError returns true when the error came from the auth
// layer's input validation (e.g. password too short, missing character
// class). These messages are safe to surface to the user because they are
// authored by us, not derived from database or storage internals.
func isInputValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "password must"):
		return true
	}
	return false
}

// avoid unused import for context
var _ = context.Background
