package cli

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/generate"
)

func TestMultipartMaxMemoryFlag_Set(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		want    int64
		wantErr string
	}{
		{name: "decimal MB", input: "32MB", want: 32_000_000},
		{name: "binary MiB", input: "32MiB", want: 32 << 20},
		{name: "GB", input: "1GB", want: 1_000_000_000},
		{name: "bare bytes", input: "1024", want: 1024},
		{name: "empty", input: "", wantErr: "invalid byte size"},
		{name: "zero", input: "0", wantErr: "must be positive"},
		{name: "garbage", input: "not-a-size", wantErr: "invalid byte size"},
		{name: "negative", input: "-1MB", wantErr: "invalid byte size"},
		{name: "overflow int64", input: "10EB", wantErr: "exceeds int64 maximum"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &generate.RoutesFileConfiguration{}
			f := &multipartMaxMemoryFlag{cfg: cfg}
			err := f.Set(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("Set(%q) = nil, want error containing %q", tc.input, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Set(%q) error = %q, want containing %q", tc.input, err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Set(%q) = %v, want no error", tc.input, err)
			}
			if cfg.MultipartMaxMemory != tc.want {
				t.Fatalf("Set(%q) stored %d, want %d", tc.input, cfg.MultipartMaxMemory, tc.want)
			}
		})
	}
}

func TestMultipartMaxMemoryFlag_String(t *testing.T) {
	t.Run("zero shows default", func(t *testing.T) {
		f := &multipartMaxMemoryFlag{cfg: &generate.RoutesFileConfiguration{}}
		got := f.String()
		if !strings.Contains(got, "MiB") && !strings.Contains(got, "MB") {
			t.Fatalf("String() = %q, want a human-readable size", got)
		}
	})
	t.Run("override shows override", func(t *testing.T) {
		f := &multipartMaxMemoryFlag{cfg: &generate.RoutesFileConfiguration{MultipartMaxMemory: 64 << 20}}
		got := f.String()
		if !strings.Contains(got, "64") {
			t.Fatalf("String() = %q, want containing 64", got)
		}
	})
}
