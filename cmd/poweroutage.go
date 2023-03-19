package main

import (
	"context"
	"strings"
	"time"

	"github.com/code-for-venezuela/poweroutage/pkg/eventsyncer"
	"github.com/code-for-venezuela/poweroutage/pkg/store"
	"github.com/code-for-venezuela/poweroutage/pkg/ups"
	"github.com/code-for-venezuela/poweroutage/pkg/util"
	log "github.com/sirupsen/logrus"
)

func main() {
	upsManager := ups.NewManager()

	defer upsManager.Close()

	eventsRecorder, err := store.NewFileSystemRecorder("ccs-petare-zona-device-1", "/home/pi/events", "/home/pi/finished-events")
	if err != nil {
		panic("can't initialize new filesystem recorder")
	}

	uploader := store.NewAngosturaPubliser("https://us-central1-event-pipeline.cloudfunctions.net/prod-angosturagate")

	syncManager := eventsyncer.NewEventSyncer(1*time.Minute, eventsRecorder, uploader)
	defer syncManager.Close()
	go syncManager.Run(context.Background())

	var event *store.OutageEvent
	event, err = eventsRecorder.GetMostRecentEvent()

	if err == nil {
		log.Infof("warning, there is already an ongoing event. It started at: %v", event.StartTime)
	}

	if err != nil && !strings.Contains(err.Error(), "no outage events recorded") {
		log.Fatalf("unexpected error reading most recent evet: %v", err)
	}

	mainLoop(upsManager, event, eventsRecorder)
	log.Infof("Program is exiting")
}

func mainLoop(upsManager *ups.UPSManager, event *store.OutageEvent, eventsRecorder store.OutageRecorder) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	statsd := util.GetProvider()
	baseTags := []string{"state:dtocapital", "city:caracas", "municipio:sucre", "parroquia:petare", "id:hawking"}
	for {
		select {
		case <-ticker.C:

			percentage := powerPercentage(upsManager)
			current, err := upsManager.GetCurrent_mA()
			if err != nil {
				log.Fatalf("unexpected error reading current: %v", err)
			}

			statsd.Gauge(
				"powermonitor.batterylevel",
				float64(percentage),
				baseTags,
				1,
			)
			if current < 0 {
				log.Infof("Power is not available. This is the remaining battery: %.1f%%", percentage)
				statsd.Gauge(
					"powermonitor.outage",
					1,
					baseTags,
					1,
				)
				if event == nil {
					log.Infof("There is no ongoing incident. Starting a new one.")
					newEvent, err := eventsRecorder.StartIncident()
					if err != nil {
						log.Fatalf("error starting new event: %v", err)
					}
					event = newEvent
				}
				if percentage < 10 {
					log.Warningf("Percentage is really low. Going to exit")
					return
				}
				continue
			}
			log.Infof("Power is available. This is the remaining battery: %.1f%%", percentage)
			statsd.Gauge(
				"powermonitor.outage",
				0,
				baseTags,
				1,
			)
			if event != nil {
				log.Infof("Power outage ended. Recording event")
				err := eventsRecorder.FinishIncident()
				if err != nil {
					log.Fatalf("unexpected error finishing incident: %v", err)
				}
				event = nil
			}
		}
	}
}

func powerPercentage(upsManager *ups.UPSManager) float32 {
	busVoltage, err := upsManager.GetBusVoltage_V()
	if err != nil {
		panic(err)
	}
	p := (busVoltage - 3) / 1.2 * 100
	if p > 100 {
		p = 100
	}
	if p < 0 {
		p = 0
	}
	return p
}
