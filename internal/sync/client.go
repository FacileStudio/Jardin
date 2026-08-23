package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client talks to a Mycelium server over HTTP, scoped to one space when Space
// is set.
//
// AllowBulkDelete waives the limit on how many local files one Sync may
// remove. It is a field rather than a parameter so Push, Pull and the daemon's
// unattended sync are untouched by it, and so only the human typing --force
// can turn it on.
type Client struct {
	BaseURL         string
	Token           string
	Space           string
	AllowBulkDelete bool
	HTTPClient      *http.Client
}

// FileEntry is one file's identity as the sync reconciler compares it: path
// plus checksum, size and modification time.
type FileEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
}

// NewClient builds a Client for one server, trimming a trailing slash from
// the base URL.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{},
	}
}

func (c *Client) do(method, path string, body io.Reader) (*http.Response, error) {
	target := c.BaseURL + path
	if c.Space != "" {
		u, err := url.Parse(target)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("space_id", c.Space)
		u.RawQuery = q.Encode()
		target = u.String()
	}
	req, err := http.NewRequest(method, target, body)
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tree: %s", resp.Status)
	}
	var entries []FileEntry
	return entries, json.NewDecoder(resp.Body).Decode(&entries)
}

func (c *Client) Download(filePath string) ([]byte, error) {
	resp, err := c.do("GET", "/api/sync/files/"+filePath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
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

func (c *Client) Delete(filePath string) error {
	resp, err := c.do("DELETE", "/api/sync/files/"+filePath, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete %s: %s", filePath, resp.Status)
	}
	return nil
}
