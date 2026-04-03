package atlassian

import (
	"context"
	"fmt"
	"time"
)

const (
	longTaskPollInterval = 3 * time.Second
	longTaskTimeout      = 5 * time.Minute
)

// longTaskResponse is the response from GET /wiki/rest/api/longtask/{id}.
type longTaskResponse struct {
	Finished          bool `json:"finished"`
	Successful        bool `json:"successful"`
	ErrorCode         int  `json:"errorCode"`
	AdditionalDetails struct {
		WaitingDescription string `json:"waitingDescription"`
	} `json:"additionalDetails"`
}

// PollLongTask polls a Confluence long task until it finishes or times out.
// taskPath should be the relative path like "/wiki/rest/api/longtask/{id}".
func (c *Client) PollLongTask(ctx context.Context, taskPath string) error {
	deadline := time.Now().Add(longTaskTimeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("long task %s timed out after %s", taskPath, longTaskTimeout)
		}

		var task longTaskResponse
		if err := c.Get(ctx, taskPath, &task); err != nil {
			return fmt.Errorf("polling long task %s: %w", taskPath, err)
		}

		if task.Finished {
			if !task.Successful {
				return fmt.Errorf("long task %s failed (errorCode=%d)", taskPath, task.ErrorCode)
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(longTaskPollInterval):
		}
	}
}
