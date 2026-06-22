package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/felipedsvit/erreia/internal/board"
	"github.com/felipedsvit/erreia/internal/config"
	"github.com/felipedsvit/erreia/internal/realtime"
	"github.com/felipedsvit/erreia/internal/user"
)

// TestSSEBroadcastsCardUpdatedEvent wires a real client through the
// /events/board/{id} endpoint and confirms an event published on the
// hub lands as a card-updated frame on the wire.
func TestSSEBroadcastsCardUpdatedEvent(t *testing.T) {
	t.Parallel()
	users := newFakeUserStore()
	boards := newFakeBoardService()
	hub := realtime.NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))

	users.putUser(&user.User{ID: "u-sse", Email: "sse@example.com", DisplayName: "Sse User"})
	boards.boards["b-1"] = &board.Board{ID: "b-1", OwnerID: "u-sse", Title: "Inbox"}
	boards.cards["card-1"] = &board.Card{ID: "card-1", ColumnID: "col-1", Title: "buy milk"}

	cfg := &config.Config{Env: "test"}
	deps := Dependencies{
		Users:  users,
		Auth:   &fakeAuthService{store: users},
		Boards: boards,
		Store:  fakeObjectStore{},
		Hub:    hub,
	}
	sm := scs.New()
	handler := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), sm, deps, nil)

	// Build a session with the userID, then make an SSE request that
	// reuses the cookie.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm.Put(r.Context(), "userID", "u-sse")
	})).ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Open the SSE stream in a goroutine. The context is owned by the test so
	// it can cancel the goroutine and join it before returning; logging from a
	// goroutine after the test completes would panic.
	streamBody := make(chan string, 4)
	streamDone := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer func() {
		cancel()
		<-streamDone
	}()
	go func() {
		defer close(streamDone)
		r2, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events/board/b-1", nil)
		for _, c := range cookies {
			r2.AddCookie(c)
		}
		resp, err := http.DefaultClient.Do(r2)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		buf := make([]byte, 4096)
		var acc strings.Builder
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				acc.Write(buf[:n])
				for {
					s := acc.String()
					idx := strings.Index(s, "\n\n")
					if idx < 0 {
						break
					}
					frame := s[:idx]
					acc.Reset()
					acc.WriteString(s[idx+2:])
					select {
					case streamBody <- frame:
					case <-ctx.Done():
						return
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for the stream to subscribe.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, clients, _ := hub.Stats()
		if clients > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	hub.Broadcast(realtime.Event{BoardID: "b-1", Action: "card-updated", CardID: "card-1"})

	timeout := time.After(2 * time.Second)
	for {
		select {
		case frame, ok := <-streamBody:
			if !ok {
				t.Fatal("stream closed before card-updated arrived")
			}
			if strings.Contains(frame, "event: card-updated") {
				return
			}
		case <-timeout:
			t.Fatal("timeout waiting for SSE frame")
		}
	}
}
