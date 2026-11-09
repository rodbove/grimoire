//go:build !windows && !linux

package platform

import "io/fs"

func IsPlaceholder(fi fs.FileInfo) bool { return false }

func IsHiddenSystem(fi fs.FileInfo) bool { return false }

func Volume(path string) (id, rel string) { return "", "" }

func Locate(id string) string { return "" }
