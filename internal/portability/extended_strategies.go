package portability

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neko233-com/buildworld/internal/store"
)

type credentialRecord struct {
	Name        string               `json:"name"`
	Type        store.CredentialType `json:"type"`
	Host        string               `json:"host,omitempty"`
	Username    string               `json:"username,omitempty"`
	Password    string               `json:"password,omitempty"`
	PrivateKey  string               `json:"private_key,omitempty"`
	PublicKey   string               `json:"public_key,omitempty"`
	Token       string               `json:"token,omitempty"`
	Description string               `json:"description,omitempty"`
	IsSecret    bool                 `json:"is_secret"`
}

type credentialStrategy struct{ store *store.Store }

func (credentialStrategy) Capability() Capability {
	return Capability{
		Key: "credentials", Version: 1, Sensitive: true,
		Description: "Version-control credentials and key material",
	}
}

func (s credentialStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	values, err := s.store.ListCredentials(nil)
	if err != nil {
		return nil, err
	}
	records := make([]credentialRecord, 0, len(values))
	for _, value := range values {
		records = append(records, credentialRecord{
			Name: value.Name, Type: value.Type, Host: value.Host, Username: value.Username,
			Password: value.Password, PrivateKey: value.PrivateKey, PublicKey: value.PublicKey,
			Token: value.Token, Description: value.Description, IsSecret: value.IsSecret,
		})
	}
	return json.Marshal(records)
}

func (credentialStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []credentialRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}

func (s credentialStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []credentialRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.Name == "" {
			return result, fmt.Errorf("credential name is required")
		}
		existing, err := s.store.GetCredentialByName(record.Name)
		switch {
		case err == nil && options.Mode == "skip":
			result.Skipped++
		case err == nil:
			if err := s.store.UpdateCredential(existing.ID, record.Name, record.Type, record.Host, record.Username, record.Password, record.PrivateKey, record.PublicKey, record.Token, record.Description, record.IsSecret); err != nil {
				return result, err
			}
			result.Updated++
		case errors.Is(err, sql.ErrNoRows):
			if _, err := s.store.CreateCredential(record.Name, record.Type, record.Host, record.Username, record.Password, record.PrivateKey, record.PublicKey, record.Token, record.Description, record.IsSecret); err != nil {
				return result, err
			}
			result.Created++
		default:
			return result, err
		}
	}
	return result, nil
}

type notificationChannelRecord struct {
	Name        string                        `json:"name"`
	Type        store.NotificationChannelType `json:"type"`
	Config      string                        `json:"config"`
	Conditions  string                        `json:"conditions"`
	Description string                        `json:"description,omitempty"`
	Enabled     bool                          `json:"enabled"`
}

type notificationChannelStrategy struct{ store *store.Store }

func (notificationChannelStrategy) Capability() Capability {
	return Capability{
		Key: "notification_channels", Version: 1, Sensitive: true,
		Description: "Notification channels, delivery rules, and provider credentials",
	}
}

func (s notificationChannelStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	values, err := s.store.ListNotificationChannels()
	if err != nil {
		return nil, err
	}
	records := make([]notificationChannelRecord, 0, len(values))
	for _, value := range values {
		records = append(records, notificationChannelRecord{
			Name: value.Name, Type: value.Type, Config: value.Config,
			Conditions: value.Conditions, Description: value.Description, Enabled: value.Enabled,
		})
	}
	return json.Marshal(records)
}

func (notificationChannelStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []notificationChannelRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}

func (s notificationChannelStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []notificationChannelRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.Name == "" {
			return result, fmt.Errorf("notification channel name is required")
		}
		existing, err := s.store.GetNotificationChannelByName(record.Name)
		switch {
		case err == nil && options.Mode == "skip":
			result.Skipped++
		case err == nil:
			if err := s.store.UpdateNotificationChannel(existing.ID, record.Name, record.Type, record.Config, record.Conditions, record.Description, record.Enabled); err != nil {
				return result, err
			}
			result.Updated++
		case errors.Is(err, sql.ErrNoRows):
			if _, err := s.store.CreateNotificationChannel(record.Name, record.Type, record.Config, record.Conditions, record.Description, record.Enabled); err != nil {
				return result, err
			}
			result.Created++
		default:
			return result, err
		}
	}
	return result, nil
}

