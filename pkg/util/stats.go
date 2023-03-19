package util

import "github.com/DataDog/datadog-go/statsd"

var statsdClient *statsd.Client

func init() {
	var err error
	statsdClient, err = statsd.New("127.0.0.1:8125")
	if err != nil {
		panic("Failed to initialize stats provider. Check that statsd daemon is running")
	}
}

func GetProvider() *statsd.Client {
	return statsdClient
}
