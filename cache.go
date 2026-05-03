package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func resolveLocalPath(target Target, cfg Config, refresh bool, dryRun bool) (string, error) {
	switch target.Kind {
	case TargetLocal:
		return resolveLocal(target)
	case TargetSSH:
		return fetchSSH(target, cfg, refresh, dryRun)
	case TargetObject:
		return fetchObject(target, cfg, refresh, dryRun)
	default:
		return "", fmt.Errorf("unsupported target kind %q", target.Kind)
	}
}

func resolveLocal(target Target) (string, error) {
	p := target.Path
	if !filepath.IsAbs(p) {
		cwd := target.Cwd
		if cwd == "" {
			var err error
			cwd, err = os.Getwd()
			if err != nil {
				return "", err
			}
		}
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(p), nil
}

func cachePathForSSH(cacheDir string, host string, remotePath string) string {
	rel := strings.TrimPrefix(remotePath, "/")
	if rel == "" || strings.HasPrefix(remotePath, "~") {
		rel = hashedName(remotePath)
	}
	return filepath.Join(cacheDir, "ssh", safeName(host), filepath.FromSlash(rel))
}

func cachePathForURI(cacheDir string, rawURI string) string {
	ext := filepath.Ext(rawURI)
	if len(ext) > 32 || strings.ContainsAny(ext, `/\:`) {
		ext = ""
	}
	return filepath.Join(cacheDir, "object", hashedName(rawURI)+ext)
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func hashedName(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:24]
}

func atomicTarget(path string) string {
	return path + ".tmp-" + fmt.Sprintf("%d", time.Now().UnixNano())
}

func pruneCache(cacheDir string, maxAge time.Duration) (removed int, bytes int64, err error) {
	cutoff := time.Now().Add(-maxAge)
	err = filepath.WalkDir(cacheDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(cutoff) {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		removed++
		bytes += info.Size()
		return nil
	})
	if os.IsNotExist(err) {
		return 0, 0, nil
	}
	return removed, bytes, err
}
