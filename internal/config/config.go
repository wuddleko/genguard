package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var configNames = []string{"genguard.yaml", "genguard.yml"}

type Group struct {
	Name    string
	Command string
	Outputs []string
	Inputs  []string
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
	cfg, err := parseConfig(path, data)
	if err != nil {
		return Config{}, err
	}
	cfg.Path, err = filepath.Abs(path)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
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

	if err := rejectUnknownKeys(raw, []string{"groups", "clean"}, path); err != nil {
		return Config{}, err
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
		loc := fmt.Sprintf("groups[%d]", index)
		if err := rejectUnknownKeys(groupMap, []string{"name", "command", "outputs", "inputs", "clean"}, loc); err != nil {
			return Config{}, err
		}

		name := loc
		if rawName, exists := groupMap["name"]; exists && rawName != nil {
			value, ok := rawName.(string)
			if !ok {
				return Config{}, fmt.Errorf("%s.name must be a string", loc)
			}
			if value != "" {
				name = value
			}
		}

		command, ok := groupMap["command"].(string)
		if !ok || strings.TrimSpace(command) == "" {
			return Config{}, fmt.Errorf("groups[%d] requires a non-empty 'command' string", index)
		}

		outputs, err := requiredPaths(groupMap, "outputs", loc)
		if err != nil {
			return Config{}, err
		}
		inputs, err := optionalPaths(groupMap, "inputs", loc)
		if err != nil {
			return Config{}, err
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
			Inputs:  inputs,
			Clean:   clean,
		})
	}

	return Config{Path: path, Groups: groups}, nil
}

func rejectUnknownKeys(m map[string]any, known []string, loc string) error {
	allow := make(map[string]struct{}, len(known))
	for _, key := range known {
		allow[key] = struct{}{}
	}
	var extra []string
	for key := range m {
		if _, ok := allow[key]; !ok {
			extra = append(extra, key)
		}
	}
	if len(extra) == 0 {
		return nil
	}
	sort.Strings(extra)
	return fmt.Errorf("%s: unknown key %q", loc, extra[0])
}

func requiredPaths(m map[string]any, key, loc string) ([]string, error) {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil, fmt.Errorf("%s requires a non-empty '%s' list", loc, key)
	}
	return pathsFrom(raw, key, loc)
}

func optionalPaths(m map[string]any, key, loc string) ([]string, error) {
	raw, ok := m[key]
	if !ok {
		return nil, nil
	}
	if raw == nil {
		return nil, fmt.Errorf("%s requires a non-empty '%s' list", loc, key)
	}
	return pathsFrom(raw, key, loc)
}

func pathsFrom(raw any, key, loc string) ([]string, error) {
	entries, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s '%s' must be a list", loc, key)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s requires a non-empty '%s' list", loc, key)
	}
	paths := make([]string, 0, len(entries))
	for i, entry := range entries {
		value, ok := entry.(string)
		if !ok {
			return nil, fmt.Errorf("%s.%s[%d] must be a string", loc, key, i)
		}
		value = strings.TrimSpace(value)
		if value != "" {
			paths = append(paths, value)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%s '%s' has no usable paths", loc, key)
	}
	return paths, nil
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
