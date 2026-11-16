// Package goldens holds real pi session files (format 3) for tests.
// Each file is a copy of a session pi wrote, kept byte for byte.
package goldens

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

//go:embed testdata/*.jsonl
var files embed.FS

// File is a golden session file.
type File struct {
	// Name is the file name without the .jsonl extension, e.g. "tags".
	Name string
	Data []byte
}

// All returns every golden file, sorted by name.
func All() []File {
	paths, err := fs.Glob(files, "testdata/*.jsonl")
	if err != nil {
		panic(err) // the pattern is valid
	}
	out := make([]File, 0, len(paths))
	for _, p := range paths {
		data, err := files.ReadFile(p)
		if err != nil {
			panic(err) // embedded files are always readable
		}
		out = append(out, File{Name: strings.TrimSuffix(path.Base(p), ".jsonl"), Data: data})
	}
	return out
}
