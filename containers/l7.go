package containers

import (
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
	klog.Infof("L7Metrics.observe: [CONTAINER=%s] ENTER status=%q method=%q duration=%v", m.containerID, status, method, duration)

	if m.Requests != nil {
		var err error
		var c prometheus.Counter
		if method != "" {
			c, err = m.Requests.GetMetricWithLabelValues(status, method)
			klog.Infof("L7Metrics.observe: [CONTAINER=%s] getting counter with labels status=%q method=%q", m.containerID, status, method)
		} else {
			c, err = m.Requests.GetMetricWithLabelValues(status)
			klog.Infof("L7Metrics.observe: [CONTAINER=%s] getting counter with label status=%q", m.containerID, status)
		}
		if err != nil {
			klog.Warningf("L7Metrics.observe: [CONTAINER=%s] FAILED to get counter metric (status=%q, method=%q, duration=%v): %v", m.containerID, status, method, duration, err)
		} else {
			c.Inc()
			klog.Infof("L7Metrics.observe: [CONTAINER=%s] counter incremented SUCCESS (status=%q, method=%q)", m.containerID, status, method)
		}
	} else {
		klog.Warningf("L7Metrics.observe: [CONTAINER=%s] Requests counter is NIL (status=%q, method=%q, duration=%v)", m.containerID, status, method, duration)
	}
	if m.Latency != nil && duration != 0 {
		m.Latency.Observe(duration.Seconds())
		klog.Infof("L7Metrics.observe: [CONTAINER=%s] histogram updated with duration=%v", m.containerID, duration)
	} else if m.Latency != nil && duration == 0 {
		klog.Infof("L7Metrics.observe: [CONTAINER=%s] histogram skipped (zero duration, status=%q)", m.containerID, status)
	} else if m.Latency == nil {
		klog.Infof("L7Metrics.observe: [CONTAINER=%s] Latency histogram is NIL (status=%q, duration=%v)", m.containerID, status, duration)
	}

	klog.Infof("L7Metrics.observe: [CONTAINER=%s] EXIT", m.containerID)
}

type L7Stats map[l7.Protocol]map[common.DestinationKey]*L7Metrics // protocol -> dst:actual_dst -> metrics

func (s L7Stats) get(protocol l7.Protocol, key common.DestinationKey, containerID string) *L7Metrics {
	klog.Infof("L7Stats.get: [CONTAINER=%s] ENTER protocol=%s destination=%s", containerID, protocol, key)

	if protocol == l7.ProtocolHTTP2 {
		protocol = l7.ProtocolHTTP
		klog.Infof("L7Stats.get: [CONTAINER=%s] HTTP2 -> HTTP conversion", containerID)
	}
	protoStats := s[protocol]
	if protoStats == nil {
		protoStats = map[common.DestinationKey]*L7Metrics{}
		s[protocol] = protoStats
		klog.Infof("L7Stats.get: [CONTAINER=%s] created new protocol map for %s", containerID, protocol)
	}
	m := protoStats[key]
	if m == nil {
		klog.Infof("L7Stats.get: [CONTAINER=%s] creating NEW L7Metrics for protocol=%s destination=%s", containerID, protocol, key)
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
			klog.Infof("L7Stats.get: [CONTAINER=%s] created HISTOGRAM %s for %s", containerID, hOpts.Name, key)
		}
		cOpts := L7Requests[protocol]
		m.Requests = prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: cOpts.Name, Help: cOpts.Help, ConstLabels: constLabels}, labels,
		)
		klog.Infof("L7Stats.get: [CONTAINER=%s] created COUNTER %s with labels %v for %s", containerID, cOpts.Name, labels, key)
	} else {
		klog.Infof("L7Stats.get: [CONTAINER=%s] REUSING existing L7Metrics for protocol=%s destination=%s", containerID, protocol, key)
	}
	klog.Infof("L7Stats.get: [CONTAINER=%s] EXIT - returning metrics", containerID)
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
