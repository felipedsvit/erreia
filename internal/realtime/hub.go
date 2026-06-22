package realtime

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

// Event is the in-memory payload produced by the listener and consumed by
// the SSE handler. The PostgreSQL NOTIFY payload is decoded into this struct.
type Event struct {
	BoardID  string `json:"b"`
	Action   string `json:"a"` // e.g. "card-moved", "card-updated", "card-deleted", "card-created", "column-created"
	CardID   string `json:"id,omitempty"`
	ColumnID string `json:"c,omitempty"`
	Position int    `json:"p,omitempty"`
	Version  int64  `json:"v"`
}

// Hub fans events out to SSE clients subscribed to a given board.
// All operations are O(subscribers) per broadcast.
type Hub struct {
	mu      sync.RWMutex
	rooms   map[string]map[chan Event]struct{}
	logger  *slog.Logger
	dropped atomic.Uint64
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		rooms:  make(map[string]map[chan Event]struct{}),
		logger: logger,
	}
}

// Subscribe registers a new client for boardID and returns a receive-only
// channel and a cancel func. The channel buffer is small on purpose so
// a slow client is dropped fast and never blocks the broadcaster.
//
// The cancel func only unregisters the channel from the hub; it does NOT
// close the channel. Closing would race with any in-flight Broadcast
// (a send on a closed channel panics). Instead, the channel is dropped
// from the map and becomes garbage once the subscriber stops referencing
// it. Any in-flight send lands in the small buffer and is GC'd.
func (h *Hub) Subscribe(boardID string) (<-chan Event, func()) {
	ch := make(chan Event, 4)
	h.mu.Lock()
	if _, ok := h.rooms[boardID]; !ok {
		h.rooms[boardID] = make(map[chan Event]struct{})
	}
	h.rooms[boardID][ch] = struct{}{}
	clients := len(h.rooms[boardID])
	h.mu.Unlock()
	h.logger.Debug("sse subscribe", "board", boardID, "clients", clients)

	cancel := func() {
		h.mu.Lock()
		if room, ok := h.rooms[boardID]; ok {
			if _, ok := room[ch]; ok {
				delete(room, ch)
			}
			if len(room) == 0 {
				delete(h.rooms, boardID)
			}
		}
		h.mu.Unlock()
		h.logger.Debug("sse unsubscribe", "board", boardID)
	}
	return ch, cancel
}

// Broadcast sends ev to every subscriber of ev.BoardID. The send is
// non-blocking: if a client buffer is full the event is dropped and the
// counter is bumped. We do not hold the hub lock while sending so a
// slow client cannot stall other rooms.
func (h *Hub) Broadcast(ev Event) {
	h.mu.RLock()
	room, ok := h.rooms[ev.BoardID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	clients := make([]chan Event, 0, len(room))
	for c := range room {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c <- ev:
		default:
			h.dropped.Add(1)
			h.logger.Warn("sse client too slow, dropping event",
				"board", ev.BoardID, "action", ev.Action)
		}
	}
}

// Stats returns lightweight metrics useful for /healthz or logs.
func (h *Hub) Stats() (rooms int, clients int, dropped uint64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	rooms = len(h.rooms)
	for _, r := range h.rooms {
		clients += len(r)
	}
	return rooms, clients, h.dropped.Load()
}

// DecodeEvent parses a NOTIFY payload (a JSON blob) into an Event.
func DecodeEvent(payload string) (Event, error) {
	var ev Event
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return Event{}, fmt.Errorf("decode event: %w", err)
	}
	return ev, nil
}

// EncodeEvent is exported so service code (or tests) can build a payload.
func EncodeEvent(ev Event) (string, error) {
	b, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
