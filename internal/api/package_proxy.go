package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neko233-com/buildworld/internal/engine"
)

type packageProxyDefinition struct {
	ID          string
	Label       string
	Description string
	Accelerated map[string]string
	Official    map[string]string
}

type packageProxyState struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	Description string            `json:"description"`
	Mode        string            `json:"mode"`
	Variables   map[string]string `json:"variables"`
}

type packageProxyRequest struct {
	Mode     string   `json:"mode"`
	Packages []string `json:"packages"`
}

var persistPackageProxyEnvironment = persistUserEnvironment

func packageProxyDefinitions(root string) []packageProxyDefinition {
	managed := filepath.Join(root, "env", "package-proxies")
	mavenSettings := filepath.Join(managed, "maven-settings.xml")
	gradleInit := filepath.Join(managed, "gradle-init.gradle")
	nugetConfig := filepath.Join(managed, "NuGet.Config")
	return []packageProxyDefinition{
		{ID: "go", Label: "Go modules", Description: "GOPROXY and checksum database", Accelerated: map[string]string{"GOPROXY": "https://goproxy.cn,direct", "GOSUMDB": "sum.golang.google.cn"}, Official: map[string]string{"GOPROXY": "https://proxy.golang.org,direct", "GOSUMDB": "sum.golang.org"}},
		{ID: "node", Label: "npm / pnpm / Yarn", Description: "Node package registry", Accelerated: map[string]string{"NPM_CONFIG_REGISTRY": "https://registry.npmmirror.com", "YARN_NPM_REGISTRY_SERVER": "https://registry.npmmirror.com"}, Official: map[string]string{"NPM_CONFIG_REGISTRY": "https://registry.npmjs.org", "YARN_NPM_REGISTRY_SERVER": "https://registry.npmjs.org"}},
		{ID: "maven", Label: "Maven", Description: "Managed Maven settings.xml mirror", Accelerated: map[string]string{"MAVEN_ARGS": "-s " + mavenSettings}, Official: map[string]string{"MAVEN_ARGS": ""}},
		{ID: "gradle", Label: "Gradle", Description: "Managed Gradle init script", Accelerated: map[string]string{"GRADLE_OPTS": "-I " + gradleInit}, Official: map[string]string{"GRADLE_OPTS": ""}},
		{ID: "python", Label: "Python / pip", Description: "pip package index", Accelerated: map[string]string{"PIP_INDEX_URL": "https://pypi.tuna.tsinghua.edu.cn/simple", "PIP_TRUSTED_HOST": "pypi.tuna.tsinghua.edu.cn"}, Official: map[string]string{"PIP_INDEX_URL": "https://pypi.org/simple", "PIP_TRUSTED_HOST": ""}},
		{ID: "cargo", Label: "Rust / Cargo", Description: "crates.io sparse registry", Accelerated: map[string]string{"CARGO_REGISTRIES_CRATES_IO_INDEX": "sparse+https://rsproxy.cn/index/"}, Official: map[string]string{"CARGO_REGISTRIES_CRATES_IO_INDEX": "sparse+https://index.crates.io/"}},
		{ID: "nuget", Label: ".NET / NuGet", Description: "Managed NuGet.Config package source", Accelerated: map[string]string{"NUGET_CONFIG_FILE": nugetConfig}, Official: map[string]string{"NUGET_CONFIG_FILE": ""}},
		{ID: "composer", Label: "PHP / Composer", Description: "Packagist repository", Accelerated: map[string]string{"COMPOSER_REPO_PACKAGIST": "https://mirrors.aliyun.com/composer/"}, Official: map[string]string{"COMPOSER_REPO_PACKAGIST": "https://repo.packagist.org"}},
	}
}

