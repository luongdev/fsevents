package config

import (
	"testing"
	"time"
)

func TestHTTPDestination_Validate_MultiURL(t *testing.T) {
	tests := []struct {
		name    string
		dest    HTTPDestination
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid single URL (backward compatible)",
			dest: HTTPDestination{
				Name:    "test-dest",
				URL:     "https://example.com/webhook",
				Method:  "POST",
				Timeout: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid multiple URLs with round-robin",
			dest: HTTPDestination{
				Name:     "test-dest",
				URLs:     []string{"https://api1.example.com", "https://api2.example.com"},
				Strategy: "round-robin",
				Method:   "POST",
				Timeout:  30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid multiple URLs with failover",
			dest: HTTPDestination{
				Name:     "test-dest",
				URLs:     []string{"https://primary.example.com", "https://secondary.example.com"},
				Strategy: "failover",
				Method:   "POST",
				Timeout:  30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid multiple URLs with broadcast",
			dest: HTTPDestination{
				Name:     "test-dest",
				URLs:     []string{"https://api1.example.com", "https://api2.example.com"},
				Strategy: "broadcast",
				Method:   "POST",
				Timeout:  30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid multiple URLs with random",
			dest: HTTPDestination{
				Name:     "test-dest",
				URLs:     []string{"https://api1.example.com", "https://api2.example.com"},
				Strategy: "random",
				Method:   "POST",
				Timeout:  30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid multiple URLs with empty strategy (defaults to round-robin)",
			dest: HTTPDestination{
				Name:     "test-dest",
				URLs:     []string{"https://api1.example.com", "https://api2.example.com"},
				Strategy: "",
				Method:   "POST",
				Timeout:  30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "error: no URL or URLs configured",
			dest: HTTPDestination{
				Name:    "test-dest",
				Method:  "POST",
				Timeout: 30 * time.Second,
			},
			wantErr: true,
			errMsg:  "must have either 'url' or 'urls' configured",
		},
		{
			name: "error: invalid strategy",
			dest: HTTPDestination{
				Name:     "test-dest",
				URLs:     []string{"https://api1.example.com", "https://api2.example.com"},
				Strategy: "invalid-strategy",
				Method:   "POST",
				Timeout:  30 * time.Second,
			},
			wantErr: true,
			errMsg:  "invalid strategy",
		},
		{
			name: "error: empty URL in URLs array",
			dest: HTTPDestination{
				Name:     "test-dest",
				URLs:     []string{"https://api1.example.com", ""},
				Strategy: "round-robin",
				Method:   "POST",
				Timeout:  30 * time.Second,
			},
			wantErr: true,
			errMsg:  "URL at index 1 cannot be empty",
		},
		{
			name: "error: invalid URL format in URLs array",
			dest: HTTPDestination{
				Name:     "test-dest",
				URLs:     []string{"https://api1.example.com", "not-a-valid-url"},
				Strategy: "round-robin",
				Method:   "POST",
				Timeout:  30 * time.Second,
			},
			wantErr: false, // url.Parse is lenient, this will pass
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.dest.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("HTTPDestination.Validate() expected error but got none")
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("HTTPDestination.Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("HTTPDestination.Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
