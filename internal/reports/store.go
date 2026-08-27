package reports

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func Dir(dataDir string) string { return filepath.Join(dataDir, Dirname) }

func allDirs(dataDir string) []string {
	dirs := []string{filepath.Join(dataDir, "artifacts")}
	repDir := filepath.Join(dataDir, "reports")
	if _, err := os.Stat(repDir); err == nil {
		dirs = append(dirs, repDir)
	}
	return dirs
}

func parseHTMLMeta(raw []byte, rep *Report) {
	for _, m := range metaPair.FindAllSubmatch(raw, -1) {
		value := html.UnescapeString(string(m[2]))
		switch strings.ToLower(string(m[1])) {
		case "title":
			rep.Title = value
		case "machine":
			rep.Machine = value
		case "created":
			rep.Created, _ = time.Parse(timeFormat, value)
		case "expires":
			rep.Expires, _ = time.Parse(timeFormat, value)
		}
	}
}

func parseFrontmatter(content string, rep *Report) {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return
	}
	head := content[4 : 4+end]
	for _, line := range strings.Split(head, "\n") {
		line = strings.TrimSpace(line)
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:colon]))
		val := strings.Trim(strings.TrimSpace(line[colon+1:]), `"'`)
		switch key {
		case "title":
			rep.Title = val
		case "machine":
			rep.Machine = val
		case "created":
			rep.Created, _ = time.Parse(timeFormat, val)
		case "expires":
			rep.Expires, _ = time.Parse(timeFormat, val)
		}
	}
}

func parse(path string, raw []byte) Report {
	ext := strings.ToLower(filepath.Ext(path))
	id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	rep := Report{ID: id, Path: path}

	if ext == ".html" || ext == ".htm" {
		parseHTMLMeta(raw, &rep)
	} else {
		parseFrontmatter(string(raw), &rep)
	}

	if rep.Title == "" {
		rep.Title = titleFrom(raw, path)
	}
	if rep.Created.IsZero() {
		if info, err := os.Stat(path); err == nil {
			rep.Created = info.ModTime()
		} else {
			rep.Created = time.Now()
		}
	}
	return rep
}

func List(dataDir string) ([]Report, error) {
	seen := map[string]bool{}
	var out []Report

	for _, root := range allDirs(dataDir) {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if ext != ".md" && ext != ".html" && ext != ".htm" {
				return nil
			}
			id := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			if seen[id] {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			seen[id] = true
			out = append(out, parse(path, raw))
			return nil
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

var hashSuffix = regexp.MustCompile(`-[a-f0-9]{6}$`)

func matchesID(base, target, cleanSlug string) bool {
	if strings.EqualFold(base, target) || strings.EqualFold(base, cleanSlug) {
		return true
	}
	if strings.HasSuffix(base, "-"+cleanSlug) || strings.HasPrefix(base, cleanSlug+"-") {
		return true
	}
	strippedDate := datePrefix.ReplaceAllString(base, "")
	strippedAll := hashSuffix.ReplaceAllString(strippedDate, "")
	if strings.EqualFold(strippedAll, cleanSlug) || strings.EqualFold(strippedDate, cleanSlug) {
		return true
	}
	return strings.Contains(base, "-"+cleanSlug+"-") || strings.Contains(base, "-"+cleanSlug)
}

func findInRoot(root, target, cleanSlug string) (string, []byte) {
	var matchedPath string
	var matchedRaw []byte

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || matchedPath != "" {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".md" && ext != ".html" && ext != ".htm" {
			return nil
		}
		base := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		if !matchesID(base, target, cleanSlug) {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr == nil {
			matchedPath = path
			matchedRaw = raw
		}
		return nil
	})

	return matchedPath, matchedRaw
}

func Find(dataDir, id string) (Report, error) {
	target := strings.TrimSpace(id)
	cleanSlug := Slug(target)

	for _, root := range allDirs(dataDir) {
		path, raw := findInRoot(root, target, cleanSlug)
		if path != "" {
			return parse(path, raw), nil
		}
	}

	return Report{}, fmt.Errorf("no report named %q", id)
}

func Remove(dataDir, id string) error {
	rep, err := Find(dataDir, id)
	if err != nil {
		return err
	}
	if err := os.Remove(rep.Path); err != nil {
		return fmt.Errorf("failed to delete %s: %w", rep.Path, err)
	}
	return nil
}

func Sweep(dataDir string, now time.Time) ([]string, error) {
	all, err := List(dataDir)
	if err != nil {
		return nil, err
	}
	var swept []string
	for _, rep := range all {
		if !rep.Expired(now) {
			continue
		}
		if err := os.Remove(rep.Path); err != nil {
			continue
		}
		swept = append(swept, rep.ID)
	}
	return swept, nil
}


