package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/felipedsvit/erreia/internal/realtime"
	"github.com/felipedsvit/erreia/internal/session"
	"github.com/felipedsvit/erreia/internal/web/templates"
)

// handleBoardEvents streams card/column updates for a board to the browser
// over Server-Sent Events. The connection lives for as long as the tab is
// open; on disconnect the hub subscription is cancelled.
func (s *Server) handleBoardEvents(w http.ResponseWriter, r *http.Request) {
	u, err := s.CurrentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	boardID := chi.URLParam(r, "boardID")
	owner, err := s.deps.Boards.BoardOwnerOf(r.Context(), boardID)
	if err != nil || owner != u.ID {
		http.NotFound(w, r)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := s.deps.Hub.Subscribe(boardID)
	defer cancel()

	csrfToken, _ := session.GetOrCreateCSRF(s.session, r)

	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	pingTicker := time.NewTicker(20 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-pingTicker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := s.writeEvent(w, flusher, r, ev, csrfToken); err != nil {
				s.logger.Debug("sse write error", "err", err, "board", boardID)
				return
			}
		}
	}
}

// writeEvent translates a hub event into SSE frames containing pre-rendered
// templ fragments. Each event name maps to an HTMX swap target on the page.
func (s *Server) writeEvent(w http.ResponseWriter, flusher http.Flusher, r *http.Request, ev realtime.Event, csrfToken string) error {
	ctx := r.Context()
	switch ev.Action {
	case "card-moved", "card-updated":
		card, err := s.deps.Boards.GetCard(ctx, ev.CardID)
		if err != nil {
			return nil // deleted between NOTIFY and render
		}
		var buf bytes.Buffer
		if err := templates.CardView(*card).Render(ctx, &buf); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: card-updated\ndata: %s\n\n", jsonEscape(buf.String())); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	case "card-created":
		column, err := s.deps.Boards.GetColumn(ctx, ev.ColumnID)
		if err != nil {
			return nil
		}
		var buf bytes.Buffer
		if err := templates.ColumnFragment(ev.BoardID, *column, csrfToken).Render(ctx, &buf); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: column-updated\ndata: %s\n\n", jsonEscape(buf.String())); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	case "card-deleted":
		if _, err := fmt.Fprintf(w, "event: card-deleted\ndata: %s\n\n", ev.CardID); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	case "column-created":
		b, err := s.deps.Boards.GetBoard(ctx, ev.BoardID)
		if err != nil {
			return nil
		}
		var buf bytes.Buffer
		if err := templates.BoardFragment(b, csrfToken).Render(ctx, &buf); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: board-replaced\ndata: %s\n\n", jsonEscape(buf.String())); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	case "board-created":
		return nil
	}
	return nil
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
