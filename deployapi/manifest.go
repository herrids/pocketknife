package deployapi

import (
	"encoding/json"
	"fmt"
)

// ensureFrontendPointer injects a default frontend block
// ({"dist":"dist","entry":"index.html"}) into a manifest that omits one,
// since the agent's manifest only ever describes the data schema -- the
// deploy-time bundle location is this endpoint's concern, not the agent's. A
// manifest that already declares a frontend block is returned unchanged.
func ensureFrontendPointer(manifestBytes []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(manifestBytes, &raw); err != nil {
		return nil, fmt.Errorf("manifest is not a JSON object: %w", err)
	}
	if existing, ok := raw["frontend"]; ok && len(existing) > 0 && string(existing) != "null" {
		return manifestBytes, nil
	}

	fe, err := json.Marshal(map[string]string{"dist": "dist", "entry": "index.html"})
	if err != nil {
		return nil, err
	}
	raw["frontend"] = fe

	out, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("re-marshal manifest: %w", err)
	}
	return out, nil
}
