package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestPackageProxyDefinitionsCoverCoreToolchains(t *testing.T) {
	definitions := packageProxyDefinitions(t.TempDir())
	ids := map[string]bool{}
	for _, definition := range definitions {
		ids[definition.ID] = true
		if len(definition.Accelerated) == 0 || len(definition.Official) == 0 {
			t.Fatalf("%s does not define both proxy modes", definition.ID)
		}
	}
	for _, id := range []string{"go", "node", "maven", "python", "cargo", "nuget"} {
		if !ids[id] {
			t.Fatalf("missing core package ecosystem %q", id)
		}
	}
}

func TestPackageProxyMatchesRequiresAnExplicitValue(t *testing.T) {
	definition := packageProxyDefinitions(t.TempDir())[0]
	if packageProxyMatches(map[string]string{}, definition.Official) {
		t.Fatal("an unmanaged proxy must not be reported as an official BuildWorld setting")
	}
	if !packageProxyMatches(definition.Official, definition.Official) {
		t.Fatal("an exact official configuration must match")
	}
}

func TestApplyPackageProxiesPersistsBuildEnvironment(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "package-proxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	t.Setenv("GOPROXY", "")
	t.Setenv("GOSUMDB", "")

	originalPersistence := persistPackageProxyEnvironment
	persistPackageProxyEnvironment = func(values map[string]string) error {
		if values["GOPROXY"] != "https://goproxy.cn,direct" {
			t.Fatalf("unexpected persisted values: %#v", values)
		}
		return nil
	}
	defer func() { persistPackageProxyEnvironment = originalPersistence }()

	handler := &handlers{d: Deps{Cfg: &config.Config{Storage: config.StorageConfig{BuildTemp: t.TempDir()}}, Store: database}}
	request := httptest.NewRequest(http.MethodPost, "/api/settings/package-proxies/apply", bytes.NewBufferString(`{"mode":"accelerated","packages":["go"]}`))
	response := httptest.NewRecorder()
	handler.applyPackageProxies(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	values, err := database.ListEnvVars("global", nil)
	if err != nil {
		t.Fatal(err)
	}
	configured := map[string]string{}
	for _, value := range values {
		configured[value.Name] = value.Value
	}
	if configured["GOPROXY"] != "https://goproxy.cn,direct" || configured["GOSUMDB"] != "sum.golang.google.cn" {
		t.Fatalf("stored Go proxy values = %#v", configured)
	}
}

func TestApplyPackageProxiesRejectsUnknownPackage(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "package-proxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	handler := &handlers{d: Deps{Cfg: &config.Config{Storage: config.StorageConfig{BuildTemp: t.TempDir()}}, Store: database}}
	request := httptest.NewRequest(http.MethodPost, "/api/settings/package-proxies/apply", bytes.NewBufferString(`{"mode":"official","packages":["unknown"]}`))
	response := httptest.NewRecorder()
	handler.applyPackageProxies(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
