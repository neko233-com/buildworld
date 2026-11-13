// Package workerupdate validates official release archives before extracting
// only the worker and release identity into a private staging directory.
package workerupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const MaxBundleBytes int64 = 512 << 20

var DigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var VersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

type Identity struct {
	SchemaVersion int    `json:"schemaVersion"`
	Repository    string `json:"repository"`
	Version       string `json:"version"`
	SourceCommit  string `json:"sourceCommit"`
	Platform      string `json:"-"`
	BinarySHA256  string `json:"-"`
}

// StageArchive checks the administrator-supplied SHA against the whole upload.
func StageArchive(source io.Reader, destination, expected string) error {
	if !DigestPattern.MatchString(expected) {
		return errors.New("sha256 must be canonical lower-case SHA256")
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(destination)
		}
	}()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(source, MaxBundleBytes+1))
	if err != nil {
		return err
	}
	if n > MaxBundleBytes {
		return errors.New("worker bundle exceeds upload limit")
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return errors.New("worker bundle SHA256 mismatch")
	}
	if err = out.Sync(); err != nil {
		return err
	}
	ok = true
	return nil
}

// Extract rejects links, duplicate paths, traversal, oversized entries and
// wrong architecture. No archive-controlled path is ever used for output.
func Extract(archive, dir, version, platform string) (*Identity, error) {
	if !VersionPattern.MatchString(version) {
		return nil, errors.New("invalid release version")
	}
	parts := strings.Split(platform, "/")
	if len(parts) != 2 || (parts[0] != "windows" && parts[0] != "linux" && parts[0] != "darwin") || (parts[1] != "amd64" && parts[1] != "arm64") {
		return nil, errors.New("invalid worker platform")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	binaryName := "buildworld-worker"
	if parts[0] == "windows" {
		binaryName += ".exe"
	}
	seen := map[string]bool{}
	var expanded int64
	consume := func(name string, size int64, mode os.FileMode, reader io.Reader) error {
		name = strings.TrimPrefix(name, "./")
		clean := path.Clean(name)
		if clean == "." {
			return nil
		}
		if clean != strings.TrimSuffix(name, "/") || path.IsAbs(name) || strings.Contains(name, "\\") || strings.Contains(name, ":") || clean == ".." || strings.HasPrefix(clean, "../") || mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
			return errors.New("unsafe worker bundle entry")
		}
		if seen[clean] {
			return errors.New("duplicate worker bundle entry")
		}
		seen[clean] = true
		expanded += size
		if size < 0 || expanded > 2*MaxBundleBytes || len(seen) > 20000 {
			return errors.New("expanded worker bundle exceeds limits")
		}
		if clean != binaryName && clean != "release-info.json" {
			return nil
		}
		limit := int64(128 << 20)
		if clean == "release-info.json" {
			limit = 64 << 10
		}
		if size > limit {
			return errors.New("worker identity or binary exceeds limit")
		}
		if !mode.IsRegular() {
			return errors.New("worker release entry is not a regular file")
		}
		out, err := os.OpenFile(filepath.Join(dir, clean), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			return err
		}
		n, err := io.Copy(out, io.LimitReader(reader, size+1))
		syncErr := out.Sync()
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if n != size {
			return errors.New("worker entry size mismatch")
		}
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	}
	file, err := os.Open(archive)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	magic := make([]byte, 2)
	if _, err = io.ReadFull(file, magic); err != nil {
		return nil, err
	}
	_, _ = file.Seek(0, 0)
	if string(magic) == "PK" {
		stat, _ := file.Stat()
		reader, openErr := zip.NewReader(file, stat.Size())
		if openErr != nil {
			return nil, openErr
		}
		for _, entry := range reader.File {
			in, e := entry.Open()
			if e != nil {
				return nil, e
			}
			e = consume(entry.Name, int64(entry.UncompressedSize64), entry.Mode(), in)
			in.Close()
			if e != nil {
				return nil, e
			}
		}
	} else {
		compressed, e := gzip.NewReader(file)
		if e != nil {
			return nil, e
		}
		defer compressed.Close()
		reader := tar.NewReader(compressed)
		for {
			entry, e := reader.Next()
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil {
				return nil, e
			}
			if entry.Typeflag != tar.TypeReg && entry.Typeflag != tar.TypeDir {
				return nil, errors.New("unsupported tar entry")
			}
			if e = consume(entry.Name, entry.Size, entry.FileInfo().Mode(), reader); e != nil {
				return nil, e
			}
		}
	}
	for _, required := range []string{binaryName, "release-info.json", "web/dist/index.html", "sdk/pipeline/index.js", "sdk/pipeline/index.d.ts", "sdk/pipeline/package.json"} {
		if !seen[required] {
			return nil, fmt.Errorf("official release bundle is incomplete: missing %s", required)
		}
	}
	metadata, err := os.ReadFile(filepath.Join(dir, "release-info.json"))
	if err != nil {
		return nil, errors.New("release-info.json is required")
	}
	var identity Identity
	if json.Unmarshal(metadata, &identity) != nil || identity.SchemaVersion != 1 || identity.Repository != "https://github.com/neko233-com/buildworld" || strings.TrimPrefix(identity.Version, "v") != strings.TrimPrefix(version, "v") || !commitPattern.MatchString(identity.SourceCommit) {
		return nil, errors.New("release identity mismatch")
	}
	binary := filepath.Join(dir, binaryName)
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		return nil, errors.New("worker is not a Go release binary")
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != parts[0] || settings["GOARCH"] != parts[1] {
		return nil, fmt.Errorf("worker platform mismatch: need %s", platform)
	}
	in, err := os.Open(binary)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, in)
	in.Close()
	if err != nil {
		return nil, err
	}
	identity.Platform, identity.BinarySHA256 = platform, hex.EncodeToString(hash.Sum(nil))
	return &identity, nil
}
