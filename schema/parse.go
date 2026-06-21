package schema

import (
	"encoding/json"
	"fmt"
)

// raw* mirror the manifest JSON shape. They are intentionally permissive: the
// structural (JSON Schema) layer is what rejects malformed manifests. Parse
// turns a structurally-valid document into the typed model and normalises
// defaults that the manifest left implicit.

type rawManifest struct {
	App      rawApp      `json:"app"`
	Entities []rawEntity `json:"entities"`
}

type rawApp struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Emoji   string `json:"emoji"`
	Version int    `json:"version"`
}

type rawEntity struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Operations []string   `json:"operations"`
	Fields     []rawField `json:"fields"`
}

type rawField struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Required bool            `json:"required"`
	Unique   bool            `json:"unique"`
	Default  json.RawMessage `json:"default"`
	Min      *float64        `json:"min"`
	Max      *float64        `json:"max"`
	Values   []string        `json:"values"`
	Target   string          `json:"target"`
	OnDelete string          `json:"onDelete"`
}

// Parse converts manifest bytes into the typed model. It assumes the bytes have
// already passed structural validation; it returns an error only on
// genuinely-unparseable JSON or a default that cannot be coerced to the field's
// type (the latter is also reported, with a path, by the semantic validator).
func Parse(data []byte) (*App, error) {
	var raw rawManifest
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("manifest is not valid JSON: %w", err)
	}

	app := &App{
		ID:      raw.App.ID,
		Name:    raw.App.Name,
		Emoji:   raw.App.Emoji,
		Version: raw.App.Version,
	}

	for _, re := range raw.Entities {
		ent := &Entity{
			ID:   re.ID,
			Name: re.Name,
		}
		if len(re.Operations) == 0 {
			ent.Operations = append([]Operation(nil), AllOperations...)
		} else {
			for _, op := range re.Operations {
				ent.Operations = append(ent.Operations, Operation(op))
			}
		}

		for _, rf := range re.Fields {
			f := &Field{
				ID:       rf.ID,
				Name:     rf.Name,
				Type:     FieldType(rf.Type),
				Required: rf.Required,
				Unique:   rf.Unique,
				Min:      rf.Min,
				Max:      rf.Max,
				Values:   rf.Values,
				Target:   rf.Target,
			}
			if f.Type == TypeReference {
				f.OnDelete = rf.OnDelete
				if f.OnDelete == "" {
					f.OnDelete = OnDeleteSetNull
				}
			}
			if len(rf.Default) > 0 && string(rf.Default) != "null" {
				v, err := normaliseDefault(f.Type, rf.Default)
				if err != nil {
					return nil, fmt.Errorf("entity %q field %q: %w", re.Name, rf.Name, err)
				}
				f.HasDefault = true
				f.Default = v
			}
			ent.Fields = append(ent.Fields, f)
		}
		app.Entities = append(app.Entities, ent)
	}
	return app, nil
}

// normaliseDefault decodes a raw default into the canonical Go type for the
// field, so the rest of the system never has to re-interpret JSON numbers.
func normaliseDefault(t FieldType, raw json.RawMessage) (any, error) {
	switch t {
	case TypeText, TypeDatetime, TypeEnum:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("default must be a string")
		}
		return s, nil
	case TypeInteger:
		var n int64
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, fmt.Errorf("default must be an integer")
		}
		return n, nil
	case TypeReal:
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, fmt.Errorf("default must be a number")
		}
		return n, nil
	case TypeBoolean:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("default must be a boolean")
		}
		return b, nil
	default:
		return nil, fmt.Errorf("type %q does not support a default", t)
	}
}
