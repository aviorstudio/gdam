package resolve

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fixture map[string]Release

func (f fixture) fetch(_ context.Context, name, tag string) (Release, error) {
	r, ok := f[name+"@"+tag]
	if !ok {
		return Release{}, errors.New("not published")
	}
	return r, nil
}

func rel(name, tag string, deps map[string]string, published time.Time) Release {
	return Release{Name: name, Tag: tag, GitHubReleaseID: 1, AssetID: 2, AssetName: "a.zip", AssetDigest: "sha256:x", CommitSHA: "c", PublishedAt: published, Dependencies: deps}
}

var t0 = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func paths(plan Plan) map[string]string {
	out := map[string]string{}
	for _, c := range plan.Copies {
		out[c.Path] = c.Release.Name + "@" + c.Release.Tag
	}
	return out
}

func TestAgreeingPinsShareOneHoistedCopy(t *testing.T) {
	f := fixture{
		"@o/clerk@v1":   rel("@o/clerk", "v1", map[string]string{"@o/session": "v1"}, t0),
		"@o/pb@v1":      rel("@o/pb", "v1", map[string]string{"@o/session": "v1"}, t0),
		"@o/session@v1": rel("@o/session", "v1", nil, t0),
	}
	plan, err := Resolve(context.Background(), map[string]string{"@o/clerk": "v1", "@o/pb": "v1"}, f.fetch)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"addons/@o_clerk": "@o/clerk@v1", "addons/@o_pb": "@o/pb@v1", "addons/@o_session": "@o/session@v1"}
	got := paths(plan)
	if len(got) != len(want) {
		t.Fatalf("copies %v", got)
	}
	for p, v := range want {
		if got[p] != v {
			t.Fatalf("copies %v", got)
		}
	}
	for _, c := range plan.Copies {
		if c.Release.Name == "@o/session" && strings.Join(c.RequiredBy, ",") != "@o/clerk@v1,@o/pb@v1" {
			t.Fatalf("required_by %v", c.RequiredBy)
		}
		if c.Release.Name == "@o/clerk" && c.Dependencies["@o/session"] != "addons/@o_session" {
			t.Fatalf("clerk deps %v", c.Dependencies)
		}
	}
}

func TestProjectPinWinsTheShelfAndDisagreeingConsumerNests(t *testing.T) {
	f := fixture{
		"@o/clerk@v1":   rel("@o/clerk", "v1", map[string]string{"@o/session": "v1"}, t0),
		"@o/session@v1": rel("@o/session", "v1", nil, t0),
		"@o/session@v2": rel("@o/session", "v2", nil, t0.Add(time.Hour)),
	}
	plan, err := Resolve(context.Background(), map[string]string{"@o/clerk": "v1", "@o/session": "v2"}, f.fetch)
	if err != nil {
		t.Fatal(err)
	}
	got := paths(plan)
	if got["addons/@o_session"] != "@o/session@v2" || got["addons/@o_clerk/.gdam/@o_session"] != "@o/session@v1" || len(got) != 3 {
		t.Fatalf("copies %v", got)
	}
	for _, c := range plan.Copies {
		if c.Release.Name == "@o/clerk" && c.Dependencies["@o/session"] != "addons/@o_clerk/.gdam/@o_session" {
			t.Fatalf("clerk deps %v", c.Dependencies)
		}
		if c.Path == "addons/@o_clerk/.gdam/@o_session" && (!c.Nested || c.RequiredBy[0] != "@o/clerk@v1") {
			t.Fatalf("nested copy %+v", c)
		}
	}
	// Install order: hoisted copies before nested ones.
	if plan.Copies[len(plan.Copies)-1].Path != "addons/@o_clerk/.gdam/@o_session" {
		t.Fatalf("order %v", plan.Copies)
	}
}

