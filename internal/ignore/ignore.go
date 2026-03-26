package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Matcher struct {
	patterns []pattern
}

type pattern struct {
	raw    string
	negate bool
	dir    bool
}

func New(workDir string) *Matcher {
	m := &Matcher{}
	m.addDefaults()
	m.loadFile(filepath.Join(workDir, ".gitignore"))
	m.loadFile(filepath.Join(workDir, ".otaignore"))
	return m
}

func (m *Matcher) addDefaults() {
	defaults := []string{
		".git",
		".git/**",
		"node_modules",
		"node_modules/**",
		".DS_Store",
		"*.swp",
		"*.swo",
		"*~",
		".ota",
		".ota/**",
		"__pycache__",
		"__pycache__/**",
		".venv",
		".venv/**",
		"vendor",
		"vendor/**",
	}
	for _, d := range defaults {
		m.patterns = append(m.patterns, parsePattern(d))
	}
}

func (m *Matcher) loadFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m.patterns = append(m.patterns, parsePattern(line))
	}
}

func parsePattern(raw string) pattern {
	p := pattern{raw: raw}
	if strings.HasPrefix(raw, "!") {
		p.negate = true
		p.raw = raw[1:]
	}
	if strings.HasSuffix(p.raw, "/") {
		p.dir = true
		p.raw = strings.TrimSuffix(p.raw, "/")
	}
	return p
}

func (m *Matcher) Match(relPath string, isDir bool) bool {
	matched := false
	for _, p := range m.patterns {
		if p.dir && !isDir {
			name := filepath.Base(relPath)
			if matchGlob(p.raw, name) || matchGlob(p.raw, relPath) {
				if p.negate {
					matched = false
				} else {
					matched = true
				}
			}
			continue
		}
		name := filepath.Base(relPath)
		if matchGlob(p.raw, name) || matchGlob(p.raw, relPath) {
			if p.negate {
				matched = false
			} else {
				matched = true
			}
		}
	}
	return matched
}

func matchGlob(pattern, name string) bool {
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := strings.TrimSuffix(parts[0], "/")
			suffix := strings.TrimPrefix(parts[1], "/")
			if prefix == "" && suffix == "" {
				return true
			}
			if prefix == "" {
				matched, _ := filepath.Match(suffix, filepath.Base(name))
				return matched
			}
			if suffix == "" {
				return strings.HasPrefix(name, prefix+"/") || name == prefix
			}
			if strings.HasPrefix(name, prefix+"/") {
				rest := strings.TrimPrefix(name, prefix+"/")
				matched, _ := filepath.Match(suffix, filepath.Base(rest))
				return matched
			}
		}
	}
	matched, _ := filepath.Match(pattern, name)
	if matched {
		return true
	}
	matched, _ = filepath.Match(pattern, filepath.Base(name))
	return matched
}
