package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

type Client struct {
	client *asynq.Client
}

func NewClient(redisAddr string) *Client {
	return &Client{
		client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr}),
	}
}

func (c *Client) Close() error {
	return c.client.Close()
}

// EnqueueOutlineGeneration implements service.TaskEnqueuer.
func (c *Client) EnqueueOutlineGeneration(ctx context.Context, jobID, requestID, userID string) (string, error) {
	data, err := json.Marshal(OutlinePayload{
		RequestID: requestID,
		JobID:     jobID,
		UserID:    userID,
	})
	if err != nil {
		return "", fmt.Errorf("marshal outline payload: %w", err)
	}
	task := asynq.NewTask(TaskOutlineGeneration, data, asynq.Queue("document"))
	info, err := c.client.EnqueueContext(ctx, task)
	if err != nil {
		return "", fmt.Errorf("enqueue outline generation: %w", err)
	}
	return info.ID, nil
}
