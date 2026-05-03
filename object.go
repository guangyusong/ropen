package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

func fetchObject(target Target, cfg Config, refresh bool, dryRun bool) (string, error) {
	localPath := cachePathForURI(cfg.CacheDir, target.URI)
	if !refresh {
		if _, err := os.Stat(localPath); err == nil {
			return localPath, nil
		}
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "would copy %s -> %s\n", target.URI, localPath)
		return localPath, nil
	}

	if size, ok := objectSize(target); ok && size > cfg.MaxBytes {
		return "", fmt.Errorf("remote object is %s, above max-size %s", formatBytes(size), formatBytes(cfg.MaxBytes))
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return "", err
	}
	tmp := atomicTarget(localPath)
	if err := copyObject(target, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, localPath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return localPath, nil
}

func objectSize(target Target) (int64, bool) {
	switch target.Scheme {
	case "s3":
		bucket, key, ok := parseBucketKey(target.URI)
		if !ok {
			return 0, false
		}
		out, err := exec.Command("aws", "s3api", "head-object", "--bucket", bucket, "--key", key, "--query", "ContentLength", "--output", "text").Output()
		if err != nil {
			return 0, false
		}
		n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		return n, err == nil
	case "gs":
		out, err := exec.Command("gsutil", "stat", target.URI).Output()
		if err != nil {
			return 0, false
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			lower := strings.ToLower(line)
			if strings.HasPrefix(lower, "content-length:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					return 0, false
				}
				value := strings.TrimSpace(parts[1])
				n, err := strconv.ParseInt(value, 10, 64)
				return n, err == nil
			}
		}
	}
	return 0, false
}

func copyObject(target Target, localPath string) error {
	var cmd *exec.Cmd
	switch target.Scheme {
	case "s3":
		cmd = exec.Command("aws", "s3", "cp", target.URI, localPath)
	case "gs":
		if _, err := exec.LookPath("gcloud"); err == nil {
			cmd = exec.Command("gcloud", "storage", "cp", target.URI, localPath)
		} else {
			cmd = exec.Command("gsutil", "cp", target.URI, localPath)
		}
	case "az":
		return copyAzureObject(target.URI, localPath)
	case "rclone":
		remote := strings.TrimPrefix(target.URI, "rclone://")
		cmd = exec.Command("rclone", "copyto", remote, localPath)
	default:
		return fmt.Errorf("unsupported object scheme %q", target.Scheme)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s copy failed: %w: %s", target.Scheme, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func copyAzureObject(rawURI string, localPath string) error {
	u, err := url.Parse(rawURI)
	if err != nil {
		return err
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if u.Host == "" || len(parts) < 2 {
		return fmt.Errorf("az URI must be az://account/container/blob")
	}
	account := u.Host
	container := parts[0]
	blob := path.Join(parts[1:]...)
	cmd := exec.Command(
		"az", "storage", "blob", "download",
		"--account-name", account,
		"--container-name", container,
		"--name", blob,
		"--file", localPath,
		"--auth-mode", "login",
		"--only-show-errors",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("az copy failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func parseBucketKey(rawURI string) (bucket string, key string, ok bool) {
	u, err := url.Parse(rawURI)
	if err != nil || u.Host == "" {
		return "", "", false
	}
	key = strings.TrimPrefix(u.Path, "/")
	if key == "" {
		return "", "", false
	}
	return u.Host, key, true
}
