package engine

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

type EnvVar struct {
	ID          int64  `json:"id"`
	Scope       string `json:"scope"` // global, project
	Name        string `json:"name"`
	Value       string `json:"value"`
	IsSecret    bool   `json:"is_secret"`
	Description string `json:"description,omitempty"`
	ProjectID   int64  `json:"project_id,omitempty"`
}

type EnvManager struct {
	mu      sync.RWMutex
	global  map[string]*EnvVar
	project map[int64]map[string]*EnvVar
}

func NewEnvManager() *EnvManager {
	return &EnvManager{
		global:  make(map[string]*EnvVar),
		project: make(map[int64]map[string]*EnvVar),
	}
}

func (m *EnvManager) SetGlobal(name, value string, isSecret bool, description string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.global[name] = &EnvVar{
		Scope:       "global",
		Name:        name,
		Value:       value,
		IsSecret:    isSecret,
		Description: description,
	}
}

func (m *EnvManager) SetProject(projectID int64, name, value string, isSecret bool, description string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.project[projectID] == nil {
		m.project[projectID] = make(map[string]*EnvVar)
	}
	m.project[projectID][name] = &EnvVar{
		Scope:       "project",
		Name:        name,
		Value:       value,
		IsSecret:    isSecret,
		Description: description,
		ProjectID:   projectID,
	}
}

func (m *EnvManager) Get(name string, projectID int64) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Project variables override global
	if projectVars, ok := m.project[projectID]; ok {
		if v, ok := projectVars[name]; ok {
			return v.Value, true
		}
	}
	
	// Fall back to global
	if v, ok := m.global[name]; ok {
		return v.Value, true
	}
	
	return "", false
}

func (m *EnvManager) GetAll(projectID int64) map[string]*EnvVar {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]*EnvVar)
	
	// Add global vars
	for k, v := range m.global {
		result[k] = v
	}
	
	// Override with project vars
	if projectVars, ok := m.project[projectID]; ok {
		for k, v := range projectVars {
			result[k] = v
		}
	}
	
	return result
}

func (m *EnvManager) Resolve(template string, projectID int64) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := template
	
	// Replace ${global.X} patterns
	for name, v := range m.global {
		placeholder := fmt.Sprintf("${global.%s}", name)
		result = strings.ReplaceAll(result, placeholder, v.Value)
	}
	
	// Replace ${project.X} patterns (override global)
	if projectVars, ok := m.project[projectID]; ok {
		for name, v := range projectVars {
			placeholder := fmt.Sprintf("${project.%s}", name)
			result = strings.ReplaceAll(result, placeholder, v.Value)
		}
	}
	
	return result
}

func (m *EnvManager) MaskSecrets(projectID int64) map[string]*EnvVar {
	vars := m.GetAll(projectID)
	masked := make(map[string]*EnvVar)
	
	for k, v := range vars {
		maskedVar := *v
		if v.IsSecret {
			maskedVar.Value = "***"
		}
		masked[k] = &maskedVar
	}
	
	return masked
}

func (m *EnvManager) Delete(name string, projectID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if projectID > 0 {
		if projectVars, ok := m.project[projectID]; ok {
			if _, ok := projectVars[name]; ok {
				delete(projectVars, name)
				return true
			}
		}
		return false
	}
	
	if _, ok := m.global[name]; ok {
		delete(m.global, name)
		return true
	}
	return false
}

func (m *EnvManager) LoadFromEnv() {
	for _, env := range os.Environ() {
		parts := splitEnv(env)
		if len(parts) == 2 {
			m.SetGlobal(parts[0], parts[1], false, "From system environment")
		}
	}
}

func splitEnv(s string) []string {
	idx := -1
	for i, c := range s {
		if c == '=' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}
