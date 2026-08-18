package flow

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FacileStudio/Mycelium/internal/config"
)

const trustFile = ".flow-trust.json"

// TrustPath returns the file pinning each flow name to the checksum this
// machine accepted. It is a dotfile, so sync never carries it and trust stays
// per machine.
func TrustPath() string { return filepath.Join(config.DataDir(), trustFile) }

// TrustedChecksum returns the checksum pinned for a flow, or an empty string
// when the flow was never trusted. A store that cannot be parsed is an error:
// an unreadable pin must refuse execution, not wave it through.
func TrustedChecksum(name string) (string, error) {
	pins, err := readTrust()
	if err != nil {
		return "", err
	}
	return pins[name], nil
}

// IsTrusted reports whether a flow's bytes still match its pinned checksum. A
// flow that was never pinned, or that changed since, is not trusted.
func IsTrusted(f *Flow) (bool, error) {
	pinned, err := TrustedChecksum(f.Name)
	if err != nil {
		return false, err
	}
	if pinned == "" {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(pinned), []byte(f.Checksum)) == 1, nil
}

// Trust pins a flow's current checksum, leaving every other entry alone.
func Trust(f *Flow) error {
	pins, err := readTrust()
	if err != nil {
		return err
	}
	pins[f.Name] = f.Checksum
	return writeTrust(pins)
}

// Untrust drops a flow's pin. Removing a flow that was never trusted is not an
// error.
func Untrust(name string) error {
	pins, err := readTrust()
	if err != nil {
		return err
	}
	if _, ok := pins[name]; !ok {
		return nil
	}
	delete(pins, name)
	return writeTrust(pins)
}

func readTrust() (map[string]string, error) {
	path := TrustPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	pins := map[string]string{}
	if err := json.Unmarshal(data, &pins); err != nil {
		return nil, fmt.Errorf("trust store %s is corrupt: %w", path, err)
	}
	return pins, nil
}

func writeTrust(pins map[string]string) error {
	data, err := json.MarshalIndent(pins, "", "  ")
	if err != nil {
		return err
	}
	path := TrustPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), trustFile+".*")
	if err != nil {
		return err
	}
	if err := finishTemp(tmp, append(data, '\n')); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func finishTemp(tmp *os.File, data []byte) error {
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Chmod(tmp.Name(), 0600)
}
