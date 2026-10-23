package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestHTTPTokenCommandPrintsOnlyPrivatePath(t *testing.T) {
	old := configFile
	configFile = t.TempDir() + "/config.yaml"
	defer func() { configFile = old; httpTokenCmd.SetOut(nil) }()
	_, cfg, err := ensureConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.New(cfg.Database.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.SetupDefaultAdmin(db); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	var out bytes.Buffer
	httpTokenCmd.SetOut(&out)
	for range 2 {
		if err := httpTokenCmd.RunE(httpTokenCmd, nil); err != nil {
			t.Fatal(err)
		}
	}
	path, _ := auth.LocalHTTPTokenPath(cfg.Database.Path)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != path+"\n"+path+"\n" || strings.Contains(out.String(), strings.TrimSpace(string(body))) {
		t.Fatal("command exposed token or unexpected output")
	}
}
