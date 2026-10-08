package titan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleTPD = `<?xml version="1.0" encoding="UTF-8"?>
<TITAN_Project_File_Information version="1.0">
  <ProjectName>HelloTitan</ProjectName>
  <ReferencedProjects>
    <ReferencedProject name="Common" projectLocationURI="../common/common.tpd"/>
  </ReferencedProjects>
  <Folders>
    <FolderResource projectRelativePath="lib" relativeURI="lib"/>
  </Folders>
  <Files>
    <FileResource projectRelativePath="src/main.ttcn3" relativeURI="src/main.ttcn3"/>
    <FileResource projectRelativePath="src/util.ttcn3" relativeURI="src/util.ttcn3"/>
  </Files>
</TITAN_Project_File_Information>
`

func writeTPD(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.tpd")
	if err := os.WriteFile(path, []byte(sampleTPD), 0o644); err != nil {
		t.Fatalf("write tpd: %v", err)
	}
	return path
}

func TestLoad_PopulatesSourcesAndReferences(t *testing.T) {
	path := writeTPD(t)
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if p.Name != "HelloTitan" {
		t.Errorf("Name = %q, want HelloTitan", p.Name)
	}
	if p.Root != filepath.Dir(path) {
		t.Errorf("Root = %q, want %q", p.Root, filepath.Dir(path))
	}
	if len(p.Sources) != 2 {
		t.Fatalf("Sources = %v, want 2 entries", p.Sources)
	}
	if !strings.HasSuffix(filepath.ToSlash(p.Sources[0]), "src/main.ttcn3") {
		t.Errorf("Sources[0] = %q, want suffix src/main.ttcn3", p.Sources[0])
	}
	if len(p.ImportDirs) != 1 || !strings.HasSuffix(filepath.ToSlash(p.ImportDirs[0]), "/lib") {
		t.Errorf("ImportDirs = %v, want one entry ending in /lib", p.ImportDirs)
	}
	if len(p.References) != 1 || p.References[0].Name != "Common" {
		t.Fatalf("References = %v, want one entry named Common", p.References)
	}
	if !strings.HasSuffix(filepath.ToSlash(p.References[0].Path), "/common/common.tpd") {
		t.Errorf("Reference path = %q, want suffix /common/common.tpd", p.References[0].Path)
	}
}

func TestLoad_FallbackName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Unnamed.tpd")
	body := `<?xml version="1.0"?><TITAN_Project_File_Information version="1.0"><Files/></TITAN_Project_File_Information>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.Name != "Unnamed" {
		t.Errorf("Name = %q, want Unnamed (derived from file basename)", p.Name)
	}
}

func TestLoad_AbsoluteSourcePathPreserved(t *testing.T) {
	dir := t.TempDir()
	absSrc := filepath.Join(dir, "external.ttcn3")
	if err := os.WriteFile(absSrc, []byte("// noop"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "abs.tpd")
	body := `<?xml version="1.0"?><TITAN_Project_File_Information version="1.0">
		<ProjectName>Abs</ProjectName>
		<Files>
			<FileResource projectRelativePath="` + absSrc + `" relativeURI="` + absSrc + `"/>
		</Files>
	</TITAN_Project_File_Information>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(p.Sources) != 1 || p.Sources[0] != absSrc {
		t.Errorf("Sources = %v, want [%q]", p.Sources, absSrc)
	}
}

func TestLoad_BadXMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.tpd")
	if err := os.WriteFile(path, []byte("not xml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for malformed XML")
	}
}
