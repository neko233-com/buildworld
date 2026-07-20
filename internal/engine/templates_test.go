package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "template-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewTemplateManager(tmpDir)

	// Test Get
	tmpl := m.Get("node-typescript")
	if tmpl == nil {
		t.Fatal("Get(node-typescript) returned nil")
	}
	if tmpl.Name != "Node.js TypeScript" {
		t.Errorf("Name = %s, want Node.js TypeScript", tmpl.Name)
	}

	// Test GetAll
	all := m.GetAll()
	if len(all) < 5 {
		t.Errorf("GetAll() length = %d, want >= 5", len(all))
	}

	// Test non-existent
	if m.Get("nonexistent") != nil {
		t.Error("Get(nonexistent) should return nil")
	}
}

func TestTemplateManagerGetByCategory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "template-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewTemplateManager(tmpDir)

	languages := m.GetByCategory("languages")
	if len(languages) < 3 {
		t.Errorf("GetByCategory(languages) length = %d, want >= 3", len(languages))
	}
}

func TestTemplateManagerSearch(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "template-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewTemplateManager(tmpDir)

	results := m.Search("typescript")
	if len(results) < 1 {
		t.Errorf("Search(typescript) length = %d, want >= 1", len(results))
	}
}

func TestTemplateManagerCreate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "template-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewTemplateManager(tmpDir)

	tmpl := &Template{
		ID:          "custom-template",
		Name:        "Custom Template",
		Description: "A custom template",
		Category:    "custom",
		Difficulty:  "beginner",
		Tags:        []string{"custom"},
		Config: &BuildConfig{
			Name: "custom-build",
		},
	}

	err = m.Create(tmpl)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify creation
	fetched := m.Get("custom-template")
	if fetched == nil {
		t.Fatal("Get(custom-template) returned nil after create")
	}

	// Test duplicate
	err = m.Create(tmpl)
	if err == nil {
		t.Error("Create() should fail for duplicate")
	}
}

func TestTemplateManagerUpdate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "template-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewTemplateManager(tmpDir)

	tmpl := &Template{
		ID:   "update-test",
		Name: "Original",
	}

	err = m.Create(tmpl)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tmpl.Name = "Updated"
	err = m.Update("update-test", tmpl)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	fetched := m.Get("update-test")
	if fetched.Name != "Updated" {
		t.Errorf("Name = %s, want Updated", fetched.Name)
	}
}

func TestTemplateManagerDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "template-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewTemplateManager(tmpDir)

	// Create custom template
	tmpl := &Template{
		ID:   "delete-test",
		Name: "To Delete",
	}

	err = m.Create(tmpl)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete it
	err = m.Delete("delete-test")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if m.Get("delete-test") != nil {
		t.Error("Get(delete-test) should return nil after delete")
	}

	// Test deleting builtin
	err = m.Delete("node-typescript")
	if err == nil {
		t.Error("Delete() should fail for builtin template")
	}
}

func TestTemplateManagerExportImport(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "template-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewTemplateManager(tmpDir)

	// Export
	data, err := m.Export("node-typescript")
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Export() returned empty data")
	}

	// Import with new ID
	importData := []byte(`{
		"id": "imported-template",
		"name": "Imported Template",
		"description": "Imported from file",
		"category": "custom",
		"difficulty": "beginner",
		"tags": ["imported"],
		"config": {"name": "imported-build"}
	}`)

	err = m.Import(importData)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	if m.Get("imported-template") == nil {
		t.Error("Get(imported-template) returned nil after import")
	}
}

func TestTemplateManagerSaveLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "template-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewTemplateManager(tmpDir)

	// Create a custom template to save
	customTmpl := &Template{
		ID:          "custom-save-test",
		Name:        "Custom Save Test",
		Description: "For testing save/load",
		Category:    "custom",
		Difficulty:  "beginner",
		Tags:        []string{"custom"},
		Config: &BuildConfig{
			Name: "custom-build",
		},
	}

	err = m.Create(customTmpl)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Save
	err = m.SaveToFile("custom-save-test")
	if err != nil {
		t.Fatalf("SaveToFile() error = %v", err)
	}

	// Verify file exists
	filePath := filepath.Join(tmpDir, "templates", "custom-save-test.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("SaveToFile() did not create file")
	}

	// Load into new manager
	m2 := NewTemplateManager(tmpDir + "-new")
	err = m2.LoadFromFile(filePath)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	tmpl := m2.Get("custom-save-test")
	if tmpl == nil {
		t.Error("Get(custom-save-test) returned nil after load")
	}
}

func TestProductionValidationTemplates(t *testing.T) {
	manager := NewTemplateManager(t.TempDir())
	goTemplate := manager.Get("go-cli")
	if goTemplate == nil || goTemplate.Config == nil {
		t.Fatal("Go validation template is missing")
	}
	if goTemplate.Config.Toolchains["go"][0] != "1.26" || len(goTemplate.Config.Stages) < 5 {
		t.Fatalf("Go validation template is incomplete: %#v", goTemplate.Config)
	}
	nodeTemplate := manager.Get("node-typescript")
	if nodeTemplate == nil || nodeTemplate.Config == nil {
		t.Fatal("TypeScript validation template is missing")
	}
	if nodeTemplate.Config.Toolchains["node"][0] != "24" || len(nodeTemplate.Config.Stages) < 6 {
		t.Fatalf("TypeScript validation template is incomplete: %#v", nodeTemplate.Config)
	}
}
