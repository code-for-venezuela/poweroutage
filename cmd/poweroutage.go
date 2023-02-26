package main

import (
	"time"

	"github.com/code-for-venezuela/poweroutage/pkg/store"
	"github.com/code-for-venezuela/poweroutage/pkg/ups"
	log "github.com/sirupsen/logrus"
)

func main() {
	upsManager := ups.NewManager()
	defer upsManager.Close()
	eventsRecorder, err := store.NewFileSystemRecorder("ccs-petare-zona-device-1", "/home/pi/events")
	if err != nil {
		panic("can't initialize new filesystem recorder")
	}

	var event *store.OutageEvent

	event, err = eventsRecorder.GetMostRecentEvent()
	if err != nil {
		log.Infof("warning, there is already an ongoing event. It started at: %v", event.StartTime)
	}
	mainLoop(upsManager, event, eventsRecorder)
	log.Infof("Program is exiting")
}

func mainLoop(upsManager *ups.UPSManager, event *store.OutageEvent, eventsRecorder store.OutageRecorder) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:

			percentage := powerPercentage(upsManager)
			current, err := upsManager.GetCurrent_mA()
			if err != nil {
				log.Fatalf("unexpected error reading current: %v", err)
			}
			if current < 0 {
				log.Infof("Power is not available. Recording event")
				if event != nil {
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
			log.Infof("Power is available. This is the remaining battery: %.1f%", percentage)
			if event != nil {
				log.Infof("Power outage ended. Recording event")
				eventsRecorder.FinishIncident()
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
