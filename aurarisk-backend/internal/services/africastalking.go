package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	AfricasTalkingSMSURL        = "https://api.africastalking.com/version1/messaging"
	AfricasTalkingSandboxSMSURL = "https://api.sandbox.africastalking.com/version1/messaging"
)

// SMSSender sends SMS replies.
type SMSSender interface {
	Send(ctx context.Context, to, message string) error
}

// AfricasTalkingSMS sends SMS through Africa's Talking's messaging API.
type AfricasTalkingSMS struct {
	URL      string
	Username string
	APIKey   string
	// Short code or sender ID to send from; empty uses the account default.
	From string
	HTTP *http.Client
}

func (a *AfricasTalkingSMS) Send(ctx context.Context, to, message string) error {
	form := url.Values{"username": {a.Username}, "to": {to}, "message": {message}}
	if a.From != "" {
		form.Set("from", a.From)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.URL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apiKey", a.APIKey)

	client := a.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("africa's talking: status %d", resp.StatusCode)
	}
	// A 2xx can still carry a per-recipient failure.
	var result struct {
		SMSMessageData struct {
			Recipients []struct {
				Status     string `json:"status"`
				StatusCode int    `json:"statusCode"`
			} `json:"Recipients"`
		} `json:"SMSMessageData"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("africa's talking: %w", err)
	}
	for _, r := range result.SMSMessageData.Recipients {
		if r.StatusCode != 100 && r.StatusCode != 101 && r.StatusCode != 102 {
			return fmt.Errorf("africa's talking: recipient status %q", r.Status)
		}
	}
	return nil
}
