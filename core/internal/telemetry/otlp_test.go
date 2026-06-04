package telemetry

import (
	"context"
	"strings"
	"testing"
)

func TestOTLP_NoopWhenEndpointEmpty(t *testing.T) {
	o := OTLPExporter{}
	if err := o.Export(context.Background(), nil); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestOTLP_NoopWhenAllowedHostsEmpty(t *testing.T) {
	o := OTLPExporter{Endpoint: "https://myotlp.local/v1/traces"}
	if err := o.Export(context.Background(), nil); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestOTLP_RejectsHostNotInAllow(t *testing.T) {
	o := OTLPExporter{Endpoint: "https://evil.com/v1/traces", AllowedHosts: []string{"myotlp.local"}}
	err := o.Export(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "not in allowed_hosts") {
		t.Fatalf("want allow-list error, got %v", err)
	}
}

func TestOTLP_AcceptsAllowedHost(t *testing.T) {
	o := OTLPExporter{Endpoint: "https://myotlp.local/v1/traces", AllowedHosts: []string{"myotlp.local"}}
	if err := o.Export(context.Background(), nil); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestOTLP_AcceptsAllowedSubdomain(t *testing.T) {
	o := OTLPExporter{Endpoint: "https://east.collector.myotlp.local/v1/traces", AllowedHosts: []string{"myotlp.local"}}
	if err := o.Export(context.Background(), nil); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}
