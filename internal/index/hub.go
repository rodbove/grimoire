package index

import (
	"encoding/json"
	"sync"
	"time"
)

type Status struct {
	Assets       int64            `json:"assets"`
	Unclassified int64            `json:"unclassified"`
	Jobs         map[string]int64 `json:"jobs"`
	Failed       int64            `json:"failed"`
	Scanning     []int64          `json:"scanning"`
}

type Hub struct {
	ix   *Indexer
	mu   sync.Mutex
	subs map[chan []byte]struct{}
	last []byte
}

func newHub(ix *Indexer) *Hub {
	return &Hub{ix: ix, subs: map[chan []byte]struct{}{}}
}

func (h *Hub) Subscribe() (chan []byte, func()) {
	ch := make(chan []byte, 4)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	last := h.last
	h.mu.Unlock()
	if last != nil {
		ch <- last
	}
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

func (ix *Indexer) Status() Status {
	s := Status{Jobs: map[string]int64{}, Scanning: []int64{}}
	ix.DB.QueryRow(`SELECT count(*), coalesce(sum(classified_at IS NULL), 0) FROM assets a WHERE a.kind != 'live_video'
		AND EXISTS (SELECT 1 FROM files f WHERE f.asset_id = a.id AND f.status = 'ok')`).Scan(&s.Assets, &s.Unclassified)
	rows, err := ix.DB.Query(`SELECT type, status, count(*) FROM jobs GROUP BY type, status`)
	if err == nil {
		for rows.Next() {
			var t, st string
			var n int64
			if rows.Scan(&t, &st, &n) == nil {
				if st == "failed" {
					s.Failed += n
				} else {
					s.Jobs[t] += n
				}
			}
		}
		rows.Close()
	}
	ix.scanning.Range(func(k, _ any) bool {
		s.Scanning = append(s.Scanning, k.(int64))
		return true
	})
	return s
}

func (h *Hub) run(stop chan struct{}) {
	t := time.NewTicker(1500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		h.mu.Lock()
		n := len(h.subs)
		h.mu.Unlock()
		if n == 0 {
			continue
		}
		b, _ := json.Marshal(h.ix.Status())
		h.mu.Lock()
		if string(b) != string(h.last) {
			h.last = b
			for ch := range h.subs {
				select {
				case ch <- b:
				default:
				}
			}
		}
		h.mu.Unlock()
	}
}
