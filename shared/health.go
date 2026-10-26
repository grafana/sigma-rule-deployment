package shared

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// DefaultUnavailableAfter is how many consecutive failed requests mark an instance unavailable.
// It is above one so a single dropped request doesn't take a healthy instance out for the run,
// and low enough that a retired instance costs a few timeouts, not one per alert.
const DefaultUnavailableAfter = 3

// ErrInstanceUnavailable is returned, without making a request, for an instance that has been
// marked unavailable. Use errors.Is to detect it.
var ErrInstanceUnavailable = errors.New("instance is unavailable")

// InstanceHealth tracks which Grafana instances stop responding during a run, so a retired or
// unreachable instance is skipped after a few failed requests rather than timing out once for every
// request that would have gone to it. Instances are identified by their base URL, so every client
// for an instance shares its health. An instance that is marked unavailable stays so for the
// rest of the run. It is safe for concurrent use.
type InstanceHealth struct {
	mu        sync.Mutex
	threshold int
	failures  map[string]int   // consecutive failures of each instance that is still available
	down      map[string]error // the last failure of each unavailable instance
}

// NewInstanceHealth creates a tracker that marks an instance unavailable after threshold
// consecutive failed requests. A threshold below one means DefaultUnavailableAfter.
func NewInstanceHealth(threshold int) *InstanceHealth {
	if threshold < 1 {
		threshold = DefaultUnavailableAfter
	}
	return &InstanceHealth{
		threshold: threshold,
		failures:  map[string]int{},
		down:      map[string]error{},
	}
}

// Check returns an error wrapping ErrInstanceUnavailable if the instance has been marked
// unavailable, and nil otherwise. It is nil-safe, so a client without a tracker is never blocked.
func (h *InstanceHealth) Check(baseURL string) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if last, ok := h.down[baseURL]; ok {
		return fmt.Errorf("%s was marked unavailable after %d consecutive failed requests (last: %v): %w", baseURL, h.threshold, last, ErrInstanceUnavailable)
	}
	return nil
}

// Record notes the outcome of a request to an instance: an error, or the status code of a
// response. It returns true when this failure is the one that marks the instance unavailable.
// Only an instance that doesn't answer counts as failing: a transport error, or a 502, 503 or 504
// from a gateway in front of it. Any other response, even an error such as 401 or 404, shows
// the instance is up.
func (h *InstanceHealth) Record(baseURL string, err error, statusCode int) (markedUnavailable bool) {
	if h == nil {
		return false
	}
	if err == nil && !isGatewayFailure(statusCode) {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.failures, baseURL)
		return false
	}
	if err == nil {
		err = fmt.Errorf("HTTP status %d", statusCode)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.down[baseURL]; ok {
		return false
	}
	h.failures[baseURL]++
	if h.failures[baseURL] < h.threshold {
		return false
	}
	delete(h.failures, baseURL)
	h.down[baseURL] = err
	return true
}

// Unavailable returns the base URLs of the instances marked unavailable, sorted.
func (h *InstanceHealth) Unavailable() []string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	urls := make([]string, 0, len(h.down))
	for baseURL := range h.down {
		urls = append(urls, baseURL)
	}
	sort.Strings(urls)
	return urls
}

func isGatewayFailure(statusCode int) bool {
	return statusCode == http.StatusBadGateway || statusCode == http.StatusServiceUnavailable || statusCode == http.StatusGatewayTimeout
}
