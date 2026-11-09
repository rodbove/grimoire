package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func DefaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		if pd := os.Getenv("ProgramData"); pd != "" {
			return filepath.Join(pd, "grimoire")
		}
		return `C:\ProgramData\grimoire`
	default:
		if os.Geteuid() == 0 {
			return "/var/lib/grimoire"
		}
		if x := os.Getenv("XDG_DATA_HOME"); x != "" {
			return filepath.Join(x, "grimoire")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "grimoire")
	}
}

func CaseInsensitive() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

func samePath(a, b string) bool {
	if CaseInsensitive() {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func Contains(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if samePath(parent, child) {
		return true
	}
	sep := string(filepath.Separator)
	p := parent
	if !strings.HasSuffix(p, sep) {
		p += sep
	}
	if len(child) < len(p) {
		return false
	}
	return samePath(child[:len(p)], p)
}

var reserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

func ValidName(name string) bool {
	if name == "" || name == "." || name == ".." || len(name) > 255 {
		return false
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.HasPrefix(name, " ") {
		return false
	}
	for _, r := range name {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return false
		}
	}
	base := strings.ToUpper(name)
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	return !reserved[base]
}

func CleanRel(rel string) (string, bool) {
	rel = strings.ReplaceAll(rel, `\`, "/")
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return "", true
	}
	parts := strings.Split(rel, "/")
	for _, p := range parts {
		if !ValidName(p) || strings.HasPrefix(p, ".") {
			return "", false
		}
	}
	return strings.Join(parts, "/"), true
}

func Join(root, rel string) string {
	if rel == "" {
		return root
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

func Skip(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "$recycle.bin", "system volume information", "thumbs.db", "desktop.ini", "lost+found", "@eadir":
		return true
	}
	return false
}
