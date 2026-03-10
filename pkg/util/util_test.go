package util

import (
	"testing"
)

func TestValidURL(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		valid bool
	}{
		{
			name:  "valid https URL",
			url:   "https://example.com",
			valid: true,
		},
		{
			name:  "valid http URL",
			url:   "http://example.com",
			valid: true,
		},
		{
			name:  "valid URL with path",
			url:   "https://example.com/path/to/resource",
			valid: true,
		},
		{
			name:  "valid URL with query parameters",
			url:   "https://example.com/path?param=value",
			valid: true,
		},
		{
			name:  "valid URL with port",
			url:   "https://example.com:8080",
			valid: true,
		},
		{
			name:  "empty string",
			url:   "",
			valid: false,
		},
		{
			name:  "no scheme",
			url:   "example.com",
			valid: false,
		},
		{
			name:  "no host",
			url:   "https://",
			valid: false,
		},
		{
			name:  "malformed URL",
			url:   "not a url",
			valid: false,
		},
		{
			name:  "just a path",
			url:   "/path/to/resource",
			valid: false,
		},
		{
			name:  "just a scheme and path",
			url:   "https:///path/to/resource",
			valid: false,
		},
		{
			name:  "scheme with no slashes",
			url:   "http:example.com",
			valid: false,
		},
		{
			name:  "ftp URL not allowed",
			url:   "ftp://ftp.example.com",
			valid: false,
		},
		{
			name:  "ws URL not allowed",
			url:   "ws://example.com",
			valid: false,
		},
		{
			name:  "file URL not allowed",
			url:   "file:///etc/passwd",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidURL(tt.url)
			if result != tt.valid {
				t.Errorf("ValidURL(%q) = %v, expected %v", tt.url, result, tt.valid)
			}
		})
	}
}
