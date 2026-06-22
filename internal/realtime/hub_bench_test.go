package realtime

import (
	"strconv"
	"sync"
	"testing"
)

// BenchmarkHubBroadcast measures the cost of fanning an event out to N
// subscribers on the same board. This is the hot path of the realtime
// pipeline: every drag, every keystroke, every card create funnels here.
//
// We use b.Loop() (Go 1.24+) so the compiler cannot eliminate the
// Broadcast call, and we drain each subscriber in a bounded worker that
// exits when the test ends.
func BenchmarkHubBroadcast(b *testing.B) {
	cases := []int{1, 10, 100, 500}
	for _, n := range cases {
		b.Run("subs-"+strconv.Itoa(n), func(b *testing.B) {
			h := NewHub(discardLogger())
			chans := make([]<-chan Event, n)
			cancels := make([]func(), n)
			for i := 0; i < n; i++ {
				chans[i], cancels[i] = h.Subscribe("board-1")
			}

			// Bounded drain pool: each worker reads with a small loop,
			// guarded by a stop channel that fires on test cleanup.
			stop := make(chan struct{})
			var wg sync.WaitGroup
			for _, ch := range chans {
				wg.Add(1)
				go func(c <-chan Event) {
					defer wg.Done()
					for {
						select {
						case _, ok := <-c:
							if !ok {
								return
							}
						case <-stop:
							// Drain remaining events non-blockingly so
							// we don't leak messages after the test
							// releases subscribers.
							for {
								select {
								case _, ok := <-c:
									if !ok {
										return
									}
								default:
									return
								}
							}
						}
					}
				}(ch)
			}
			b.Cleanup(func() {
				close(stop)
				for _, c := range cancels {
					c()
				}
				wg.Wait()
			})

			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				h.Broadcast(Event{BoardID: "board-1", Action: "card-moved"})
			}
		})
	}
}

// BenchmarkHubBroadcastIsolated measures broadcast cost with no
// subscribers — the common case when only the originator is on the page.
func BenchmarkHubBroadcastIsolated(b *testing.B) {
	h := NewHub(discardLogger())
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		h.Broadcast(Event{BoardID: "lone", Action: "card-updated"})
	}
}

// BenchmarkEncodeDecodeEvent measures the JSON payload cost.
func BenchmarkEncodeDecodeEvent(b *testing.B) {
	ev := Event{BoardID: "b-1", Action: "card-moved", CardID: "c-1", ColumnID: "col-1", Position: 3, Version: 42}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		s, _ := EncodeEvent(ev)
		_, _ = DecodeEvent(s)
	}
}