func (h *handlers) packageProxyStates() ([]packageProxyState, error) {
	values, err := h.d.Store.ListEnvVars("global", nil)
	if err != nil {
		return nil, err
	}
	configured := make(map[string]string, len(values))
	for _, value := range values {
		configured[value.Name] = value.Value
	}
	definitions := packageProxyDefinitions(engine.ResolveBuildTempRoot(h.d.Cfg.Storage.BuildTemp))
	// A build-global value takes precedence, but display inherited user settings too.
	// This keeps the screen truthful before BuildWorld has managed a package tool.
	for _, definition := range definitions {
		for key := range definition.Accelerated {
			if _, managed := configured[key]; managed {
				continue
			}
			if value, exists := os.LookupEnv(key); exists {
				configured[key] = value
			}
		}
	}
	states := make([]packageProxyState, 0, len(definitions))
	for _, definition := range definitions {
		mode := "custom"
		if packageProxyMatches(configured, definition.Accelerated) {
			mode = "accelerated"
		} else if packageProxyMatches(configured, definition.Official) {
			mode = "official"
		}
		states = append(states, packageProxyState{ID: definition.ID, Label: definition.Label, Description: definition.Description, Mode: mode, Variables: packageProxyVariables(definition, mode)})
	}
	return states, nil
}

func packageProxyMatches(values, expected map[string]string) bool {
	for key, value := range expected {
		configured, exists := values[key]
		if !exists || configured != value {
			return false
		}
	}
	return true
}

func packageProxyVariables(definition packageProxyDefinition, mode string) map[string]string {
	if mode == "accelerated" {
		return definition.Accelerated
	}
	return definition.Official
}

func (h *handlers) getPackageProxies(w http.ResponseWriter, _ *http.Request) {
	states, err := h.packageProxyStates()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, states)
}

func (h *handlers) applyPackageProxies(w http.ResponseWriter, r *http.Request) {
	var req packageProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Mode != "accelerated" && req.Mode != "official" {
		writeErr(w, http.StatusBadRequest, "mode must be accelerated or official")
		return
	}

	definitions := packageProxyDefinitions(engine.ResolveBuildTempRoot(h.d.Cfg.Storage.BuildTemp))
	selected := make(map[string]bool, len(definitions))
	if len(req.Packages) == 0 {
		for _, definition := range definitions {
			selected[definition.ID] = true
		}
	} else {
		for _, id := range req.Packages {
			selected[id] = true
		}
	}

	updates := map[string]string{}
	for _, definition := range definitions {
		if !selected[definition.ID] {
			continue
		}
		if req.Mode == "accelerated" {
			for key, value := range definition.Accelerated {
				updates[key] = value
			}
		} else {
			for key, value := range definition.Official {
				updates[key] = value
			}
		}
	}
	for id := range selected {
		known := false
		for _, definition := range definitions {
			if definition.ID == id {
				known = true
				break
			}
		}
		if !known {
			writeErr(w, http.StatusBadRequest, "unsupported package ecosystem: "+id)
			return
		}
	}
	if len(updates) == 0 {
		writeErr(w, http.StatusBadRequest, "select at least one supported package ecosystem")
		return
	}
	if err := writeManagedProxyConfigs(engine.ResolveBuildTempRoot(h.d.Cfg.Storage.BuildTemp)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for key, value := range updates {
		if err := h.d.Store.SetEnvVar("global", nil, key, value, false, "Managed by BuildWorld package proxy settings"); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := os.Setenv(key, value); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := persistPackageProxyEnvironment(updates); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("package proxy saved for builds but could not persist user environment: %v", err))
		return
	}
	states, err := h.packageProxyStates()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	h.audit(r, "update", "package_proxy", strings.Join(ids, ","), req.Mode)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "mode": req.Mode, "updated": ids, "proxies": states})
}

func writeManagedProxyConfigs(root string) error {
	directory := filepath.Join(root, "env", "package-proxies")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create package proxy directory: %w", err)
	}
	files := map[string]string{
		"maven-settings.xml": `<?xml version="1.0" encoding="UTF-8"?><settings><mirrors><mirror><id>buildworld-cn</id><name>BuildWorld China mirror</name><url>https://maven.aliyun.com/repository/public</url><mirrorOf>*</mirrorOf></mirror></mirrors></settings>`,
		"gradle-init.gradle": `allprojects { repositories { maven { url "https://maven.aliyun.com/repository/public" }; mavenCentral(); gradlePluginPortal() } }`,
		"NuGet.Config":       `<?xml version="1.0" encoding="utf-8"?><configuration><packageSources><clear /><add key="BuildWorld China mirror" value="https://nuget.cdn.azure.cn/v3/index.json" protocolVersion="3" /></packageSources></configuration>`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents+"\n"), 0o600); err != nil {
			return fmt.Errorf("write managed %s: %w", name, err)
		}
	}
	return nil
}
