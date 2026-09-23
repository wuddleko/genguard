package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var configNames = []string{"genguard.yaml", "genguard.yml"}

type Group struct {
	Name    string
	Command string
	Outputs []string
	Clean   bool
}

type Config struct {
	Path   string
	Groups []Group
}

func (c Config) Root() string {
	return filepath.Dir(c.Path)
}

func FindConfig(start string) (string, error) {
	here, err := resolveStart(start)
	if err != nil {
		return "", err
	}
	for dir := here; ; dir = filepath.Dir(dir) {
		found, err := configFile(dir)
		if err != nil {
			return "", err
		}
		if found != "" {
			return found, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", nil
}

// configFile returns the only config file in dir. A directory holds one of
// genguard.yaml or genguard.yml. Both files is an error so a walk-up check
// and genguard check --all see the same layout.
func configFile(dir string) (string, error) {
	var found string
	for _, name := range configNames {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		if info.IsDir() {
			continue
		}
		if found != "" {
			return "", bothConfigNamesError(dir)
		}
		found = candidate
	}
	return found, nil
}

func bothConfigNamesError(dir string) error {
	return fmt.Errorf("%s contains both genguard.yaml and genguard.yml; keep one", dir)
}

// rejectBothConfigNames reports a directory that contains both config names.
// A nested config can sort between genguard.yaml and genguard.yml, so the
// check groups by directory instead of comparing adjacent paths.
func rejectBothConfigNames(paths []string) error {
	seen := make(map[string]string, len(paths))
	for _, path := range paths {
		dir := filepath.Dir(path)
		base := filepath.Base(path)
		if prev, ok := seen[dir]; ok && prev != base {
			return bothConfigNamesError(dir)
		}
		seen[dir] = base
	}
	return nil
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return parseConfig(path, data)
}

func parseConfig(path string, data []byte) (Config, error) {
	var parsed any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	parsed = normalizeRoot(parsed)

	raw, ok := parsed.(map[string]any)
	if !ok {
		return Config{}, fmt.Errorf("%s must be a mapping", path)
	}

	groupsRaw, ok := raw["groups"].([]any)
	if !ok || len(groupsRaw) == 0 {
		return Config{}, fmt.Errorf("%s must include a non-empty 'groups' list", path)
	}

	defaultClean, err := optionalBool(raw, "clean", "clean")
	if err != nil {
		return Config{}, err
	}

	groups := make([]Group, 0, len(groupsRaw))
	for index, item := range groupsRaw {
		groupMap, ok := item.(map[string]any)
		if !ok {
			return Config{}, fmt.Errorf("groups[%d] must be a mapping", index)
		}

		name := fmt.Sprintf("groups[%d]", index)
		if value, ok := groupMap["name"].(string); ok && value != "" {
			name = value
		}

		command, ok := groupMap["command"].(string)
		if !ok || strings.TrimSpace(command) == "" {
			return Config{}, fmt.Errorf("groups[%d] requires a non-empty 'command' string", index)
		}

		outputsRaw, ok := groupMap["outputs"].([]any)
		if !ok || len(outputsRaw) == 0 {
			return Config{}, fmt.Errorf("groups[%d] requires a non-empty 'outputs' list", index)
		}

		outputs := make([]string, 0, len(outputsRaw))
		for _, entry := range outputsRaw {
			value := strings.TrimSpace(fmt.Sprint(entry))
			if value != "" {
				outputs = append(outputs, value)
			}
		}
		if len(outputs) == 0 {
			return Config{}, fmt.Errorf("groups[%d] 'outputs' has no usable paths", index)
		}

		clean := defaultClean.value
		groupClean, err := optionalBool(groupMap, "clean", fmt.Sprintf("groups[%d].clean", index))
		if err != nil {
			return Config{}, err
		}
		if groupClean.set {
			clean = groupClean.value
		}

		groups = append(groups, Group{
			Name:    name,
			Command: command,
			Outputs: outputs,
			Clean:   clean,
		})
	}

	return Config{Path: path, Groups: groups}, nil
}

func normalizeRoot(parsed any) any {
	if parsed == nil {
		return map[string]any{}
	}
	if list, ok := parsed.([]any); ok && len(list) == 0 {
		return map[string]any{}
	}
	return parsed
}

func resolveStart(start string) (string, error) {
	if start == "" {
		return os.Getwd()
	}
	return filepath.Abs(start)
}

type boolOpt struct {
	value bool
	set   bool
}

func optionalBool(m map[string]any, key, loc string) (boolOpt, error) {
	value, ok := m[key]
	if !ok || value == nil {
		return boolOpt{}, nil
	}
	b, ok := value.(bool)
	if !ok {
		return boolOpt{}, fmt.Errorf("%s must be a boolean", loc)
	}
	return boolOpt{value: b, set: true}, nil
}
