package containers

import (
	"strings"
	"time"

	"github.com/coroot/coroot-node-agent/common"
	"github.com/coroot/coroot-node-agent/ebpftracer/l7"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"
)

type L7Metrics struct {
	Requests    *prometheus.CounterVec
	Latency     prometheus.Histogram
	containerID string
}

func (m *L7Metrics) observe(status, method string, duration time.Duration) {
	// Debug logs only for Envoy
	isEnvoy := strings.HasPrefix(m.containerID, "/k8s/contour/contour-envoy")
	if isEnvoy {
		klog.Infof("L7Metrics.observe: [CONTAINER=%s] status=%q method=%q duration=%v", m.containerID, status, method, duration)
	}
	// Counter
	if m.Requests != nil {
		var err error
		var c prometheus.Counter
		if method != "" {
			c, err = m.Requests.GetMetricWithLabelValues(status, method)
		} else {
			c, err = m.Requests.GetMetricWithLabelValues(status)
		}
		if err != nil {
			if isEnvoy {
				klog.Warningf("L7Metrics.observe: [CONTAINER=%s] COUNTER FAILED status=%q method=%q err=%v", m.containerID, status, method, err)
			}
		} else {
			c.Inc()
			if isEnvoy {
				klog.Infof("L7Metrics.observe: [CONTAINER=%s] COUNTER OK status=%q", m.containerID, status)
			}
		}
	} else {
		if isEnvoy {
			klog.Warningf("L7Metrics.observe: [CONTAINER=%s] COUNTER IS NIL!", m.containerID)
		}
	}

	// Histogram
	if m.Latency != nil && duration != 0 {
		m.Latency.Observe(duration.Seconds())
		if isEnvoy {
			klog.Infof("L7Metrics.observe: [CONTAINER=%s] HISTOGRAM OK duration=%v", m.containerID, duration)
		}
	} else if m.Latency == nil {
		if isEnvoy {
			klog.Warningf("L7Metrics.observe: [CONTAINER=%s] HISTOGRAM IS NIL!", m.containerID)
		}
	}
}

type L7Stats map[l7.Protocol]map[common.DestinationKey]*L7Metrics // protocol -> dst:actual_dst -> metrics

func (s L7Stats) get(protocol l7.Protocol, key common.DestinationKey, containerID string) *L7Metrics {
	isEnvoy := strings.HasPrefix(containerID, "/k8s/contour/contour-envoy")

	if protocol == l7.ProtocolHTTP2 {
		protocol = l7.ProtocolHTTP
	}
	protoStats := s[protocol]
	if protoStats == nil {
		protoStats = map[common.DestinationKey]*L7Metrics{}
		s[protocol] = protoStats
	}
	m := protoStats[key]
	if m == nil {
		if isEnvoy {
			klog.Infof("L7Stats.get: [CONTAINER=%s] CREATE NEW metrics for %s -> %s", containerID, protocol, key)
		}
		m = &L7Metrics{containerID: containerID}
		protoStats[key] = m
		constLabels := map[string]string{"destination": key.DestinationLabelValue(), "actual_destination": key.ActualDestinationLabelValue()}
		labels := []string{"status"}
		switch protocol {
		case l7.ProtocolRabbitmq, l7.ProtocolNats:
			labels = append(labels, "method")
		default:
			hOpts := L7Latency[protocol]
			m.Latency = prometheus.NewHistogram(
				prometheus.HistogramOpts{Name: hOpts.Name, Help: hOpts.Help, ConstLabels: constLabels},
			)
		}
		cOpts := L7Requests[protocol]
		m.Requests = prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: cOpts.Name, Help: cOpts.Help, ConstLabels: constLabels}, labels,
		)
		if isEnvoy {
			klog.Infof("L7Stats.get: [CONTAINER=%s] CREATED counter=%s histogram=%v", containerID, cOpts.Name, m.Latency != nil)
		}
	}
	return m
}

func (s L7Stats) collect(ch chan<- prometheus.Metric) {
	for _, protoStats := range s {
		for _, m := range protoStats {
			if m.Requests != nil {
				m.Requests.Collect(ch)
			}
			if m.Latency != nil {
				m.Latency.Collect(ch)
			}
		}
	}
}

func (s L7Stats) delete(dst common.HostPort) {
	for _, protoStats := range s {
		for d := range protoStats {
			if d.Destination() == dst {
				delete(protoStats, d)
			}
		}
	}
}
