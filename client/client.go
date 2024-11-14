// internal/client/client.go
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"tig/internal/intent"
	"tig/internal/stream"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Second * 10,
		},
	}
}

// Intent operations
func (c *Client) CreateIntent(description, intentType string) (*intent.Intent, error) {
	i := &intent.Intent{
		Type:        intentType,
		Description: description,
		Impact:      intent.Impact{},
		Metadata:    intent.Metadata{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	data, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/intents", c.baseURL),
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var result intent.Intent
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) ListIntents() ([]*intent.Intent, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/api/intents", c.baseURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var intents []*intent.Intent
	if err := json.NewDecoder(resp.Body).Decode(&intents); err != nil {
		return nil, err
	}

	return intents, nil
}

// Stream operations
func (c *Client) CreateStream(name, streamType string) (*stream.Stream, error) {
	st := &stream.Stream{
		Name: name,
		Type: streamType,
		Config: stream.Config{
			AutoMerge:    true,
			FeatureFlags: []stream.FeatureFlag{},
			Protection: stream.Protection{
				RequiredReviewers: 1,
				RequiredChecks:    []string{},
			},
		},
		State: stream.State{
			Active:   true,
			Status:   "stable",
			LastSync: time.Now(),
			Intents:  []string{},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	data, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/streams", c.baseURL),
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var result stream.Stream
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) AddIntentToStream(streamID, intentID string) error {
	data, err := json.Marshal(map[string]string{
		"intent_id": intentID,
	})
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/streams/%s/intents", c.baseURL, streamID),
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	return nil
}