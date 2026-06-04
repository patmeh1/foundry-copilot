// otlp.go is the opt-in OTLP HTTP exporter for telemetry events.
//
// It is intentionally restrictive: a non-empty Endpoint AND a non-empty
// AllowedHosts list are both required, and the endpoint's host MUST appear
// (dot-anchored) in AllowedHosts. The default zero-valued exporter is a
// no-op so users who do nothing emit nothing off-machine.
//
// v0.2.0 ships the host gate + a stub send path that returns nil on accept.
// v0.2.1 will land the real protobuf-encoded OTLP/HTTP body once we have a
// concrete customer using a specific collector.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// OTLPExporter is the rule-restricted outbound exporter.
type OTLPExporter struct {
	Endpoint     string
	AllowedHosts []string
	HTTP         *http.Client
}

// Export sends events to Endpoint if the gate allows it. Returns nil when
// the gate is closed (no Endpoint, no AllowedHosts) so callers can use this
// in a fire-and-forget manner without ceremony.
func (o *OTLPExporter) Export(ctx context.Context, events []Event) error {
	if o == nil || o.Endpoint == "" {
		return nil
	}
	if len(o.AllowedHosts) == 0 {
		return nil
	}
	u, err := url.Parse(o.Endpoint)
	if err != nil {
		return fmt.Errorf("telemetry.otlp: parse endpoint: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	allowed := false
	for _, h := range o.AllowedHosts {
		hh := strings.ToLower(strings.TrimSpace(h))
		if hh == "" {
			continue
		}
		if host == hh || strings.HasSuffix(host, "."+hh) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("telemetry.otlp: host %q not in allowed_hosts", host)
	}
	// v0.2.0 stub: gate-only. v0.2.1 wires the real OTLP body.
	_ = events
	return nil
}
