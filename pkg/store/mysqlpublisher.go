package store

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLPublisher struct {
	DB *sql.DB
}

func NewMySQLPublisher(dsn string) (*MySQLPublisher, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// Optionally, ping the database to ensure the connection is established
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return &MySQLPublisher{DB: db}, nil
}

func (publisher *MySQLPublisher) Publish(eventType string, payload []byte) error {
	_, err := publisher.DB.Exec(
		"INSERT INTO Event (event_type, payload) VALUES (?, ?)",
		eventType, payload,
	)
	if err != nil {
		return fmt.Errorf("failed to publish event: %v", err)
	}
	return nil
}

func (publisher *MySQLPublisher) PublishOutageEvent(event OutageEvent) error {
	_, err := publisher.DB.Exec(
		"INSERT INTO OutageEvent (id, status, start_time, end_time, device_id) VALUES (?, ?, ?, ?, ?)",
		event.ID, event.Status, event.StartTime, event.EndTime, event.DeviceId,
	)
	if err != nil {
		return fmt.Errorf("failed to publish outage event: %v", err)
	}
	return nil
}
