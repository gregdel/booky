package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validConfig = `
listen_addr: ":8080"
app_title: "Vacation House"

caldav:
  url: "https://cloud.example.com/remote.php/dav/calendars/family-house/vacation-house/"
  user: "family-house"
  pass: "app-password"
`

func TestLoadValidConfig(t *testing.T) {
	cfg := loadConfig(t, validConfig)

	if cfg.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8080")
	}
	if cfg.AppTitle != "Vacation House" {
		t.Fatalf("AppTitle = %q, want %q", cfg.AppTitle, "Vacation House")
	}
	if cfg.CalDAV.URL != "https://cloud.example.com/remote.php/dav/calendars/family-house/vacation-house/" {
		t.Fatalf("CalDAV.URL = %q", cfg.CalDAV.URL)
	}
	if cfg.CalDAV.User != "family-house" {
		t.Fatalf("CalDAV.User = %q, want %q", cfg.CalDAV.User, "family-house")
	}
	if cfg.CalDAV.Pass != "app-password" {
		t.Fatalf("CalDAV.Pass = %q, want %q", cfg.CalDAV.Pass, "app-password")
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	_, err := loadConfigErr(t, validConfig+"\npublic_secret: nope\n")
	if err == nil {
		t.Fatal("Load returned nil error")
	}
	if !strings.Contains(err.Error(), "public_secret") {
		t.Fatalf("error = %q, want unknown field name", err)
	}
}

func TestLoadRequiresFields(t *testing.T) {
	tests := map[string]string{
		"listen_addr": `
app_title: "Vacation House"
caldav:
  url: "https://cloud.example.com/calendar/"
  user: "family-house"
  pass: "app-password"
`,
		"app_title": `
listen_addr: ":8080"
caldav:
  url: "https://cloud.example.com/calendar/"
  user: "family-house"
  pass: "app-password"
`,
		"caldav.url": `
listen_addr: ":8080"
app_title: "Vacation House"
caldav:
  user: "family-house"
  pass: "app-password"
`,
		"caldav.user": `
listen_addr: ":8080"
app_title: "Vacation House"
caldav:
  url: "https://cloud.example.com/calendar/"
  pass: "app-password"
`,
		"caldav.pass": `
listen_addr: ":8080"
app_title: "Vacation House"
caldav:
  url: "https://cloud.example.com/calendar/"
  user: "family-house"
`,
	}

	for field, input := range tests {
		t.Run(field, func(t *testing.T) {
			_, err := loadConfigErr(t, input)
			if err == nil {
				t.Fatal("Load returned nil error")
			}
			if !strings.Contains(err.Error(), field+" is required") {
				t.Fatalf("error = %q, want required field %q", err, field)
			}
		})
	}
}

func TestLoadValidatesCalDAVURL(t *testing.T) {
	tests := map[string]string{
		"invalid":          ":://nope",
		"unsupported":      "ftp://cloud.example.com/calendar/",
		"relative":         "/remote.php/dav/calendars/family-house/vacation-house/",
		"missing trailing": "https://cloud.example.com/remote.php/dav/calendars/family-house/vacation-house",
	}

	for name, caldavURL := range tests {
		t.Run(name, func(t *testing.T) {
			input := strings.Replace(validConfig, "https://cloud.example.com/remote.php/dav/calendars/family-house/vacation-house/", caldavURL, 1)
			_, err := loadConfigErr(t, input)
			if err == nil {
				t.Fatal("Load returned nil error")
			}
		})
	}
}

func TestLoadWrapsPathInError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load returned nil error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %q, want path %q", err, path)
	}
}

func loadConfig(t *testing.T, input string) Config {
	t.Helper()

	cfg, err := loadConfigErr(t, input)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	return cfg
}

func loadConfigErr(t *testing.T, input string) (Config, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return Load(path)
}
