package platform

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"pocketknife/build"
	"pocketknife/registry"
)

// registryEntry is the JSON shape returned by GET /platform/registry.
type registryEntry struct {
	AppID           string  `json:"appId"`
	Emoji           string  `json:"emoji"`
	Color           string  `json:"color"`
	DisplayName     string  `json:"displayName"`
	GridOrder       int     `json:"gridOrder"`
	BuildState      string  `json:"buildState"`
	ManifestVersion *int    `json:"manifestVersion"`
	ActiveBuildID   *string `json:"activeBuildId"`
}

type registryServer struct {
	bst *build.Store
	reg *registry.Registry
}

func (rs *registryServer) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	metas, err := rs.bst.ListAppMeta()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	var entries []registryEntry
	for _, m := range metas {
		e := registryEntry{
			AppID:       m.AppID,
			Emoji:       m.Emoji,
			Color:       m.Color,
			DisplayName: m.DisplayName,
			GridOrder:   m.GridOrder,
			BuildState:  "none",
		}
		// Attempt to find the most recent non-terminal job for this app.
		jobs, _ := rs.bst.ListForApp(m.AppID)
		for _, j := range jobs {
			if j.State != build.StateReady && j.State != build.StateFailed {
				state := string(j.State)
				e.BuildState = state
				e.ActiveBuildID = &j.ID
				v := j.ManifestVersion
				e.ManifestVersion = &v
				break
			}
			if j.State == build.StateReady {
				state := "ready"
				e.BuildState = state
				e.ActiveBuildID = &j.ID
				v := j.ManifestVersion
				e.ManifestVersion = &v
				break
			}
			break
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []registryEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

func (rs *registryServer) handlePatch(w http.ResponseWriter, r *http.Request, appID string) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	existing, err := rs.bst.GetAppMeta(appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "not_found", "app not found")
		return
	}

	var body struct {
		Emoji       *string `json:"emoji"`
		Color       *string `json:"color"`
		DisplayName *string `json:"displayName"`
		GridOrder   *int    `json:"gridOrder"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	updated := *existing
	if body.Emoji != nil {
		if utf8.RuneCountInString(*body.Emoji) != 1 {
			writeError(w, http.StatusBadRequest, "invalid_emoji", "emoji must be exactly one grapheme cluster")
			return
		}
		updated.Emoji = *body.Emoji
	}
	if body.Color != nil {
		updated.Color = *body.Color
	}
	if body.DisplayName != nil {
		updated.DisplayName = *body.DisplayName
	}
	if body.GridOrder != nil {
		updated.GridOrder = *body.GridOrder
	}

	if err := rs.bst.UpsertAppMeta(build.AppMeta{
		AppID:       updated.AppID,
		Emoji:       updated.Emoji,
		Color:       updated.Color,
		DisplayName: updated.DisplayName,
		GridOrder:   updated.GridOrder,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Re-read to get the updated_at.
	result, _ := rs.bst.GetAppMeta(appID)
	if result == nil {
		result = &updated
	}
	writeJSON(w, http.StatusOK, build.AppMeta{
		AppID:       result.AppID,
		Emoji:       result.Emoji,
		Color:       result.Color,
		DisplayName: result.DisplayName,
		GridOrder:   result.GridOrder,
		UpdatedAt:   result.UpdatedAt,
	})
}

func (rs *registryServer) handleReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Order []string `json:"order"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	// Validate all ids exist in app_meta.
	metas, err := rs.bst.ListAppMeta()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	known := make(map[string]bool, len(metas))
	for _, m := range metas {
		known[m.AppID] = true
	}
	for _, id := range body.Order {
		if !known[id] {
			writeError(w, http.StatusBadRequest, "unknown_app_id", "unknown app id: "+id)
			return
		}
	}

	if err := rs.bst.ReorderApps(body.Order); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// routeRegistry dispatches /platform/registry and /platform/registry/{appId}
// and /platform/registry/reorder.
func (rs *registryServer) route(mux *http.ServeMux) {
	mux.HandleFunc("/platform/registry", rs.handleList)
	mux.HandleFunc("/platform/registry/reorder", rs.handleReorder)
	mux.HandleFunc("/platform/registry/", func(w http.ResponseWriter, r *http.Request) {
		// Extract appId from path: /platform/registry/{appId}
		appID := strings.TrimPrefix(r.URL.Path, "/platform/registry/")
		appID = strings.Trim(appID, "/")
		if appID == "" {
			rs.handleList(w, r)
			return
		}
		rs.handlePatch(w, r, appID)
	})
}
