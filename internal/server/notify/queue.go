package notify

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/mby/fenghuo/internal/server/store"
)

const deliveryWorkers = 2

func (n *Notifier) CommitAlert(ctx context.Context, state store.AlertState, event store.Event, msg Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return errors.New("invalid_payload")
	}
	return n.st.CommitAlertDelivery(ctx, state, event, payload)
}

func (n *Notifier) Enqueue(ctx context.Context, msg Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return errors.New("invalid_payload")
	}
	return n.st.EnqueueDeliveries(ctx, payload, msg.Kind, msg.Firing, time.Now())
}

// Run blocks until cancellation and joins its workers. New/Broadcast never start
// background goroutines. Expired leases allow a subsequent Run to resume work.
func (n *Notifier) Run(ctx context.Context) {
	if !n.running.CompareAndSwap(false, true) {
		return
	}
	defer n.running.Store(false)
	n.maintain(ctx)
	var wg sync.WaitGroup
	for range deliveryWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.worker(ctx)
		}()
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			n.hc.CloseIdleConnections()
			return
		case <-ticker.C:
			n.maintain(ctx)
		}
	}
}

func (n *Notifier) maintain(ctx context.Context) {
	if err := n.st.MaintainDeliveries(ctx, time.Now()); err != nil && ctx.Err() == nil {
		n.log.Error("notification maintenance failed")
	}
}

func (n *Notifier) worker(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		d, err := n.st.ClaimDelivery(ctx, time.Now())
		if err != nil && ctx.Err() == nil {
			n.log.Error("notification claim failed")
		}
		if err == nil && d != nil {
			n.deliver(ctx, *d)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (n *Notifier) deliver(ctx context.Context, d store.Delivery) {
	code := ""
	ch, err := n.st.DeliveryChannel(ctx, d)
	switch {
	case errors.Is(err, store.ErrNotFound):
		code = "channel_unavailable"
	case err != nil:
		code = "interrupted"
	default:
		var msg Message
		if err := json.Unmarshal(d.Payload, &msg); err != nil {
			code = "invalid_payload"
		} else {
			code = n.sendAttempt(ctx, d, ch, msg)
		}
	}
	// Persist the outcome even during shutdown, with a bounded cleanup deadline.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := n.st.FinishDelivery(finishCtx, d, code, time.Now()); err != nil {
		n.log.Error("notification completion failed", "delivery_id", d.ID)
	} else if code != "" {
		n.log.Warn("notification attempt failed", "delivery_id", d.ID, "attempt", d.Attempts, "error", code)
	}
}

func (n *Notifier) sendAttempt(ctx context.Context, d store.Delivery, ch store.Channel, msg Message) string {
	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- n.Send(sendCtx, ch, msg) }()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				return store.DeliveryErrorCode(err.Error())
			}
			return ""
		case <-sendCtx.Done():
			<-done // Join the bounded send before the worker may exit.
			if ctx.Err() != nil {
				return "canceled"
			}
			return "timeout"
		case <-ticker.C:
			if _, err := n.st.DeliveryChannel(sendCtx, d); errors.Is(err, store.ErrNotFound) {
				cancel()
				<-done
				return "channel_unavailable"
			}
		}
	}
}
