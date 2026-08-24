package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ratifiedFile holds, per page, the checksum a human accepted on this machine.
// It is a dotfile beside the tree rather than anything inside a page, because
// the wiki's own rule forbids a mechanism writing its state into the document
// it governs, and because sync skips every path starting with a dot.
const ratifiedFile = ".memory-ratified.json"

// normativeType is the frontmatter value that makes a page normative. The type
// is the declaration, not the directory: a page says what it is, and the rules
// then say normative pages live under standards/. Reading the frontmatter means
// a page moved out of that directory does not quietly stop being governed.
const normativeType = "standard"

// Standing is what this machine can say about a normative page.
//
// The four values are deliberately the same shape as flow trust, which the
// human reading them has already met: a thing that was never approved here, a
// thing that matches what was approved, and a thing that has moved since. The
// fourth has no flow equivalent because a flow that disappears simply stops
// being listed, while a standard that disappears is the loudest accident of the
// set.
type Standing string

const (
	// Ratified means the bytes on disk are exactly the bytes a human accepted
	// on this machine.
	Ratified Standing = "ratified"

	// Unratified means nobody has ever accepted this page here. It is not a
	// failure. Every machine starts in it, and so does every genuinely new
	// standard.
	Unratified Standing = "not ratified"

	// Changed means this machine accepted the page once and its content has
	// moved since. This is the state the whole mechanism exists to surface.
	Changed Standing = "CHANGED"

	// Missing means a page that was accepted here is no longer in the tree.
	Missing Standing = "MISSING"
)

// Ratification is one accepted page: the content that was accepted, and where
// and when it was accepted.
//
// It carries the machine and the date because a bare checksum can only ever say
// "different", and the useful sentence is "this moved since you accepted it on
// lucy on 2026-08-24". That is the one idea worth taking from the attestation
// formats: the record is a statement *about* the content — subject, act, who,
// when — rather than a fingerprint of it. The signature those formats add
// answers a distribution problem this does not have.
type Ratification struct {
	Checksum string `json:"checksum"`
	Machine  string `json:"machine"`
	Date     string `json:"date"`
}

// NormativePage is one governed page and what this machine makes of it. On and
// Machine describe the accepted version and are empty when there is none.
type NormativePage struct {
	Path     string   `json:"path"`
	Standing Standing `json:"standing"`
	On       string   `json:"ratified_on,omitempty"`
	Machine  string   `json:"ratified_by,omitempty"`
}

// OK reports whether the page needs no attention. Unratified counts as OK: a
// machine that has never ratified anything is a normal machine, and a check
// that failed there would fail on every fresh install and be ignored.
func (p NormativePage) OK() bool { return p.Standing == Ratified || p.Standing == Unratified }

// NormativePages lists every page carrying `type: standard`, plus any page a
// pin still names that has since left the tree, sorted by path.
//
// Ratification never gates reading. A page in any standing is still on disk,
// still synced, still searched and still returned; the standing decides only
// whether it is presented as authoritative. Memory is local-first and a
// standard must reach every machine whether or not anyone has looked at it yet.
func NormativePages(dataDir string) ([]NormativePage, error) {
	root := memoryRoot(dataDir)
	pins, err := readPins(dataDir)
	if err != nil {
		return nil, err
	}
	pages := []NormativePage{}
	seen := map[string]bool{}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		front, _ := frontmatter(string(data))
		if front.kind != normativeType {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
		seen[key] = true
		pages = append(pages, describe(key, pins[key], standing(pins[key].Checksum, Checksum(data))))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read %s: %w", root, err)
	}
	for key, pin := range pins {
		if !seen[key] {
			pages = append(pages, describe(key, pin, Missing))
		}
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Path < pages[j].Path })
	return pages, nil
}

func describe(key string, pin Ratification, s Standing) NormativePage {
	return NormativePage{Path: key, Standing: s, On: pin.Date, Machine: pin.Machine}
}

func standing(pinned, actual string) Standing {
	switch pinned {
	case "":
		return Unratified
	case actual:
		return Ratified
	default:
		return Changed
	}
}

