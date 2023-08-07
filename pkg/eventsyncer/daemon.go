package eventsyncer

import (
	"context"
	"time"

	"github.com/code-for-venezuela/poweroutage/pkg/store"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

type EventSyncer struct {
	recorder  store.OutageRecorder
	publisher *store.AngosturaUploader
	t         *time.Ticker
}

func NewEventSyncer(
	interval time.Duration,
	recorder store.OutageRecorder,
	publisher *store.AngosturaUploader) *EventSyncer {
	es := &EventSyncer{recorder: recorder, publisher: publisher}
	if recorder == nil || publisher == nil {
		log.Fatalf("can't start an event syncer without a recorder and publisher. Got nil")
	}
	es.t = time.NewTicker(interval)
	return es
}

func (es *EventSyncer) Close() error {
	es.t.Stop()
	return nil
}

func (es *EventSyncer) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return es.Close()

		case <-es.t.C:
			fileName, events, err := es.recorder.GetFinishedEvents()
			if err != nil {
				return errors.Wrapf(err, "error reading finished events")
			}
			for i, event := range events {
				log.Infof("publishing event: %v", string(event))
				err := es.publisher.Publish("power_outage_incident", event)
				if err != nil {
					log.Warnf("Could not publish event: %v. Will retry later", string(event))
					continue
				}
				err = es.recorder.DeleteEventFile(fileName[i])
				if err != nil {
					log.Warnf("Could not publish event: %v. Will retry later", string(event))
				}
			}
		}
	}
}
