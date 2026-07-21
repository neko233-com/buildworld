package portability

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neko233-com/buildworld/internal/plugin"
	"github.com/neko233-com/buildworld/internal/store"
)

func NewDefaultRegistry(data *store.Store, loaders ...*plugin.Loader) (*Registry, error) {
	var loader *plugin.Loader
	if len(loaders) > 0 {
		loader = loaders[0]
	}
	return NewRegistry(
		credentialStrategy{store: data},
		projectGroupStrategy{store: data},
		projectStrategy{store: data},
		settingsStrategy{store: data},
		vcsRootStrategy{store: data},
		templateStrategy{store: data},
		notificationChannelStrategy{store: data},
		pluginSettingsStrategy{store: data, loader: loader},
	)
}

type projectRecord struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	RepoURL       string   `json:"repo_url,omitempty"`
	RepoType      string   `json:"repo_type,omitempty"`
	DefaultBranch string   `json:"default_branch,omitempty"`
	VCSRootName   string   `json:"vcs_root_name,omitempty"`
	TemplateName  string   `json:"template_name,omitempty"`
	GroupName     string   `json:"group_name,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Config        string   `json:"config"`
}

type projectStrategy struct{ store *store.Store }

func (projectStrategy) Capability() Capability {
	return Capability{Key: "projects", Version: 3, Description: "Projects and build-flow configuration", DependsOn: []string{"templates", "vcs_roots", "project_groups"}}
}
func (s projectStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	projects, err := s.store.ListProjects()
	if err != nil {
		return nil, err
	}
	records := make([]projectRecord, 0, len(projects))
	for _, project := range projects {
		var vcsRootName, templateName, groupName string
		if project.VCSRootID != nil {
			root, err := s.store.GetVCSRoot(*project.VCSRootID)
			if err != nil {
				return nil, fmt.Errorf("resolve VCS root for project %q: %w", project.Name, err)
			}
			vcsRootName = root.Name
		}
		if project.TemplateID != nil {
			template, err := s.store.GetBuildTemplate(*project.TemplateID)
			if err != nil {
				return nil, fmt.Errorf("resolve template for project %q: %w", project.Name, err)
			}
			templateName = template.Name
		}
		if project.GroupID != nil {
			group, err := s.store.GetProjectGroup(*project.GroupID)
			if err != nil {
				return nil, fmt.Errorf("resolve group for project %q: %w", project.Name, err)
			}
			groupName = group.Name
		}
		records = append(records, projectRecord{
			Name: project.Name, Description: project.Description, RepoURL: project.RepoURL,
			RepoType: project.RepoType, DefaultBranch: project.DefaultBranch,
			VCSRootName: vcsRootName, TemplateName: templateName,
			GroupName: groupName,
			Tags:      append([]string(nil), project.Tags...), Config: project.Config,
		})
	}
	return json.Marshal(records)
}
func (projectStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []projectRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}
func (s projectStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []projectRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.Name == "" {
			return result, fmt.Errorf("project name is required")
		}
		var vcsRootID, templateID, groupID *int64
		if record.VCSRootName != "" {
			root, err := s.store.GetVCSRootByName(record.VCSRootName)
			if err != nil {
				return result, fmt.Errorf("project %q VCS root %q: %w", record.Name, record.VCSRootName, err)
			}
			value := root.ID
			vcsRootID = &value
		}
		if record.TemplateName != "" {
			template, err := s.store.GetBuildTemplateByName(record.TemplateName)
			if err != nil {
				return result, fmt.Errorf("project %q template %q: %w", record.Name, record.TemplateName, err)
			}
			value := template.ID
			templateID = &value
		}
		if record.GroupName != "" {
			group, err := s.store.GetProjectGroupByName(record.GroupName)
			if err != nil {
				return result, fmt.Errorf("project %q group %q: %w", record.Name, record.GroupName, err)
			}
			value := group.ID
			groupID = &value
		}
		existing, err := s.store.GetProjectByName(record.Name)
		switch {
		case err == nil && options.Mode == "skip":
			result.Skipped++
		case err == nil:
			if err := s.store.UpdateProject(existing.ID, record.Name, record.Description, record.RepoURL, record.RepoType, record.DefaultBranch, record.Config, vcsRootID, templateID, record.Tags); err != nil {
				return result, err
			}
			if err := s.store.SetProjectGroup(existing.ID, groupID); err != nil {
				return result, err
			}
			result.Updated++
		case err == sql.ErrNoRows:
			project, err := s.store.CreateProject(record.Name, record.Description, record.RepoURL, record.RepoType, record.DefaultBranch, record.Config, 0, vcsRootID, templateID, record.Tags)
			if err != nil {
				return result, err
			}
			if err := s.store.SetProjectGroup(project.ID, groupID); err != nil {
				return result, err
			}
			result.Created++
		default:
			return result, err
		}
	}
	return result, nil
}

type projectGroupRecord struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

type projectGroupStrategy struct{ store *store.Store }

func (projectGroupStrategy) Capability() Capability {
	return Capability{Key: "project_groups", Version: 1, Description: "Project groups"}
}

func (s projectGroupStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	groups, err := s.store.ListProjectGroups()
	if err != nil {
		return nil, err
	}
	records := make([]projectGroupRecord, 0, len(groups))
	for _, group := range groups {
		records = append(records, projectGroupRecord{Name: group.Name, Description: group.Description, Color: group.Color})
	}
	return json.Marshal(records)
}

func (projectGroupStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []projectGroupRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}

func (s projectGroupStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []projectGroupRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.Name == "" {
			return result, fmt.Errorf("project group name is required")
		}
		color := record.Color
		if color == "" {
			color = store.ProjectGroupColorNeutral
		}
		if !store.IsValidProjectGroupColor(color) {
			return result, fmt.Errorf("project group %q: %w", record.Name, store.ErrInvalidProjectGroupColor)
		}
		existing, err := s.store.GetProjectGroupByName(record.Name)
		switch {
		case err == nil && options.Mode == "skip":
			result.Skipped++
		case err == nil:
			if err := s.store.UpdateProjectGroupWithColor(existing.ID, record.Name, record.Description, color); err != nil {
				return result, err
			}
			result.Updated++
		case errors.Is(err, sql.ErrNoRows):
			if _, err := s.store.CreateProjectGroupWithColor(record.Name, record.Description, color); err != nil {
				return result, err
			}
			result.Created++
		default:
			return result, err
		}
	}
	return result, nil
}

type settingRecord struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	IsSecret    bool   `json:"is_secret"`
	Description string `json:"description,omitempty"`
	Redacted    bool   `json:"redacted,omitempty"`
}

type settingsStrategy struct{ store *store.Store }

func (settingsStrategy) Capability() Capability {
	return Capability{Key: "settings", Version: 1, Description: "Runtime and validation settings"}
}
func (s settingsStrategy) Export(_ context.Context, options ExportOptions) (json.RawMessage, error) {
	values, err := s.store.ListEnvVars("system", nil)
	if err != nil {
		return nil, err
	}
	records := make([]settingRecord, 0, len(values))
	for _, value := range values {
		record := settingRecord{Name: value.Name, Value: value.Value, IsSecret: value.IsSecret, Description: value.Description}
		if value.IsSecret && !options.IncludeSecrets {
			record.Value = ""
			record.Redacted = true
		}
		records = append(records, record)
	}
	return json.Marshal(records)
}
func (settingsStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []settingRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}
func (s settingsStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []settingRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	existingValues, err := s.store.ListEnvVars("system", nil)
	if err != nil {
		return result, err
	}
	existing := make(map[string]struct{}, len(existingValues))
	for _, value := range existingValues {
		existing[value.Name] = struct{}{}
	}
	for _, record := range records {
		if record.Name == "" {
			return result, fmt.Errorf("setting name is required")
		}
		if record.Redacted {
			result.Skipped++
			continue
		}
		_, exists := existing[record.Name]
		if exists && options.Mode == "skip" {
			result.Skipped++
			continue
		}
		if err := s.store.SetEnvVar("system", nil, record.Name, record.Value, record.IsSecret, record.Description); err != nil {
			return result, err
		}
		if exists {
			result.Updated++
		} else {
			result.Created++
			existing[record.Name] = struct{}{}
		}
	}
	return result, nil
}

type vcsRootRecord struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	URL            string `json:"url"`
	Branch         string `json:"branch"`
	CredentialName string `json:"credential_name,omitempty"`
	PollInterval   int    `json:"poll_interval"`
	AutoCheckout   bool   `json:"auto_checkout"`
	Config         string `json:"config,omitempty"`
}

type vcsRootStrategy struct{ store *store.Store }

func (vcsRootStrategy) Capability() Capability {
	return Capability{Key: "vcs_roots", Version: 2, Description: "Version-control roots and credential references", DependsOn: []string{"credentials"}}
}
func (s vcsRootStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	values, err := s.store.ListVCSRoots()
	if err != nil {
		return nil, err
	}
	records := make([]vcsRootRecord, 0, len(values))
	for _, value := range values {
		var credentialName string
		if value.CredentialID != nil {
			credential, err := s.store.GetCredential(*value.CredentialID)
			if err != nil {
				return nil, fmt.Errorf("resolve credential for VCS root %q: %w", value.Name, err)
			}
			credentialName = credential.Name
		}
		records = append(records, vcsRootRecord{
			Name: value.Name, Type: value.Type, URL: value.URL, Branch: value.Branch,
			CredentialName: credentialName, PollInterval: value.PollInterval,
			AutoCheckout: value.AutoCheckout, Config: value.Config,
		})
	}
	return json.Marshal(records)
}
func (vcsRootStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []vcsRootRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}
func (s vcsRootStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []vcsRootRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.Name == "" {
			return result, fmt.Errorf("VCS root name is required")
		}
		var credentialID *int64
		if record.CredentialName != "" {
			credential, err := s.store.GetCredentialByName(record.CredentialName)
			if err != nil {
				return result, fmt.Errorf("VCS root %q credential %q: %w", record.Name, record.CredentialName, err)
			}
			value := credential.ID
			credentialID = &value
		}
		existing, err := s.store.GetVCSRootByName(record.Name)
		switch {
		case err == nil && options.Mode == "skip":
			result.Skipped++
		case err == nil:
			if err := s.store.UpdateVCSRoot(existing.ID, record.Name, record.Type, record.URL, record.Branch, credentialID, record.PollInterval, record.AutoCheckout, record.Config); err != nil {
				return result, err
			}
			result.Updated++
		case err == sql.ErrNoRows:
			if _, err := s.store.CreateVCSRoot(record.Name, record.Type, record.URL, record.Branch, credentialID, record.PollInterval, record.AutoCheckout, record.Config); err != nil {
				return result, err
			}
			result.Created++
		default:
			return result, err
		}
	}
	return result, nil
}

type templateRecord struct {
	Name        string `json:"name"`
	Config      string `json:"config"`
	Description string `json:"description,omitempty"`
}

type templateStrategy struct{ store *store.Store }

func (templateStrategy) Capability() Capability {
	return Capability{Key: "templates", Version: 1, Description: "Reusable build templates"}
}
func (s templateStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	values, err := s.store.ListBuildTemplates()
	if err != nil {
		return nil, err
	}
	records := make([]templateRecord, 0, len(values))
	for _, value := range values {
		records = append(records, templateRecord{Name: value.Name, Config: value.Config, Description: value.Description})
	}
	return json.Marshal(records)
}
func (templateStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []templateRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}
func (s templateStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []templateRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.Name == "" {
			return result, fmt.Errorf("template name is required")
		}
		existing, err := s.store.GetBuildTemplateByName(record.Name)
		switch {
		case err == nil && options.Mode == "skip":
			result.Skipped++
		case err == nil:
			if err := s.store.UpdateBuildTemplate(existing.ID, record.Name, record.Config, record.Description); err != nil {
				return result, err
			}
			result.Updated++
		case err == sql.ErrNoRows:
			if _, err := s.store.CreateBuildTemplate(record.Name, record.Config, record.Description); err != nil {
				return result, err
			}
			result.Created++
		default:
			return result, err
		}
	}
	return result, nil
}
