package store

type Publisher interface {
	Publish(eventType string, payload []byte) error
	PublishOutageEvent(event OutageEvent) error
}
