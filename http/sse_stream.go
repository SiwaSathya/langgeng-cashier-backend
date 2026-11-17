package http

import (
	"bufio"
	"fmt"
	"time"

	"backend-cashier/service"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// StreamSSE memulai streaming HTTP SSE untuk topik tertentu dengan pencegahan kebocoran goroutine (leak prevention)
func StreamSSE(c *fiber.Ctx, sse *service.SSEService, topic string) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	if sse == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("SSE service unavailable")
	}

	ch, unsubscribe := sse.Subscribe(topic)
	ctx := c.Context()

	ctx.SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer unsubscribe()

		fmt.Fprintf(w, "event: connected\ndata: {\"topic\":\"%s\",\"status\":\"connected\"}\n\n", topic)
		if err := w.Flush(); err != nil {
			return
		}

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Goroutine Leak Prevention: Client terputus (tab browser ditutup / network drop)
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, evt.Data)
				if err := w.Flush(); err != nil {
					return
				}
			case <-ticker.C:
				fmt.Fprintf(w, ": heartbeat\n\n")
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	}))

	return nil
}
