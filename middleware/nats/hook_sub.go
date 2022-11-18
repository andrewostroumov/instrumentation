package nats

import (
	"context"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/instrument/asyncint64"
	"go.opentelemetry.io/otel/metric/unit"
	"sync"
)

// SubscriptionMetric provide important infroma
type SubscriptionMetric struct {
	list     sync.Map
	counters map[string]asyncint64.Gauge
}

func NewSubscriptionMetrics(opts ...Option) (*SubscriptionMetric, error) {
	cfg := newConfig(opts)

	c := make(map[string]asyncint64.Gauge)
	msgs, _ := cfg.meter.AsyncInt64().Gauge(SubscriptionsPendingCount)
	bs, _ := cfg.meter.AsyncInt64().Gauge(SubscriptionsPendingBytes,
		instrument.WithUnit(unit.Bytes))

	dd, _ := cfg.meter.AsyncInt64().Gauge(SubscriptionsDroppedMsgs)
	cc, _ := cfg.meter.AsyncInt64().Gauge(SubscriptionCountMsgs)

	c[SubscriptionsPendingCount] = msgs
	c[SubscriptionsPendingBytes] = bs
	c[SubscriptionsDroppedMsgs] = dd
	c[SubscriptionCountMsgs] = cc

	res := &SubscriptionMetric{
		counters: c,
	}

	err := cfg.meter.RegisterCallback([]instrument.Asynchronous{msgs, bs, dd, cc}, res.callback)
	if err != nil {
		return nil, errors.WithMessagef(err, "reggister callback")
	}

	return res, nil
}

func (s *SubscriptionMetric) Register(sub ...*nats.Subscription) {
	for _, v := range sub {
		s.list.Store(v, v)
	}
}

func (s *SubscriptionMetric) callback(ctx context.Context) {
	// we could have multi subscriptions with the same subject
	// we should set total number of that
	data := make(map[string]struct {
		msgs    int64
		bytes   int64
		dropped int64
		count   int64
	})

	s.list.Range(func(key, value interface{}) bool {
		v, ok := value.(*nats.Subscription)
		if !ok {
			return true
		}

		pMsg, pBytes, _ := v.Pending()
		dropped, _ := v.Dropped()
		count, _ := v.Delivered()

		vc := data[v.Subject]
		vc.msgs += int64(pMsg)
		vc.bytes += int64(pBytes)
		vc.dropped += int64(dropped)
		vc.count += count

		data[v.Subject] = vc
		return true
	})

	for k, v := range data {
		s.counters[SubscriptionsPendingCount].Observe(ctx, v.msgs, Subject.String(k))
		s.counters[SubscriptionsPendingBytes].Observe(ctx, v.bytes, Subject.String(k))
		s.counters[SubscriptionsDroppedMsgs].Observe(ctx, v.dropped, Subject.String(k))
		s.counters[SubscriptionCountMsgs].Observe(ctx, v.count, Subject.String(k))
	}
}
