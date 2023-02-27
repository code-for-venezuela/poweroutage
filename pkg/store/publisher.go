package store

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

type AngosturaUploader struct {
	Endpoint string
}

func NewAngosturaPubliser(endpoint string) *AngosturaUploader {
	return &AngosturaUploader{Endpoint: endpoint}
}

func (uploader *AngosturaUploader) Publish(payload []byte) error {
	base64Payload := base64.StdEncoding.EncodeToString(payload)
	requestBody := strings.NewReader(fmt.Sprintf("{\"type\":\"power_outage_tester\",\"version\":\"1\",\"payload\":\"%s\"}", base64Payload))
	resp, err := http.Post(uploader.Endpoint, "application/json", requestBody)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
