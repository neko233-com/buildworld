package store

import "fmt"

const pullRequestWebhookSetting = "pull_request_webhook"

// SetProjectPullRequestWebhook configures only a specific existing project.
func (s *Store) SetProjectPullRequestWebhook(projectID int64, enabled bool) error {
	value := "disabled"
	if enabled {
		value = "same-repository"
	}
	result, err := s.db.Exec(`INSERT INTO env_vars(scope, project_id, name, value, is_secret, description)
		SELECT 'project_config', id, ?, ?, FALSE, '' FROM projects WHERE id=?
		ON CONFLICT(scope, project_id, name) DO UPDATE SET value=excluded.value`, pullRequestWebhookSetting, value, projectID)
	if err != nil {
		return fmt.Errorf("update PR webhook setting: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("PR webhook project does not exist")
	}
	return nil
}
