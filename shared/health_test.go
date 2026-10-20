package shared

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInstanceHealthRecord(t *testing.T) {
	failure := errors.New("connection refused")
	type outcome struct {
		err    error
		status int
	}
	tests := []struct {
		name            string
		outcomes        []outcome
		wantUnavailable bool
		wantMarkedAt    int // the 1-based outcome that marked the instance unavailable, or 0
	}{
		{name: "no requests"},
		{name: "failures below the threshold", outcomes: []outcome{{err: failure}, {err: failure}}},
		{name: "consecutive failures mark it unavailable", outcomes: []outcome{{err: failure}, {err: failure}, {err: failure}}, wantUnavailable: true, wantMarkedAt: 3},
		{name: "a success resets the count", outcomes: []outcome{{err: failure}, {err: failure}, {status: 200}, {err: failure}, {err: failure}}},
		{name: "failures after a reset still add up", outcomes: []outcome{{err: failure}, {status: 200}, {err: failure}, {err: failure}, {err: failure}}, wantUnavailable: true, wantMarkedAt: 5},
		{name: "gateway errors count as failures", outcomes: []outcome{{status: 502}, {status: 503}, {status: 504}}, wantUnavailable: true, wantMarkedAt: 3},
		{name: "other error responses show it is up", outcomes: []outcome{{status: 401}, {status: 403}, {status: 404}, {status: 500}, {status: 429}, {status: 400}}},
		{name: "an error response resets the count", outcomes: []outcome{{err: failure}, {err: failure}, {status: 500}, {err: failure}, {err: failure}}},
		{name: "later outcomes don't mark it again", outcomes: []outcome{{err: failure}, {err: failure}, {err: failure}, {err: failure}, {err: failure}}, wantUnavailable: true, wantMarkedAt: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewInstanceHealth(3)
			markedAt := 0
			for i, o := range tt.outcomes {
				if h.Record("https://a.example/", o.err, o.status) {
					assert.Zero(t, markedAt, "an instance is only marked unavailable once")
					markedAt = i + 1
				}
			}
			assert.Equal(t, tt.wantMarkedAt, markedAt)
			err := h.Check("https://a.example/")
			if tt.wantUnavailable {
				assert.ErrorIs(t, err, ErrInstanceUnavailable)
				assert.Equal(t, []string{"https://a.example/"}, h.Unavailable())
			} else {
				assert.NoError(t, err)
				assert.Empty(t, h.Unavailable())
			}
		})
	}
}

func TestInstanceHealthIsPerInstance(t *testing.T) {
	h := NewInstanceHealth(2)
	for range 2 {
		h.Record("https://b.example/", errors.New("timeout"), 0)
	}
	h.Record("https://a.example/", errors.New("timeout"), 0)
	h.Record("https://c.example/", errors.New("timeout"), 0)
	h.Record("https://c.example/", nil, http.StatusOK)

	assert.NoError(t, h.Check("https://a.example/"))
	assert.ErrorIs(t, h.Check("https://b.example/"), ErrInstanceUnavailable)
	assert.ErrorContains(t, h.Check("https://b.example/"), "timeout", "the error says what the last failure was")
	assert.NoError(t, h.Check("https://c.example/"))
	assert.Equal(t, []string{"https://b.example/"}, h.Unavailable())
}

func TestInstanceHealthDefaultsAndNil(t *testing.T) {
	// A threshold below one means the default
	h := NewInstanceHealth(0)
	for range DefaultUnavailableAfter - 1 {
		assert.False(t, h.Record("https://a.example/", errors.New("down"), 0))
	}
	assert.True(t, h.Record("https://a.example/", errors.New("down"), 0))

	// A client with no tracker is never blocked
	var none *InstanceHealth
	assert.NoError(t, none.Check("https://a.example/"))
	assert.False(t, none.Record("https://a.example/", errors.New("down"), 0))
	assert.Empty(t, none.Unavailable())
}

func TestInstanceHealthIsSafeForConcurrentUse(t *testing.T) {
	h := NewInstanceHealth(50)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				_ = h.Check("https://a.example/")
				h.Record("https://a.example/", errors.New("down"), 0)
				h.Record("https://b.example/", nil, http.StatusOK+i)
				_ = h.Unavailable()
			}
		}()
	}
	wg.Wait()
	assert.ErrorIs(t, h.Check("https://a.example/"), ErrInstanceUnavailable)
}

// get makes a request and returns its error, closing any response
func get(ctx context.Context, client *GrafanaClient) error {
	resp, err := client.Get(ctx, "api/health")
	if resp != nil {
		resp.Body.Close()
	}
	return err
}

func TestGrafanaClientSkipsAnInstanceThatStopsResponding(t *testing.T) {
	var requests atomic.Int32
	// An instance that accepts connections but never answers in time, like a retired one behind a proxy
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer server.Close()

	health := NewInstanceHealth(3)
	// Two clients for the same instance, as the deployer makes for a configuration and its removals
	first := NewGrafanaClient(server.URL, "tok", "test", 20*time.Millisecond).WithHealth(health)
	second := NewGrafanaClient(server.URL, "tok", "test", 20*time.Millisecond).WithHealth(health)

	for i := range 3 {
		err := get(context.Background(), first)
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrInstanceUnavailable, "request %d", i+1)
	}
	assert.Equal(t, int32(3), requests.Load())

	for _, client := range []*GrafanaClient{first, second} {
		start := time.Now()
		for range 1000 {
			assert.ErrorIs(t, get(context.Background(), client), ErrInstanceUnavailable)
		}
		assert.Less(t, time.Since(start), time.Second, "requests to an unavailable instance fail without waiting")
	}
	assert.Equal(t, int32(3), requests.Load(), "no request reaches an unavailable instance")
}

func TestGrafanaClientHealthIgnoresWhatIsNotTheInstanceFailing(t *testing.T) {
	var status atomic.Int64
	status.Store(http.StatusUnauthorized)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(status.Load()))
	}))
	defer server.Close()

	health := NewInstanceHealth(2)
	client := NewGrafanaClient(server.URL, "tok", "test", time.Second).WithHealth(health)

	// An instance that answers, even with an error, is up
	for _, code := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError, http.StatusUnauthorized} {
		status.Store(int64(code))
		assert.NoError(t, get(context.Background(), client))
	}
	assert.Empty(t, health.Unavailable())

	// A request cancelled by the caller says nothing about the instance
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 5 {
		assert.Error(t, get(ctx, client))
	}
	assert.Empty(t, health.Unavailable())

	// A gateway reporting the instance is down does count
	status.Store(http.StatusBadGateway)
	for range 2 {
		assert.NoError(t, get(context.Background(), client))
	}
	assert.ErrorIs(t, get(context.Background(), client), ErrInstanceUnavailable)
}

func TestGrafanaClientWithoutHealthIsNeverBlocked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewGrafanaClient(server.URL, "tok", "test", time.Second)
	for range 10 {
		assert.NoError(t, get(context.Background(), client))
	}
}
