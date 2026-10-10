package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCleanupCLIReadOnlyAndRejections(t *testing.T) {
	if os.Getenv("ESTRM_CLEANUP_CLI_CHILD") == "1" {
		for index, argument := range os.Args {
			if argument == "--" {
				os.Args = append([]string{os.Args[0]}, os.Args[index+1:]...)
				break
			}
		}
		main()
		return
	}
	for _, scenario := range []struct {
		name    string
		args    []string
		failure bool
	}{
		{"default-read-only", []string{"-scope", "all-share"}, false},
		{"reset-rejected", []string{"-mode", "RESET", "-scope", "all-share"}, true},
		{"removed-flag-rejected", []string{"-confirm-recognition-cache-destroyed"}, true},
		{"apply-unconfirmed", []string{"-scope", "all-share", "-apply"}, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			arguments := append([]string{"-test.run=^TestCleanupCLIReadOnlyAndRejections$", "--"}, scenario.args...)
			command := exec.Command(os.Args[0], arguments...)
			command.Env = append(os.Environ(), "ESTRM_CLEANUP_CLI_CHILD=1")
			output, err := command.CombinedOutput()
			if (err != nil) != scenario.failure {
				t.Fatalf("err=%v output=%s", err, output)
			}
			if strings.Contains(string(output), "DELETE FROM") {
				t.Fatal("unexpected write SQL")
			}
			if !scenario.failure && (!strings.Contains(string(output), "READ ONLY") || strings.Count(string(output), " AS table_name,count(*)") != 10) {
				t.Fatalf("not ten-table dry run: %s", output)
			}
		})
	}
}
