package service_test

import (
	"backend-cashier/service"
	"testing"
	"time"
)

func TestSSEServiceBroadcastAndSubscribe(t *testing.T) {
	sse := service.NewSSEService()

	ch, unsub := sse.Subscribe("opname:99")
	defer unsub()

	done := make(chan bool)
	go func() {
		select {
		case evt := <-ch:
			if evt.Event != "item_updated" {
				t.Errorf("expected event 'item_updated', got %s", evt.Event)
			}
			if evt.Data != "{\"product_id\":10}" {
				t.Errorf("unexpected data: %s", evt.Data)
			}
			done <- true
		case <-time.After(2 * time.Second):
			t.Errorf("timed out waiting for SSE event")
			done <- false
		}
	}()

	time.Sleep(50 * time.Millisecond)
	sse.Broadcast("opname:99", "item_updated", "{\"product_id\":10}")

	success := <-done
	if !success {
		t.FailNow()
	}
}
