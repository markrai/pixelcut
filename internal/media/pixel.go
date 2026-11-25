package media

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func (v Stream) DisplayRotation() (int, error) {
	if tag := v.Tags["rotate"]; tag != "" && tag != "0" {
		return 0, errors.New("legacy rotation tag is not supported without a verified display matrix")
	}
	if len(v.SideData) == 0 {
		return 0, nil
	}
	if len(v.SideData) != 1 || v.SideData[0]["side_data_type"] != "Display Matrix" {
		return 0, errors.New("unsupported video side data")
	}
	d := v.SideData[0]
	matrix, ok := d["displaymatrix"].(string)
	angle, angleOK := d["rotation"].(float64)
	if !ok || !angleOK {
		return 0, errors.New("incomplete display matrix")
	}
	var values [9]int64
	rows := strings.Split(strings.TrimSpace(matrix), "\n")
	if len(rows) != 3 {
		return 0, errors.New("invalid display matrix")
	}
	for i, row := range rows {
		fields := strings.Fields(row)
		if len(fields) != 4 || fields[0] != fmt.Sprintf("%08x:", i) {
			return 0, errors.New("invalid display matrix row")
		}
		for j := 0; j < 3; j++ {
			n, err := strconv.ParseInt(fields[j+1], 10, 32)
			if err != nil {
				return 0, errors.New("invalid display matrix coefficient")
			}
			values[i*3+j] = n
		}
	}
	const one = 65536
	const scale = 1073741824
	var expected [9]int64
	switch angle {
	case 0:
		expected = [9]int64{one, 0, 0, 0, one, 0, 0, 0, scale}
	case -90:
		expected = [9]int64{0, one, 0, -one, 0, 0, 0, 0, scale}
	case 90:
		expected = [9]int64{0, -one, 0, one, 0, 0, 0, 0, scale}
	case 180, -180:
		expected = [9]int64{-one, 0, 0, 0, -one, 0, 0, 0, scale}
		angle = 180
	default:
		return 0, errors.New("only cardinal display rotations are supported")
	}
	if values != expected {
		return 0, errors.New("unsupported display matrix transform (expected pure rotation)")
	}
	return int(angle), nil
}

func (v Stream) DisplayGeometry() (Stream, error) {
	rotation, err := v.DisplayRotation()
	if err != nil {
		return v, err
	}
	if rotation == 90 || rotation == -90 {
		v.Width, v.Height = v.Height, v.Width
	}
	return v, nil
}

func (s Stream) pixelMetadata() bool {
	return s.CodecType == "data" && s.CodecTag == "mett" && s.Tags["handler_name"] == "MetaHandle" && s.CodecName == ""
}

func (r Report) validateExportStreams() error {
	for _, s := range r.Streams {
		if s.CodecType != "video" && s.CodecType != "audio" && !s.pixelMetadata() {
			return fmt.Errorf("unsupported stream %d (%s/%s); refusing to discard it", s.Index, s.CodecType, s.CodecTag)
		}
	}
	return nil
}

func (r Report) reportExcluded() {
	for _, s := range r.Streams {
		if s.pixelMetadata() {
			fmt.Fprintf(os.Stderr, "pixelcut: excluded timed metadata stream %d (MetaHandle/mett); source unchanged\n", s.Index)
		}
	}
}

func (r Report) verifyExportStreams(source Report, copyAudio bool) error {
	if len(r.Streams) != 1+source.audioCount() || r.audioCount() != source.audioCount() {
		return errors.New("unexpected output stream count")
	}
	if copyAudio {
		var want, got []string
		for _, s := range source.Streams {
			if s.CodecType == "audio" {
				want = append(want, s.CodecName)
			}
		}
		for _, s := range r.Streams {
			if s.CodecType == "audio" {
				got = append(got, s.CodecName)
			}
		}
		for i := range want {
			if want[i] != got[i] {
				return errors.New("output changed audio codec")
			}
		}
	}
	if tags, ok := r.Format["tags"].(map[string]any); ok {
		for k := range tags {
			if strings.Contains(strings.ToLower(k), "location") || strings.Contains(strings.ToLower(k), "gps") {
				return errors.New("output contains unexpected location metadata")
			}
		}
	}
	return nil
}

func findMPV(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	if name == "mpv" && runtime.GOOS == "windows" {
		for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramW6432"), os.Getenv("ProgramFiles(x86)"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs")} {
			if !filepath.IsAbs(root) {
				continue
			}
			for _, dir := range []string{"MPV Player", "mpv"} {
				p := filepath.Join(root, dir, "mpv.exe")
				if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
					return p, nil
				}
			}
		}
	}
	return "", fmt.Errorf("mpv executable %q not found; install mpv or add its directory to PATH", name)
}
