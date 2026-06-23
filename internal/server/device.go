package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	deviceCodeTTL      = 10 * time.Minute
	devicePollInterval = 5
	maxPendingDevices  = 256
	// No vowels (avoids real words) and no 0/1/I/L/O/U (avoids confusion).
	userCodeAlphabet = "23456789BCDFGHJKMNPQRSTVWXYZ"
)

// ErrTooManyDevices is returned when too many device authorizations are pending.
var ErrTooManyDevices = errors.New("too many pending device authorizations")

type deviceStatus string

const (
	devicePending  deviceStatus = "pending"
	deviceApproved deviceStatus = "approved"
	deviceDenied   deviceStatus = "denied"
)

type deviceRequest struct {
	DeviceCode string
	UserCode   string
	Machine    string
	IP         string
	Status     deviceStatus
	Token      string
	Expires    time.Time
}

type deviceStore struct {
	mu       sync.Mutex
	byDevice map[string]*deviceRequest
	byUser   map[string]*deviceRequest
}

func newDeviceStore() *deviceStore {
	return &deviceStore{
		byDevice: make(map[string]*deviceRequest),
		byUser:   make(map[string]*deviceRequest),
	}
}

// normalizeUserCode upper-cases and strips anything that isn't part of the
// code (hyphens, spaces) so user entry is forgiving and matches the stored key.
func normalizeUserCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (d *deviceStore) sweep(now time.Time) {
	for code, req := range d.byDevice {
		if now.After(req.Expires) {
			delete(d.byDevice, code)
			delete(d.byUser, normalizeUserCode(req.UserCode))
		}
	}
}

func (d *deviceStore) create(machine, ip string, now time.Time) (deviceRequest, error) {
	deviceCode, err := generateToken()
	if err != nil {
		return deviceRequest{}, err
	}
	userCode, err := generateUserCode()
	if err != nil {
		return deviceRequest{}, err
	}
	req := &deviceRequest{
		DeviceCode: deviceCode,
		UserCode:   userCode,
		Machine:    machine,
		IP:         ip,
		Status:     devicePending,
		Expires:    now.Add(deviceCodeTTL),
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sweep(now)
	if len(d.byDevice) >= maxPendingDevices {
		return deviceRequest{}, ErrTooManyDevices
	}
	d.byDevice[deviceCode] = req
	d.byUser[normalizeUserCode(userCode)] = req
	return *req, nil
}

func (d *deviceStore) info(userCode string) (deviceRequest, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sweep(time.Now())
	req, ok := d.byUser[normalizeUserCode(userCode)]
	if !ok {
		return deviceRequest{}, false
	}
	return *req, true
}

func (d *deviceStore) approve(userCode, token string) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sweep(time.Now())
	req, ok := d.byUser[normalizeUserCode(userCode)]
	if !ok || req.Status != devicePending {
		return "", false
	}
	req.Status = deviceApproved
	req.Token = token
	return req.Machine, true
}

func (d *deviceStore) deny(userCode string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sweep(time.Now())
	req, ok := d.byUser[normalizeUserCode(userCode)]
	if !ok || req.Status != devicePending {
		return false
	}
	req.Status = deviceDenied
	return true
}

// poll returns the request status; once approved it returns the token and
// consumes the request so a token can only be retrieved once.
func (d *deviceStore) poll(deviceCode string) (deviceStatus, string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sweep(time.Now())
	req, ok := d.byDevice[deviceCode]
	if !ok {
		return "", "", false
	}
	if req.Status == deviceApproved {
		token := req.Token
		delete(d.byDevice, req.DeviceCode)
		delete(d.byUser, normalizeUserCode(req.UserCode))
		return deviceApproved, token, true
	}
	return req.Status, "", true
}

func generateUserCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 0, 9)
	for i, c := range b {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, userCodeAlphabet[int(c)%len(userCodeAlphabet)])
	}
	return string(out), nil
}

func (s *Server) baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return scheme + "://" + r.Host
}

func (s *Server) deviceStart(w http.ResponseWriter, r *http.Request) {
	if !s.devStarts.allow(clientIP(r), time.Now()) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Machine string `json:"machine"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	machine := strings.TrimSpace(req.Machine)
	if machine == "" {
		machine = "device"
	}

	dr, err := s.devices.create(machine, clientIP(r), time.Now().UTC())
	if err != nil {
		if errors.Is(err, ErrTooManyDevices) {
			http.Error(w, "too many pending authorizations", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	base := s.baseURL(r)
	jsonReply(w, map[string]any{
		"device_code":               dr.DeviceCode,
		"user_code":                 dr.UserCode,
		"machine":                   dr.Machine,
		"verification_uri":          base + "/authorize",
		"verification_uri_complete": base + "/authorize?code=" + dr.UserCode,
		"interval":                  devicePollInterval,
		"expires_in":                int(deviceCodeTTL.Seconds()),
	})
}

func (s *Server) devicePoll(w http.ResponseWriter, r *http.Request) {
	if !s.devPolls.allow(clientIP(r), time.Now()) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	status, token, ok := s.devices.poll(req.DeviceCode)
	if !ok {
		http.Error(w, "unknown or expired device code", http.StatusBadRequest)
		return
	}
	switch status {
	case deviceApproved:
		jsonReply(w, map[string]string{"token": token})
	case deviceDenied:
		http.Error(w, "authorization denied", http.StatusForbidden)
	default:
		jsonStatus(w, http.StatusAccepted, map[string]string{"status": "pending"})
	}
}

func (s *Server) deviceInfo(w http.ResponseWriter, r *http.Request) {
	req, ok := s.devices.info(r.URL.Query().Get("code"))
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	jsonReply(w, map[string]string{
		"user_code": req.UserCode,
		"machine":   req.Machine,
		"ip":        req.IP,
		"status":    string(req.Status),
	})
}

func (s *Server) deviceApprove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserCode string `json:"user_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req, ok := s.devices.info(body.UserCode)
	if !ok || req.Status != devicePending {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	token, err := s.mintToken(req.Machine, scopeSync)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if _, ok := s.devices.approve(body.UserCode, token); !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	jsonReply(w, map[string]string{"machine": req.Machine})
}

func (s *Server) deviceDeny(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserCode string `json:"user_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.devices.deny(body.UserCode) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func jsonStatus(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
