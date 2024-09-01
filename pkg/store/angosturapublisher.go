package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type AngosturaUploader struct {
	Endpoint string
}

func NewAngosturaPubliser(endpoint string) Publisher {
	return &AngosturaUploader{Endpoint: endpoint}
}

func (uploader *AngosturaUploader) Publish(eventType string, payload []byte) error {
	base64Payload := base64.StdEncoding.EncodeToString(payload)
	requestBody := strings.NewReader(fmt.Sprintf("{\"type\":\"%s\",\"version\":\"1\",\"payload\":\"%s\"}", eventType, base64Payload))
	resp, err := http.Post(uploader.Endpoint, "application/json", requestBody)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (uploader *AngosturaUploader) PublishOutageEvent(event OutageEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return uploader.Publish("power_outage_incident", payload)
}
