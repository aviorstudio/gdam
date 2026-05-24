package project

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/fsutil"
)

func ReplaceAutoloadAddonDir(projectGodotPath, fromAddonDirName, toAddonDirName string) (bool, error) {
	fromAddonDirName = strings.TrimSpace(fromAddonDirName)
	toAddonDirName = strings.TrimSpace(toAddonDirName)
	if fromAddonDirName == "" {
		return false, fmt.Errorf("empty fromAddonDirName")
	}
	if toAddonDirName == "" {
		return false, fmt.Errorf("empty toAddonDirName")
	}
	if fromAddonDirName == toAddonDirName {
		return false, nil
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

	updated, changed, err := replaceAutoloadAddonDirText(string(in), fromAddonDirName, toAddonDirName)
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

func replaceAutoloadAddonDirText(input, fromAddonDirName, toAddonDirName string) (string, bool, error) {
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

	updatedLines, changed, err := replaceAutoloadAddonDirLines(lines, fromAddonDirName, toAddonDirName)
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

func replaceAutoloadAddonDirLines(lines []string, fromAddonDirName, toAddonDirName string) ([]string, bool, error) {
	sectionStart, sectionEnd := findSection(lines, "autoload")
	if sectionStart == -1 {
		return lines, false, nil
	}

	oldPrefix := "res://addons/" + fromAddonDirName + "/"
	newPrefix := "res://addons/" + toAddonDirName + "/"

	out := append([]string{}, lines...)
	changed := false
	for i := sectionStart + 1; i < sectionEnd; i++ {
		key, value, ok := splitKeyValue(out[i])
		if !ok {
			continue
		}

		raw, ok, err := parseGodotStringLiteral(value)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			continue
		}
		if !strings.Contains(raw, oldPrefix) {
			continue
		}

		updated := strings.ReplaceAll(raw, oldPrefix, newPrefix)
		out[i] = key + "=" + strconv.Quote(updated)
		changed = true
	}
	return out, changed, nil
}

func parseGodotStringLiteral(value string) (string, bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false, nil
	}
	if !strings.HasPrefix(trimmed, "\"") {
		return "", false, nil
	}
	v, err := strconv.Unquote(trimmed)
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}
