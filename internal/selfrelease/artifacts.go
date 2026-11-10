package selfrelease

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/neko233-com/buildworld/internal/systemupdate"
)

type releaseMetadata struct {
	SchemaVersion int    `json:"schemaVersion"`
	Repository    string `json:"repository"`
	Version       string `json:"version"`
	SourceCommit  string `json:"sourceCommit"`
}

func fileSHA256(filename string) (string, error) {
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("release asset is not a regular file: %s", filepath.Base(filename))
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func verifyMetadata(data []byte, version, commit string) error {
	var info releaseMetadata
	if err := json.Unmarshal(data, &info); err != nil || info.SchemaVersion != 1 ||
		info.Repository != "https://github.com/neko233-com/buildworld" ||
		strings.TrimPrefix(info.Version, "v") != version || info.SourceCommit != commit {
		return errors.New("release metadata does not match the requested version and source commit")
	}
	return nil
}

// verifyArtifacts verifies every checksummed asset and both outer and bundled metadata.
func verifyArtifacts(directory, version, commit, platform string) (string, string, error) {
	manifest, err := os.Open(filepath.Join(directory, "checksums.txt"))
	if err != nil {
		return "", "", fmt.Errorf("open release checksums: %w", err)
	}
	defer manifest.Close()
	checksums := map[string]string{}
	scanner := bufio.NewScanner(io.LimitReader(manifest, 10<<20))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return "", "", errors.New("malformed release checksum entry")
		}
		hash, name := line[:64], line[66:]
		if _, err := hex.DecodeString(hash); err != nil || strings.ToLower(hash) != hash || filepath.Base(name) != name || name == "." || name == "" {
			return "", "", errors.New("unsafe release checksum entry")
		}
		if _, exists := checksums[name]; exists {
			return "", "", errors.New("duplicate release checksum entry")
		}
		actual, err := fileSHA256(filepath.Join(directory, name))
		if err != nil || actual != hash {
			return "", "", fmt.Errorf("release checksum mismatch: %s", name)
		}
		checksums[name] = hash
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}
	for _, name := range []string{
		"buildworld-windows-amd64.zip", "buildworld-windows-arm64.zip",
		"buildworld-linux-amd64.tar.gz", "buildworld-linux-arm64.tar.gz",
		"buildworld-darwin-amd64.tar.gz", "buildworld-darwin-arm64.tar.gz",
		"install.ps1", "install.sh", "release-info.json",
	} {
		if checksums[name] == "" {
			return "", "", fmt.Errorf("required release asset is missing: %s", name)
		}
	}
	metadata, err := os.ReadFile(filepath.Join(directory, "release-info.json"))
	if err != nil || verifyMetadata(metadata, version, commit) != nil {
		return "", "", errors.New("outer release metadata is invalid")
	}
	archiveName := "buildworld-" + platform + ".tar.gz"
	archiveHash := checksums[archiveName]
	if archiveHash == "" {
		return "", "", errors.New("the host platform has no Unix release bundle")
	}
	archive := filepath.Join(directory, archiveName)
	info, err := os.Stat(archive)
	if err != nil || info.Size() > systemupdate.MaxBundleBytes {
		return "", "", errors.New("host update bundle exceeds the supported size")
	}
	if err := systemupdate.ValidateBundle(archive); err != nil {
		return "", "", fmt.Errorf("invalid host update bundle: %w", err)
	}
	if err := verifyArchiveMetadata(archive, version, commit); err != nil {
		return "", "", err
	}
	return archive, archiveHash, nil
}

func verifyArchiveMetadata(filename, version, commit string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	found := false
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if path.Clean(header.Name) != "release-info.json" {
			continue
		}
		if found || header.Typeflag != tar.TypeReg || header.Size > 64<<10 {
			return errors.New("invalid bundled release metadata")
		}
		found = true
		data, err := io.ReadAll(io.LimitReader(tarReader, 64<<10))
		if err != nil {
			return err
		}
		if err := verifyMetadata(data, version, commit); err != nil {
			return err
		}
	}
	if !found {
		return errors.New("bundle is missing release metadata")
	}
	return nil
}
