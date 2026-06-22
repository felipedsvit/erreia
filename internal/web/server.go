package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/felipedsvit/erreia/internal/config"
	sess "github.com/felipedsvit/erreia/internal/session"
	mw "github.com/felipedsvit/erreia/internal/web/middleware"
)

type Server struct {
	cfg            *config.Config
	logger         *slog.Logger
	session        *scs.SessionManager
	deps           Dependencies
	avatarUploader AvatarUploader
}

// AvatarUploader is the port the handlers depend on; the *avatar.Uploader
// in the internal/avatar package implements it. We keep it as an interface
// here so tests can supply a fake without touching the storage layer.
type AvatarUploader interface {
	Upload(ctx context.Context, userID, contentType string, body io.Reader) (string, error)
}

// New returns a fully configured *chi.Mux ready to be wrapped in *http.Server.
func New(cfg *config.Config, logger *slog.Logger, session *scs.SessionManager, deps Dependencies, avatarUploader AvatarUploader) http.Handler {
	s := &Server{
		cfg:            cfg,
		logger:         logger,
		session:        session,
		deps:           deps,
		avatarUploader: avatarUploader,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	//nolint:staticcheck // app runs behind a trusted TLS-terminating reverse proxy that sets X-Forwarded-For; see deployment notes.
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))
	r.Use(session.LoadAndSave)
	r.Use(mw.SecurityHeaders)

	// CSRF middleware protects all state-changing routes globally.
	csrfMW := &mw.CSRF{
		FieldName: "csrf_token",
		Header:    "X-CSRF-Token",
		Session:   sess.CSRFStore{SM: session},
	}
	r.Use(csrfMW.Protect)

	// Rate limiter for auth endpoints.
	rl := mw.NewRateLimiter(10, time.Minute)

	// Static & health
	r.Handle("/static/*", AssetsHandler())
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/favicon.ico", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Public
	r.Get("/", s.handleHome)
	r.Get("/login", s.handleLoginForm)
	r.With(rl.Limit).Post("/login", s.handleLoginSubmit)
	r.Get("/register", s.handleRegisterForm)
	r.With(rl.Limit).Post("/register", s.handleRegisterSubmit)
	r.Post("/logout", s.handleLogout)

	// Authenticated
	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/dashboard", s.handleDashboard)
		r.Get("/me", s.handleMe)
		r.Get("/me/avatar/image", s.handleAvatarImage)
		r.Post("/me/avatar", s.handleAvatarUpload)
		r.Get("/boards/new", s.handleNewBoardForm)
		r.Post("/boards", s.handleCreateBoard)
		r.Get("/boards/{boardID}", s.handleBoardView)
		r.Post("/boards/{boardID}/columns", s.handleCreateColumn)
		r.Post("/columns/{columnID}/cards", s.handleCreateCard)
		r.Get("/cards/{cardID}/edit", s.handleCardEditForm)
		r.Get("/cards/{cardID}", s.handleCardViewFragment)
		r.Post("/cards/{cardID}", s.handleCardUpdate)
		r.Post("/cards/{cardID}/move", s.handleCardMove)
		r.Delete("/cards/{cardID}", s.handleCardDelete)
		r.Get("/events/board/{boardID}", s.handleBoardEvents)
	})

	// The per-handler checkCSRF calls remain as defense in depth.

	return r
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"dur_ms", time.Since(start).Milliseconds(),
				"req_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
