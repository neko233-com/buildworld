package engine

import (
	"crypto/sha256"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/neko233-com/buildworld233/internal/store"
)

type ArtifactManager struct {
	store *store.Store
	root  string
}

func NewArtifactManager(s *store.Store, root string) *ArtifactManager {
	if root == "" {
		root = "./artifacts"
	}
	os.MkdirAll(root, 0o755)
	return &ArtifactManager{store: s, root: root}
}

func (am *ArtifactManager) Save(buildID int64, name string, reader io.Reader) (*store.Artifact, error) {
	build, err := am.store.GetBuild(buildID)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(am.root, fmt.Sprintf("project-%d", build.ProjectID), fmt.Sprintf("build-%d", build.Number))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	safeName := filepath.Base(name)
	destPath := filepath.Join(dir, safeName)
	f, err := os.Create(destPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	hash := sha256.New()
	w := io.MultiWriter(f, hash)
	size, err := io.Copy(w, reader)
	if err != nil {
		return nil, err
	}
	sha := fmt.Sprintf("%x", hash.Sum(nil))
	ct := mime.TypeByExtension(filepath.Ext(safeName))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return am.store.CreateArtifact(buildID, safeName, destPath, size, sha, ct)
}

func (am *ArtifactManager) Open(id int64) (io.ReadCloser, *store.Artifact, error) {
	a, err := am.store.GetArtifact(id)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, nil, err
	}
	_ = am.store.IncArtifactDownloads(id)
	return f, a, nil
}

func (am *ArtifactManager) ListByBuild(buildID int64) ([]*store.Artifact, error) {
	return am.store.ListArtifactsByBuild(buildID)
}

func (am *ArtifactManager) DeleteByBuild(buildID int64) error {
	arts, err := am.store.ListArtifactsByBuild(buildID)
	if err != nil {
		return err
	}
	for _, a := range arts {
		os.Remove(a.Path)
	}
	build, err := am.store.GetBuild(buildID)
	if err == nil {
		dir := filepath.Join(am.root, fmt.Sprintf("project-%d", build.ProjectID), fmt.Sprintf("build-%d", build.Number))
		os.RemoveAll(dir)
	}
	return am.store.DeleteArtifactsByBuild(buildID)
}

func (am *ArtifactManager) CollectGlob(buildID int64, workspace string, patterns []string) error {
	build, err := am.store.GetBuild(buildID)
	if err != nil {
		return err
	}
	var matched []string
	for _, pattern := range patterns {
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(workspace, pattern)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			matched = append(matched, m)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	dir := filepath.Join(am.root, fmt.Sprintf("project-%d", build.ProjectID), fmt.Sprintf("build-%d", build.Number))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, src := range matched {
		rel, err := filepath.Rel(workspace, src)
		if err != nil {
			rel = filepath.Base(src)
		}
		rel = filepath.ToSlash(rel)
		rel = strings.ReplaceAll(rel, "/", "_")
		safeName := filepath.Base(rel)
		destPath := filepath.Join(dir, safeName)
		if err := copyFile(src, destPath); err != nil {
			continue
		}
		info, _ := os.Stat(destPath)
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		sha := fileSHA256(destPath)
		ct := mime.TypeByExtension(filepath.Ext(safeName))
		if ct == "" {
			ct = "application/octet-stream"
		}
		am.store.CreateArtifact(buildID, safeName, destPath, size, sha, ct)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	io.Copy(h, f)
	return fmt.Sprintf("%x", h.Sum(nil))
}
