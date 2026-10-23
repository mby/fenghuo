package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestDrainHTTPWaitsForHandlerDefers(t *testing.T) {
	for _, forced := range []bool{false, true} {
		name := "graceful"
		if forced {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			var requests sync.WaitGroup
			entered, exited := make(chan struct{}), make(chan struct{})
			ln, err := listen("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(exited)
				close(entered)
				if forced {
					<-r.Context().Done()
					time.Sleep(15 * time.Millisecond)
				}
				w.WriteHeader(http.StatusNoContent)
			}), log, ctx, &requests)
			if err != nil {
				t.Fatal(err)
			}
			defer ln.srv.Close()
			go ln.srv.Serve(ln.ln)
			done := make(chan struct{})
			go func() {
				defer close(done)
				client := http.Client{Timeout: time.Second}
				if res, err := client.Get("http://" + ln.ln.Addr().String()); err == nil {
					res.Body.Close()
				}
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("request never started")
			}
			duration := time.Second
			if forced {
				duration = 10 * time.Millisecond
			}
			shutdown, stop := context.WithTimeout(context.Background(), duration)
			defer stop()
			drainHTTP(shutdown, []*http.Server{ln.srv}, cancel, &requests, log)
			select {
			case <-exited:
			default:
				t.Fatal("handler defer outlived shutdown")
			}
			<-done
		})
	}
}