func TestWithoutAProjectPinTheMostRequestedTagIsHoisted(t *testing.T) {
	f := fixture{
		"@o/a@v1":       rel("@o/a", "v1", map[string]string{"@o/session": "v1"}, t0),
		"@o/b@v1":       rel("@o/b", "v1", map[string]string{"@o/session": "v2"}, t0),
		"@o/c@v1":       rel("@o/c", "v1", map[string]string{"@o/session": "v2"}, t0),
		"@o/session@v1": rel("@o/session", "v1", nil, t0),
		"@o/session@v2": rel("@o/session", "v2", nil, t0.Add(time.Hour)),
	}
	plan, err := Resolve(context.Background(), map[string]string{"@o/a": "v1", "@o/b": "v1", "@o/c": "v1"}, f.fetch)
	if err != nil {
		t.Fatal(err)
	}
	got := paths(plan)
	if plan.Hoisted["@o/session"] != "v2" || got["addons/@o_a/.gdam/@o_session"] != "@o/session@v1" || len(got) != 5 {
		t.Fatalf("hoisted %v copies %v", plan.Hoisted, got)
	}
}

func TestTiesHoistTheNewerRelease(t *testing.T) {
	f := fixture{
		"@o/a@v1":       rel("@o/a", "v1", map[string]string{"@o/session": "v1"}, t0),
		"@o/b@v1":       rel("@o/b", "v1", map[string]string{"@o/session": "v2"}, t0),
		"@o/session@v1": rel("@o/session", "v1", nil, t0),
		"@o/session@v2": rel("@o/session", "v2", nil, t0.Add(time.Hour)),
	}
	plan, err := Resolve(context.Background(), map[string]string{"@o/a": "v1", "@o/b": "v1"}, f.fetch)
	if err != nil || plan.Hoisted["@o/session"] != "v2" {
		t.Fatalf("%v hoisted %v", err, plan.Hoisted)
	}
}

func TestNestedCopiesResolveTheirOwnDependencies(t *testing.T) {
	// clerk pins session v1; session v1 needs util v1; the project pins
	// session v2 and util v2. clerk gets nested session v1, which in turn
	// gets nested util v1 under itself.
	f := fixture{
		"@o/clerk@v1":   rel("@o/clerk", "v1", map[string]string{"@o/session": "v1"}, t0),
		"@o/session@v1": rel("@o/session", "v1", map[string]string{"@o/util": "v1"}, t0),
		"@o/session@v2": rel("@o/session", "v2", map[string]string{"@o/util": "v2"}, t0),
		"@o/util@v1":    rel("@o/util", "v1", nil, t0),
		"@o/util@v2":    rel("@o/util", "v2", nil, t0),
	}
	plan, err := Resolve(context.Background(), map[string]string{"@o/clerk": "v1", "@o/session": "v2", "@o/util": "v2"}, f.fetch)
	if err != nil {
		t.Fatal(err)
	}
	got := paths(plan)
	if got["addons/@o_clerk/.gdam/@o_session/.gdam/@o_util"] != "@o/util@v1" || len(got) != 5 {
		t.Fatalf("copies %v", got)
	}
}

func TestUnpublishedDependencyNamesTheRequester(t *testing.T) {
	f := fixture{"@o/clerk@v1": rel("@o/clerk", "v1", map[string]string{"@o/session": "v9"}, t0)}
	_, err := Resolve(context.Background(), map[string]string{"@o/clerk": "v1"}, f.fetch)
	if err == nil || !strings.Contains(err.Error(), "@o/session@v9 (required by @o/clerk@v1)") {
		t.Fatalf("err %v", err)
	}
}

func TestLinkedAddonMustAgreeWithTheShelf(t *testing.T) {
	f := fixture{
		"@o/session@v1": rel("@o/session", "v1", nil, t0),
		"@o/session@v2": rel("@o/session", "v2", nil, t0),
	}
	fetch := func(ctx context.Context, name, tag string) (Release, error) {
		if name == "@o/clerk" {
			return Release{Name: name, Tag: tag, Local: true, Dependencies: map[string]string{"@o/session": "v1"}}, nil
		}
		return f.fetch(ctx, name, tag)
	}
	plan, err := Resolve(context.Background(), map[string]string{"@o/clerk": "linked", "@o/session": "v1"}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plan.Copies {
		if c.Release.Local && c.Dependencies["@o/session"] != "addons/@o_session" {
			t.Fatalf("linked deps %v", c.Dependencies)
		}
	}
	_, err = Resolve(context.Background(), map[string]string{"@o/clerk": "linked", "@o/session": "v2"}, fetch)
	if err == nil || !strings.Contains(err.Error(), "linked addon @o/clerk pins @o/session@v1 but the project installs @o/session@v2") {
		t.Fatalf("err %v", err)
	}
}
