// Package api is the one generic, schema-driven HTTP surface that serves every
// app. Nothing here is specific to any app: each handler resolves the app by id
// from the registry and serves it from its registered schema. Routes are
// namespaced by app: /apps/{app_id}/{entity_name}.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"pocketknife/registry"
	"pocketknife/schema"
	"pocketknife/store"
)

// Server wraps the registry and exposes an http.Handler.
type Server struct {
	reg *registry.Registry
}

// NewServer builds the generic HTTP handler over the given registry.
func NewServer(reg *registry.Registry) http.Handler {
	s := &Server{reg: reg}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /apps/{app}/{entity}", s.handleCreate)
	mux.HandleFunc("GET /apps/{app}/{entity}", s.handleList)
	mux.HandleFunc("GET /apps/{app}/{entity}/{id}", s.handleRead)
	mux.HandleFunc("PATCH /apps/{app}/{entity}/{id}", s.handleUpdate)
	mux.HandleFunc("DELETE /apps/{app}/{entity}/{id}", s.handleDelete)
	return mux
}

// resolve looks up the app and entity from the path, writing a 404 and
// returning ok=false if either is unknown.
func (s *Server) resolve(w http.ResponseWriter, r *http.Request) (*registry.RegisteredApp, *schema.Entity, bool) {
	appID := r.PathValue("app")
	entName := r.PathValue("entity")

	ra, ok := s.reg.App(appID)
	if !ok {
		writeError(w, http.StatusNotFound, "app_not_found", "no app with id "+appID)
		return nil, nil, false
	}
	ent := ra.Schema.Entity(entName)
	if ent == nil {
		writeError(w, http.StatusNotFound, "entity_not_found", "no entity "+entName+" in app "+appID)
		return nil, nil, false
	}
	return ra, ent, true
}

// requireOp enforces the entity's operation set, writing 405 if disabled.
func requireOp(w http.ResponseWriter, ent *schema.Entity, op schema.Operation) bool {
	if !ent.Allows(op) {
		writeError(w, http.StatusMethodNotAllowed, "operation_disabled",
			"operation "+string(op)+" is not enabled for entity "+ent.Name)
		return false
	}
	return true
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	ra, ent, ok := s.resolve(w, r)
	if !ok {
		return
	}
	if !requireOp(w, ent, schema.OpCreate) {
		return
	}

	body, ok := decodeBody(w, r)
	if !ok {
		return
	}

	values := map[string]any{}
	var issues []any

	for key := range body {
		if ent.Field(key) == nil {
			issues = append(issues, fieldIssue{Field: key, Message: "unknown field"})
		}
	}

	for _, f := range ent.Fields {
		raw, present := body[f.Name]
		if !present {
			if f.HasDefault {
				values[f.Name] = defaultStoreValue(f)
			} else if f.Required {
				issues = append(issues, fieldIssue{Field: f.Name, Message: "is required"})
			}
			continue
		}
		val, isNull, issue := coerce(ra, f, raw)
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		if isNull {
			if f.Required {
				issues = append(issues, fieldIssue{Field: f.Name, Message: "is required and cannot be null"})
				continue
			}
			values[f.Name] = nil
			continue
		}
		values[f.Name] = val
	}

	if len(issues) > 0 {
		writeError(w, http.StatusBadRequest, "validation_failed", "request body failed validation", issues...)
		return
	}

	now := store.NowUTC()
	values["id"] = store.NewID()
	values["created_at"] = now
	values["updated_at"] = now

	row, err := ra.Store.Insert(ent, values)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	ra, ent, ok := s.resolve(w, r)
	if !ok {
		return
	}
	if !requireOp(w, ent, schema.OpRead) {
		return
	}
	row, err := ra.Store.GetByID(ent, r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if row == nil {
		writeNotFoundRow(w, ent, r.PathValue("id"))
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	ra, ent, ok := s.resolve(w, r)
	if !ok {
		return
	}
	if !requireOp(w, ent, schema.OpRead) {
		return
	}
	q, issue := parseListQuery(ent, r.URL.Query())
	if issue != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", issue.Message, *issue)
		return
	}
	rows, total, err := ra.Store.List(ent, q)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":   rows,
		"total":  total,
		"limit":  q.Limit,
		"offset": q.Offset,
	})
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	ra, ent, ok := s.resolve(w, r)
	if !ok {
		return
	}
	if !requireOp(w, ent, schema.OpUpdate) {
		return
	}
	id := r.PathValue("id")

	existing, err := ra.Store.GetByID(ent, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if existing == nil {
		writeNotFoundRow(w, ent, id)
		return
	}

	body, ok := decodeBody(w, r)
	if !ok {
		return
	}

	values := map[string]any{}
	var issues []any
	for key := range body {
		if ent.Field(key) == nil {
			issues = append(issues, fieldIssue{Field: key, Message: "unknown field"})
		}
	}
	for _, f := range ent.Fields {
		raw, present := body[f.Name]
		if !present {
			continue // partial update: untouched fields are left as-is
		}
		val, isNull, issue := coerce(ra, f, raw)
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		if isNull {
			if f.Required {
				issues = append(issues, fieldIssue{Field: f.Name, Message: "is required and cannot be null"})
				continue
			}
			values[f.Name] = nil
			continue
		}
		values[f.Name] = val
	}
	if len(issues) > 0 {
		writeError(w, http.StatusBadRequest, "validation_failed", "request body failed validation", issues...)
		return
	}

	values["updated_at"] = store.NowUTC()

	row, err := ra.Store.Update(ent, id, values)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if row == nil {
		writeNotFoundRow(w, ent, id)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	ra, ent, ok := s.resolve(w, r)
	if !ok {
		return
	}
	if !requireOp(w, ent, schema.OpDelete) {
		return
	}
	deleted, err := ra.Store.Delete(ent, r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !deleted {
		writeNotFoundRow(w, ent, r.PathValue("id"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeBody reads a JSON object body into raw per-key messages, deferring
// per-field decoding to coerce. A non-object or malformed body is a 400.
func decodeBody(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, bool) {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not read request body")
		return nil, false
	}
	if len(data) == 0 {
		return map[string]json.RawMessage{}, true
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body must be a JSON object")
		return nil, false
	}
	return body, true
}

// defaultStoreValue converts a field's declared default into the storage-ready
// value. Datetime defaults are canonicalised; other types are already in their
// canonical Go form from manifest parsing.
func defaultStoreValue(f *schema.Field) any {
	if f.Type == schema.TypeDatetime {
		if s, ok := f.Default.(string); ok {
			if canon, err := store.CanonicalDatetime(s); err == nil {
				return canon
			}
		}
	}
	return f.Default
}

func writeNotFoundRow(w http.ResponseWriter, ent *schema.Entity, id string) {
	writeError(w, http.StatusNotFound, "row_not_found", "no "+ent.Name+" with id "+id)
}

// writeStoreError maps store sentinel errors to HTTP status codes; anything
// else is a 500.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrUnique):
		writeError(w, http.StatusConflict, "unique_violation", "a row with this value already exists")
	case errors.Is(err, store.ErrForeignKey):
		writeError(w, http.StatusConflict, "reference_conflict", "operation violates a reference constraint")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}
