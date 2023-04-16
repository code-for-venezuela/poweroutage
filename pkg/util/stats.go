package util

import (
	"time"

	"github.com/DataDog/datadog-go/statsd"
	log "github.com/sirupsen/logrus"
)

var statsdClient statsd.ClientInterface

type DummyStatsdClient struct{}

func (c *DummyStatsdClient) Gauge(name string, value float64, tags []string, rate float64) error {
	return nil
}

func (c *DummyStatsdClient) Count(name string, value int64, tags []string, rate float64) error {
	return nil
}

func (c *DummyStatsdClient) Histogram(name string, value float64, tags []string, rate float64) error {
	return nil
}

func (c *DummyStatsdClient) Distribution(name string, value float64, tags []string, rate float64) error {
	return nil
}

func (c *DummyStatsdClient) Decr(name string, tags []string, rate float64) error {
	return nil
}

func (c *DummyStatsdClient) Incr(name string, tags []string, rate float64) error {
	return nil
}

func (c *DummyStatsdClient) Set(name string, value string, tags []string, rate float64) error {
	return nil
}

func (c *DummyStatsdClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	return nil
}

func (c *DummyStatsdClient) TimeInMilliseconds(name string, value float64, tags []string, rate float64) error {
	return nil
}

func (c *DummyStatsdClient) Event(e *statsd.Event) error {
	return nil
}

func (c *DummyStatsdClient) SimpleEvent(title, text string) error {
	return nil
}

func (c *DummyStatsdClient) ServiceCheck(sc *statsd.ServiceCheck) error {
	return nil
}

func (c *DummyStatsdClient) SimpleServiceCheck(name string, status statsd.ServiceCheckStatus) error {
	return nil
}

func (c *DummyStatsdClient) Close() error {
	return nil
}

func (c *DummyStatsdClient) Flush() error {
	return nil
}

func (c *DummyStatsdClient) SetWriteTimeout(d time.Duration) error {
	return nil
}

func init() {
	var err error
	statsdClient, err = statsd.New("127.0.0.1:8125")
	if err != nil {
		log.Warn("statsd doesn't seem to be working")
		statsdClient = &DummyStatsdClient{}
	}
}

func GetProvider() statsd.ClientInterface {
	return statsdClient
}
