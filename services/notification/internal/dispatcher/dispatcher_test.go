package dispatcher //nolint:revive // package-комментарий — отдельная находка N1/CH18

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// roundTripFunc — stub-транспорт: позволяет проверять ретраи Mattermost без
// сети (guarded dialer из CH04 всё равно заблокировал бы loopback httptest).
type roundTripFunc struct {
	calls  atomic.Int64
	status int
	err    error
}

func (rt *roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.calls.Add(1)
	if rt.err != nil {
		return nil, rt.err
	}
	return &http.Response{
		StatusCode: rt.status,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestMattermost_SendSuccess(t *testing.T) {
	t.Parallel()
	rt := &roundTripFunc{status: http.StatusOK}
	d := &Mattermost{httpClient: &http.Client{Transport: rt}}

	if err := d.Send(context.Background(), "https://mm.example.com/hook", "alerts", "hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := rt.calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1 on success", got)
	}
}

// C5 — отмена ctx обрывает ожидание между ретраями немедленно, без сна на всю
// величину backoff (первый интервал = 1s).
func TestMattermost_CtxCancelAbortsRetry(t *testing.T) {
	t.Parallel()
	rt := &roundTripFunc{status: http.StatusInternalServerError} // всегда retryable
	d := &Mattermost{httpClient: &http.Client{Transport: rt}}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := d.Send(ctx, "https://mm.example.com/hook", "alerts", "hi")
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if elapsed >= time.Second {
		t.Errorf("Send waited %v — backoff was not cancelled by ctx", elapsed)
	}
}

// C5 — email-диспетчер тоже прерывает ретрай по отмене ctx, а не спит весь backoff.
func TestEmail_CtxCancelAbortsRetry(t *testing.T) {
	t.Parallel()
	// 127.0.0.1:1 — закрытый порт: SendMail падает быстро (refused), затем
	// цикл уходит в ожидание backoff, которое должно прерваться по ctx.
	d := NewEmail("127.0.0.1", "1", "", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменён заранее

	start := time.Now()
	err := d.Send(ctx, "from@x", "to@x", EmailMessage{IncidentID: "i1", Tier: 1})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if elapsed >= time.Second {
		t.Errorf("Send waited %v — backoff was not cancelled by ctx", elapsed)
	}
}
