package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type EmailQueue struct {
	Token     string
	WorkerURL string
	Client    *http.Client
}

func NewEmailQueue(token, workerURL string) *EmailQueue {
	return &EmailQueue{
		Token:     token,
		WorkerURL: strings.TrimRight(workerURL, "/"),
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (q *EmailQueue) QueueOrderEmail(data OrderEmailData) error {
	if q.Token == "" || q.WorkerURL == "" {
		return fmt.Errorf("email queue is not configured")
	}

	// Convert the email data into JSON.
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("could not encode email job: %w", err)
	}

	// QStash will make a POST request to this URL.
	qstashURL := "https://qstash.upstash.io/v2/publish/" + q.WorkerURL

	req, err := http.NewRequest(
		http.MethodPost,
		qstashURL,
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+q.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Upstash-Retries", "3")

	resp, err := q.Client.Do(req)
	if err != nil {
		return fmt.Errorf("could not publish email job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(
			io.LimitReader(resp.Body, 4096),
		)

		return fmt.Errorf(
			"QStash returned status %d: %s",
			resp.StatusCode,
			string(responseBody),
		)
	}

	return nil
}
