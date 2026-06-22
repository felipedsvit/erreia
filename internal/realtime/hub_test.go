package realtime

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHubSubscribeAndBroadcast verifies the happy path: a subscriber
// receives every event broadcast to its room.
func TestHubSubscribeAndBroadcast(t *testing.T) {
	t.Parallel()
	h := NewHub(discardLogger())

	ch, cancel := h.Subscribe("board-1")
	defer cancel()

	ev := Event{BoardID: "board-1", Action: "card-moved", CardID: "card-9"}
	h.Broadcast(ev)

	select {
	case got := <-ch:
		if got != ev {
			t.Fatalf("event mismatch: got %+v want %+v", got, ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

// TestHubIsolation ensures that subscribers of one room do not receive
// events targeted at a different room.
func TestHubIsolation(t *testing.T) {
	t.Parallel()
	h := NewHub(discardLogger())

	a, cancelA := h.Subscribe("A")
	defer cancelA()
	b, cancelB := h.Subscribe("B")
	defer cancelB()

	h.Broadcast(Event{BoardID: "A", Action: "card-moved"})

	select {
	case <-a:
	case <-time.After(time.Second):
		t.Fatal("subscriber A did not receive event")
	}
	select {
	case ev := <-b:
		t.Fatalf("subscriber B received event meant for A: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestHubDropOnSlowClient ensures that a subscriber with a full buffer
// does not block the broadcaster; the event is dropped and the dropped
// counter increments.
func TestHubDropOnSlowClient(t *testing.T) {
	t.Parallel()
	h := NewHub(discardLogger())
	ch, cancel := h.Subscribe("X")
	defer cancel()

	// ch has buffer 4; broadcast 10 events without reading.
	for i := 0; i < 10; i++ {
		h.Broadcast(Event{BoardID: "X", Action: "card-moved"})
	}
	_, _, dropped := h.Stats()
	if dropped == 0 {
		t.Fatal("expected drops on full subscriber buffer")
	}
	// Drain to confirm we still got the first 4.
	for i := 0; i < 4; i++ {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("drain stuck at %d", i)
		}
	}
}

// TestHubConcurrentBroadcasts runs many broadcasters and subscribers
// against the same hub to surface any race conditions. The test must
// be run with `go test -race` to be meaningful.
func TestHubConcurrentBroadcasts(t *testing.T) {
	t.Parallel()
	h := NewHub(discardLogger())

	const (
		rooms     = 5
		clients   = 20
		events    = 200
		goroutine = 8
	)

	var received int64
	var wg sync.WaitGroup
	for r := 0; r < rooms; r++ {
		room := "room-" + itoa(r)
		for c := 0; c < clients; c++ {
			ch, cancel := h.Subscribe(room)
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel()
				for {
					select {
					case _, ok := <-ch:
						if !ok {
							return
						}
						atomic.AddInt64(&received, 1)
					case <-time.After(50 * time.Millisecond):
						return
					}
				}
			}()
		}
	}

	pubWG := sync.WaitGroup{}
	for g := 0; g < goroutine; g++ {
		pubWG.Add(1)
		go func() {
			defer pubWG.Done()
			for i := 0; i < events; i++ {
				h.Broadcast(Event{
					BoardID: "room-" + itoa(i%rooms),
					Action:  "card-moved",
					CardID:  itoa(i),
				})
			}
		}()
	}
	pubWG.Wait()

	// Give subscribers a moment to drain before cancelling.
	time.Sleep(20 * time.Millisecond)
	// Trigger cancellations by broadcasting one more time after cancel
	// is invoked from the goroutine above; here we just wait for them.
	wg.Wait()
	if atomic.LoadInt64(&received) == 0 {
		t.Fatal("no events received under concurrent load")
	}
}

// TestHubUnsubscribeRemovesRoom ensures the room map is cleaned up.
func TestHubUnsubscribeRemovesRoom(t *testing.T) {
	t.Parallel()
	h := NewHub(discardLogger())
	_, cancel := h.Subscribe("solo")
	cancel()

	// Wait for any internal state to settle.
	time.Sleep(10 * time.Millisecond)
	rooms, clients, _ := h.Stats()
	if rooms != 0 || clients != 0 {
		t.Fatalf("room leaked: rooms=%d clients=%d", rooms, clients)
	}
}

// TestDecodeEncodeEvent is a small roundtrip test.
func TestDecodeEncodeEvent(t *testing.T) {
	t.Parallel()
	ev := Event{BoardID: "b", Action: "card-moved", CardID: "c", ColumnID: "col", Position: 3, Version: 42}
	s, err := EncodeEvent(ev)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeEvent(s)
	if err != nil {
		t.Fatal(err)
	}
	if got != ev {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", got, ev)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// itoa avoids importing strconv just for the tests.
func itoa(n int) string { return strconv.Itoa(n) }

// _ keeps context import alive for future use without adding to the
// goimports order.
var _ = context.Background
