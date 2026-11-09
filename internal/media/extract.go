package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Meta struct {
	TakenAt     int64
	TakenSource string
	Make        string
	Model       string
	Lat         *float64
	Lon         *float64
	Width       int
	Height      int
	DurationMs  int64
	Codec       string
	ContentID   string
	Motion      bool
}

var photoTags = []string{
	"SubSecDateTimeOriginal", "DateTimeOriginal", "OffsetTimeOriginal", "CreateDate", "Make", "Model",
	"GPSLatitude", "GPSLongitude", "ImageWidth", "ImageHeight", "Orientation",
	"ContentIdentifier", "MotionPhoto", "MicroVideo", "EmbeddedVideoType",
}

func (t *Tools) Extract(path, kind, mime string) Meta {
	switch kind {
	case KindVideo, KindLiveVideo:
		return t.probeVideo(path)
	case KindPhoto, KindLivePhoto:
		m, ok := t.exifPhoto(path)
		if !ok {
			m = t.vipsHeader(path)
		}
		if !m.Motion && mime == "image/jpeg" {
			m.Motion = MotionOffset(path) > 0
		}
		return m
	}
	return Meta{}
}

var dateLayouts = []string{
	"2006:01:02 15:04:05.999999999-07:00",
	"2006:01:02 15:04:05.999999999Z07:00",
	"2006:01:02 15:04:05-07:00",
	"2006:01:02 15:04:05Z07:00",
	"2006:01:02 15:04:05.999999999",
	"2006:01:02 15:04:05",
	time.RFC3339Nano,
}

func ParseDate(s string, loc *time.Location) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "0000") {
		return 0, false
	}
	for _, l := range dateLayouts {
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			if t.Year() < 1900 {
				return 0, false
			}
			return t.UnixMilli(), true
		}
	}
	return 0, false
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

func (t *Tools) exifPhoto(path string) (Meta, bool) {
	out, err := t.exif(path, photoTags)
	if err != nil {
		return Meta{}, false
	}
	var rows []map[string]any
	if json.Unmarshal(out, &rows) != nil || len(rows) == 0 {
		return Meta{}, false
	}
	r := rows[0]
	var m Meta
	offset := str(r["OffsetTimeOriginal"])
	for _, k := range []string{"SubSecDateTimeOriginal", "DateTimeOriginal", "CreateDate"} {
		s := str(r[k])
		if s == "" {
			continue
		}
		if offset != "" && !strings.ContainsAny(s[min(len(s), 19):], "+-Z") {
			s += offset
		}
		if ms, ok := ParseDate(s, time.Local); ok {
			m.TakenAt, m.TakenSource = ms, "exif"
			break
		}
	}
	m.Make = strings.TrimSpace(str(r["Make"]))
	m.Model = strings.TrimSpace(str(r["Model"]))
	if lat, ok := num(r["GPSLatitude"]); ok {
		if lon, ok := num(r["GPSLongitude"]); ok && (lat != 0 || lon != 0) {
			m.Lat, m.Lon = &lat, &lon
		}
	}
	if w, ok := num(r["ImageWidth"]); ok {
		m.Width = int(w)
	}
	if h, ok := num(r["ImageHeight"]); ok {
		m.Height = int(h)
	}
	if o, ok := num(r["Orientation"]); ok && o >= 5 && o <= 8 {
		m.Width, m.Height = m.Height, m.Width
	}
	m.ContentID = str(r["ContentIdentifier"])
	if v, ok := num(r["MotionPhoto"]); ok && v == 1 {
		m.Motion = true
	}
	if v, ok := num(r["MicroVideo"]); ok && v == 1 {
		m.Motion = true
	}
	if str(r["EmbeddedVideoType"]) != "" {
		m.Motion = true
	}
	return m, true
}

var rationalRe = regexp.MustCompile(`-?\d+(?:\.\d+)?(?:/\d+)?`)

func parseCoord(s string) (float64, bool) {
	if i := strings.Index(s, " ("); i >= 0 {
		s = s[:i]
	}
	parts := rationalRe.FindAllString(s, 3)
	if len(parts) == 0 {
		return 0, false
	}
	var vals []float64
	for _, p := range parts {
		if a, b, ok := strings.Cut(p, "/"); ok {
			n, _ := strconv.ParseFloat(a, 64)
			d, _ := strconv.ParseFloat(b, 64)
			if d == 0 {
				return 0, false
			}
			vals = append(vals, n/d)
		} else {
			v, _ := strconv.ParseFloat(p, 64)
			vals = append(vals, v)
		}
	}
	v := vals[0]
	if len(vals) > 1 {
		v += vals[1] / 60
	}
	if len(vals) > 2 {
		v += vals[2] / 3600
	}
	return v, true
}

