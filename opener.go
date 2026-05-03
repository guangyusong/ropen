package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

var codeExtensions = map[string]bool{
	".bash": true,
	".c":    true,
	".cc":   true,
	".conf": true,
	".cpp":  true,
	".css":  true,
	".go":   true,
	".h":    true,
	".html": true,
	".ini":  true,
	".java": true,
	".js":   true,
	".json": true,
	".jsx":  true,
	".log":  true,
	".lua":  true,
	".md":   true,
	".php":  true,
	".py":   true,
	".rb":   true,
	".rs":   true,
	".sh":   true,
	".toml": true,
	".ts":   true,
	".tsx":  true,
	".txt":  true,
	".xml":  true,
	".yaml": true,
	".yml":  true,
	".zsh":  true,
}

func openPath(openCommand string, localPath string, line string, col string) error {
	if line != "" && shouldUseCode(localPath) {
		args := []string{"--reuse-window", "--goto", localPath + ":" + line}
		if col != "" {
			args[len(args)-1] += ":" + col
		}
		if err := exec.Command("code", args...).Run(); err == nil {
			return nil
		}
	}
	if openCommand == "" {
		return fmt.Errorf("open command is empty")
	}
	return exec.Command(openCommand, localPath).Run()
}

func shouldUseCode(localPath string) bool {
	return codeExtensions[filepath.Ext(localPath)]
}
