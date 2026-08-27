package build

import "hash/fnv"

// defaultTileColors mirrors shell/tailwind.config.ts's "App tile colors" —
// keep the two in sync if either changes. It exists so an app that never
// sets an explicit "color" in its manifest still gets a distinct, stable
// tile color instead of one flat gray shared by every app.
var defaultTileColors = []string{
	"#3E9D93", // teal
	"#8E86CF", // purple
	"#E0A12E", // amber
	"#8FA968", // app-green
	"#5B9BD0", // app-blue
	"#DD8AA6", // app-pink
	"#B5544A", // app-rust
}

// pickDefaultColor deterministically maps an app id to one of
// defaultTileColors, so the same app always gets the same default color
// across boots.
func pickDefaultColor(appID string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(appID))
	return defaultTileColors[h.Sum32()%uint32(len(defaultTileColors))]
}

// defaultTileEmojis gives an app that never sets an explicit "emoji" in its
// manifest a distinct, stable tile emoji instead of one shared 📦 for every
// app. 📦 is deliberately excluded so it remains a safe sentinel for "never
// assigned a default" in existing app_meta rows.
var defaultTileEmojis = []string{
	"🧭", "🎯", "🔷", "🌟", "🧩", "🪁", "🔶",
}

// pickDefaultEmoji deterministically maps an app id to one of
// defaultTileEmojis, so the same app always gets the same default emoji
// across boots.
func pickDefaultEmoji(appID string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(appID))
	return defaultTileEmojis[h.Sum32()%uint32(len(defaultTileEmojis))]
}
