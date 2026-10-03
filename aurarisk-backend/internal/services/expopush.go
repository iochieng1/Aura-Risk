package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultExpoPushURL   = "https://exp.host/--/api/v2/push/send"
	expoMaxBatchSize     = 100
	RiskAlertChannelID   = "risk-alerts"
	expoDeviceNotFoundID = "DeviceNotRegistered"
)

type PushMessage struct {
	To        string         `json:"to"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Data      map[string]any `json:"data,omitempty"`
	Sound     string         `json:"sound,omitempty"`
	Priority  string         `json:"priority,omitempty"`
	ChannelID string         `json:"channelId,omitempty"`
}

type PushTicket struct {
	Status  string `json:"status"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
	Details struct {
		Error string `json:"error,omitempty"`
	} `json:"details,omitempty"`
}

// DeviceGone reports whether Expo says the token should no longer be used.
func (t PushTicket) DeviceGone() bool {
	return t.Status == "error" && t.Details.Error == expoDeviceNotFoundID
}

type ExpoPushClient struct {
	URL         string
	AccessToken string
	HTTP        *http.Client
}

func NewExpoPushClient(url, accessToken string) *ExpoPushClient {
	if url == "" {
		url = DefaultExpoPushURL
	}
	return &ExpoPushClient{
		URL:         url,
		AccessToken: accessToken,
		HTTP:        &http.Client{Timeout: 15 * time.Second},
	}
}

func IsExpoPushToken(token string) bool {
	return (strings.HasPrefix(token, "ExponentPushToken[") || strings.HasPrefix(token, "ExpoPushToken[")) &&
		strings.HasSuffix(token, "]") && len(token) <= 200
}

// Send delivers messages in batches and returns one ticket per message, in order.
func (c *ExpoPushClient) Send(ctx context.Context, messages []PushMessage) ([]PushTicket, error) {
	tickets := make([]PushTicket, 0, len(messages))
	for start := 0; start < len(messages); start += expoMaxBatchSize {
		end := start + expoMaxBatchSize
		if end > len(messages) {
			end = len(messages)
		}
		batch, err := c.sendBatch(ctx, messages[start:end])
		if err != nil {
			return tickets, err
		}
		tickets = append(tickets, batch...)
	}
	return tickets, nil
}

func (c *ExpoPushClient) sendBatch(ctx context.Context, messages []PushMessage) ([]PushTicket, error) {
	payload, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("expo push request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("expo push returned status %d", resp.StatusCode)
	}

	var body struct {
		Data []PushTicket `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode expo push response: %w", err)
	}
	if len(body.Data) != len(messages) {
		return nil, fmt.Errorf("expo push returned %d tickets for %d messages", len(body.Data), len(messages))
	}
	return body.Data, nil
}
