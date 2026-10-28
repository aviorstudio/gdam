package lockfile

import "testing"

func TestInstallPaths(t *testing.T) {
	for _, test := range []struct {
		path, name string
		ok         bool
	}{
		{"addons/@o_a", "@o/a", true}, {"addons/@o_parent/.gdam/@o_a", "@o/a", true},
		{"addons/../../outside", "@o/a", false}, {"/addons/@o_a", "@o/a", false}, {"addons/./@o_a", "@o/a", false},
		{"addons\\@o_a", "@o/a", false}, {"addons/@other", "@o/a", false}, {"addons/@parent/other/@o_a", "@o/a", false},
	} {
		if ValidInstallPath(test.path, test.name) != test.ok {
			t.Errorf("path %q", test.path)
		}
	}
}
