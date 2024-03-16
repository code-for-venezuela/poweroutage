package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

type State int

const (
	Unkown State = iota
	Ongoing
	Resolved
)

var stateStrings = [...]string{
	"unknown",
	"ongoing",
	"resolved",
}

func (s State) String() string {
	if int(s) < 0 || int(s) >= len(stateStrings) {
		return "Unknown"
	}
	return stateStrings[s]
}
func (s State) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

func (s *State) UnmarshalJSON(data []byte) error {
	var stateStr string
	if err := json.Unmarshal(data, &stateStr); err != nil {
		return err
	}

	for i, str := range stateStrings {
		if stateStr == str {
			*s = State(i)
			return nil
		}
	}

	return fmt.Errorf("invalid State value: %s", stateStr)
}

type OutageEvent struct {
	Status    State     `json:"status"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	DeviceId  string    `json:"device_id"`
	ID        string    `json:"id"`
}

type OutageRecorder interface {
	StartIncident() (*OutageEvent, error)
	FinishIncident() error
	GetMostRecentEvent() (*OutageEvent, error)
	GetFinishedEvents() ([]string, [][]byte, error)
	DeleteEventFile(eventFile string) error
}

type fileSystemRecorder struct {
	locationID string
	eventsDir  string
	finishDir  string
}

func NewFileSystemRecorder(locationID string, eventsDir string, finishDir string) (OutageRecorder, error) {
	r := &fileSystemRecorder{locationID: locationID, eventsDir: eventsDir, finishDir: finishDir}
	if err := r.createEventsDirIfNotExists(); err != nil {
		return nil, fmt.Errorf("error creating events directory: %v", err)
	}
	return r, nil
}

func (r *fileSystemRecorder) StartIncident() (*OutageEvent, error) {
	event := OutageEvent{
		Status:    Ongoing,
		StartTime: time.Now(),
		DeviceId:  r.locationID,
		ID:        uuid.New().String(),
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
	eventFilename := getEventFilename(event.DeviceId, event.StartTime)
	eventFilePath := filepath.Join(r.eventsDir, eventFilename)

	if _, err := os.Stat(r.finishDir); os.IsNotExist(err) {
		if err := os.Mkdir(r.finishDir, 0755); err != nil {
			return err
		}
	}

	finishedFilename := getEventFilename(event.DeviceId, event.StartTime)
	finishedFilePath := filepath.Join(r.finishDir, finishedFilename)

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

func (r *fileSystemRecorder) getEventsDir(dir string) (string, error) {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return absPath, nil
}

func (r *fileSystemRecorder) GetFinishedEvents() ([]string, [][]byte, error) {
	dir, err := r.getEventsDir(r.finishDir)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting events directory: %v", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading events directory: %v", err)
	}
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error reading finished events")
	}

	events := make([][]byte, len(files))
	fileNames := make([]string, len(files))
	for i, file := range files {
		eventBytes, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			return nil, nil, fmt.Errorf("error reading event file: %v", err)
		}
		events[i] = eventBytes
		fileNames[i] = file.Name()
	}

	return fileNames, events, nil
}

func (r *fileSystemRecorder) DeleteEventFile(eventFile string) error {
	dir, err := r.getEventsDir(r.finishDir)
	if err != nil {
		return fmt.Errorf("error getting events directory: %v", err)
	}

	err = os.Remove(filepath.Join(dir, eventFile))
	if err != nil {
		return err
	}
	return nil
}

func (r *fileSystemRecorder) GetMostRecentEvent() (*OutageEvent, error) {
	dir, err := r.getEventsDir(r.eventsDir)
	if err != nil {
		return nil, fmt.Errorf("error getting events directory: %v", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("error reading events directory: %v", err)
	}
	if len(files) != 1 {
		return nil, fmt.Errorf("no outage events recorded")
	}
	file := files[0]
	eventBytes, err := os.ReadFile(filepath.Join(dir, file.Name()))
	if err != nil {
		return nil, fmt.Errorf("error reading event file: %v", err)
	}
	var event OutageEvent
	if err := json.Unmarshal(eventBytes, &event); err != nil {
		filePath := filepath.Join(dir, file.Name())
		deleteFileErr := os.Remove(filePath)
		if deleteFileErr != nil {
			log.Errorf("Error deleting bad event file: %v", deleteFileErr)
		} else {
			log.Infof(
				"Deleted event file that had bad schema: %v. This was the event: %v",
				filePath,
				string(eventBytes),
			)
		}
		return nil, fmt.Errorf("error unmarshaling event file: %v", err)
	}
	return &event, nil
}

func (r *fileSystemRecorder) writeEventToFile(event OutageEvent) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshaling event to JSON: %v", err)
	}
	filename := fmt.Sprintf("%v_%v.json", event.DeviceId, event.StartTime.Unix())
	dir, err := r.getEventsDir(r.eventsDir)
	if err != nil {
		return fmt.Errorf("error getting events directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), eventBytes, 0644); err != nil {
		return fmt.Errorf("error writing event file: %v", err)
	}
	return nil
}