func (t *Tools) vipsHeader(path string) Meta {
	var m Meta
	out, err := run(context.Background(), 30*time.Second, t.VipsHeader, "-a", path)
	if err != nil {
		return m
	}
	fields := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ": ")
		if ok {
			fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	val := func(k string) string {
		v := fields[k]
		if i := strings.Index(v, " ("); i >= 0 {
			v = v[:i]
		}
		return strings.TrimSpace(v)
	}
	m.Width, _ = strconv.Atoi(val("width"))
	m.Height, _ = strconv.Atoi(val("height"))
	if o, _ := strconv.Atoi(val("orientation")); o >= 5 && o <= 8 {
		m.Width, m.Height = m.Height, m.Width
	}
	offset := val("exif-ifd2-OffsetTimeOriginal")
	for _, k := range []string{"exif-ifd2-DateTimeOriginal", "exif-ifd0-DateTime"} {
		if s := val(k); s != "" {
			if ms, ok := ParseDate(s+offset, time.Local); ok {
				m.TakenAt, m.TakenSource = ms, "exif"
				break
			}
			if ms, ok := ParseDate(s, time.Local); ok {
				m.TakenAt, m.TakenSource = ms, "exif"
				break
			}
		}
	}
	m.Make = val("exif-ifd0-Make")
	m.Model = val("exif-ifd0-Model")
	lat, okLat := parseCoord(fields["exif-ifd3-GPSLatitude"])
	lon, okLon := parseCoord(fields["exif-ifd3-GPSLongitude"])
	if okLat && okLon && (lat != 0 || lon != 0) {
		if strings.HasPrefix(val("exif-ifd3-GPSLatitudeRef"), "S") {
			lat = -lat
		}
		if strings.HasPrefix(val("exif-ifd3-GPSLongitudeRef"), "W") {
			lon = -lon
		}
		m.Lat, m.Lon = &lat, &lon
	}
	return m
}

type probe struct {
	Streams []struct {
		CodecType string            `json:"codec_type"`
		CodecName string            `json:"codec_name"`
		Width     int               `json:"width"`
		Height    int               `json:"height"`
		Tags      map[string]string `json:"tags"`
		SideData  []struct {
			Rotation float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

var iso6709 = regexp.MustCompile(`^([+-]\d+(?:\.\d+)?)([+-]\d+(?:\.\d+)?)`)

func (t *Tools) probeVideo(path string) Meta {
	var m Meta
	out, err := run(context.Background(), 60*time.Second, t.FFprobe, "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
	if err != nil {
		return m
	}
	var p probe
	if json.Unmarshal(out, &p) != nil {
		return m
	}
	if d, err := strconv.ParseFloat(p.Format.Duration, 64); err == nil {
		m.DurationMs = int64(d * 1000)
	}
	tags := map[string]string{}
	for k, v := range p.Format.Tags {
		tags[strings.ToLower(k)] = v
	}
	for _, s := range p.Streams {
		if s.CodecType != "video" || m.Codec != "" {
			continue
		}
		m.Codec, m.Width, m.Height = s.CodecName, s.Width, s.Height
		rot := 0.0
		for _, sd := range s.SideData {
			if sd.Rotation != 0 {
				rot = sd.Rotation
			}
		}
		if r, ok := s.Tags["rotate"]; ok {
			rot, _ = strconv.ParseFloat(r, 64)
		}
		if int(rot)%180 != 0 {
			m.Width, m.Height = m.Height, m.Width
		}
		if m.ContentID == "" {
			m.ContentID = s.Tags["com.apple.quicktime.content.identifier"]
		}
	}
	if s := tags["com.apple.quicktime.creationdate"]; s != "" {
		if ms, ok := ParseDate(s, time.Local); ok {
			m.TakenAt, m.TakenSource = ms, "quicktime"
		} else if t, err := time.Parse("2006-01-02T15:04:05-0700", s); err == nil {
			m.TakenAt, m.TakenSource = t.UnixMilli(), "quicktime"
		}
	}
	if m.TakenAt == 0 {
		if s := tags["creation_time"]; s != "" && !strings.HasPrefix(s, "1970") && !strings.HasPrefix(s, "1904") {
			if ms, ok := ParseDate(s, time.UTC); ok {
				m.TakenAt, m.TakenSource = ms, "quicktime"
			}
		}
	}
	m.Make = tags["com.apple.quicktime.make"]
	m.Model = tags["com.apple.quicktime.model"]
	if m.ContentID == "" {
		m.ContentID = tags["com.apple.quicktime.content.identifier"]
	}
	loc := tags["com.apple.quicktime.location.iso6709"]
	if loc == "" {
		loc = tags["location"]
	}
	if mm := iso6709.FindStringSubmatch(loc); mm != nil {
		lat, _ := strconv.ParseFloat(mm[1], 64)
		lon, _ := strconv.ParseFloat(mm[2], 64)
		if lat != 0 || lon != 0 {
			m.Lat, m.Lon = &lat, &lon
		}
	}
	return m
}

var ftyp = []byte("ftyp")

func MotionOffset(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() > 64<<20 {
		return 0
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return 0
	}
	start := 4096
	if len(data) <= start {
		return 0
	}
	i := bytes.LastIndex(data[start:], ftyp)
	if i < 0 {
		return 0
	}
	off := int64(start + i - 4)
	brand := string(data[start+i+4 : min(len(data), start+i+8)])
	switch {
	case strings.HasPrefix(brand, "mp4"), strings.HasPrefix(brand, "iso"), brand == "qt  ", brand == "avc1", brand == "M4V ":
		return off
	}
	return 0
}
