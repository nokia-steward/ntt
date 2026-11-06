package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile writes a file inside dir/sub creating directories as needed.
func writeFile(t *testing.T, dir, sub, body string) string {
	t.Helper()
	path := filepath.Join(dir, sub)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const minimalTPD = `<?xml version="1.0"?>
<TITAN_Project_File_Information version="1.0">
  <ProjectName>Toplevel</ProjectName>
  <ReferencedProjects>
    <ReferencedProject name="Common" projectLocationURI="common/common.tpd"/>
  </ReferencedProjects>
  <Folders>
    <FolderResource projectRelativePath="lib" relativeURI="lib"/>
  </Folders>
  <Files>
    <FileResource projectRelativePath="src/main.ttcn3" relativeURI="src/main.ttcn3"/>
  </Files>
</TITAN_Project_File_Information>
`

const commonTPD = `<?xml version="1.0"?>
<TITAN_Project_File_Information version="1.0">
  <ProjectName>Common</ProjectName>
  <Files>
    <FileResource projectRelativePath="util.ttcn3" relativeURI="util.ttcn3"/>
  </Files>
</TITAN_Project_File_Information>
`

func TestWithTPD_LoadsRecursive(t *testing.T) {
	dir := t.TempDir()
	tpd := writeFile(t, dir, "top.tpd", minimalTPD)
	writeFile(t, dir, "common/common.tpd", commonTPD)
	writeFile(t, dir, "src/main.ttcn3", "// noop")
	writeFile(t, dir, "common/util.ttcn3", "// noop")
	writeFile(t, dir, "lib/x.ttcn3", "// noop")

	cfg, err := NewConfig(WithTPD(tpd))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	if cfg.Root != dir {
		t.Errorf("Root = %q, want %q", cfg.Root, dir)
	}
	wantSrc := []string{
		filepath.Join(dir, "src", "main.ttcn3"),
		filepath.Join(dir, "common", "util.ttcn3"),
	}
	if len(cfg.Sources) != len(wantSrc) {
		t.Fatalf("Sources = %v, want %v", cfg.Sources, wantSrc)
	}
	for _, want := range wantSrc {
		found := false
		for _, got := range cfg.Sources {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Sources missing %q (got %v)", want, cfg.Sources)
		}
	}
	if len(cfg.Imports) != 1 || !strings.HasSuffix(filepath.ToSlash(cfg.Imports[0]), "/lib") {
		t.Errorf("Imports = %v, want one entry ending in /lib", cfg.Imports)
	}
}

func TestOpen_PrefersTPDWhenNoManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "myproj.tpd", minimalTPD)
	writeFile(t, dir, "src/main.ttcn3", "// noop")
	writeFile(t, dir, "common/common.tpd", commonTPD)
	writeFile(t, dir, "common/util.ttcn3", "// noop")

	cfg, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if cfg.Root != dir {
		t.Errorf("Root = %q, want %q", cfg.Root, dir)
	}
	if len(cfg.Sources) == 0 {
		t.Fatal("expected non-empty Sources from .tpd discovery")
	}
}

func TestOpen_ExplicitTPDFile(t *testing.T) {
	dir := t.TempDir()
	tpd := writeFile(t, dir, "explicit.tpd", minimalTPD)
	writeFile(t, dir, "src/main.ttcn3", "// noop")
	writeFile(t, dir, "common/common.tpd", commonTPD)
	writeFile(t, dir, "common/util.ttcn3", "// noop")

	cfg, err := Open(tpd)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(cfg.Sources) == 0 {
		t.Fatal("expected Sources populated from .tpd")
	}
}

func TestDedupePaths(t *testing.T) {
	in := []string{"/a", "/b", "/a", "/c", "/b"}
	got := dedupePaths(in)
	want := []string{"/a", "/b", "/c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
