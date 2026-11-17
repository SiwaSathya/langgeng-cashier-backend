package service

import (
	"encoding/json"
	"sync"
)

// SSEEvent merepresentasikan event SSE standar (Event + Data JSON)
type SSEEvent struct {
	Event string
	Data  string
}

// SSEService mengelola subscription SSE secara thread-safe dan non-blocking
type SSEService struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan SSEEvent]struct{}
}

var globalSSE *SSEService

// SetGlobalSSE menetapkan instance SSEService global untuk diakses fungsi-fungsi domain
func SetGlobalSSE(s *SSEService) {
	globalSSE = s
}

// GetGlobalSSE mengembalikan instance SSEService global
func GetGlobalSSE() *SSEService {
	return globalSSE
}

// NewSSEService membuat instance SSEService baru
func NewSSEService() *SSEService {
	return &SSEService{
		subscribers: make(map[string]map[chan SSEEvent]struct{}),
	}
}

// Subscribe mendaftarkan listener channel ke sebuah topik (misal "opname:19" atau "sales:live")
func (s *SSEService) Subscribe(topic string) (chan SSEEvent, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan SSEEvent, 64)
	if _, ok := s.subscribers[topic]; !ok {
		s.subscribers[topic] = make(map[chan SSEEvent]struct{})
	}
	s.subscribers[topic][ch] = struct{}{}

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if subs, ok := s.subscribers[topic]; ok {
				delete(subs, ch)
				close(ch)
				if len(subs) == 0 {
					delete(s.subscribers, topic)
				}
			}
		})
	}

	return ch, unsubscribe
}

// Broadcast menyiarkan pesan ke seluruh subscriber pada topik yang bersangkutan
func (s *SSEService) Broadcast(topic string, eventName string, payload interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subs, ok := s.subscribers[topic]
	if !ok || len(subs) == 0 {
		return
	}

	var dataStr string
	if str, isStr := payload.(string); isStr {
		dataStr = str
	} else {
		bytes, err := json.Marshal(payload)
		if err != nil {
			return
		}
		dataStr = string(bytes)
	}

	event := SSEEvent{
		Event: eventName,
		Data:  dataStr,
	}

	for ch := range subs {
		select {
		case ch <- event:
		default:
			// Non-blocking jika buffer client penuh agar tidak menghambat broadcaster
		}
	}
}
