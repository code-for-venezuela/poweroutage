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
	log.Infof("This is the config: %+v", config)

	customFormatter := new(log.TextFormatter)
	customFormatter.TimestampFormat = "2006-01-02 15:04:05"
	customFormatter.FullTimestamp = true
	log.SetFormatter(customFormatter)

	// Check required flags
	if config.State == "" ||
		config.City == "" ||
		config.Municipality == "" ||
		config.Parish == "" ||
		config.MonitorID == "" ||
		config.Lat == 0 ||
		config.Long == 0 {
		fmt.Println("state, city, municipality, parish, and monitor-id are required flags")
		flag.Usage()
		return
	}

	log.Infof("starting power outage monitor for %s, %s, %s, %s (monitor ID: %s)",
		config.State,
		config.City,
		config.Municipality,
		config.Parish,
		config.MonitorID)

	upsManager := ups.NewManager()

	defer upsManager.Close()

	eventsRecorder, err := store.NewFileSystemRecorder(
		config.MonitorID,
		config.EventsFolder,
		config.FinishedEventsFolder)

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

	ticker := time.NewTicker(config.TickerDuration)
	defer ticker.Stop()
	statsd := util.GetProvider()
	baseTags := []string{
		"state:" + config.State,
		"city:" + config.City,
		"municipality:" + config.Municipality,
		"parish:" + config.Parish,
		"monitor-id:" + config.MonitorID,
	}

	pusherClient := pusher.Client{
		AppID:   config.AppID,
		Key:     config.Key,
		Secret:  config.Secret,
		Cluster: config.Cluster,
		Secure:  config.Secure,
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
			if current < -10 {
				log.Infof(
					"Power is not available. This is the remaining battery: %.1f%%, current: %.1f",
					percentage,
					current,
				)
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
				DeviceID:  config.MonitorID,
				Latitude:  config.Lat,
				Longitude: config.Long,
				State:     "online",
				EventTime: time.Now(),
			}

			err = pusherClient.Trigger("poweroutages", "device-status", pusherEvent)
			if err != nil {
				log.Infof("failed to publish to pusher: %v", err)
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
	AppID                string        `mapstructure:"PUSHER_APP_ID"`
	Key                  string        `mapstructure:"PUSHER_APP_KEY"`
	Secret               string        `mapstructure:"PUSHER_SECRET"`
	Cluster              string        `mapstructure:"PUSHER_CLUSTER"`
	Secure               bool          `mapstructure:"PUSHER_SECURE"`
	State                string        `mapstructure:"STATE"`
	City                 string        `mapstructure:"CITY"`
	Municipality         string        `mapstructure:"MUNICIPALITY"`
	Parish               string        `mapstructure:"PARISH"`
	MonitorID            string        `mapstructure:"ID"`
	TickerDuration       time.Duration `mapstructure:"TICKER"`
	Lat                  float64       `mapstructure:"LAT"`
	Long                 float64       `mapstructure:"LONG"`
	EventsFolder         string        `mapstructure:"EVENTS_FOLDER"`
	FinishedEventsFolder string        `mapstructure:"FINISHED_EVENTS_FOLDER"`
}

func loadConfig() Config {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/c4v/poweroutage/")
	viper.SetConfigType("env")
	viper.SetEnvPrefix("monitor")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("failed to unmarshal config: %v", err)
	}

	return config
}
