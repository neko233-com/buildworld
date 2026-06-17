package engine

import (
	"testing"
)

func TestBuildManagerCreate(t *testing.T) {
	m := NewBuildManager()
	
	config := &BuildConfig{
		Name: "test-build",
		Parameters: []BuildParameter{
			{Name: "env", Type: "choice", Choices: []string{"dev", "prod"}, Required: true},
			{Name: "debug", Type: "boolean", Default: false},
		},
	}
	
	params := map[string]interface{}{
		"env": "dev",
	}
	
	build := m.CreateBuild(1, config, params)
	if build == nil {
		t.Fatal("CreateBuild() returned nil")
	}
	
	if build.Number != 1 {
		t.Errorf("Number = %d, want 1", build.Number)
	}
	
	if build.Status != "pending" {
		t.Errorf("Status = %s, want pending", build.Status)
	}
}

func TestBuildManagerValidate(t *testing.T) {
	m := NewBuildManager()
	
	config := &BuildConfig{
		Parameters: []BuildParameter{
			{Name: "env", Type: "choice", Choices: []string{"dev", "prod"}, Required: true},
			{Name: "count", Type: "string"},
		},
	}
	
	// Missing required param
	err := m.ValidateParameters(config.Parameters, map[string]interface{}{})
	if err == nil {
		t.Error("Validate() should fail for missing required param")
	}
	
	// Invalid choice
	err = m.ValidateParameters(config.Parameters, map[string]interface{}{"env": "staging"})
	if err == nil {
		t.Error("Validate() should fail for invalid choice")
	}
	
	// Valid params
	err = m.ValidateParameters(config.Parameters, map[string]interface{}{"env": "dev"})
	if err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestBuildManagerGetBuilds(t *testing.T) {
	m := NewBuildManager()
	
	config := &BuildConfig{
		Name: "test-build",
		Parameters: []BuildParameter{
			{Name: "env", Type: "string"},
		},
	}
	
	m.CreateBuild(1, config, map[string]interface{}{"env": "dev"})
	m.CreateBuild(1, config, map[string]interface{}{"env": "prod"})
	m.CreateBuild(2, config, map[string]interface{}{"env": "dev"})
	
	builds := m.GetBuilds(1)
	if len(builds) != 2 {
		t.Errorf("GetBuilds(1) length = %d, want 2", len(builds))
	}
	
	builds = m.GetBuilds(2)
	if len(builds) != 1 {
		t.Errorf("GetBuilds(2) length = %d, want 1", len(builds))
	}
}

func TestBuildManagerGetBuild(t *testing.T) {
	m := NewBuildManager()
	
	config := &BuildConfig{
		Name: "test-build",
		Parameters: []BuildParameter{
			{Name: "env", Type: "string"},
		},
	}
	
	m.CreateBuild(1, config, map[string]interface{}{"env": "dev"})
	m.CreateBuild(1, config, map[string]interface{}{"env": "prod"})
	
	build := m.GetBuild(1, 1)
	if build == nil {
		t.Fatal("GetBuild(1, 1) returned nil")
	}
	
	if build.Number != 1 {
		t.Errorf("Number = %d, want 1", build.Number)
	}
	
	build = m.GetBuild(1, 2)
	if build == nil {
		t.Fatal("GetBuild(1, 2) returned nil")
	}
	
	if build.Number != 2 {
		t.Errorf("Number = %d, want 2", build.Number)
	}
}

func TestBuildConfigParse(t *testing.T) {
	jsonStr := `{
		"name": "test-build",
		"parameters": [
			{"name": "env", "type": "choice", "choices": ["dev", "prod"], "required": true},
			{"name": "debug", "type": "boolean", "default": false}
		]
	}`
	
	config, err := ParseBuildConfig(jsonStr)
	if err != nil {
		t.Fatalf("ParseBuildConfig() error = %v", err)
	}
	
	if config.Name != "test-build" {
		t.Errorf("Name = %s, want test-build", config.Name)
	}
	
	if len(config.Parameters) != 2 {
		t.Errorf("Parameters length = %d, want 2", len(config.Parameters))
	}
}
