package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Template struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Category    string           `json:"category"`
	Difficulty  string           `json:"difficulty"` // beginner, intermediate, advanced
	Tags        []string         `json:"tags"`
	Config      *BuildConfig     `json:"config"`
	Icon        string           `json:"icon,omitempty"`
	IsBuiltin   bool             `json:"is_builtin"`
}

type TemplateManager struct {
	mu        sync.RWMutex
	templates map[string]*Template
	workspace string
}

func NewTemplateManager(workspace string) *TemplateManager {
	m := &TemplateManager{
		templates: make(map[string]*Template),
		workspace: workspace,
	}
	m.loadBuiltinTemplates()
	return m
}

func (m *TemplateManager) loadBuiltinTemplates() {
	// Node.js TypeScript
	m.templates["node-typescript"] = &Template{
		ID:          "node-typescript",
		Name:        "Node.js TypeScript",
		Description: "Build, test, and deploy Node.js TypeScript projects",
		Category:    "languages",
		Difficulty:  "beginner",
		Tags:        []string{"node", "typescript", "npm", "javascript"},
		IsBuiltin:   true,
		Config: &BuildConfig{
			Name: "node-typescript-build",
			Parameters: []BuildParameter{
				{Name: "node_version", Type: "choice", Choices: []string{"20", "22", "24"}, Default: "20", Required: false},
			},
			Stages: []Stage{
				{Name: "Checkout", Steps: []Step{{Name: "git", Type: "git", Config: map[string]string{"action": "clone"}}}},
				{Name: "Install", Steps: []Step{{Name: "npm ci", Type: "shell", Command: "npm ci"}}},
				{Name: "Lint", Steps: []Step{{Name: "npm lint", Type: "shell", Command: "npm run lint"}}},
				{Name: "Test", Steps: []Step{{Name: "npm test", Type: "shell", Command: "npm test"}}},
				{Name: "Build", Steps: []Step{{Name: "npm build", Type: "shell", Command: "npm run build"}}},
			},
		},
	}
	
	// Go CLI
	m.templates["go-cli"] = &Template{
		ID:          "go-cli",
		Name:        "Go CLI Application",
		Description: "Build and test Go CLI applications",
		Category:    "languages",
		Difficulty:  "beginner",
		Tags:        []string{"go", "cli", "golang"},
		IsBuiltin:   true,
		Config: &BuildConfig{
			Name: "go-cli-build",
			Parameters: []BuildParameter{
				{Name: "go_version", Type: "choice", Choices: []string{"1.21", "1.22", "1.26"}, Default: "1.26", Required: false},
			},
			Stages: []Stage{
				{Name: "Checkout", Steps: []Step{{Name: "git", Type: "git", Config: map[string]string{"action": "clone"}}}},
				{Name: "Test", Steps: []Step{{Name: "go test", Type: "shell", Command: "go test ./..."}}},
				{Name: "Build", Steps: []Step{{Name: "go build", Type: "shell", Command: "go build -o app ./cmd/app"}}},
			},
		},
	}
	
	// Python Django
	m.templates["python-django"] = &Template{
		ID:          "python-django",
		Name:        "Python Django",
		Description: "Build and test Python Django applications",
		Category:    "languages",
		Difficulty:  "intermediate",
		Tags:        []string{"python", "django", "web"},
		IsBuiltin:   true,
		Config: &BuildConfig{
			Name: "python-django-build",
			Stages: []Stage{
				{Name: "Checkout", Steps: []Step{{Name: "git", Type: "git", Config: map[string]string{"action": "clone"}}}},
				{Name: "Install", Steps: []Step{{Name: "pip install", Type: "shell", Command: "pip install -r requirements.txt"}}},
				{Name: "Test", Steps: []Step{{Name: "pytest", Type: "shell", Command: "pytest"}}},
				{Name: "Migrate", Steps: []Step{{Name: "migrate", Type: "shell", Command: "python manage.py migrate"}}},
			},
		},
	}
	
	// Docker Build
	m.templates["docker-build"] = &Template{
		ID:          "docker-build",
		Name:        "Docker Build & Push",
		Description: "Build and push Docker images",
		Category:    "platforms",
		Difficulty:  "intermediate",
		Tags:        []string{"docker", "containers", "registry"},
		IsBuiltin:   true,
		Config: &BuildConfig{
			Name: "docker-build",
			Parameters: []BuildParameter{
				{Name: "registry", Type: "string", Default: "docker.io", Required: true},
				{Name: "image_name", Type: "string", Required: true},
				{Name: "tag", Type: "string", Default: "latest", Required: false},
			},
			Stages: []Stage{
				{Name: "Checkout", Steps: []Step{{Name: "git", Type: "git", Config: map[string]string{"action": "clone"}}}},
				{Name: "Build", Steps: []Step{{Name: "docker build", Type: "shell", Command: "docker build -t ${parameter.registry}/${parameter.image_name}:${parameter.tag} ."}}},
				{Name: "Push", Steps: []Step{{Name: "docker push", Type: "shell", Command: "docker push ${parameter.registry}/${parameter.image_name}:${parameter.tag}"}}},
			},
		},
	}
	
	// K8s Deploy
	m.templates["k8s-deploy"] = &Template{
		ID:          "k8s-deploy",
		Name:        "Kubernetes Deploy",
		Description: "Deploy to Kubernetes cluster",
		Category:    "platforms",
		Difficulty:  "advanced",
		Tags:        []string{"kubernetes", "k8s", "deploy"},
		IsBuiltin:   true,
		Config: &BuildConfig{
			Name: "k8s-deploy",
			Parameters: []BuildParameter{
				{Name: "namespace", Type: "string", Default: "default", Required: true},
				{Name: "manifest", Type: "string", Default: "k8s/", Required: false},
			},
			Stages: []Stage{
				{Name: "Checkout", Steps: []Step{{Name: "git", Type: "git", Config: map[string]string{"action": "clone"}}}},
				{Name: "Deploy", Steps: []Step{{Name: "kubectl apply", Type: "shell", Command: "kubectl apply -n ${parameter.namespace} -f ${parameter.manifest}"}}},
			},
		},
	}
	
	// Unity Android
	m.templates["unity-android"] = &Template{
		ID:          "unity-android",
		Name:        "Unity Android Build",
		Description: "Build Unity game for Android",
		Category:    "game-dev",
		Difficulty:  "advanced",
		Tags:        []string{"unity", "android", "game", "mobile"},
		IsBuiltin:   true,
		Config: &BuildConfig{
			Name: "unity-android-build",
			Parameters: []BuildParameter{
				{Name: "unity_version", Type: "string", Default: "2022.3", Required: true},
				{Name: "build_target", Type: "choice", Choices: []string{"Android", "iOS", "WebGL"}, Default: "Android", Required: true},
			},
			Stages: []Stage{
				{Name: "Checkout", Steps: []Step{{Name: "git", Type: "git", Config: map[string]string{"action": "clone"}}}},
				{Name: "Build", Steps: []Step{{Name: "unity build", Type: "shell", Command: "unity -batchmode -quit -buildTarget ${parameter.build_target}"}}},
			},
		},
	}
	
	// React Vercel
	m.templates["react-vercel"] = &Template{
		ID:          "react-vercel",
		Name:        "React (Vercel)",
		Description: "Deploy React app to Vercel",
		Category:    "frontend",
		Difficulty:  "beginner",
		Tags:        []string{"react", "vercel", "frontend", "deploy"},
		IsBuiltin:   true,
		Config: &BuildConfig{
			Name: "react-vercel-deploy",
			Stages: []Stage{
				{Name: "Checkout", Steps: []Step{{Name: "git", Type: "git", Config: map[string]string{"action": "clone"}}}},
				{Name: "Install", Steps: []Step{{Name: "npm ci", Type: "shell", Command: "npm ci"}}},
				{Name: "Build", Steps: []Step{{Name: "npm build", Type: "shell", Command: "npm run build"}}},
				{Name: "Deploy", Steps: []Step{{Name: "vercel", Type: "shell", Command: "vercel --prod"}}},
			},
		},
	}
}

