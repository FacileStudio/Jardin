package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func deviceTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := New(t.TempDir(), "secret")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Admin/session token, as the dashboard obtains it.
	status, body := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{"password": "secret"})
	if status != http.StatusOK {
		t.Fatalf("login failed: %d", status)
	}
	return ts, body["token"]
}

func doJSON(t *testing.T, ts *httptest.Server, method, path, token string, payload any) (int, map[string]string) {
	t.Helper()
	var reader *bytes.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]string{}
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestDeviceFlowApprove(t *testing.T) {
	ts, admin := deviceTestServer(t)

	status, start := doJSON(t, ts, "POST", "/api/auth/device/start", "", map[string]string{"machine": "laptop"})
	if status != http.StatusOK {
		t.Fatalf("start: %d", status)
	}
	deviceCode, userCode := start["device_code"], start["user_code"]
	if deviceCode == "" || userCode == "" {
		t.Fatalf("missing codes: %+v", start)
	}

	// Before approval, polling is pending.
	if code, _ := doJSON(t, ts, "POST", "/api/auth/device/poll", "", map[string]string{"device_code": deviceCode}); code != http.StatusAccepted {
		t.Fatalf("expected pending (202), got %d", code)
	}

	// Dashboard sees the pending request.
	code, info := doJSON(t, ts, "GET", "/api/auth/device/info?code="+userCode, admin, nil)
	if code != http.StatusOK || info["machine"] != "laptop" || info["status"] != "pending" {
		t.Fatalf("info wrong: %d %+v", code, info)
	}

	// Approve from the authenticated session.
	if code, _ := doJSON(t, ts, "POST", "/api/auth/device/approve", admin, map[string]string{"user_code": userCode}); code != http.StatusOK {
		t.Fatalf("approve: %d", code)
	}

	// Polling now yields a working token.
	code, res := doJSON(t, ts, "POST", "/api/auth/device/poll", "", map[string]string{"device_code": deviceCode})
	if code != http.StatusOK || res["token"] == "" {
		t.Fatalf("expected token, got %d %+v", code, res)
	}
	if s, _ := doJSON(t, ts, "GET", "/api/status", res["token"], nil); s != http.StatusOK {
		t.Fatalf("issued token rejected: %d", s)
	}

	// The token can only be retrieved once.
	if code, _ := doJSON(t, ts, "POST", "/api/auth/device/poll", "", map[string]string{"device_code": deviceCode}); code != http.StatusBadRequest {
		t.Fatalf("expected consumed (400), got %d", code)
	}
}

func TestDeviceApproveRequiresAdmin(t *testing.T) {
	ts, admin := deviceTestServer(t)

	// A sync-scoped machine token must not be able to approve devices.
	_, syncTok := doJSON(t, ts, "POST", "/api/auth/login", "", map[string]string{"password": "secret", "machine": "box"})
	_, start := doJSON(t, ts, "POST", "/api/auth/device/start", "", map[string]string{"machine": "laptop"})

	if code, _ := doJSON(t, ts, "POST", "/api/auth/device/approve", syncTok["token"], map[string]string{"user_code": start["user_code"]}); code != http.StatusForbidden {
		t.Fatalf("sync token should be forbidden, got %d", code)
	}
	// Unauthenticated approve is rejected too.
	if code, _ := doJSON(t, ts, "POST", "/api/auth/device/approve", "", map[string]string{"user_code": start["user_code"]}); code != http.StatusUnauthorized {
		t.Fatalf("anonymous approve should be 401, got %d", code)
	}
	_ = admin
}

func TestDeviceStoreCapAndNormalization(t *testing.T) {
	d := newDeviceStore()
	now := time.Now()

	var last deviceRequest
	for i := 0; i < maxPendingDevices; i++ {
		req, err := d.create("m", "1.2.3.4", now)
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		last = req
	}
	if _, err := d.create("m", "1.2.3.4", now); !errors.Is(err, ErrTooManyDevices) {
		t.Fatalf("expected cap error, got %v", err)
	}

	// User entry is forgiving: lowercase and a mangled separator still resolve.
	scrambled := strings.ToLower(strings.ReplaceAll(last.UserCode, "-", "  "))
	if _, ok := d.info(scrambled); !ok {
		t.Fatalf("normalized lookup failed for %q", last.UserCode)
	}
}

func TestDeviceDeny(t *testing.T) {
	ts, admin := deviceTestServer(t)
	_, start := doJSON(t, ts, "POST", "/api/auth/device/start", "", map[string]string{"machine": "laptop"})

	if code, _ := doJSON(t, ts, "POST", "/api/auth/device/deny", admin, map[string]string{"user_code": start["user_code"]}); code != http.StatusNoContent {
		t.Fatalf("deny: %d", code)
	}
	if code, _ := doJSON(t, ts, "POST", "/api/auth/device/poll", "", map[string]string{"device_code": start["device_code"]}); code != http.StatusForbidden {
		t.Fatalf("denied poll should be 403, got %d", code)
	}
}
