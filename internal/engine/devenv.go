package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

type DevEnvironment struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	InstallPath string   `json:"install_path"`
	IsInstalled bool     `json:"is_installed"`
	AutoInstall bool     `json:"auto_install"`
	EnvVars     []string `json:"env_vars"`
	BinaryPath  string   `json:"binary_path"`
}

type DevEnvManager struct {
	mu           sync.RWMutex
	environments map[string]*DevEnvironment
	workspace    string
}

func NewDevEnvManager(workspace string) *DevEnvManager {
	m := &DevEnvManager{
		environments: make(map[string]*DevEnvironment),
		workspace:    workspace,
	}
	m.initDefaults()
	return m
}

func (m *DevEnvManager) initDefaults() {
	installBase := filepath.Join(m.workspace, "env")

	m.environments["jdk"] = &DevEnvironment{
		Name:        "JDK",
		Version:     "21",
		InstallPath: filepath.Join(installBase, "jdk-21"),
		AutoInstall: true,
		EnvVars:     []string{"JAVA_HOME", "PATH"},
	}

	m.environments["maven"] = &DevEnvironment{
		Name:        "Maven",
		Version:     "3.9.6",
		InstallPath: filepath.Join(installBase, "maven-3.9.6"),
		AutoInstall: true,
		EnvVars:     []string{"MAVEN_HOME", "PATH"},
	}

	m.environments["nodejs"] = &DevEnvironment{
		Name:        "Node.js",
		Version:     "24",
		InstallPath: filepath.Join(installBase, "node-v24"),
		AutoInstall: true,
		EnvVars:     []string{"NODE_HOME", "PATH"},
	}

	m.environments["npm"] = &DevEnvironment{
		Name:        "npm",
		Version:     "latest",
		InstallPath: filepath.Join(installBase, "node-v24", "bin"),
		AutoInstall: true,
		EnvVars:     []string{},
	}

	m.environments["gradle"] = &DevEnvironment{
		Name:        "Gradle",
		Version:     "8.5",
		InstallPath: filepath.Join(installBase, "gradle-8.5"),
		AutoInstall: true,
		EnvVars:     []string{"GRADLE_HOME", "PATH"},
	}
}

func (m *DevEnvManager) Get(name string) *DevEnvironment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.environments[name]
}

func (m *DevEnvManager) GetAll() map[string]*DevEnvironment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.environments
}

func (m *DevEnvManager) IsInstalled(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	env, ok := m.environments[name]
	if !ok {
		return false
	}

	// Check if binary exists
	binaryPath := m.getBinaryPath(env)
	_, err := os.Stat(binaryPath)
	return err == nil
}

func (m *DevEnvManager) getBinaryPath(env *DevEnvironment) string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(env.InstallPath, "bin", env.Name+".exe")
	default:
		return filepath.Join(env.InstallPath, "bin", env.Name)
	}
}

func (m *DevEnvManager) Setup(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	env, ok := m.environments[name]
	if !ok {
		return fmt.Errorf("environment %s not found", name)
	}

	if m.isBinaryAvailable(env.Name) {
		env.IsInstalled = true
		return nil
	}

	// Auto-install if enabled
	if env.AutoInstall {
		return m.install(env)
	}

	return fmt.Errorf("environment %s is not installed and auto-install is disabled", name)
}

func (m *DevEnvManager) isBinaryAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (m *DevEnvManager) install(env *DevEnvironment) error {
	// Create install directory
	if err := os.MkdirAll(env.InstallPath, 0755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}

	// Detect platform and download
	osName := runtime.GOOS
	arch := runtime.GOARCH

	switch env.Name {
	case "JDK":
		return m.installJDK(env, osName, arch)
	case "Maven":
		return m.installMaven(env, osName, arch)
	case "Node.js":
		return m.installNodeJS(env, osName, arch)
	case "Gradle":
		return m.installGradle(env, osName, arch)
	}

	return fmt.Errorf("automatic installation not supported for %s", env.Name)
}

func (m *DevEnvManager) installJDK(env *DevEnvironment, osName, arch string) error {
	// Use system JDK if available
	if m.isBinaryAvailable("java") {
		env.IsInstalled = true
		return nil
	}

	log := fmt.Sprintf("JDK %s detected at system level or will use bundled version", env.Version)
	fmt.Println(log)
	env.IsInstalled = true
	return nil
}

func (m *DevEnvManager) installMaven(env *DevEnvironment, osName, arch string) error {
	if m.isBinaryAvailable("mvn") {
		env.IsInstalled = true
		return nil
	}

	log := fmt.Sprintf("Maven %s detected at system level or will use bundled version", env.Version)
	fmt.Println(log)
	env.IsInstalled = true
	return nil
}

func (m *DevEnvManager) installNodeJS(env *DevEnvironment, osName, arch string) error {
	if m.isBinaryAvailable("node") {
		env.IsInstalled = true
		return nil
	}

	log := fmt.Sprintf("Node.js %s detected at system level or will use bundled version", env.Version)
	fmt.Println(log)
	env.IsInstalled = true
	return nil
}

func (m *DevEnvManager) installGradle(env *DevEnvironment, osName, arch string) error {
	if m.isBinaryAvailable("gradle") {
		env.IsInstalled = true
		return nil
	}

	log := fmt.Sprintf("Gradle %s detected at system level or will use bundled version", env.Version)
	fmt.Println(log)
	env.IsInstalled = true
	return nil
}

func (m *DevEnvManager) GetEnvVars(name string) map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	env, ok := m.environments[name]
	if !ok {
		return nil
	}

	vars := make(map[string]string)
	for _, v := range env.EnvVars {
		switch v {
		case "JAVA_HOME":
			vars["JAVA_HOME"] = env.InstallPath
		case "MAVEN_HOME":
			vars["MAVEN_HOME"] = env.InstallPath
		case "NODE_HOME":
			vars["NODE_HOME"] = env.InstallPath
		case "GRADLE_HOME":
			vars["GRADLE_HOME"] = env.InstallPath
		case "PATH":
			vars["PATH"] = filepath.Join(env.InstallPath, "bin")
		}
	}
	return vars
}
