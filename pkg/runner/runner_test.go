package runner

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestStartOptionsArgs(t *testing.T) {
	testCases := []struct {
		name string
		opt  StartOptions
		want []string
	}{
		{
			name: "required only",
			opt: StartOptions{
				AuthPath: "/auth",
				Scenario: "reliability-small-cluster.yaml",
			},
			want: []string{"-p", "/auth", "-c", "reliability-small-cluster.yaml"},
		},
		{
			name: "all flags",
			opt: StartOptions{
				AuthPath:      "/auth",
				Scenario:      "reliability.yaml",
				Duration:      "2d",
				ToleranceRate: "5",
				FolderName:    "run1",
				Operators:     "netobserv-operator",
				Infra:         true,
				Upgrade:       true,
			},
			want: []string{
				"-p", "/auth",
				"-c", "reliability.yaml",
				"-t", "2d",
				"-r", "5",
				"-n", "run1",
				"-o", "netobserv-operator",
				"-i",
				"-u",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.opt.Args()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("StartOptions.Args() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStartedDocument(t *testing.T) {
	now := time.Unix(100, 0)
	testCases := []struct {
		name     string
		metadata string
		podUID   string
		want     Metadata
		wantErr  bool
	}{
		{
			name:     "adds timestamp and pod uid",
			metadata: `{"name":"small","namespace":"harness","podName":"small-runner","scenario":"reliability-small-cluster.yaml","image":"quay.io/example/reliability:latest","authSecret":"cluster-auth"}`,
			podUID:   "uid-1",
			want: Metadata{
				Name:       "small",
				Namespace:  "harness",
				PodName:    "small-runner",
				Scenario:   "reliability-small-cluster.yaml",
				Image:      "quay.io/example/reliability:latest",
				AuthSecret: "cluster-auth",
				Timestamp:  100,
				PodUID:     "uid-1",
			},
		},
		{
			name:     "omits an empty pod uid",
			metadata: `{"name":"small"}`,
			want: Metadata{
				Name:      "small",
				Timestamp: 100,
			},
		},
		{
			name:     "invalid metadata",
			metadata: `{`,
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := startedDocument(Options{
				Metadata: []byte(tc.metadata),
				PodUID:   tc.podUID,
				Now:      func() time.Time { return now },
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("startedDocument() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("startedDocument() error: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("startedDocument() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
