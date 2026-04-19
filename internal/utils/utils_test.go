package utils

import (
	"strings"
	"testing"
)

func TestGetWSAddr(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "Valid HTTPS URL",
			input:   "https://example.com",
			want:    "wss://example.com",
			wantErr: false,
		},
		{
			name:    "Valid HTTP URL",
			input:   "http://example.com",
			want:    "ws://example.com",
			wantErr: false,
		},
		{
			name:    "Invalid scheme - FTP",
			input:   "ftp://example.com",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Empty string",
			input:   "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "No scheme prefix",
			input:   "example.com",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Partial scheme - http:/",
			input:   "http:/example.com",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetWSAddr(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetWSAddr(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("GetWSAddr(%q) unexpected error: %v", tt.input, err)
				return
			}

			if got != tt.want {
				t.Errorf("GetWSAddr(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetWSAddr_HTTPS(t *testing.T) {
	got, err := GetWSAddr("https://example.com")
	if err != nil {
		t.Errorf("GetWSAddr(https://example.com) unexpected error: %v", err)
	}
	want := "wss://example.com"
	if got != want {
		t.Errorf("GetWSAddr(https://example.com) = %q; want %q", got, want)
	}
}

func TestGetWSAddr_HTTP(t *testing.T) {
	got, err := GetWSAddr("http://example.com")
	if err != nil {
		t.Errorf("GetWSAddr(http://example.com) unexpected error: %v", err)
	}
	want := "ws://example.com"
	if got != want {
		t.Errorf("GetWSAddr(http://example.com) = %q; want %q", got, want)
	}
}

func TestGetWSAddr_FTPError(t *testing.T) {
	got, err := GetWSAddr("ftp://example.com")
	if err == nil {
		t.Error("GetWSAddr(ftp://example.com) expected error, got nil")
	}
	if got != "" {
		t.Errorf("GetWSAddr(ftp://example.com) = %q; want empty string", got)
	}
	if !strings.Contains(err.Error(), "Error: address provided was not an HTTP(S) address") {
		t.Errorf("GetWSAddr(ftp://example.com) error message does not match expected: %v", err)
	}
}

func TestGetWSAddr_Empty(t *testing.T) {
	got, err := GetWSAddr("")
	if err == nil {
		t.Error("GetWSAddr(\"\") expected error, got nil")
	}
	if got != "" {
		t.Errorf("GetWSAddr(\"\") = %q; want empty string", got)
	}
}

func TestGetWSAddr_NoScheme(t *testing.T) {
	got, err := GetWSAddr("example.com")
	if err == nil {
		t.Error("GetWSAddr(example.com) expected error, got nil")
	}
	if got != "" {
		t.Errorf("GetWSAddr(example.com) = %q; want empty string", got)
	}
}

func TestGetWSAddr_PartialScheme(t *testing.T) {
	got, err := GetWSAddr("http:/example.com")
	if err == nil {
		t.Error("GetWSAddr(http:/example.com) expected error, got nil")
	}
	if got != "" {
		t.Errorf("GetWSAddr(http:/example.com) = %q; want empty string", got)
	}
}
