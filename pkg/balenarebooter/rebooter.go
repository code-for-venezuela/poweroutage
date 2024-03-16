package balenarerebooter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)

// Rebooter provides functionality to periodically check a condition and restart the app using Balena's Supervisor API.
type Rebooter struct {
	CheckInterval  time.Duration
	RebootInterval time.Duration
	FilePath       string
	cancelFunc     context.CancelFunc // Store the cancel function to allow stopping
}

// New creates a new Restarter instance.
func New(interval, rebootInterval time.Duration, filePath string) *Rebooter {
	return &Rebooter{
		CheckInterval:  interval,
		RebootInterval: rebootInterval,
		FilePath:       filePath,
	}
}

// Start begins the periodic check and restart process.
// Start begins the periodic check and restart process. It now returns an error if it fails to initialize the restart timestamp file.
func (r *Rebooter) Start(ctx context.Context) error {
	var cancelCtx context.Context
	cancelCtx, r.cancelFunc = context.WithCancel(ctx)

	if _, err := os.Stat(r.FilePath); os.IsNotExist(err) {
		// File does not exist, so initialize it with the current Unix timestamp
		currentTimestamp := strconv.FormatInt(time.Now().Unix(), 10)
		if err := os.WriteFile(r.FilePath, []byte(currentTimestamp), 0644); err != nil {
			return fmt.Errorf("failed to initialize the timestamp file: %v", err)
		}
	} else if err != nil {
		// An error occurred that isn't related to the file's existence
		return fmt.Errorf("error checking the timestamp file: %v", err)
	}

	go func() {
		ticker := time.NewTicker(r.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-cancelCtx.Done():
				log.Info("Stopping rebooter")
				return
			case <-ticker.C:
				if r.shouldRestart() {
					r.restartApp()
				}
			}
		}
	}()
	return nil
}

// Stop stops the rebooter goroutine.
func (r *Rebooter) Stop() {
	if r.cancelFunc != nil {
		r.cancelFunc()
	}
}

// shouldRestart checks the condition for restarting the app.
func (r *Rebooter) shouldRestart() bool {
	data, err := os.ReadFile(r.FilePath)
	if err != nil {
		log.Errorf("Error reading file: %v", err)
		// Consider whether you want to trigger a restart if the file can't be read.
		// For this example, we assume no restart if there's an error.
		return false
	}

	// Convert the read data to a string and then to an int64 (Unix timestamp)
	lastRestartTimestamp, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		log.Errorf("Error parsing timestamp: %v", err)
		// If the timestamp is invalid, do not restart.
		return false
	}

	// Get the current time and the time of the last restart
	currentTime := time.Now()
	lastRestartTime := time.Unix(lastRestartTimestamp, 0)

	// Check if more than 24 hours have passed since the last restart
	if currentTime.Sub(lastRestartTime) > r.RebootInterval {
		// Update the file with the current Unix timestamp
		currentTimestamp := strconv.FormatInt(currentTime.Unix(), 10)
		err = os.WriteFile(r.FilePath, []byte(currentTimestamp), 0644)
		if err != nil {
			log.Errorf("Error writing current timestamp to file: %v", err)
			// If unable to write the new timestamp, consider how you want to handle this.
			// For this example, we proceed with the restart logic despite the file write error.
		}

		// More than RebootInterval hours have passed, so indicate a restart is needed
		return true
	}

	// Less than 24 hours have passed since the last restart, so no restart is needed
	return false
}

// restartApp uses the Balena Supervisor API to restart the application.
func (r *Rebooter) restartApp() {
	supervisorAddress := os.Getenv("BALENA_SUPERVISOR_ADDRESS")
	apiKey := os.Getenv("BALENA_SUPERVISOR_API_KEY")

	if supervisorAddress == "" || apiKey == "" {
		log.Errorf("Supervisor address or API key not set.")
		return
	}

	requestBody, err := json.Marshal(map[string]bool{"force": true})
	if err != nil {
		log.Errorf("Error marshaling request body: %v", err)
		return
	}

	url := fmt.Sprintf("%s/v1/reboot?apikey=%s", supervisorAddress, apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		log.Errorf("Error creating request: %v", err)
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Error making request: %v", err)
		return
	}
	defer resp.Body.Close()

	log.Infof("App restarted successfully.")
}
