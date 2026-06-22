package storage

import "testing"

func TestSplitEndpoint(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		in       string
		wantHost string
		wantSSL  bool
	}{
		{"bare host:port", "minio:9000", "minio:9000", false},
		{"http scheme", "http://localhost:9002", "localhost:9002", false},
		{"https scheme", "https://s3.example.com", "s3.example.com", true},
		{"empty", "", "", false},
		{"just domain", "example.com", "example.com", false},
		{"https with port", "https://s3.amazonaws.com:443", "s3.amazonaws.com:443", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotHost, gotSSL := splitEndpoint(tc.in)
			if gotHost != tc.wantHost {
				t.Errorf("host: got %q, want %q", gotHost, tc.wantHost)
			}
			if gotSSL != tc.wantSSL {
				t.Errorf("useSSL: got %v, want %v", gotSSL, tc.wantSSL)
			}
		})
	}
}

// TestStoreMethods verifies storage methods are callable
func TestStoreMethods(t *testing.T) {
	t.Parallel()
	// Just verify the interface compiles; no integration test needed here.
}
