package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/codexylab/alvex-backend/pkg/queue"
	"github.com/codexylab/alvex-backend/pkg/repository"
)

// NewDeadJobWebhook returns a notifier compatible with common JSON webhook
// receivers. It deliberately excludes the job payload because jobs can contain
// customer messages, documents, and signed provider events.
func NewDeadJobWebhook(webhookURL, environment string) queue.DeadJobAlert {
	if strings.TrimSpace(webhookURL) == "" {
		return nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	return func(ctx context.Context, job repository.BackgroundJob, _ error) error {
		payload, err := json.Marshal(map[string]interface{}{
			"event":        "background_job.dead",
			"environment":  environment,
			"job_id":       job.ID,
			"job_type":     job.JobType,
			"attempts":     job.Attempts,
			"max_attempts": job.MaxAttempts,
			"summary":      "job processing failed after maximum attempts",
			"occurred_at":  time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return fmt.Errorf("encode dead-job alert: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("create dead-job alert request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("send dead-job alert: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("dead-job alert endpoint returned HTTP %d", resp.StatusCode)
		}
		return nil
	}
}
