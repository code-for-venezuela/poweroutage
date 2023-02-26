package store

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

type State int

const (
	Unkown State = iota
	Ongoing
	Resolved
)

type OutageEvent struct {
	Status     State
	StartTime  time.Time
	EndTime    time.Time
	LocationID string
}

type OutageRecorder interface {
	StartIncident() (*OutageEvent, error)
	FinishIncident() error
	GetMostRecentEvent() (*OutageEvent, error)
}

type fileSystemRecorder struct {
	locationID string
	eventsDir  string
}

func NewFileSystemRecorder(locationID string, eventsDir string) (OutageRecorder, error) {
	r := &fileSystemRecorder{locationID: locationID, eventsDir: eventsDir}
	if err := r.createEventsDirIfNotExists(); err != nil {
		return nil, fmt.Errorf("error creating events directory: %v", err)
	}
	return r, nil
}

func (r *fileSystemRecorder) StartIncident() (*OutageEvent, error) {
	event := OutageEvent{
		Status:     Ongoing,
		StartTime:  time.Now(),
		LocationID: r.locationID,
	}
	err := r.writeEventToFile(event)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *fileSystemRecorder) FinishIncident() error {
	event, err := r.GetMostRecentEvent()
	if err != nil {
		return fmt.Errorf("error getting most recent event: %v", err)
	}
	if event.Status != Ongoing {
		return fmt.Errorf("cannot finish incident with status %v", event.Status)
	}
	event.Status = Resolved
	event.EndTime = time.Now()

	// Write updated event to the original file
	if err := r.writeEventToFile(*event); err != nil {
		return err
	}

	// Move the file to the finished events folder
	if err := r.moveEventFileToFinishedDir(event); err != nil {
		return err
	}

	return nil
}

func (r *fileSystemRecorder) moveEventFileToFinishedDir(event *OutageEvent) error {
	eventFilename := getEventFilename(event.LocationID, event.StartTime)
	eventFilePath := filepath.Join(r.eventsDir, eventFilename)
	finishedDir := filepath.Join(r.eventsDir, "finished-events")

	if _, err := os.Stat(finishedDir); os.IsNotExist(err) {
		if err := os.Mkdir(finishedDir, 0755); err != nil {
			return err
		}
	}

	finishedFilename := getEventFilename(event.LocationID, event.StartTime) + ".finished"
	finishedFilePath := filepath.Join(finishedDir, finishedFilename)

	if err := os.Rename(eventFilePath, finishedFilePath); err != nil {
		return err
	}

	return nil
}

func getEventFilename(locationID string, startTime time.Time) string {
	return fmt.Sprintf("%v_%v.json", locationID, startTime.Unix())
}

func (r *fileSystemRecorder) createEventsDirIfNotExists() error {
	if _, err := os.Stat(r.eventsDir); os.IsNotExist(err) {
		if err := os.Mkdir(r.eventsDir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func (r *fileSystemRecorder) getEventsDir() (string, error) {
	absPath, err := filepath.Abs(r.eventsDir)
	if err != nil {
		return "", err
	}
	return absPath, nil
}

func (r *fileSystemRecorder) GetMostRecentEvent() (*OutageEvent, error) {
	dir, err := r.getEventsDir()
	if err != nil {
		return &OutageEvent{}, fmt.Errorf("error getting events directory: %v", err)
	}
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return &OutageEvent{}, fmt.Errorf("error reading events directory: %v", err)
	}
	if len(files) != 1 {
		return &OutageEvent{}, fmt.Errorf("no outage events recorded")
	}
	file := files[0]
	eventBytes, err := ioutil.ReadFile(filepath.Join(dir, file.Name()))
	if err != nil {
		return &OutageEvent{}, fmt.Errorf("error reading event file: %v", err)
	}
	var event OutageEvent
	if err := json.Unmarshal(eventBytes, &event); err != nil {
		return &OutageEvent{}, fmt.Errorf("error unmarshaling event file: %v", err)
	}
	return &event, nil
}

func (r *fileSystemRecorder) writeEventToFile(event OutageEvent) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshaling event to JSON: %v", err)
	}
	filename := fmt.Sprintf("%v_%v.json", event.LocationID, event.StartTime.Unix())
	dir, err := r.getEventsDir()
	if err != nil {
		return fmt.Errorf("error getting events directory: %v", err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, filename), eventBytes, 0644); err != nil {
		return fmt.Errorf("error writing event file: %v", err)
	}
	return nil
}
