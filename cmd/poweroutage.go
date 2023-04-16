package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/code-for-venezuela/poweroutage/pkg/eventsyncer"
	"github.com/code-for-venezuela/poweroutage/pkg/store"
	"github.com/code-for-venezuela/poweroutage/pkg/ups"
	"github.com/code-for-venezuela/poweroutage/pkg/util"
	"github.com/spf13/viper"

	"github.com/pusher/pusher-http-go/v5"

	log "github.com/sirupsen/logrus"
)

func main() {

	// Parse command line arguments
	flag.Parse()
	config := loadConfig()

	// Check required flags
	if config.Monitor.State == "" ||
		config.Monitor.City == "" ||
		config.Monitor.Municipality == "" ||
		config.Monitor.Parish == "" ||
		config.Monitor.MonitorID == "" ||
		config.Monitor.Lat == 0 ||
		config.Monitor.Long == 0 {
		fmt.Println("state, city, municipality, parish, and monitor-id are required flags")
		flag.Usage()
		return
	}

	log.Infof("starting power outage monitor for %s, %s, %s, %s (monitor ID: %s)",
		config.Monitor.State,
		config.Monitor.City,
		config.Monitor.Municipality,
		config.Monitor.Parish,
		config.Monitor.MonitorID)

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

	mainLoop(upsManager, event, eventsRecorder, config)
	log.Infof("Program is exiting")
}

type DeviceEvent struct {
	DeviceID  string    `json:"device-id"`
	Latitude  float64   `json:"lat"`
	Longitude float64   `json:"long"`
	State     string    `json:"state"`
	EventTime time.Time `json:"event_time"`
}

func mainLoop(upsManager *ups.UPSManager,
	event *store.OutageEvent,
	eventsRecorder store.OutageRecorder,
	config Config) {
	pusherConfig := config.Pusher
	deviceConfig := config.Monitor

	ticker := time.NewTicker(deviceConfig.TickerDuration)
	defer ticker.Stop()
	statsd := util.GetProvider()
	baseTags := []string{
		"state:" + deviceConfig.State,
		"city:" + deviceConfig.City,
		"municipality:" + deviceConfig.Municipality,
		"parish:" + deviceConfig.Parish,
		"monitor-id:" + deviceConfig.MonitorID,
	}

	pusherClient := pusher.Client{
		AppID:   pusherConfig.AppID,
		Key:     pusherConfig.Key,
		Secret:  pusherConfig.Secret,
		Cluster: pusherConfig.Cluster,
		Secure:  pusherConfig.Secure,
	}

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
					0,
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
				1,
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

			pusherEvent := DeviceEvent{
				DeviceID:  deviceConfig.MonitorID,
				Latitude:  deviceConfig.Lat,
				Longitude: deviceConfig.Long,
				State:     "online",
				EventTime: time.Now(),
			}

			err = pusherClient.Trigger("poweroutages", "device-status", pusherEvent)
			if err != nil {
				log.Infof("failed to publish to pusher")
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

type Config struct {
	Pusher  pusherConfig  `mapstructure:"pusher"`
	Monitor MonitorConfig `mapstructure:"monitor-config"`
}

type pusherConfig struct {
	AppID   string `mapstructure:"app_id"`
	Key     string `mapstructure:"key"`
	Secret  string `mapstructure:"secret"`
	Cluster string `mapstructure:"cluster"`
	Secure  bool   `mapstructure:"secure"`
}

type MonitorConfig struct {
	State          string        `mapstructure:"state"`
	City           string        `mapstructure:"city"`
	Municipality   string        `mapstructure:"municipality"`
	Parish         string        `mapstructure:"parish"`
	MonitorID      string        `mapstructure:"monitor-id"`
	TickerDuration time.Duration `mapstructure:"ticker"`
	Lat            float64       `mapstructure:"lat"`
	Long           float64       `mapstructure:"long"`
}

func loadConfig() Config {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/c4v/poweroutage/")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("failed to unmarshal config: %v", err)
	}

	return config
}
