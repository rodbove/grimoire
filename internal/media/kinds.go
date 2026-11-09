package media

import (
	"path/filepath"
	"strings"
)

const (
	KindPhoto     = "photo"
	KindVideo     = "video"
	KindLivePhoto = "live_photo"
	KindLiveVideo = "live_video"
	KindDocument  = "document"
)

var photoTypes = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".jpe": "image/jpeg",
	".heic": "image/heic", ".heif": "image/heif", ".hif": "image/heif",
	".png": "image/png", ".webp": "image/webp", ".gif": "image/gif",
	".tif": "image/tiff", ".tiff": "image/tiff", ".bmp": "image/bmp", ".avif": "image/avif",
	".dng": "image/x-adobe-dng", ".cr2": "image/x-canon-cr2", ".cr3": "image/x-canon-cr3",
	".nef": "image/x-nikon-nef", ".arw": "image/x-sony-arw", ".orf": "image/x-olympus-orf",
	".rw2": "image/x-panasonic-rw2", ".raf": "image/x-fuji-raf",
}

var videoTypes = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime", ".3gp": "video/3gpp",
	".avi": "video/x-msvideo", ".mkv": "video/x-matroska", ".webm": "video/webm",
	".mts": "video/mp2t", ".m2ts": "video/mp2t", ".wmv": "video/x-ms-wmv", ".mpg": "video/mpeg", ".mpeg": "video/mpeg",
}

var docTypes = map[string]string{
	".pdf": "application/pdf", ".txt": "text/plain", ".md": "text/markdown", ".csv": "text/csv", ".rtf": "application/rtf",
	".doc": "application/msword", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls": "application/vnd.ms-excel", ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt": "application/vnd.ms-powerpoint", ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".odt": "application/vnd.oasis.opendocument.text", ".ods": "application/vnd.oasis.opendocument.spreadsheet",
	".odp": "application/vnd.oasis.opendocument.presentation",
}

func Classify(name string) (kind, mime string, ok bool) {
	ext := strings.ToLower(filepath.Ext(name))
	if m, ok := photoTypes[ext]; ok {
		return KindPhoto, m, true
	}
	if m, ok := videoTypes[ext]; ok {
		return KindVideo, m, true
	}
	if m, ok := docTypes[ext]; ok {
		return KindDocument, m, true
	}
	return "", "", false
}

func IsRaw(mime string) bool {
	return strings.HasPrefix(mime, "image/x-")
}

func BrowserImage(mime string) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/webp", "image/gif", "image/avif", "image/bmp":
		return true
	}
	return false
}

func BrowserVideo(codec string) bool {
	switch codec {
	case "h264", "vp8", "vp9", "av1":
		return true
	}
	return false
}
