package project

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/markrai/pixelcut/internal/media"
)

func TestImportStaticHDRMetadata(t *testing.T) {
	var report media.Report
	if err := json.Unmarshal([]byte(`{"streams":[{"codec_type":"video","codec_name":"hevc","profile":"Main 10","codec_tag_string":"hvc1","pix_fmt":"yuv420p10le","width":1920,"height":1080,"color_primaries":"bt2020","color_transfer":"arib-std-b67","color_space":"bt2020nc","color_range":"tv","chroma_location":"left","sample_aspect_ratio":"1:1","duration":"2.969633","start_pts":990,"time_base":"1/30000","side_data_list":[{"side_data_type":"Mastering display metadata","max_luminance":"10000000/10000","min_luminance":"50/10000"},{"side_data_type":"Content light level metadata","max_content":0,"max_average":0}]}]}`), &report); err != nil {
		t.Fatal(err)
	}
	a, err := FromReport("static", filepath.Join(t.TempDir(), "test_hlg.mp4"), report)
	if err != nil || a.Width != 1920 || a.Height != 1080 || a.Duration != 2969633 || a.HasAudio {
		t.Fatalf("static HDR import: %+v %v", a, err)
	}
	report.Streams[0].SideData = append(report.Streams[0].SideData, map[string]any{"side_data_type": "unknown"})
	if _, err := FromReport("static", a.Path, report); err == nil {
		t.Fatal("unknown side data accepted")
	}
}
