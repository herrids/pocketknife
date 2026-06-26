package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// buildTarGz writes entries (name -> content) into a gzipped tar, in order.
// A nil content with name ending in "/" writes a directory entry; an entry
// whose linkname is non-empty writes a symlink instead of a regular file.
type tarEntry struct {
	name     string
	content  string
	linkname string
}

func buildTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		switch {
		case e.linkname != "":
			if err := tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: tar.TypeSymlink, Linkname: e.linkname, Mode: 0o777}); err != nil {
				t.Fatal(err)
			}
		case len(e.name) > 0 && e.name[len(e.name)-1] == '/':
			if err := tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
				t.Fatal(err)
			}
		default:
			hdr := &tar.Header{Name: e.name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(e.content))}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(e.content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBundleWritesFilesAndDirs(t *testing.T) {
	dest := t.TempDir()
	data := buildTarGz(t, []tarEntry{
		{name: "index.html", content: "<html></html>"},
		{name: "assets/", content: ""},
		{name: "assets/app.js", content: "console.log(1)"},
	})

	if err := ExtractBundle(bytes.NewReader(data), dest); err != nil {
		t.Fatalf("extract: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "index.html"))
	if err != nil || string(got) != "<html></html>" {
		t.Fatalf("index.html = %q, %v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(dest, "assets", "app.js"))
	if err != nil || string(got) != "console.log(1)" {
		t.Fatalf("assets/app.js = %q, %v", got, err)
	}
}

// TestExtractBundleHandlesDotSlashPrefixedEntries guards against the exact
// shape node-tar produces when packing a directory with `tar.create({cwd,
// gzip:true}, ["."])` -- which is what the agent's HttpSubmitter does: every
// entry name is "./"-prefixed (e.g. "./index.html", "./assets/"), not bare.
func TestExtractBundleHandlesDotSlashPrefixedEntries(t *testing.T) {
	dest := t.TempDir()
	data := buildTarGz(t, []tarEntry{
		{name: "./"},
		{name: "./assets/"},
		{name: "./index.html", content: "<html></html>"},
		{name: "./assets/app.js", content: "console.log(1)"},
	})

	if err := ExtractBundle(bytes.NewReader(data), dest); err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "index.html"))
	if err != nil || string(got) != "<html></html>" {
		t.Fatalf("index.html = %q, %v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(dest, "assets", "app.js"))
	if err != nil || string(got) != "console.log(1)" {
		t.Fatalf("assets/app.js = %q, %v", got, err)
	}
}

func TestExtractBundleRejectsParentTraversal(t *testing.T) {
	dest := t.TempDir()
	data := buildTarGz(t, []tarEntry{{name: "../escape.txt", content: "evil"}})

	if err := ExtractBundle(bytes.NewReader(data), dest); err == nil {
		t.Fatal("expected an error for a path-traversal entry")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("traversal entry must not have been written outside dest")
	}
}

func TestExtractBundleRejectsDeepParentTraversal(t *testing.T) {
	dest := t.TempDir()
	data := buildTarGz(t, []tarEntry{{name: "a/b/../../../escape.txt", content: "evil"}})

	if err := ExtractBundle(bytes.NewReader(data), dest); err == nil {
		t.Fatal("expected an error for a deep path-traversal entry")
	}
}

func TestExtractBundleRejectsAbsolutePath(t *testing.T) {
	dest := t.TempDir()
	data := buildTarGz(t, []tarEntry{{name: "/etc/passwd", content: "evil"}})

	if err := ExtractBundle(bytes.NewReader(data), dest); err == nil {
		t.Fatal("expected an error for an absolute path entry")
	}
}

func TestExtractBundleRejectsSymlink(t *testing.T) {
	dest := t.TempDir()
	data := buildTarGz(t, []tarEntry{{name: "link", linkname: "/etc/passwd"}})

	if err := ExtractBundle(bytes.NewReader(data), dest); err == nil {
		t.Fatal("expected an error for a symlink entry")
	}
	if _, err := os.Lstat(filepath.Join(dest, "link")); !os.IsNotExist(err) {
		t.Fatal("symlink entry must not have been created")
	}
}

func TestExtractBundleRejectsTooManyEntries(t *testing.T) {
	dest := t.TempDir()
	var entries []tarEntry
	for i := 0; i <= MaxBundleEntries; i++ {
		entries = append(entries, tarEntry{name: fmt.Sprintf("f/%d.txt", i), content: "x"})
	}
	data := buildTarGz(t, entries)

	if err := ExtractBundle(bytes.NewReader(data), dest); err == nil {
		t.Fatal("expected an error for too many entries")
	}
}

func TestExtractBundleRejectsTooManyBytes(t *testing.T) {
	dest := t.TempDir()
	big := make([]byte, MaxBundleBytes/2+1)
	data := buildTarGz(t, []tarEntry{
		{name: "a.bin", content: string(big)},
		{name: "b.bin", content: string(big)},
	})

	if err := ExtractBundle(bytes.NewReader(data), dest); err == nil {
		t.Fatal("expected an error for exceeding the total byte cap")
	}
}

func TestExtractBundleRejectsNonGzip(t *testing.T) {
	dest := t.TempDir()
	if err := ExtractBundle(bytes.NewReader([]byte("not gzip")), dest); err == nil {
		t.Fatal("expected an error for a non-gzip stream")
	}
}
