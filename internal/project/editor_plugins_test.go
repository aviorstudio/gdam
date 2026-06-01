package project

import (
	"strings"
	"testing"
)

func TestUpdateEditorPluginsText_AddsSectionWhenMissing_Godot4(t *testing.T) {
	in := "config_version=5\n\n[application]\nconfig/name=\"Test\"\n"
	out, changed, err := updateEditorPluginsText(in, "res://addons/@user_plugin/plugin.cfg", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if out == in {
		t.Fatalf("expected output to differ")
	}
	if want := "[editor_plugins]\nenabled=PackedStringArray(\"res://addons/@user_plugin/plugin.cfg\")\n"; !strings.Contains(out, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, out)
	}
}

func TestUpdateEditorPluginsText_AddsSectionWhenMissing_Godot3(t *testing.T) {
	in := "config_version=4\n\n[application]\nconfig/name=\"Test\"\n"
	out, changed, err := updateEditorPluginsText(in, "res://addons/@user_plugin/plugin.cfg", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if want := "[editor_plugins]\nenabled=PoolStringArray(\"res://addons/@user_plugin/plugin.cfg\")\n"; !strings.Contains(out, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, out)
	}
}

func TestUpdateEditorPluginsText_UpdatesExistingEnabledLine(t *testing.T) {
	in := "config_version=5\n\n[editor_plugins]\nenabled=PackedStringArray(\"res://addons/a/plugin.cfg\")\n"
	out, changed, err := updateEditorPluginsText(in, "res://addons/b/plugin.cfg", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if want := "enabled=PackedStringArray(\"res://addons/a/plugin.cfg\", \"res://addons/b/plugin.cfg\")\n"; !strings.Contains(out, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, out)
	}
}

func TestUpdateEditorPluginsText_NoChangeWhenAlreadyEnabled(t *testing.T) {
	in := "config_version=5\n\n[editor_plugins]\nenabled=PackedStringArray(\"res://addons/a/plugin.cfg\")\n"
	out, changed, err := updateEditorPluginsText(in, "res://addons/a/plugin.cfg", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if changed {
		t.Fatalf("expected changed=false")
	}
	if out != in {
		t.Fatalf("expected output unchanged")
	}
}

func TestUpdateEditorPluginsText_RemovesFromEnabledLine(t *testing.T) {
	in := "config_version=5\n\n[editor_plugins]\nenabled=PackedStringArray(\"res://addons/a/plugin.cfg\", \"res://addons/b/plugin.cfg\")\n"
	out, changed, err := updateEditorPluginsText(in, "res://addons/a/plugin.cfg", false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if want := "enabled=PackedStringArray(\"res://addons/b/plugin.cfg\")\n"; !strings.Contains(out, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, out)
	}
}

func TestUpdateEditorPluginsText_NoChangeWhenNotEnabledAndDisabling(t *testing.T) {
	in := "config_version=5\n\n[editor_plugins]\nenabled=PackedStringArray(\"res://addons/a/plugin.cfg\")\n"
	out, changed, err := updateEditorPluginsText(in, "res://addons/missing/plugin.cfg", false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if changed {
		t.Fatalf("expected changed=false")
	}
	if out != in {
		t.Fatalf("expected output unchanged")
	}
}
