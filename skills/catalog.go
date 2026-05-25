package skills

import (
	"embed"
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
)

var ErrNotFound = errors.New("skill not found")

//go:embed *
var content embed.FS

type Bundle struct {
	Name  string `json:"name"`
	Files []File `json:"files"`
}

type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func ListBundles() ([]Bundle, error) {
	entries, err := fs.ReadDir(content, ".")
	if err != nil {
		return nil, err
	}

	bundles := make([]Bundle, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundle, err := readBundle(entry.Name())
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, bundle)
	}

	sort.Slice(bundles, func(i, j int) bool {
		return bundles[i].Name < bundles[j].Name
	})
	return bundles, nil
}

func ReadBundle(name string) (*Bundle, error) {
	bundle, err := readBundle(name)
	if err != nil {
		return nil, err
	}
	return &bundle, nil
}

func readBundle(name string) (Bundle, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return Bundle{}, ErrNotFound
	}

	if _, err := fs.Stat(content, name); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Bundle{}, ErrNotFound
		}
		return Bundle{}, err
	}

	files := make([]File, 0, 8)
	err := fs.WalkDir(content, name, func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !isSkillTextFile(filePath) {
			return nil
		}
		raw, err := content.ReadFile(filePath)
		if err != nil {
			return err
		}
		files = append(files, File{
			Path:    strings.TrimPrefix(filePath, name+"/"),
			Content: string(raw),
		})
		return nil
	})
	if err != nil {
		return Bundle{}, err
	}
	if len(files) == 0 {
		return Bundle{}, ErrNotFound
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Path == "SKILL.md" {
			return true
		}
		if files[j].Path == "SKILL.md" {
			return false
		}
		return files[i].Path < files[j].Path
	})

	return Bundle{
		Name:  name,
		Files: files,
	}, nil
}

func isSkillTextFile(filePath string) bool {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".md", ".markdown", ".yaml", ".yml", ".json", ".txt", ".js", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}
