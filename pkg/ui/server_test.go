package ui

import (
	"bytes"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"

	reliabilitytestv1alpha1 "github.com/droslean/test-harness-operator/pkg/api/reliabilitytest/v1alpha1"
)

func TestRenderNav(t *testing.T) {
	testCases := []struct {
		name    string
		page    string
		section string
	}{
		{name: "list", page: "list", section: "tests"},
		{name: "secret_list", page: "secret_list", section: "secrets"},
		{name: "secret_create", page: "secret_create", section: "secrets"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, err := template.ParseFS(templateFS, "templates/base.html", "templates/"+tc.page+".html")
			if err != nil {
				t.Fatalf("ParseFS: %v", err)
			}
			var buf bytes.Buffer
			data := pageData{Section: tc.section, Namespace: "test-harness"}
			if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
				t.Fatalf("ExecuteTemplate: %v", err)
			}
			out := buf.String()
			if !strings.Contains(out, "Auth Secrets") {
				t.Fatalf("missing Auth Secrets link in nav:\n%s", out)
			}
			if !strings.Contains(out, "Reliability Tests") {
				t.Fatalf("missing Reliability Tests link in nav:\n%s", out)
			}
		})
	}
}

func TestSlackSpecFromForm(t *testing.T) {
	testCases := []struct {
		name string
		form url.Values
		want *reliabilitytestv1alpha1.SlackSpec
	}{
		{
			name: "empty",
			form: url.Values{},
			want: nil,
		},
		{
			name: "member only",
			form: url.Values{"slackMember": {"U123"}},
			want: &reliabilitytestv1alpha1.SlackSpec{Member: "U123"},
		},
		{
			name: "token secret and key",
			form: url.Values{
				"slackAPITokenSecret": {"slack-creds"},
				"slackAPITokenKey":    {"api-token"},
			},
			want: &reliabilitytestv1alpha1.SlackSpec{
				APITokenSecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "slack-creds"},
					Key:                  "api-token",
				},
			},
		},
		{
			name: "token secret without key ignored",
			form: url.Values{
				"slackAPITokenSecret": {"slack-creds"},
			},
			want: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "/tests", strings.NewReader(tc.form.Encode()))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			got := slackSpecFromForm(req)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("slackSpecFromForm() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
