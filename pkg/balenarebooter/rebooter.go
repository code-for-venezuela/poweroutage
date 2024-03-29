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

// WriteTimestampToFile writes the provided time as a Unix timestamp to the specified file.
func (r *Rebooter) WriteTimestampToFile(timestamp time.Time) error {
	currentTimestamp := strconv.FormatInt(timestamp.Unix(), 10)
	if err := os.WriteFile(r.FilePath, []byte(currentTimestamp), 0644); err != nil {
		return fmt.Errorf("failed to write the timestamp to the file: %w", err)
	}
	return nil
}

// Start begins the periodic check and restart process.
func (r *Rebooter) Start(ctx context.Context) error {
	var cancelCtx context.Context
	cancelCtx, r.cancelFunc = context.WithCancel(ctx)

	if _, err := os.Stat(r.FilePath); os.IsNotExist(err) {
		if err := r.WriteTimestampToFile(time.Now()); err != nil {
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
					lastRestartTime, err := r.GetLastRestartTimestamp()
					if err != nil {
						log.Errorf("There was an error rebooting device. Will retry again in: %v", r.CheckInterval)
						continue
					}
					err = r.restartApp()
					if err != nil {
						// This is effectively rolling back the last restart
						r.WriteTimestampToFile(lastRestartTime)
						log.Errorf("There was an error rebooting device. Will retry again in: %v", r.CheckInterval)
					}
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

func (r *Rebooter) GetLastRestartTimestamp() (time.Time, error) {
	data, err := os.ReadFile(r.FilePath)
	if err != nil {
		return time.Unix(0, 0), fmt.Errorf("error reading file: %w", err)
	}

	// Convert the read data to a string and then to an int64 (Unix timestamp)
	lastRestartTimestamp, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return time.Unix(0, 0), fmt.Errorf("error parsing timestamp: %w", err)
	}

	return time.Unix(lastRestartTimestamp, 0), nil
}

// shouldRestart checks the condition for restarting the app.
func (r *Rebooter) shouldRestart() bool {
	lastRestartTime, err := r.GetLastRestartTimestamp()
	if err != nil {
		log.Errorf("Error reading file: %v", err)
		return false
	}

	// Get the current time and the time of the last restart
	currentTime := time.Now()

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
		log.Infof("Last restart was at %s. Going to restart the app", lastRestartTime.Format("2006-01-02 15:04:05"))
		return true
	}

	// Less than 24 hours have passed since the last restart, so no restart is needed
	return false
}

// restartApp uses the Balena Supervisor API to restart the application.
// restartApp attempts to restart the application using the Balena Supervisor API and returns any errors encountered.
func (r *Rebooter) restartApp() error {
	supervisorAddress := os.Getenv("BALENA_SUPERVISOR_ADDRESS")
	apiKey := os.Getenv("BALENA_SUPERVISOR_API_KEY")

	if supervisorAddress == "" || apiKey == "" {
		return fmt.Errorf("supervisor address or API key not set")
	}

	requestBody, err := json.Marshal(map[string]bool{"force": true})
	if err != nil {
		return fmt.Errorf("error marshaling request body: %w", err)
	}

	url := fmt.Sprintf("%s/v1/reboot?apikey=%s", supervisorAddress, apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("error creating request to balena API: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// It's a good practice to check the response status code to ensure the operation was successful.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to restart app, status code: %d", resp.StatusCode)
	}

	log.Infof("App restarted successfully.")
	return nil
}
