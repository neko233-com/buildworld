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

type pluginSettingsRecord struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type pluginSettingsStrategy struct {
	store  *store.Store
	loader *plugin.Loader
}

func (pluginSettingsStrategy) Capability() Capability {
	return Capability{
		Key:         "plugin_settings",
		Version:     1,
		Description: "Activation preferences for plugins already installed on the target",
	}
}

func (s pluginSettingsStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	values, err := s.store.ListPlugins()
	if err != nil {
		return nil, err
	}
	records := make([]pluginSettingsRecord, 0, len(values))
	for _, value := range values {
		records = append(records, pluginSettingsRecord{Name: value.Name, Enabled: value.Enabled})
	}
	return json.Marshal(records)
}

func (pluginSettingsStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []pluginSettingsRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}

func (s pluginSettingsStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []pluginSettingsRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.Name == "" {
			return result, fmt.Errorf("plugin name is required")
		}
		existing, err := s.store.GetPluginByName(record.Name)
		if errors.Is(err, sql.ErrNoRows) {
			// Configuration bundles never install executable plugin code. The
			// target administrator must install and trust that plugin first.
			result.Skipped++
			continue
		}
		if err != nil {
			return result, err
		}
		if options.Mode == "skip" {
			result.Skipped++
			continue
		}
		if err := s.store.UpdatePluginEnabled(existing.ID, record.Enabled); err != nil {
			return result, err
		}
		if s.loader != nil {
			s.loader.SetEnabled(record.Name, record.Enabled)
		}
		result.Updated++
	}
	return result, nil
}
