package portability

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func openPortabilityStore(t *testing.T, name string) *store.Store {
	t.Helper()
	value, err := store.New(filepath.Join(t.TempDir(), name+".db"))
	if err != nil {
		t.Fatalf("store.New(%s) error = %v", name, err)
	}
	t.Cleanup(func() { _ = value.Close() })
	return value
}

func TestDefaultRegistryRoundTripsConfigurationGraph(t *testing.T) {
	source := openPortabilityStore(t, "source")
	credential, err := source.CreateCredential("source-control", store.CredentialTypeGit, "example.test", "robot", "", "", "", "secret-token", "automation", true)
	if err != nil {
		t.Fatal(err)
	}
	root, err := source.CreateVCSRoot("main-repository", "git", "https://example.test/repository.git", "main", &credential.ID, 0, true, "{\n  \"depth\": 1\n}")
	if err != nil {
		t.Fatal(err)
	}
	template, err := source.CreateBuildTemplate("release-template", "{\n  \"stages\": []\n}", "release pipeline")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.CreateProjectGroup("products", "product-owned projects"); err != nil {
		t.Fatal(err)
	}
	projectGroup, err := source.CreateProjectGroupWithColor("release", "release automation", "blue")
	if err != nil {
		t.Fatal(err)
	}
	project, err := source.CreateProject("portable-project", "round trip", root.URL, "git", "main", "{\n  \"stages\": []\n}", 0, &root.ID, &template.ID, []string{"portable"})
	if err != nil {
		t.Fatal(err)
	}
	if err := source.SetProjectGroup(project.ID, &projectGroup.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := source.CreateNotificationChannel("release-alerts", store.NotificationChannelWebhook, "{\n  \"url\": \"https://example.test/hook\"\n}", "{\n  \"statuses\": [\n    \"failed\"\n  ]\n}", "release notifications", true); err != nil {
		t.Fatal(err)
	}
	if err := source.SetEnvVar("system", nil, "build_timeout", "900", false, "portable setting"); err != nil {
		t.Fatal(err)
	}
	sourcePlugin, err := source.CreatePlugin("portable-plugin", "1.0.0", "activation preference", "buildworld", "", "/plugins/portable-plugin", "https://github.com/acme/portable-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if err := source.UpdatePluginEnabled(sourcePlugin.ID, false); err != nil {
		t.Fatal(err)
	}

	sourceRegistry, err := NewDefaultRegistry(source)
	if err != nil {
		t.Fatal(err)
	}
	sections := []string{
		"credentials", "notification_channels",
		"plugin_settings", "project_groups", "projects", "settings", "templates", "vcs_roots",
	}
	bundle, err := sourceRegistry.Export(context.Background(), ExportOptions{Sections: sections, IncludeSecrets: true})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if bundle.Sections["project_groups"].Version != 1 || bundle.Sections["projects"].Version != 3 || bundle.Sections["vcs_roots"].Version != 2 {
		t.Fatalf("reference-aware section versions missing: %#v", bundle.Sections)
	}

	target := openPortabilityStore(t, "target")
	targetPlugin, err := target.CreatePlugin("portable-plugin", "1.0.0", "activation preference", "buildworld", "", "/plugins/portable-plugin", "https://github.com/acme/portable-plugin")
	if err != nil {
		t.Fatal(err)
	}
	targetRegistry, err := NewDefaultRegistry(target)
	if err != nil {
		t.Fatal(err)
	}
	result, err := targetRegistry.Import(context.Background(), bundle, ImportOptions{Mode: "overwrite"})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if len(result.Sections) != len(sections) {
		t.Fatalf("imported sections = %d, want %d", len(result.Sections), len(sections))
	}

	importedCredential, err := target.GetCredentialByName("source-control")
	if err != nil || importedCredential.Token != "secret-token" {
		t.Fatalf("credential = %#v, err=%v", importedCredential, err)
	}
	importedRoot, err := target.GetVCSRootByName("main-repository")
	if err != nil || importedRoot.CredentialID == nil || *importedRoot.CredentialID != importedCredential.ID {
		t.Fatalf("VCS root reference = %#v, err=%v", importedRoot, err)
	}
	importedTemplate, err := target.GetBuildTemplateByName("release-template")
	if err != nil {
		t.Fatal(err)
	}
	importedProject, err := target.GetProjectByName("portable-project")
	importedGroup, groupErr := target.GetProjectGroupByName("release")
	_, parentErr := target.GetProjectGroupByName("products")
	if err != nil || groupErr != nil || parentErr != nil ||
		importedProject.VCSRootID == nil || *importedProject.VCSRootID != importedRoot.ID ||
		importedProject.TemplateID == nil || *importedProject.TemplateID != importedTemplate.ID ||
		importedProject.GroupID == nil || *importedProject.GroupID != importedGroup.ID ||
		importedGroup.Color != "blue" {
		t.Fatalf("project references = %#v, err=%v", importedProject, err)
	}
	if _, err := target.GetNotificationChannelByName("release-alerts"); err != nil {
		t.Fatal(err)
	}
	importedPlugin, err := target.GetPlugin(targetPlugin.ID)
	if err != nil || importedPlugin.Enabled {
		t.Fatalf("plugin activation preference = %#v, err=%v", importedPlugin, err)
	}
	second, err := targetRegistry.Import(context.Background(), bundle, ImportOptions{Mode: "skip"})
	if err != nil {
		t.Fatalf("second Import(skip) error = %v", err)
	}
	for _, section := range second.Sections {
		if section.Count > 0 && section.Skipped != section.Count {
			t.Fatalf("second import section = %#v, want all existing records skipped", section)
		}
	}
}

func TestProjectGroupImportDefaultsMissingColorToNeutral(t *testing.T) {
	target := openPortabilityStore(t, "missing-group-color")
	strategy := projectGroupStrategy{store: target}
	withoutColor := json.RawMessage(`[
		{"name":"game servers","description":"backup without explicit color"}
	]`)

	result, err := strategy.Import(context.Background(), withoutColor, ImportOptions{Mode: "overwrite"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 {
		t.Fatalf("import result = %#v, want one group", result)
	}
	group, err := target.GetProjectGroupByName("game servers")
	if err != nil {
		t.Fatal(err)
	}
	if group.Color != store.ProjectGroupColorNeutral {
		t.Fatalf("default group color = %q, want %q", group.Color, store.ProjectGroupColorNeutral)
	}

	invalid := json.RawMessage(`[{"name":"unsafe color","color":"#ff0000"}]`)
	if _, err := strategy.Import(context.Background(), invalid, ImportOptions{Mode: "overwrite"}); !errors.Is(err, store.ErrInvalidProjectGroupColor) {
		t.Fatalf("invalid color import error = %v, want ErrInvalidProjectGroupColor", err)
	}
}

func TestPluginSettingsImportNeverInstallsExecutableCode(t *testing.T) {
	target := openPortabilityStore(t, "missing-plugin")
	strategy := pluginSettingsStrategy{store: target}
	data := json.RawMessage(`[{"name":"untrusted-plugin","enabled":true}]`)

	result, err := strategy.Import(context.Background(), data, ImportOptions{Mode: "overwrite"})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.Count != 1 || result.Skipped != 1 || result.Created != 0 {
		t.Fatalf("result = %#v, want missing executable plugin skipped", result)
	}
	if _, err := target.GetPluginByName("untrusted-plugin"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing plugin error = %v, want sql.ErrNoRows", err)
	}
}
