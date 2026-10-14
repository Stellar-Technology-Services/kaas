package ui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runKaasJSONL runs a short `kaas … --jsonl` synchronously and returns its
// records. It is for quick reads and fast starts (command status, process
// list, detaching a guest command); anything slow goes through JobRunner.
func (s *Server) runKaasJSONL(ctx context.Context, dir string, args ...string) ([]map[string]any, error) {
	exe := s.opts.Executable
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	full := append(append([]string{}, args...), "--jsonl")
	if s.opts.StatePath != "" {
		full = append(full, "--state", s.opts.StatePath)
	}
	cmd := exec.CommandContext(ctx, exe, full...)
	cmd.Env = os.Environ()
	if info, err := os.Stat(dir); dir != "" && err == nil && info.IsDir() {
		cmd.Dir = dir
	} else {
		cmd.Dir, _ = os.UserHomeDir()
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	var records []map[string]any
	sc := bufio.NewScanner(&stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err == nil {
			records = append(records, rec)
		}
	}
	if runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		for _, rec := range records {
			if rec["type"] == "result" && rec["error"] != nil {
				msg = fmt.Sprint(rec["error"])
			}
		}
		if msg == "" {
			msg = runErr.Error()
		}
		return records, fmt.Errorf("kaas %s: %s", strings.Join(args, " "), truncate(lastNonEmptyLine(msg), 300))
	}
	return records, nil
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return s
}

// recordsOfType filters JSONL records by their type field.
func recordsOfType(records []map[string]any, kind string) []map[string]any {
	var out []map[string]any
	for _, rec := range records {
		if rec["type"] == kind {
			out = append(out, rec)
		}
	}
	return out
}

func recString(rec map[string]any, key string) string {
	if v, ok := rec[key]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

func recInt(rec map[string]any, key string) (int, bool) {
	switch v := rec[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	}
	return 0, false
}

func recStrings(rec map[string]any, key string) []string {
	list, ok := rec[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

// shellQuote matches the CLI's single-quote escaping so a prompt typed in the
// browser reaches the harness intact.
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
