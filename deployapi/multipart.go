package deployapi

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// request is one parsed POST /deploy body: the agent's idempotency key, the
// raw manifest bytes, and an open handle to the bundle part. Bundle must be
// closed by the caller once extraction is done; form holds whatever
// multipart spilled to temp files, also released by the caller.
type request struct {
	JobID    string
	Manifest []byte
	Bundle   multipart.File

	form *multipart.Form
}

func (req *request) Close() {
	if req.Bundle != nil {
		req.Bundle.Close()
	}
	if req.form != nil {
		req.form.RemoveAll()
	}
}

// parseRequest reads a multipart/form-data POST /deploy body: a "jobId"
// field, a "manifest" part, and a "bundle" part. Any missing part is a
// client error; nothing is written to disk here.
func parseRequest(r *http.Request) (*request, error) {
	if err := r.ParseMultipartForm(maxManifestBytes); err != nil {
		return nil, fmt.Errorf("parse multipart form: %w", err)
	}
	req := &request{form: r.MultipartForm}

	req.JobID = r.FormValue("jobId")
	if req.JobID == "" {
		req.Close()
		return nil, fmt.Errorf("missing required field %q", "jobId")
	}

	manifestFile, _, err := r.FormFile("manifest")
	if err != nil {
		req.Close()
		return nil, fmt.Errorf("missing required part %q: %w", "manifest", err)
	}
	defer manifestFile.Close()
	manifestBytes, err := io.ReadAll(io.LimitReader(manifestFile, maxManifestBytes+1))
	if err != nil {
		req.Close()
		return nil, fmt.Errorf("read manifest part: %w", err)
	}
	if int64(len(manifestBytes)) > maxManifestBytes {
		req.Close()
		return nil, fmt.Errorf("manifest part exceeds %d bytes", maxManifestBytes)
	}
	req.Manifest = manifestBytes

	bundleFile, _, err := r.FormFile("bundle")
	if err != nil {
		req.Close()
		return nil, fmt.Errorf("missing required part %q: %w", "bundle", err)
	}
	req.Bundle = bundleFile

	return req, nil
}
