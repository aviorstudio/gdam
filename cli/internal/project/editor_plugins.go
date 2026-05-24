package project

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/fsutil"
)

var godotStringLiteralRe = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)

func SetEditorPluginEnabled(projectGodotPath, pluginCfgPath string, enabled bool) (bool, error) {
	pluginCfgPath = strings.TrimSpace(pluginCfgPath)
	if pluginCfgPath == "" {
		return false, fmt.Errorf("empty plugin cfg path")
	}
	if !strings.HasPrefix(pluginCfgPath, "res://") {
		return false, fmt.Errorf("plugin cfg path must start with res:// (got %q)", pluginCfgPath)
	}

	info, err := os.Stat(projectGodotPath)
	if err != nil {
		return false, err
	}
	perm := info.Mode().Perm()

	in, err := os.ReadFile(projectGodotPath)
	if err != nil {
		return false, err
	}

	updated, changed, err := updateEditorPluginsText(string(in), pluginCfgPath, enabled)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}

	if err := fsutil.WriteFileAtomic(projectGodotPath, []byte(updated), perm); err != nil {
		return false, err
	}
	return true, nil
}

func updateEditorPluginsText(input, pluginCfgPath string, enable bool) (string, bool, error) {
	lineEnding := "\n"
	normalized := input
	if strings.Contains(normalized, "\r\n") {
		lineEnding = "\r\n"
		normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	}

	hadTrailingNewline := strings.HasSuffix(normalized, "\n")
	normalized = strings.TrimSuffix(normalized, "\n")

	var lines []string
	if normalized != "" {
		lines = strings.Split(normalized, "\n")
	}

	updatedLines, changed, err := updateEditorPluginsLines(lines, pluginCfgPath, enable)
	if err != nil {
		return "", false, err
	}
	if !changed {
		return input, false, nil
	}

	out := strings.Join(updatedLines, "\n")
	if hadTrailingNewline {
		out += "\n"
	}
	if lineEnding == "\r\n" {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out, true, nil
}

func updateEditorPluginsLines(lines []string, pluginCfgPath string, enable bool) ([]string, bool, error) {
	sectionStart, sectionEnd := findSection(lines, "editor_plugins")
	if sectionStart == -1 {
		if !enable {
			return lines, false, nil
		}

		arrayType := defaultStringArrayType(lines)
		enabledLine := "enabled=" + formatStringArray(arrayType, []string{pluginCfgPath})

		out := append([]string{}, lines...)
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, "[editor_plugins]", enabledLine)
		return out, true, nil
	}

	enabledLineIndex := -1
	var existingArrayType string
	var existingValues []string
	for i := sectionStart + 1; i < sectionEnd; i++ {
		key, value, ok := splitKeyValue(lines[i])
		if !ok || key != "enabled" {
			continue
		}
		enabledLineIndex = i
		t, v, err := parseGodotStringArray(value)
		if err != nil {
			return nil, false, err
		}
		existingArrayType = t
		existingValues = v
		break
	}

	if enabledLineIndex == -1 {
		if !enable {
			return lines, false, nil
		}

		arrayType := defaultStringArrayType(lines)
		enabledLine := "enabled=" + formatStringArray(arrayType, []string{pluginCfgPath})

		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:sectionStart+1]...)
		out = append(out, enabledLine)
		out = append(out, lines[sectionStart+1:]...)
		return out, true, nil
	}

	arrayType := existingArrayType
	if arrayType == "" {
		arrayType = defaultStringArrayType(lines)
	}

	updatedValues, changed := updateStringList(existingValues, pluginCfgPath, enable)
	if !changed {
		return lines, false, nil
	}

	out := append([]string{}, lines...)
	out[enabledLineIndex] = "enabled=" + formatStringArray(arrayType, updatedValues)
	return out, true, nil
}

func findSection(lines []string, name string) (start, end int) {
	start = -1
	end = len(lines)
	needle := "[" + name + "]"
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
			continue
		}

		if trimmed == needle {
			start = i
			continue
		}

		if start != -1 {
			end = i
			return start, end
		}
	}
	return start, end
}

func splitKeyValue(line string) (key, value string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", false
	}
	i := strings.Index(trimmed, "=")
	if i == -1 {
		return "", "", false
	}
	key = strings.TrimSpace(trimmed[:i])
	value = strings.TrimSpace(trimmed[i+1:])
	if key == "" {
		return "", "", false
	}
	return key, value, true
}

func defaultStringArrayType(lines []string) string {
	if version, ok := parseConfigVersion(lines); ok && version <= 4 {
		return "PoolStringArray"
	}
	return "PackedStringArray"
}

func parseConfigVersion(lines []string) (int, bool) {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			return 0, false
		}
		key, value, ok := splitKeyValue(trimmed)
		if !ok || key != "config_version" {
			continue
		}
		v, err := strconv.Atoi(value)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

func parseGodotStringArray(value string) (arrayType string, values []string, _ error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil, nil
	}

	inner := trimmed
	if open := strings.Index(trimmed, "("); open != -1 {
		close := strings.LastIndex(trimmed, ")")
		if close > open {
			t := strings.TrimSpace(trimmed[:open])
			if t == "PackedStringArray" || t == "PoolStringArray" {
				arrayType = t
			}
			inner = trimmed[open+1 : close]
		}
	}

	matches := godotStringLiteralRe.FindAllString(inner, -1)
	values = make([]string, 0, len(matches))
	for _, match := range matches {
		v, err := strconv.Unquote(match)
		if err != nil {
			values = append(values, strings.Trim(match, `"`))
			continue
		}
		values = append(values, v)
	}
	return arrayType, values, nil
}

func formatStringArray(arrayType string, values []string) string {
	if arrayType != "PackedStringArray" && arrayType != "PoolStringArray" {
		arrayType = "PackedStringArray"
	}

	if len(values) == 0 {
		return arrayType + "()"
	}

	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(v)
	}
	return arrayType + "(" + strings.Join(quoted, ", ") + ")"
}

func updateStringList(values []string, target string, enable bool) ([]string, bool) {
	if enable {
		for _, v := range values {
			if v == target {
				return values, false
			}
		}
		return append(values, target), true
	}

	out := values[:0]
	changed := false
	for _, v := range values {
		if v == target {
			changed = true
			continue
		}
		out = append(out, v)
	}
	return out, changed
}
