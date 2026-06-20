package sync

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type FileEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{},
	}
}

func (c *Client) do(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return c.HTTPClient.Do(req)
}

func (c *Client) Tree() ([]FileEntry, error) {
	resp, err := c.do("GET", "/api/sync/tree", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var entries []FileEntry
	return entries, json.NewDecoder(resp.Body).Decode(&entries)
}

func (c *Client) Download(filePath string) ([]byte, error) {
	resp, err := c.do("GET", "/api/sync/files/"+filePath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download %s: %s", filePath, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) Upload(filePath string, data []byte) error {
	resp, err := c.do("PUT", "/api/sync/files/"+filePath, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("upload %s: %s", filePath, resp.Status)
	}
	return nil
}

func LocalTree(dataDir string) ([]FileEntry, error) {
	var entries []FileEntry
	err := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dataDir, path)
		if strings.HasPrefix(rel, ".") || rel == "tokens.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(data))
		entries = append(entries, FileEntry{
			Path:     rel,
			Checksum: checksum,
			Size:     info.Size(),
		})
		return nil
	})
	return entries, err
}

type SyncPlan struct {
	Upload   []string
	Download []string
}

func (c *Client) Push(dataDir string) (*SyncPlan, error) {
	local, err := LocalTree(dataDir)
	if err != nil {
		return nil, err
	}
	remote, err := c.Tree()
	if err != nil {
		return nil, err
	}

	remoteMap := make(map[string]FileEntry)
	for _, e := range remote {
		remoteMap[e.Path] = e
	}

	plan := &SyncPlan{}
	for _, l := range local {
		r, exists := remoteMap[l.Path]
		if !exists || l.Checksum != r.Checksum {
			data, err := os.ReadFile(filepath.Join(dataDir, l.Path))
			if err != nil {
				return nil, err
			}
			if err := c.Upload(l.Path, data); err != nil {
				return nil, err
			}
			plan.Upload = append(plan.Upload, l.Path)
		}
	}
	return plan, nil
}

func (c *Client) Pull(dataDir string) (*SyncPlan, error) {
	local, err := LocalTree(dataDir)
	if err != nil {
		return nil, err
	}
	remote, err := c.Tree()
	if err != nil {
		return nil, err
	}

	localMap := make(map[string]FileEntry)
	for _, e := range local {
		localMap[e.Path] = e
	}

	plan := &SyncPlan{}
	for _, r := range remote {
		l, exists := localMap[r.Path]
		if !exists || r.Checksum != l.Checksum {
			data, err := c.Download(r.Path)
			if err != nil {
				return nil, err
			}
			fullPath := filepath.Join(dataDir, r.Path)
			os.MkdirAll(filepath.Dir(fullPath), 0755)
			if err := os.WriteFile(fullPath, data, 0644); err != nil {
				return nil, err
			}
			plan.Download = append(plan.Download, r.Path)
		}
	}
	return plan, nil
}
