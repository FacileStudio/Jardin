package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrHomebrew is returned when the running binary is managed by Homebrew and
// must be updated through `brew upgrade ruche` instead of self-replacement.
var ErrHomebrew = errors.New("ruche is managed by Homebrew; run: brew upgrade ruche")

const (
	releaseURL  = "https://api.github.com/repos/FacileStudio/Ruche/releases/latest"
	httpTimeout = 30 * time.Second
	userAgent   = "ruche-selfupdate (+https://github.com/FacileStudio/Ruche)"
)

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}

// assetName returns the release tarball name for the given version, OS and arch.
func assetName(version, goos, goarch string) string {
	return fmt.Sprintf("Ruche_%s_%s_%s.tar.gz", normalizeVersion(version), goos, goarch)
}

// updateNeeded reports whether moving from current to latest is an update.
// A "dev" build is always considered out of date.
func updateNeeded(current, latest string) bool {
	cur := normalizeVersion(current)
	lat := normalizeVersion(latest)
	if cur == "dev" {
		return true
	}
	return cur != lat
}

func fetchLatest() (*release, error) {
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest(http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return nil, errors.New("GitHub API rate limit reached; try again in a few minutes")
		}
		return nil, fmt.Errorf("github release lookup failed: %s", resp.Status)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, errors.New("no latest release found")
	}
	return &rel, nil
}

// CheckLatest reports the latest published version and whether it differs from
// current. It performs a network request but installs nothing.
func CheckLatest(current string) (latestVersion string, updateAvailable bool, err error) {
	rel, err := fetchLatest()
	if err != nil {
		return "", false, err
	}
	latest := normalizeVersion(rel.TagName)
	return latest, updateNeeded(current, latest), nil
}

func findAsset(assets []asset, name string) (string, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a.URL, true
		}
	}
	return "", false
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed (%s): %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func checksumFor(checksums []byte, filename string) (string, bool) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filename {
			return fields[0], true
		}
	}
	return "", false
}

func extractBinary(tarball []byte, binary string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == binary {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", binary)
}

func realExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return resolved, nil
}

func isHomebrew(path string) bool {
	return strings.Contains(path, "/Cellar/") ||
		strings.Contains(path, "/homebrew/") ||
		strings.Contains(path, "/linuxbrew/")
}

func replaceBinary(realPath string, data []byte) error {
	dir := filepath.Dir(realPath)
	tmp, err := os.CreateTemp(dir, ".ruche-update-*")
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("cannot write to %s: permission denied; re-run with elevated permissions or use your package manager", dir)
		}
		return err
	}
	tmpName := tmp.Name()

	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, realPath); err != nil {
		os.Remove(tmpName)
		if os.IsPermission(err) {
			return fmt.Errorf("cannot replace %s: permission denied; re-run with elevated permissions or use your package manager", realPath)
		}
		return err
	}
	return nil
}

// Apply downloads the latest release for this platform, verifies its checksum,
// and atomically replaces the running binary. It returns the new version. If
// the binary is managed by Homebrew it returns ErrHomebrew and changes nothing.
func Apply(current string) (newVersion string, err error) {
	realPath, err := realExecutablePath()
	if err != nil {
		return "", err
	}
	if isHomebrew(realPath) {
		return "", ErrHomebrew
	}

	rel, err := fetchLatest()
	if err != nil {
		return "", err
	}
	latest := normalizeVersion(rel.TagName)
	if !updateNeeded(current, latest) {
		return latest, nil
	}

	name := assetName(latest, runtime.GOOS, runtime.GOARCH)
	tarURL, ok := findAsset(rel.Assets, name)
	if !ok {
		return "", fmt.Errorf("no release asset for %s/%s (%s); your platform may be unsupported", runtime.GOOS, runtime.GOARCH, name)
	}
	sumURL, ok := findAsset(rel.Assets, "checksums.txt")
	if !ok {
		return "", errors.New("checksums.txt not found in release assets")
	}

	tarball, err := download(tarURL)
	if err != nil {
		return "", err
	}
	checksums, err := download(sumURL)
	if err != nil {
		return "", err
	}

	want, ok := checksumFor(checksums, name)
	if !ok {
		return "", fmt.Errorf("no checksum entry for %s", name)
	}
	sum := sha256.Sum256(tarball)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return "", fmt.Errorf("checksum mismatch for %s: got %s, want %s", name, got, want)
	}

	bin, err := extractBinary(tarball, "ruche")
	if err != nil {
		return "", err
	}

	if err := replaceBinary(realPath, bin); err != nil {
		return "", err
	}
	return latest, nil
}
