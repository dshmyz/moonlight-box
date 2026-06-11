package metrics

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
)

func counterValue(t *testing.T, metric interface{ Write(*dto.Metric) error }) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := metric.Write(m); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if m.Counter == nil {
		return 0
	}
	return m.Counter.GetValue()
}

func TestRecordProxyNegativeCacheHit(t *testing.T) {
	metric := ProxyNegativeCacheHitsTotal.WithLabelValues("npm")
	before := counterValue(t, metric)
	RecordProxyNegativeCacheHit("npm")
	after := counterValue(t, metric)
	if after != before+1 {
		t.Fatalf("negative cache hits = %v, want %v", after, before+1)
	}
}

func TestRecordProxyStaleServed(t *testing.T) {
	metric := ProxyStaleServedTotal.WithLabelValues("maven")
	before := counterValue(t, metric)
	RecordProxyStaleServed("maven")
	after := counterValue(t, metric)
	if after != before+1 {
		t.Fatalf("stale served = %v, want %v", after, before+1)
	}
}

func TestRecordProxyBlobStored(t *testing.T) {
	metric := ProxyBlobBytesTotal.WithLabelValues("pypi")
	before := counterValue(t, metric)
	RecordProxyBlobStored("pypi", 123)
	after := counterValue(t, metric)
	if after != before+123 {
		t.Fatalf("blob bytes = %v, want %v", after, before+123)
	}
}
