package config

import (
	"strings"
	"testing"
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
		}
	})
}
