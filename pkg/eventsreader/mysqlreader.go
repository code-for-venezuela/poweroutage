package eventsreader

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type Event struct {
	DeviceID string    `json:"device_id"`
	Status   string    `json:"status"`
	SentAt   time.Time `json:"sent_at"`
}

// GetEventsForDevice retrieves events for a specific device within the last 2 days.
func GetEventsForDevice(db *sql.DB, deviceID string) ([]Event, error) {
	if deviceID == "" {
		return nil, errors.New("deviceID cannot be empty")
	}

	twoDaysAgo := time.Now().AddDate(0, 0, -2)

	query := `
		SELECT payload
		FROM Event
		WHERE created_at >= ?
		AND event_type = 'power_outage_probe'
		ORDER by created_at desc
	`

	rows, err := db.Query(query, twoDaysAgo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event

	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}

		var event Event
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, err
		}

		// Filter by DeviceID at the application level
		if event.DeviceID == deviceID {
			events = append(events, event)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

// CountEventsInDuration counts the number of events that occurred within a specified duration.
func CountEventsInDuration(events []Event, duration time.Duration) int {
	if len(events) == 0 {
		return 0
	}

	var count int
	now := time.Now()

	for _, event := range events {
		if now.Sub(event.SentAt) <= duration {
			count++
		}
	}

	return count
}
