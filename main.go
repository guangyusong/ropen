package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type options struct {
	configPath string
	cacheDir   string
	maxSize    string
	host       string
	user       string
	cwd        string
	tty        string
	path       string
	line       string
	col        string
	refresh    bool
	noOpen     bool
	dryRun     bool
	gcDays     int
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ropen: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			fmt.Print(usageText())
			return nil
		case "version", "--version", "-version":
			fmt.Println(versionString())
			return nil
		case "doctor":
			return runDoctor(args[1:])
		case "iterm", "iterm2":
			return runIterm(args[1:])
		}
	}
	return runOpen(args)
}

func runOpen(args []string) error {
	var opts options
	fs := flag.NewFlagSet("ropen", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opts.configPath, "config", "", "config file path")
	fs.StringVar(&opts.cacheDir, "cache-dir", "", "cache directory")
	fs.StringVar(&opts.maxSize, "max-size", "", "maximum remote file size, e.g. 500MB or 2GB")
	fs.StringVar(&opts.host, "host", "", "remote host name or SSH config alias")
	fs.StringVar(&opts.user, "user", "", "remote user")
	fs.StringVar(&opts.cwd, "cwd", "", "remote current directory for relative paths")
	fs.StringVar(&opts.tty, "tty", "", "terminal tty; used to infer ssh host for iTerm2 integrations")
	fs.StringVar(&opts.path, "path", "", "path to open; useful for terminal integrations")
	fs.StringVar(&opts.line, "line", "", "optional line number")
	fs.StringVar(&opts.col, "col", "", "optional column number")
	fs.BoolVar(&opts.refresh, "refresh", false, "ignore cached copy and fetch again")
	fs.BoolVar(&opts.noOpen, "no-open", false, "print the local path without opening it")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "print what would happen without copying or opening")
	fs.IntVar(&opts.gcDays, "gc", 0, "delete cached files older than N days and exit")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usageText())
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig(opts.configPath)
	if err != nil {
		return err
	}
	if opts.cacheDir != "" {
		cfg.CacheDir = opts.cacheDir
	}
	if opts.maxSize != "" {
		maxBytes, err := parseSize(opts.maxSize)
		if err != nil {
			return err
		}
		cfg.MaxBytes = maxBytes
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir, err = defaultCacheDir()
		if err != nil {
			return err
		}
	}
	if cfg.OpenCommand == "" {
		cfg.OpenCommand = defaultOpenCommand()
	}

	if opts.gcDays > 0 {
		removed, bytes, err := pruneCache(cfg.CacheDir, time.Duration(opts.gcDays)*24*time.Hour)
		if err != nil {
			return err
		}
		fmt.Printf("removed %d cached files (%s)\n", removed, formatBytes(bytes))
		return nil
	}

	target, err := parseTarget(targetInput{
		args: fs.Args(),
		host: opts.host,
		user: opts.user,
		cwd:  opts.cwd,
		tty:  opts.tty,
		path: opts.path,
		line: opts.line,
		col:  opts.col,
	})
	if err != nil {
		if errors.Is(err, errNoTarget) {
			fs.Usage()
		}
		return err
	}

	localPath, err := resolveLocalPath(target, cfg, opts.refresh, opts.dryRun)
	if err != nil {
		return err
	}

	if opts.dryRun || opts.noOpen {
		fmt.Println(localPath)
		return nil
	}
	return openPath(cfg.OpenCommand, localPath, target.Line, target.Col)
}

func usageText() string {
	return `Usage:
  ropen [options] user@host:/absolute/path
  ropen [options] host:/absolute/path
  ropen [options] s3://bucket/key
	ropen [options] gs://bucket/key
	ropen [options] --host vm --cwd /work --path output.mp4
  ropen doctor
  ropen iterm install
  ropen version

Examples:
	ropen vm1:/home/user/output.mp4
  ropen --host vm1 --user ubuntu --path /var/log/app/error.log
	ropen --host "\h" --user "\u" --cwd "\d" --path "\1"

Commands:
  doctor         Check local dependencies and iTerm2 setup
  iterm install Install iTerm2 Smart Selection rules
  version        Print version information

Options:
`
}

func defaultOpenCommand() string {
	if runtime.GOOS == "darwin" {
		return "open"
	}
	return "xdg-open"
}

func defaultCacheDir() (string, error) {
	if runtime.GOOS == "darwin" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "ropen"), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ropen"), nil
}
