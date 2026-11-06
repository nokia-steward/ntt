package titan

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Project is a thin, ntt-friendly view of a parsed Titan Project
// Descriptor. We deliberately expose only the fields ntt needs (name,
// roots, files, references) so the downstream project package isn't
// coupled to the full TPD schema in titan_gen.go.
type Project struct {
	// Path is the absolute path to the .tpd file that produced this
	// project.
	Path string
	// Name is the project name from <ProjectName>. Falls back to the
	// file's basename if the descriptor omits it.
	Name string
	// Root is the directory containing the .tpd file - all relative
	// paths inside the descriptor resolve against it.
	Root string
	// Sources lists every TTCN-3 / ASN.1 source file referenced from
	// the descriptor, with relative paths resolved against Root.
	Sources []string
	// ImportDirs lists folders that should be treated as import
	// directories (Titan's "centralStorage" folders behave like
	// ntt's imports).
	ImportDirs []string
	// References lists referenced sub-projects (other .tpd files).
	// They are returned with their absolute path so the caller can
	// recursively load them.
	References []Reference
}

// Reference points at a sibling .tpd loaded transitively via the
// <ReferencedProjects> section.
type Reference struct {
	Name string
	Path string
}

// Load parses the descriptor at path and returns a ready-to-use
// Project view. The raw XML is also accessible via Raw for callers
// that want full fidelity.
func Load(path string) (*Project, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(abs, f)
}

// Parse reads a TPD document from r. path is used solely to anchor
// relative paths and to produce useful error messages, so callers that
// already have the bytes in memory can pass any meaningful identifier.
func Parse(path string, r io.Reader) (*Project, error) {
	var top TopLevelProjectType
	dec := xml.NewDecoder(r)
	// Titan ships TPDs that occasionally use the wrong namespace
	// declaration - we accept them all because Eclipse Titan itself
	// is just as forgiving.
	dec.Strict = false
	if err := dec.Decode(&top); err != nil {
		return nil, fmt.Errorf("titan: parse %s: %w", path, err)
	}
	if top.ProjectType == nil {
		return nil, fmt.Errorf("titan: %s: missing <ProjectType>", path)
	}

	root := filepath.Dir(path)
	p := &Project{
		Path: path,
		Name: top.ProjectName,
		Root: root,
	}
	if p.Name == "" {
		// Fallback so downstream code always has something to log
		// or render in error messages.
		base := filepath.Base(path)
		p.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	// Files block - direct TTCN-3 / ASN.1 sources.
	if top.Files != nil {
		for _, f := range top.Files.FileResource {
			if f == nil {
				continue
			}
			src := resolveRelative(root, f.ProjectRelativePathAttr, f.RelativeURIAttr, f.RawURIAttr)
			if src == "" {
				continue
			}
			p.Sources = append(p.Sources, src)
		}
	}

	// Folders block - directories of sources. We surface them via
	// ImportDirs so the project package can scan them recursively.
	if top.Folders != nil {
		for _, f := range top.Folders.FolderResource {
			if f == nil {
				continue
			}
			dir := resolveRelative(root, f.ProjectRelativePathAttr, f.RelativeURIAttr, f.RawURIAttr)
			if dir == "" {
				continue
			}
			p.ImportDirs = append(p.ImportDirs, dir)
		}
	}

	// Referenced projects - we only expose the *paths*, not their
	// contents. Recursive loading is the caller's job.
	if top.ReferencedProjects != nil {
		for _, ref := range top.ReferencedProjects.ReferencedProject {
			if ref == nil {
				continue
			}
			path := refPath(root, ref)
			if path == "" {
				continue
			}
			p.References = append(p.References, Reference{
				Name: ref.NameAttr,
				Path: path,
			})
		}
	}

	return p, nil
}

// resolveRelative picks the first non-empty path attribute from a
// FileResource / FolderResource and resolves it against the descriptor
// root. Titan stores both URI-style and plain paths in the XML; we
// honour them all.
func resolveRelative(root string, candidates ...string) string {
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		c = strings.TrimPrefix(c, "file:")
		c = strings.TrimPrefix(c, "//")
		if filepath.IsAbs(c) {
			return filepath.Clean(c)
		}
		return filepath.Clean(filepath.Join(root, c))
	}
	return ""
}

func refPath(root string, ref *ReferencedProject) string {
	uri := strings.TrimSpace(ref.ProjectLocationURIAttr)
	if uri == "" {
		return ""
	}
	uri = strings.TrimPrefix(uri, "file:")
	uri = strings.TrimPrefix(uri, "//")
	if filepath.IsAbs(uri) {
		return filepath.Clean(uri)
	}
	return filepath.Clean(filepath.Join(root, uri))
}
