package engine

import (
	"encoding/json"
	"fmt"
	"time"
)

type BuildParameter struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"` // string, choice, boolean, password, text
	Description string      `json:"description"`
	Default     interface{} `json:"default"`
	Required    bool        `json:"required"`
	Choices     []string    `json:"choices,omitempty"`
	IsSecret    bool        `json:"is_secret"`
}

type BuildConfig struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Parameters  []BuildParameter `json:"parameters"`
	Stages      []Stage          `json:"stages"`
	Triggers    []Trigger        `json:"triggers"`
	Environment map[string]string `json:"environment"`
}

type Stage struct {
	Name     string `json:"name"`
	Steps    []Step `json:"steps"`
	Parallel bool   `json:"parallel,omitempty"`
}

type Step struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"` // shell, git, notify
	Command string            `json:"command,omitempty"`
	Config  map[string]string `json:"config,omitempty"`
}

type Trigger struct {
	Type   string            `json:"type"` // manual, webhook, schedule
	Config map[string]string `json:"config"`
}

type Build struct {
	ID          int64                  `json:"id"`
	ProjectID   int64                  `json:"project_id"`
	Number      int                    `json:"number"`
	Status      string                 `json:"status"`
	Trigger     string                 `json:"trigger"`
	Branch      string                 `json:"branch,omitempty"`
	CommitSHA   string                 `json:"commit_sha,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	FinishedAt  *time.Time             `json:"finished_at,omitempty"`
	DurationMs  *int64                 `json:"duration_ms,omitempty"`
	Log         string                 `json:"log,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

type BuildManager struct {
	builds map[int64][]*Build
}

func NewBuildManager() *BuildManager {
	return &BuildManager{
		builds: make(map[int64][]*Build),
	}
}

func (m *BuildManager) CreateBuild(projectID int64, config *BuildConfig, params map[string]interface{}) *Build {
	// Validate parameters
	if err := m.ValidateParameters(config.Parameters, params); err != nil {
		return nil
	}
	
	// Get next build number
	number := len(m.builds[projectID]) + 1
	
	build := &Build{
		ProjectID:  projectID,
		Number:     number,
		Status:     "pending",
		Trigger:    "manual",
		Parameters: params,
		CreatedAt:  time.Now(),
	}
	
	m.builds[projectID] = append(m.builds[projectID], build)
	return build
}

func (m *BuildManager) ValidateParameters(parameters []BuildParameter, params map[string]interface{}) error {
	for _, param := range parameters {
		val, exists := params[param.Name]
		
		if param.Required && !exists {
			return fmt.Errorf("required parameter %s is missing", param.Name)
		}
		
		if !exists {
			continue
		}
		
		switch param.Type {
		case "choice":
			if !contains(param.Choices, fmt.Sprintf("%v", val)) {
				return fmt.Errorf("parameter %s: invalid choice %v, must be one of %v", param.Name, val, param.Choices)
			}
		case "boolean":
			if _, ok := val.(bool); !ok {
				return fmt.Errorf("parameter %s: must be boolean", param.Name)
			}
		case "string", "password", "text":
			if _, ok := val.(string); !ok {
				return fmt.Errorf("parameter %s: must be string", param.Name)
			}
		}
	}
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (m *BuildManager) GetBuilds(projectID int64) []*Build {
	return m.builds[projectID]
}

func (m *BuildManager) GetBuild(projectID, buildNumber int64) *Build {
	for _, b := range m.builds[projectID] {
		if b.Number == int(buildNumber) {
			return b
		}
	}
	return nil
}

func ParseBuildConfig(jsonStr string) (*BuildConfig, error) {
	config := &BuildConfig{}
	if err := json.Unmarshal([]byte(jsonStr), config); err != nil {
		return nil, err
	}
	return config, nil
}
