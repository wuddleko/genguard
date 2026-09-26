package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

var configNames = []string{"genguard.yaml", "genguard.yml"}

func ConfigNames() []string {
	names := make([]string, len(configNames))
	copy(names, configNames)
	return names
}

type Tool struct {
	Name    string
	Version string
	Command string
}

type Group struct {
	Name    string
	Command string
	Outputs []string
	Inputs  []string
	Clean   bool
	Tools   []string
}

type Config struct {
	Path   string
	Tools  []Tool
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

func RejectBothConfigNames(paths []string) error {
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

	if err := rejectUnknownKeys(raw, []string{"groups", "clean", "tools"}, path); err != nil {
		return Config{}, err
	}

	tools, err := parseTools(raw, path)
	if err != nil {
		return Config{}, err
	}
	declared := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		declared[tool.Name] = struct{}{}
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
		if err := rejectUnknownKeys(groupMap, []string{"name", "command", "outputs", "inputs", "clean", "tools"}, loc); err != nil {
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

		toolRefs, err := parseGroupTools(groupMap, loc, declared)
		if err != nil {
			return Config{}, err
		}

		groups = append(groups, Group{
			Name:    name,
			Command: command,
			Outputs: outputs,
			Inputs:  inputs,
			Clean:   clean,
			Tools:   toolRefs,
		})
	}

	return Config{Path: path, Tools: tools, Groups: groups}, nil
}

func parseTools(raw map[string]any, loc string) ([]Tool, error) {
	value, ok := raw["tools"]
	if !ok {
		return nil, nil
	}
	entries, err := toolEntries(value, loc)
	if err != nil {
		return nil, err
	}
	tools := make([]Tool, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for i, entry := range entries {
		item, ok := entry.(map[string]any)
		entryLoc := fmt.Sprintf("tools[%d]", i)
		if !ok {
			return nil, fmt.Errorf("%s must be a mapping", entryLoc)
		}
		if err := rejectUnknownKeys(item, []string{"name", "version", "command"}, entryLoc); err != nil {
			return nil, err
		}
		name, err := toolName(item, entryLoc)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("%s: duplicate name %q", entryLoc, name)
		}
		seen[name] = struct{}{}
		version, err := optionalText(item, "version", entryLoc)
		if err != nil {
			return nil, err
		}
		command, err := optionalCommand(item, entryLoc)
		if err != nil {
			return nil, err
		}
		tools = append(tools, Tool{Name: name, Version: version, Command: command})
	}
	return tools, nil
}

func parseGroupTools(groupMap map[string]any, loc string, declared map[string]struct{}) ([]string, error) {
	value, ok := groupMap["tools"]
	if !ok {
		return nil, nil
	}
	entries, err := toolEntries(value, loc)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for i, entry := range entries {
		refLoc := fmt.Sprintf("%s.tools[%d]", loc, i)
		text, ok := entry.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be a string", refLoc)
		}
		name, err := refToken(text, refLoc)
		if err != nil {
			return nil, err
		}
		if _, ok := declared[name]; !ok {
			return nil, fmt.Errorf("%s.tools: unknown name %q", loc, name)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("%s.tools: duplicate name %q", loc, name)
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

func toolEntries(value any, loc string) ([]any, error) {
	if value == nil {
		return nil, fmt.Errorf("%s requires a non-empty 'tools' list", loc)
	}
	entries, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s 'tools' must be a list", loc)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s requires a non-empty 'tools' list", loc)
	}
	return entries, nil
}

func toolName(m map[string]any, loc string) (string, error) {
	raw, ok := m["name"]
	if !ok || raw == nil {
		return "", fmt.Errorf("%s requires a non-empty 'name' string", loc)
	}
	text, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s.name must be a string", loc)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("%s requires a non-empty 'name' string", loc)
	}
	if strings.ContainsFunc(text, unicode.IsSpace) {
		return "", fmt.Errorf("%s.name must be a single token", loc)
	}
	return text, nil
}

func refToken(text, loc string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("%s requires a non-empty name", loc)
	}
	if strings.ContainsFunc(text, unicode.IsSpace) {
		return "", fmt.Errorf("%s must be a single token", loc)
	}
	return text, nil
}

func optionalText(m map[string]any, key, loc string) (string, error) {
	value, ok := m[key]
	if !ok {
		return "", nil
	}
	text, ok := value.(string)
	if !ok || value == nil {
		return "", fmt.Errorf("%s.%s must be a string", loc, key)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("%s requires a non-empty '%s' string", loc, key)
	}
	return text, nil
}

func optionalCommand(m map[string]any, loc string) (string, error) {
	value, ok := m["command"]
	if !ok {
		return "", nil
	}
	text, ok := value.(string)
	if !ok || value == nil {
		return "", fmt.Errorf("%s.command must be a string", loc)
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s requires a non-empty 'command' string", loc)
	}
	return text, nil
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
