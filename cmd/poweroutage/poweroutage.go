package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	balenarerebooter "github.com/code-for-venezuela/poweroutage/pkg/balenarebooter"
	"github.com/code-for-venezuela/poweroutage/pkg/eventsreader"
	"github.com/code-for-venezuela/poweroutage/pkg/eventsyncer"
	"github.com/code-for-venezuela/poweroutage/pkg/store"
	"github.com/code-for-venezuela/poweroutage/pkg/ups"
	"github.com/code-for-venezuela/poweroutage/pkg/util"
	"github.com/spf13/viper"

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

	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		fmt.Errorf("MYSQL_DSN environment variable is not set")
		return
	}
	publisher, err := store.NewMySQLPublisher(dsn)
	if err != nil {
		fmt.Printf("Error creating MySQLPublisher: %v\n", err)
		return
	}
	defer publisher.Close()

	syncManager := eventsyncer.NewEventSyncer(1*time.Minute, eventsRecorder, publisher)
	defer syncManager.Close()
	go syncManager.Run(context.Background())

	var event *store.OutageEvent
	event, err = eventsRecorder.GetMostRecentEvent()

	if err == nil {
		log.Infof("warning, there is already an ongoing event. It started at: %v", event.StartTime)
	}

	if err != nil && !strings.Contains(err.Error(), "no outage events recorded") {
		log.Fatalf("unexpected error reading most recent event: %v", err)
	}

	var rebooter *balenarerebooter.Rebooter
	if config.RebooterEnabled {
		log.Infof("Rebooter is enabled. Starting with the following config: (checkInterval: %v), (rebootInterval: %v), (statusFile: %v)",
			config.RebooterCheckInterval,
			config.RebooterRebootInterval,
			config.RebootStateFile,
		)
		rebooter = balenarerebooter.New(
			config.RebooterCheckInterval,
			config.RebooterRebootInterval,
			config.RebootStateFile,
		)
		err := rebooter.Start(context.Background())
		if err != nil {
			log.Panicf("Error initializing rebooter: %v", err)
		}
		defer rebooter.Stop()
	}

	mainLoop(upsManager, event, eventsRecorder, publisher, config)
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
	publisher store.Publisher,
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

	// Make sure that we log info the first time, after that only one log entry per hour.
	// This is to not spam the logs.

	events, err := fetchLastDayProbes(config.MonitorID)
	if err == nil {
		eventCount := eventsreader.CountEventsInDuration(events, 1*time.Hour)
		log.Infof("Found %v events for monitor:", eventCount, config.MonitorID)
		if eventCount >= 5 {
			if events[0].Status != "crashing" {
				publishProbe(publisher, config.MonitorID, "crashing")
			}
			log.Errorf("This device seems to be in a crash loop. There have been %v restarts in the last hour", eventCount)
			return
		}
	}

	if err != nil {
		log.Warnf("Warning, couldn't fetch events to get probe status. Failed to read from db")
	}

	lastProbeTime, lastLog := publishInitialProbe(publisher, config)

	for {
		// Let's initialize a probe timer to send keep alives to angostura
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

				if time.Since(lastLog) >= 1*time.Hour {
					log.Infof(
						"Power is not available. This is the remaining battery: %.1f%%, current: %.1f",
						percentage,
						current,
					)
					lastLog = time.Now()
				}
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
				continue
			}
			if time.Since(lastLog) >= 1*time.Hour {
				log.Infof("Power is available. This is the remaining battery: %.1f%%", percentage)
				lastLog = time.Now()
			}

			if time.Since(lastProbeTime) >= 4*time.Hour {
				newProbeTime := publishProbe(publisher, config.MonitorID, "healthy")
				if !newProbeTime.IsZero() {
					lastProbeTime = newProbeTime
				}
			}
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
		}
	}
}

func fetchLastDayProbes(deviceId string) ([]eventsreader.Event, error) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		return nil, fmt.Errorf("MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	defer db.Close()

	if err := db.Ping(); err != nil {
		return nil, err
	}
	events, err := eventsreader.GetEventsForDevice(db, deviceId)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return []eventsreader.Event{}, nil
	}

	return events, nil
}

func publishInitialProbe(publisher store.Publisher, config Config) (time.Time, time.Time) {
	probeTime := publishProbe(publisher, config.MonitorID, "restarting")
	lastProbeTime := time.Now()

	lastLog := time.Now().Add(-2 * time.Hour)
	if probeTime.IsZero() {
		log.Errorf("failed to published to angostura on start")
	} else {
		lastProbeTime = probeTime
	}
	return lastProbeTime, lastLog
}

func publishProbe(publisher store.Publisher, deviceId, status string) time.Time {
	event := eventsreader.Event{
		DeviceID: deviceId,
		SentAt:   time.Now(),
		Status:   status,
	}
	// Serialize the struct to JSON
	jsonData, err := json.Marshal(event)
	if err != nil {
		panic("Failed to serialized event json")
	}
	err = publisher.Publish("power_outage_probe", jsonData)
	if err != nil {
		log.Errorf("failed to publish probe event to angostura: %v", err)
		return time.Time{}
	}
	log.Infof("successfully published probe event to angostura")
	return time.Now()
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
	State                  string        `mapstructure:"STATE"`
	City                   string        `mapstructure:"CITY"`
	Municipality           string        `mapstructure:"MUNICIPALITY"`
	Parish                 string        `mapstructure:"PARISH"`
	MonitorID              string        `mapstructure:"ID"`
	TickerDuration         time.Duration `mapstructure:"TICKER"`
	Lat                    float64       `mapstructure:"LAT"`
	Long                   float64       `mapstructure:"LONG"`
	EventsFolder           string        `mapstructure:"EVENTS_FOLDER"`
	FinishedEventsFolder   string        `mapstructure:"FINISHED_EVENTS_FOLDER"`
	RebootStateFile        string        `mapstructure:"REBOOT_STATE_FILE"`
	RebooterEnabled        bool          `mapstructure:"REBOOTER_ENABLED"`
	RebooterCheckInterval  time.Duration `mapstructure:"REBOOTER_CHECK_INTERVAL"`
	RebooterRebootInterval time.Duration `mapstructure:"REBOOTER_REBOOT_INTERVAL"`
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
