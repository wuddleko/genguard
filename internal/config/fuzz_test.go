package config

import (
	"strings"
	"testing"
	"unicode"
)

func FuzzParseConfig(f *testing.F) {
	f.Add([]byte("groups:\n  - name: greeting\n    command: echo hi\n    outputs:\n      - gen/\n"))
	f.Add([]byte(""))
	f.Add([]byte("[]"))
	f.Add([]byte("groups: []\n"))
	f.Add([]byte("groups:\n  - not-a-mapping\n"))
	f.Add([]byte("groups:\n  - name: greeting\n    outputs:\n      - generated/\n"))
	f.Add([]byte("groups:\n  - name: greeting\n    command: echo\n    outputs: []\n"))
	f.Add([]byte("groups:\n  - name: greeting\n    command: echo\n    outputs:\n      - '  '\n"))
	f.Add([]byte("clean: 1\ngroups:\n  - command: echo\n    outputs:\n      - gen/\n"))
	f.Add([]byte("clean: true\ngroups:\n  - command: echo\n    outputs:\n      - gen/\n    clean: false\n"))
	f.Add([]byte("tools:\n  - name: buf\n    version: 1.32.0\n  - name: sqlc\n    command: sqlc version\ngroups:\n  - name: protobuf\n    command: buf generate\n    tools: [buf]\n    outputs:\n      - gen/\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		// Large inputs spend the fuzz budget in the YAML decoder.
		if len(data) > 8<<10 {
			return
		}
		cfg, err := parseConfig("genguard.yaml", data)
		if err != nil {
			return
		}
		if cfg.Path != "genguard.yaml" {
			t.Fatalf("path = %q", cfg.Path)
		}
		if len(cfg.Groups) == 0 {
			t.Fatal("success with no groups")
		}
		declared := make(map[string]struct{}, len(cfg.Tools))
		for i, tool := range cfg.Tools {
			if tool.Name == "" || strings.ContainsFunc(tool.Name, unicode.IsSpace) {
				t.Fatalf("tool %d has name %q", i, tool.Name)
			}
			if _, ok := declared[tool.Name]; ok {
				t.Fatalf("tool %d duplicates %q", i, tool.Name)
			}
			declared[tool.Name] = struct{}{}
			if tool.Version != strings.TrimSpace(tool.Version) {
				t.Fatalf("tool %d has version %q", i, tool.Version)
			}
			if tool.Command != "" && strings.TrimSpace(tool.Command) == "" {
				t.Fatalf("tool %d has a blank command", i)
			}
		}
		for i, group := range cfg.Groups {
			if group.Name == "" {
				t.Fatalf("group %d has an empty name", i)
			}
			if strings.TrimSpace(group.Command) == "" {
				t.Fatalf("group %d has an empty command", i)
			}
			if len(group.Outputs) == 0 {
				t.Fatalf("group %d has no outputs", i)
			}
			for _, output := range group.Outputs {
				if strings.TrimSpace(output) == "" {
					t.Fatalf("group %d has a blank output %q", i, output)
				}
			}
			for _, input := range group.Inputs {
				if strings.TrimSpace(input) == "" {
					t.Fatalf("group %d has a blank input %q", i, input)
				}
			}
			seen := make(map[string]struct{}, len(group.Tools))
			for _, name := range group.Tools {
				if _, ok := declared[name]; !ok {
					t.Fatalf("group %d tool %q is not declared", i, name)
				}
				if _, ok := seen[name]; ok {
					t.Fatalf("group %d repeats tool %q", i, name)
				}
				seen[name] = struct{}{}
			}
		}
	})
}
