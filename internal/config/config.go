package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

type Config struct {
	ListenAddr string       `yaml:"listen_addr"`
	AppTitle   string       `yaml:"app_title"`
	PublicPath string       `yaml:"public_path"`
	CalDAV     CalDAVConfig `yaml:"caldav"`
}

type CalDAVConfig struct {
	URL  string `yaml:"url"`
	User string `yaml:"user"`
	Pass string `yaml:"pass"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.UnmarshalWithOptions(data, &cfg, yaml.Strict()); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}

	return cfg, nil
}

func (cfg Config) Validate() error {
	var errs []error

	require(&errs, cfg.ListenAddr, "listen_addr")
	require(&errs, cfg.AppTitle, "app_title")
	require(&errs, cfg.CalDAV.URL, "caldav.url")
	require(&errs, cfg.CalDAV.User, "caldav.user")
	require(&errs, cfg.CalDAV.Pass, "caldav.pass")

	if publicPath := strings.TrimSpace(cfg.PublicPath); publicPath != "" {
		if !strings.HasPrefix(publicPath, "/") {
			errs = append(errs, errors.New("public_path must start with /"))
		}
		if publicPath != "/" && strings.HasSuffix(publicPath, "/") {
			errs = append(errs, errors.New("public_path must not end with /"))
		}
		if strings.Contains(publicPath, "//") {
			errs = append(errs, errors.New("public_path must not contain //"))
		}
		if hasInvalidPublicPathChar(publicPath) {
			errs = append(errs, errors.New("public_path must be a URL path without query, fragment, whitespace, or control characters"))
		}
	}

	if strings.TrimSpace(cfg.CalDAV.URL) != "" {
		u, err := url.Parse(cfg.CalDAV.URL)
		if err != nil {
			errs = append(errs, fmt.Errorf("caldav.url must be a valid URL: %w", err))
		} else {
			if u.Scheme != "http" && u.Scheme != "https" {
				errs = append(errs, errors.New("caldav.url must use http or https"))
			}
			if u.Host == "" {
				errs = append(errs, errors.New("caldav.url must be absolute"))
			}
			if !strings.HasSuffix(u.Path, "/") {
				errs = append(errs, errors.New("caldav.url must end with /"))
			}
		}
	}

	return errors.Join(errs...)
}

func require(errs *[]error, value, field string) {
	if strings.TrimSpace(value) == "" {
		*errs = append(*errs, fmt.Errorf("%s is required", field))
	}
}

func hasInvalidPublicPathChar(value string) bool {
	return strings.ContainsAny(value, "?#") || strings.ContainsFunc(value, func(r rune) bool {
		return r <= 0x1f || r == 0x7f || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}
