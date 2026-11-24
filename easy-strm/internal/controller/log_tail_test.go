package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTailLinesFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug_fixture.log")
	var builder strings.Builder
	levels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&builder, "[t][%s][req=r%d][task=-][module=m][trace=-] line %d\n", levels[i%4], i%7, i)
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	all, err := readTailLines(path, 3, logLineFilter{})
	if err != nil || len(all.lines) != 3 || !strings.HasSuffix(all.lines[0], "line 19999") || !strings.HasSuffix(all.lines[2], "line 19997") {
		t.Fatalf("tail order: %v %v", all.lines, err)
	}
	warn, _ := readTailLines(path, 10, newLogLineFilter("warn", ""))
	for _, line := range warn.lines {
		if !strings.Contains(line, "][WARN][") && !strings.Contains(line, "][ERROR][") {
			t.Fatalf("level filter: %s", line)
		}
	}
	if len(warn.lines) != 10 {
		t.Fatalf("level count %d", len(warn.lines))
	}
	query, _ := readTailLines(path, 5000, newLogLineFilter("", "REQ=r3 error"))
	for _, line := range query.lines {
		if !strings.Contains(line, "req=r3") || !strings.Contains(line, "ERROR") {
			t.Fatalf("query filter: %s", line)
		}
	}
	if len(query.lines) == 0 || !strings.HasSuffix(query.lines[len(query.lines)-1], fmt.Sprintf("line %d", firstMatch())) {
		t.Fatalf("query should reach file start: %d", len(query.lines))
	}
	whole, _ := readTailLines(path, 50000, logLineFilter{})
	if len(whole.lines) != 20000 || !strings.HasSuffix(whole.lines[19999], "line 0") {
		t.Fatalf("whole file: %d", len(whole.lines))
	}
}

func firstMatch() int {
	for i := 0; ; i++ {
		if i%4 == 3 && i%7 == 3 {
			return i
		}
	}
}