func (m *TemplateManager) Get(id string) *Template {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.templates[id]
}

func (m *TemplateManager) GetAll() []*Template {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var list []*Template
	for _, t := range m.templates {
		list = append(list, t)
	}
	return list
}

func (m *TemplateManager) GetByCategory(category string) []*Template {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var list []*Template
	for _, t := range m.templates {
		if t.Category == category {
			list = append(list, t)
		}
	}
	return list
}

func (m *TemplateManager) Search(query string) []*Template {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var list []*Template
	for _, t := range m.templates {
		if contains(t.Tags, query) || 
		   containsString(t.Name, query) || 
		   containsString(t.Description, query) {
			list = append(list, t)
		}
	}
	return list
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (m *TemplateManager) Create(t *Template) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.templates[t.ID]; exists {
		return fmt.Errorf("template %s already exists", t.ID)
	}
	
	t.IsBuiltin = false
	m.templates[t.ID] = t
	return nil
}

func (m *TemplateManager) Update(id string, t *Template) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.templates[id]; !exists {
		return fmt.Errorf("template %s not found", id)
	}
	
	t.ID = id
	m.templates[id] = t
	return nil
}

func (m *TemplateManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	t, exists := m.templates[id]
	if !exists {
		return fmt.Errorf("template %s not found", id)
	}
	
	if t.IsBuiltin {
		return fmt.Errorf("cannot delete builtin template %s", id)
	}
	
	delete(m.templates, id)
	return nil
}

func (m *TemplateManager) Export(id string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	t, exists := m.templates[id]
	if !exists {
		return nil, fmt.Errorf("template %s not found", id)
	}
	
	return json.MarshalIndent(t, "", "  ")
}

func (m *TemplateManager) Import(data []byte) error {
	var t Template
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	
	return m.Create(&t)
}

func (m *TemplateManager) SaveToFile(id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	t, exists := m.templates[id]
	if !exists {
		return fmt.Errorf("template %s not found", id)
	}
	
	dir := filepath.Join(m.workspace, "templates")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filepath.Join(dir, id+".json"), data, 0644)
}

func (m *TemplateManager) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	
	return m.Import(data)
}