// ChangedPages returns the set of pinned pages whose bytes no longer match the
// pin, for marking search results.
//
// It reads the pin file first and stops there when it is empty, so the common
// case — a machine where nobody has ratified anything — costs one failed stat
// and never walks the wiki. Search runs on every agent turn; a check that made
// it slower would be turned off.
//
// A page whose pin no longer has a file is reported as changed too. It cannot
// appear in a search result, so nothing is lost by not distinguishing it here,
// and NormativePages is where that difference is told.
func ChangedPages(dataDir string) (map[string]bool, error) {
	pins, err := readPins(dataDir)
	if err != nil || len(pins) == 0 {
		return nil, err
	}
	root := memoryRoot(dataDir)
	changed := map[string]bool{}
	for key, pin := range pins {
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(key)))
		if readErr != nil || Checksum(data) != pin.Checksum {
			changed[key] = true
		}
	}
	return changed, nil
}

// Ratify pins each page at the bytes it holds right now, leaving every other
// pin alone.
//
// The pin is content-addressed rather than event-addressed, which pays for
// itself on a revert: restoring a page to a version this machine had already
// accepted makes it ratified again with no second act of ratification, because
// the bytes are the bytes that were approved. Reverting to any other version
// leaves it CHANGED, which is correct — nobody approved that one here.
func Ratify(dataDir, machine string, now time.Time, paths []string) error {
	root := memoryRoot(dataDir)
	pins, err := readPins(dataDir)
	if err != nil {
		return err
	}
	for _, path := range paths {
		key, err := pageKey(path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(key)))
		if err != nil {
			return fmt.Errorf("cannot ratify %s: %w", key, err)
		}
		pins[key] = Ratification{
			Checksum: Checksum(data),
			Machine:  machine,
			Date:     now.Format(dayLayout),
		}
	}
	return writePins(dataDir, pins)
}

// Forget drops the pins for pages that are no longer in the tree and reports
// how many went. The pin store is authoritative rather than derived, so a
// deleted standard would otherwise be reported MISSING forever; this is the
// human saying the deletion was intended.
func Forget(dataDir string, paths []string) error {
	pins, err := readPins(dataDir)
	if err != nil {
		return err
	}
	for _, path := range paths {
		key, err := pageKey(path)
		if err != nil {
			return err
		}
		delete(pins, key)
	}
	return writePins(dataDir, pins)
}

// pageKey normalises a page argument to the slash-separated path the store is
// keyed by, and refuses anything that leaves the wiki. A pin names a page, so a
// caller must not be able to pin `../rules/20-memory.md` and have the standard
// it governs live somewhere the wiki does not reach.
func pageKey(path string) (string, error) {
	key := filepath.ToSlash(filepath.Clean(path))
	if filepath.IsAbs(path) || key == ".." || strings.HasPrefix(key, "../") {
		return "", fmt.Errorf("%s is not a page path inside the wiki", path)
	}
	return key, nil
}

// Checksum returns the sha256 of a page's bytes, prefixed with its algorithm so
// the stored form stays self-describing. Same shape as a flow's pin, for the
// same reason.
func Checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func memoryRoot(dataDir string) string { return filepath.Join(dataDir, "memory") }

func pinsPath(dataDir string) string { return filepath.Join(dataDir, ratifiedFile) }

// readPins loads the pin store. A store that cannot be parsed is an error
// rather than an empty map: a corrupt file must report that nothing is known,
// not that everything is unratified, because the two look identical downstream
// and only one of them is a problem.
func readPins(dataDir string) (map[string]Ratification, error) {
	path := pinsPath(dataDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Ratification{}, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	pins := map[string]Ratification{}
	if err := json.Unmarshal(data, &pins); err != nil {
		return nil, fmt.Errorf("ratification store %s is corrupt: %w", path, err)
	}
	return pins, nil
}

func writePins(dataDir string, pins map[string]Ratification) error {
	data, err := json.MarshalIndent(pins, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dataDir, ratifiedFile+".*")
	if err != nil {
		return err
	}
	if err := writeAndClose(tmp, append(data, '\n')); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), pinsPath(dataDir))
}

func writeAndClose(tmp *os.File, data []byte) error {
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Chmod(tmp.Name(), 0600)
}
