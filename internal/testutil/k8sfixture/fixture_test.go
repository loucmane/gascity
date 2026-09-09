//go:build integration

package k8sfixture

import (
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestRejectsUnexpectedTraffic(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, auth string
		want                     int
	}{
		{"other namespace", "GET", "/api/v1/namespaces/default/pods", "", 404},
		{"credentials", "GET", "/api/v1/namespaces/conformance/pods", "Bearer synthetic", 403},
		{"mutation outside contract", "PATCH", "/api/v1/namespaces/conformance/pods/p", "", 405},
		{"unsupported exec", "POST", "/api/v1/namespaces/conformance/pods/p/exec?stdin=true", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := New()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", tc.auth)
			recorder := httptest.NewRecorder()
			fixture.ServeHTTP(recorder, req)
			if recorder.Code != tc.want || len(fixture.problems) != 1 || len(fixture.pods) != 0 {
				t.Fatalf("refusal = HTTP %d, problems=%v, pods=%d", recorder.Code, fixture.problems, len(fixture.pods))
			}
		})
	}
	fixture := New()
	fixture.pods["p"] = &corev1.Pod{}
	if _, code := fixture.command("p", []string{"tmux", "unreviewed-command", "-t", "main"}); code != 127 || len(fixture.problems) != 1 {
		t.Fatal("unsupported exec was not recorded as a failing protocol obligation")
	}
	// The HTTP fixture emits a real Kubernetes error status, not an empty 200.
	recorder := httptest.NewRecorder()
	fixtureStatus(recorder, http.StatusNotFound, "NotFound")
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatal("fixture error is not a Kubernetes JSON response")
	}
}
