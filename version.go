package main

import "fmt"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func versionString() string {
	return fmt.Sprintf("ropen %s (%s, %s)", version, commit, date)
}
