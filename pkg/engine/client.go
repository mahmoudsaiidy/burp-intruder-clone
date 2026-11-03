// Package engine fires HTTP requests at high concurrency through a shared,
// connection-pooled client and streams results back on a channel. It knows
// nothing about attack modes or payloads — it just runs Jobs.
package engine

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// ClientConfig tunes the shared transport. Defaults favor high-throughput,
// high-concurrency attack traffic against a single target over general-
// purpose correctness for arbitrary multi-host browsing.
type ClientConfig struct {
	// MaxWorkers should roughly match the attack's worker pool size: set it
	// too low and the transport's per-host connection cap becomes the
	// bottleneck instead of the network; set it far higher than the pool
	// will ever use and you've just opened idle sockets for no benefit.
	MaxWorkers int
	// Timeout bounds one request+response round trip end to end.
	Timeout time.Duration
	// InsecureSkipVerify allows testing against self-signed/local targets.
	InsecureSkipVerify bool
}

func DefaultClientConfig(maxWorkers int) ClientConfig {
	return ClientConfig{
		MaxWorkers: maxWorkers,
		Timeout:    15 * time.Second,
	}
}

// NewClient builds the single *http.Client shared by every worker in the
// pool. Sharing it — and therefore its Transport's connection pool — is
// what gives keep-alive reuse across requests instead of a fresh TCP (and
// TLS, where applicable) handshake per payload, which is the main lever for
// high throughput.
func NewClient(cfg ClientConfig) *http.Client {
	workers := cfg.MaxWorkers
	if workers < 1 {
		workers = 1
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          workers * 2,
		MaxIdleConnsPerHost:   workers * 2,
		MaxConnsPerHost:       workers,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// we want the raw, on-the-wire response length for the results
		// grid, not a length the transport silently changed by negotiating
		// and then decoding compression out from under us
		DisableCompression: true,
	}
	if cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit opt-in for local/self-signed test targets
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		// an attack needs to see real 3xx responses as results (status,
		// length, headers) rather than have the client silently follow
		// them and report the final hop instead
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