type deploymentEnvironmentRecord struct {
	ProjectName string `json:"project_name"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Config      string `json:"config"`
}

type deploymentEnvironmentStrategy struct{ store *store.Store }

func (deploymentEnvironmentStrategy) Capability() Capability {
	return Capability{
		Key: "deployment_environments", Version: 1, Sensitive: true,
		Description: "Project deployment targets and provider configuration",
		DependsOn:   []string{"projects"},
	}
}

func (s deploymentEnvironmentStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	projects, err := s.store.ListProjects()
	if err != nil {
		return nil, err
	}
	records := make([]deploymentEnvironmentRecord, 0)
	for _, project := range projects {
		values, err := s.store.ListDeploymentEnvs(project.ID)
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			records = append(records, deploymentEnvironmentRecord{
				ProjectName: project.Name, Name: value.Name,
				Description: value.Description, Config: value.Config,
			})
		}
	}
	return json.Marshal(records)
}

func (deploymentEnvironmentStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []deploymentEnvironmentRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}

func (s deploymentEnvironmentStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []deploymentEnvironmentRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.ProjectName == "" || record.Name == "" {
			return result, fmt.Errorf("deployment environment project_name and name are required")
		}
		project, err := s.store.GetProjectByName(record.ProjectName)
		if err != nil {
			return result, fmt.Errorf("deployment environment %q project %q: %w", record.Name, record.ProjectName, err)
		}
		existing, err := s.store.GetDeploymentEnvByProjectAndName(project.ID, record.Name)
		switch {
		case err == nil && options.Mode == "skip":
			result.Skipped++
		case err == nil:
			if err := s.store.UpdateDeploymentEnv(existing.ID, record.Name, record.Description, record.Config); err != nil {
				return result, err
			}
			result.Updated++
		case errors.Is(err, sql.ErrNoRows):
			if _, err := s.store.CreateDeploymentEnv(project.ID, record.Name, record.Description, record.Config); err != nil {
				return result, err
			}
			result.Created++
		default:
			return result, err
		}
	}
	return result, nil
}

type gitHookRecord struct {
	ProjectName string             `json:"project_name"`
	Name        string             `json:"name"`
	Event       store.GitHookEvent `json:"event"`
	Branch      string             `json:"branch,omitempty"`
	Secret      string             `json:"secret,omitempty"`
	Enabled     bool               `json:"enabled"`
	BuildParams string             `json:"build_params"`
	Description string             `json:"description,omitempty"`
}

type gitHookStrategy struct{ store *store.Store }

func (gitHookStrategy) Capability() Capability {
	return Capability{
		Key: "git_hooks", Version: 1, Sensitive: true,
		Description: "Project-owned Git hook triggers and signing secrets",
		DependsOn:   []string{"projects"},
	}
}

func (s gitHookStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	projects, err := s.store.ListProjects()
	if err != nil {
		return nil, err
	}
	records := make([]gitHookRecord, 0)
	for _, project := range projects {
		values, err := s.store.ListGitHooks(project.ID)
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			records = append(records, gitHookRecord{
				ProjectName: project.Name, Name: value.Name, Event: value.Event,
				Branch: value.Branch, Secret: value.Secret, Enabled: value.Enabled,
				BuildParams: value.BuildParams, Description: value.Description,
			})
		}
	}
	return json.Marshal(records)
}

func (gitHookStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []gitHookRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}

func (s gitHookStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []gitHookRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.ProjectName == "" || record.Name == "" {
			return result, fmt.Errorf("Git hook project_name and name are required")
		}
		project, err := s.store.GetProjectByName(record.ProjectName)
		if err != nil {
			return result, fmt.Errorf("Git hook %q project %q: %w", record.Name, record.ProjectName, err)
		}
		existing, err := s.store.GetGitHookByProjectAndName(project.ID, record.Name)
		switch {
		case err == nil && options.Mode == "skip":
			result.Skipped++
		case err == nil:
			if err := s.store.UpdateGitHook(existing.ID, record.Name, record.Event, record.Branch, record.Secret, record.Enabled, record.BuildParams, record.Description); err != nil {
				return result, err
			}
			result.Updated++
		case errors.Is(err, sql.ErrNoRows):
			if _, err := s.store.CreateGitHook(project.ID, record.Name, record.Event, record.Branch, record.Secret, record.Enabled, record.BuildParams, record.Description); err != nil {
				return result, err
			}
			result.Created++
		default:
			return result, err
		}
	}
	return result, nil
}
