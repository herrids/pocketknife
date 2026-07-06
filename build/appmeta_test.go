package build

import "testing"

func TestPickDefaultColorIsDeterministic(t *testing.T) {
	if pickDefaultColor("tasks") != pickDefaultColor("tasks") {
		t.Fatal("pickDefaultColor should be stable for the same app id")
	}
}

func TestPickDefaultColorSpreadsAcrossPalette(t *testing.T) {
	appIDs := []string{"espresso_tracker", "gratitude_log", "project_hub", "reading_tracker", "tasks", "workouts", "baby_tracker"}
	seen := make(map[string]bool)
	for _, id := range appIDs {
		seen[pickDefaultColor(id)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected distinct colors across app ids, got only %d distinct value(s): %v", len(seen), seen)
	}
}

func TestEnsureAppMetaAssignsPaletteColorWhenManifestLeavesItBlank(t *testing.T) {
	bst := openTestStore(t)
	if err := bst.EnsureAppMeta("freshapp", "Fresh App", "", ""); err != nil {
		t.Fatalf("ensure app_meta: %v", err)
	}
	meta, err := bst.GetAppMeta("freshapp")
	if err != nil {
		t.Fatalf("get app_meta: %v", err)
	}
	if meta == nil {
		t.Fatal("expected app_meta row to exist")
	}
	if meta.Color == "#E0E0E0" {
		t.Fatal("expected a palette color, got the old flat gray default")
	}
	if meta.Emoji != "📦" {
		t.Fatalf("emoji = %q, want generic default 📦", meta.Emoji)
	}
}

func TestSyncAppMetaFromManifestUpdatesRowStuckAtSeedDefaults(t *testing.T) {
	bst := openTestStore(t)
	if err := bst.EnsureAppMeta("tasks", "Tasks", "", ""); err != nil {
		t.Fatalf("ensure app_meta: %v", err)
	}
	// Simulate a row seeded before the manifest carried these values: force
	// it back to the old universal defaults.
	if err := bst.UpsertAppMeta(AppMeta{AppID: "tasks", Emoji: "📦", Color: "#E0E0E0", DisplayName: "Tasks"}); err != nil {
		t.Fatalf("force defaults: %v", err)
	}

	if err := bst.SyncAppMetaFromManifest("tasks", "✅", ""); err != nil {
		t.Fatalf("sync: %v", err)
	}
	meta, err := bst.GetAppMeta("tasks")
	if err != nil {
		t.Fatalf("get app_meta: %v", err)
	}
	if meta.Emoji != "✅" {
		t.Fatalf("emoji = %q, want manifest emoji ✅", meta.Emoji)
	}
	if meta.Color == "#E0E0E0" {
		t.Fatal("expected color to move off the flat gray default")
	}
}

func TestSyncAppMetaFromManifestLeavesCustomizedRowAlone(t *testing.T) {
	bst := openTestStore(t)
	if err := bst.EnsureAppMeta("tasks", "Tasks", "", ""); err != nil {
		t.Fatalf("ensure app_meta: %v", err)
	}
	// Simulate a real customization (e.g. a future PATCH-based edit) that has
	// already moved this row off the generic seed defaults.
	if err := bst.UpsertAppMeta(AppMeta{AppID: "tasks", Emoji: "🎯", Color: "#123456", DisplayName: "Tasks"}); err != nil {
		t.Fatalf("customize: %v", err)
	}

	if err := bst.SyncAppMetaFromManifest("tasks", "✅", "#654321"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	meta, err := bst.GetAppMeta("tasks")
	if err != nil {
		t.Fatalf("get app_meta: %v", err)
	}
	if meta.Emoji != "🎯" {
		t.Fatalf("emoji = %q, sync should not overwrite a customized value", meta.Emoji)
	}
	if meta.Color != "#123456" {
		t.Fatalf("color = %q, sync should not overwrite a customized value", meta.Color)
	}
}
