package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultMaxBytes int64 = 1024 * 1024 * 1024

type Config struct {
	CacheDir     string                `json:"cache_dir,omitempty"`
	MaxBytes     int64                 `json:"max_bytes,omitempty"`
	OpenCommand  string                `json:"open_command,omitempty"`
	AllowedRoots []string              `json:"allowed_roots,omitempty"`
	Hosts        map[string]HostConfig `json:"hosts,omitempty"`
}

type HostConfig struct {
	Alias        string   `json:"alias,omitempty"`
	MaxBytes     int64    `json:"max_bytes,omitempty"`
	AllowedRoots []string `json:"allowed_roots,omitempty"`
}

func defaultConfig() Config {
	return Config{
		MaxBytes: defaultMaxBytes,
		AllowedRoots: []string{
			"/home",
			"/Users",
			"/tmp",
			"/var/tmp",
			"/var/log",
			"/opt",
			"/workspace",
			"/workspaces",
			"/mnt",
			"/data",
		},
		Hosts: map[string]HostConfig{},
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return cfg, err
		}
	}
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = defaultMaxBytes
	}
	if cfg.AllowedRoots == nil {
		cfg.AllowedRoots = defaultConfig().AllowedRoots
	}
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]HostConfig{}
	}
	return cfg, nil
}

func defaultConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ropen", "config.json"), nil
}

func (c Config) hostAlias(host string, user string) string {
	if hc, ok := c.Hosts[host]; ok && hc.Alias != "" {
		return hc.Alias
	}
	if alias := sshConfigAliasForHostName(host, user); alias != "" {
		return alias
	}
	if user != "" && !hasUser(host) {
		return user + "@" + host
	}
	return host
}

func sshConfigAliasForHostName(hostName string, user string) string {
	path := filepath.Join(os.Getenv("HOME"), ".ssh", "config")
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	type block struct {
		hosts    []string
		hostName string
		user     string
	}
	var blocks []block
	current := block{}
	flush := func() {
		if len(current.hosts) > 0 {
			blocks = append(blocks, current)
		}
		current = block{}
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.ToLower(fields[0])
		switch key {
		case "host":
			flush()
			current.hosts = fields[1:]
		case "hostname":
			current.hostName = fields[1]
		case "user":
			current.user = fields[1]
		}
	}
	flush()

	for _, b := range blocks {
		if b.hostName != hostName {
			continue
		}
		if user != "" && b.user != "" && b.user != user {
			continue
		}
		for _, alias := range b.hosts {
			if strings.ContainsAny(alias, "*?") || strings.HasPrefix(alias, "!") {
				continue
			}
			return alias
		}
	}
	return ""
}

func (c Config) maxBytesForHost(host string) int64 {
	if hc, ok := c.Hosts[host]; ok && hc.MaxBytes > 0 {
		return hc.MaxBytes
	}
	if c.MaxBytes > 0 {
		return c.MaxBytes
	}
	return defaultMaxBytes
}

func (c Config) allowedRootsForHost(host string) []string {
	if hc, ok := c.Hosts[host]; ok && len(hc.AllowedRoots) > 0 {
		return hc.AllowedRoots
	}
	return c.AllowedRoots
}

func hasUser(host string) bool {
	for _, ch := range host {
		if ch == '@' {
			return true
		}
	}
	return false
}
