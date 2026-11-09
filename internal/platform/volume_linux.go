//go:build linux

package platform

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func IsPlaceholder(fi fs.FileInfo) bool { return false }

func IsHiddenSystem(fi fs.FileInfo) bool { return false }

type mount struct {
	point  string
	source string
}

func unescape(s string) string {
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(s)
}

func mounts() []mount {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []mount
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		left := strings.Fields(parts[0])
		right := strings.Fields(parts[1])
		if len(left) < 5 || len(right) < 2 {
			continue
		}
		src := right[1]
		if real, err := filepath.EvalSymlinks(src); err == nil {
			src = real
		}
		out = append(out, mount{point: unescape(left[4]), source: src})
	}
	return out
}

func uuids() map[string]string {
	out := map[string]string{}
	entries, err := os.ReadDir("/dev/disk/by-uuid")
	if err != nil {
		return out
	}
	for _, e := range entries {
		real, err := filepath.EvalSymlinks(filepath.Join("/dev/disk/by-uuid", e.Name()))
		if err == nil {
			out[real] = e.Name()
		}
	}
	return out
}

func Volume(path string) (id, rel string) {
	var best mount
	for _, m := range mounts() {
		if Contains(m.point, path) && len(m.point) >= len(best.point) {
			best = m
		}
	}
	if best.point == "" {
		return "", ""
	}
	uuid, ok := uuids()[best.source]
	if !ok {
		return "", ""
	}
	r, err := filepath.Rel(best.point, path)
	if err != nil {
		return "", ""
	}
	if r == "." {
		r = ""
	}
	return "uuid:" + uuid, filepath.ToSlash(r)
}

func Locate(id string) string {
	uuid, ok := strings.CutPrefix(id, "uuid:")
	if !ok {
		return ""
	}
	var dev string
	for real, u := range uuids() {
		if u == uuid {
			dev = real
		}
	}
	if dev == "" {
		return ""
	}
	for _, m := range mounts() {
		if m.source == dev {
			return m.point
		}
	}
	return ""
}
