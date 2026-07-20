package engine

import (
	"testing"
)

func TestEnvManagerGlobal(t *testing.T) {
	m := NewEnvManager()

	m.SetGlobal("TOKEN", "abc123", true, "API token")
	m.SetGlobal("REGISTRY", "registry.example.com", false, "Docker registry")

	// Test retrieval
	val, ok := m.Get("TOKEN", 0)
	if !ok || val != "abc123" {
		t.Errorf("Get(TOKEN) = %q, %v, want abc123, true", val, ok)
	}

	val, ok = m.Get("REGISTRY", 0)
	if !ok || val != "registry.example.com" {
		t.Errorf("Get(REGISTRY) = %q, %v, want registry.example.com, true", val, ok)
	}

	// Test non-existent
	_, ok = m.Get("NONEXISTENT", 0)
	if ok {
		t.Error("Get(NONEXISTENT) should return false")
	}
}

func TestEnvManagerProject(t *testing.T) {
	m := NewEnvManager()

	m.SetGlobal("GLOBAL_VAR", "global-value", false, "")
	m.SetProject(1, "PROJECT_VAR", "project-value", false, "")
	m.SetProject(1, "GLOBAL_VAR", "overridden", false, "")

	// Project overrides global
	val, ok := m.Get("GLOBAL_VAR", 1)
	if !ok || val != "overridden" {
		t.Errorf("Get(GLOBAL_VAR, 1) = %q, want overridden", val)
	}

	// Project-specific var
	val, ok = m.Get("PROJECT_VAR", 1)
	if !ok || val != "project-value" {
		t.Errorf("Get(PROJECT_VAR, 1) = %q, want project-value", val)
	}

	// Global var visible when no project
	val, ok = m.Get("GLOBAL_VAR", 0)
	if !ok || val != "global-value" {
		t.Errorf("Get(GLOBAL_VAR, 0) = %q, want global-value", val)
	}
}

func TestEnvManagerResolve(t *testing.T) {
	m := NewEnvManager()

	m.SetGlobal("REGISTRY", "registry.example.com", false, "")
	m.SetProject(1, "API_KEY", "secret123", true, "")

	// Test resolution
	result := m.Resolve("push to ${global.REGISTRY}", 1)
	if result != "push to registry.example.com" {
		t.Errorf("Resolve() = %q, want push to registry.example.com", result)
	}

	result = m.Resolve("key=${project.API_KEY}", 1)
	if result != "key=secret123" {
		t.Errorf("Resolve() = %q, want key=secret123", result)
	}
}

func TestEnvManagerMaskSecrets(t *testing.T) {
	m := NewEnvManager()

	m.SetGlobal("TOKEN", "abc123", true, "")
	m.SetGlobal("REGISTRY", "registry.example.com", false, "")

	masked := m.MaskSecrets(0)

	if masked["TOKEN"].Value != "***" {
		t.Errorf("MaskSecrets(TOKEN) = %q, want ***", masked["TOKEN"].Value)
	}

	if masked["REGISTRY"].Value != "registry.example.com" {
		t.Errorf("MaskSecrets(REGISTRY) = %q, want registry.example.com", masked["REGISTRY"].Value)
	}
}

func TestEnvManagerDelete(t *testing.T) {
	m := NewEnvManager()

	m.SetGlobal("TO_DELETE", "value", false, "")
	m.SetProject(1, "PROJECT_DEL", "value", false, "")

	if !m.Delete("TO_DELETE", 0) {
		t.Error("Delete(TO_DELETE) should return true")
	}

	_, ok := m.Get("TO_DELETE", 0)
	if ok {
		t.Error("TO_DELETE should be deleted")
	}

	if !m.Delete("PROJECT_DEL", 1) {
		t.Error("Delete(PROJECT_DEL, 1) should return true")
	}
}

func TestEnvManagerGetAll(t *testing.T) {
	m := NewEnvManager()

	m.SetGlobal("G1", "global1", false, "")
	m.SetGlobal("G2", "global2", false, "")
	m.SetProject(1, "P1", "project1", false, "")
	m.SetProject(1, "G1", "overridden", false, "")

	all := m.GetAll(1)

	if len(all) != 3 {
		t.Errorf("GetAll() length = %d, want 3", len(all))
	}

	if all["G1"].Value != "overridden" {
		t.Errorf("GetAll()[G1] = %q, want overridden", all["G1"].Value)
	}

	if all["P1"].Value != "project1" {
		t.Errorf("GetAll()[P1] = %q, want project1", all["P1"].Value)
	}
}
