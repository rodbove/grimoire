//go:build windows

package platform

import (
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	attrHidden         = 0x2
	attrSystem         = 0x4
	attrOffline        = 0x1000
	attrRecallOnOpen   = 0x40000
	attrRecallOnAccess = 0x400000
)

func attrs(fi fs.FileInfo) uint32 {
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.FileAttributes
	}
	return 0
}

func IsPlaceholder(fi fs.FileInfo) bool {
	return attrs(fi)&(attrOffline|attrRecallOnOpen|attrRecallOnAccess) != 0
}

func IsHiddenSystem(fi fs.FileInfo) bool {
	a := attrs(fi)
	return a&attrHidden != 0 && a&attrSystem != 0
}

func Volume(path string) (id, rel string) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", ""
	}
	buf := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(p, &buf[0], uint32(len(buf))); err != nil {
		return "", ""
	}
	mount := windows.UTF16ToString(buf)
	mp, _ := windows.UTF16PtrFromString(mount)
	name := make([]uint16, 64)
	if err := windows.GetVolumeNameForVolumeMountPoint(mp, &name[0], uint32(len(name))); err != nil {
		return "", ""
	}
	r, err := filepath.Rel(mount, path)
	if err != nil {
		return "", ""
	}
	if r == "." {
		r = ""
	}
	return windows.UTF16ToString(name), filepath.ToSlash(r)
}

func Locate(id string) string {
	vp, err := windows.UTF16PtrFromString(id)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 1024)
	var n uint32
	if err := windows.GetVolumePathNamesForVolumeName(vp, &buf[0], uint32(len(buf)), &n); err != nil {
		return ""
	}
	for _, s := range strings.Split(windows.UTF16ToString(buf[:n]), "\x00") {
		if s != "" {
			return s
		}
	}
	return ""
}
