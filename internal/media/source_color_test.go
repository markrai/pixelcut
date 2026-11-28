package media

import "testing"

func taggedSDR() Stream {
	return Stream{CodecType: "video", CodecName: "h264", PixelFormat: "yuv420p", Width: 1920, Height: 1080, Primaries: "bt709", Transfer: "bt709", Matrix: "bt709", Range: "tv", ChromaLocation: "left", SampleAspectRatio: "1:1"}
}

func TestClassifySourceColor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		edit     func(*Stream)
		want     ColorFamily
		defaults ColorDefaults
	}{
		{"tagged H264", func(v *Stream) {}, ColorSDR709, 0},
		{"tagged HEVC 10 bit", func(v *Stream) { v.CodecName, v.PixelFormat = "hevc", "yuv420p10le" }, ColorSDR709, 0},
		{"untagged HD screen capture", func(v *Stream) {
			v.Primaries, v.Transfer, v.Matrix, v.Range, v.ChromaLocation, v.SampleAspectRatio = "", "unknown", "unspecified", "", "", "0:1"
		}, ColorSDR709, allColorDefaults},
		{"partly tagged full range", func(v *Stream) { v.Transfer, v.Range = "", "pc" }, ColorSDR709, DefaultTransfer},
		{"tagged small SDR", func(v *Stream) { v.Width, v.Height = 640, 480 }, ColorSDR709, 0},
		{"missing SD tags", func(v *Stream) { v.Width, v.Height, v.Matrix = 640, 480, "" }, "", 0},
		{"missing 10 bit tags", func(v *Stream) { v.PixelFormat, v.Transfer = "yuv420p10le", "" }, "", 0},
		{"contradictory BT2020", func(v *Stream) { v.Primaries = "bt2020" }, "", 0},
		{"unknown transfer not missing", func(v *Stream) { v.Transfer = "gamma22" }, "", 0},
		{"non-square", func(v *Stream) { v.SampleAspectRatio = "4:3" }, "", 0},
		{"odd", func(v *Stream) { v.Width = 1919 }, "", 0},
		{"unknown codec", func(v *Stream) { v.CodecName = "vp9" }, "", 0},
		{"unknown side data", func(v *Stream) { v.SideData = []map[string]any{{"side_data_type": "unknown"}} }, "", 0},
		{"legacy rotation", func(v *Stream) { v.Tags = map[string]string{"rotate": "90"} }, "", 0},
		{"HDR metadata on SDR", func(v *Stream) { v.SideData = []map[string]any{{"side_data_type": "Mastering display metadata"}} }, "", 0},
		{"HLG", func(v *Stream) {
			v.CodecName, v.PixelFormat, v.Primaries, v.Transfer, v.Matrix = "hevc", "yuv420p10le", "bt2020", "arib-std-b67", "bt2020nc"
		}, ColorHLG, 0},
		{"PQ", func(v *Stream) {
			v.CodecName, v.PixelFormat, v.Primaries, v.Transfer, v.Matrix = "hevc", "yuv420p10le", "bt2020", "smpte2084", "bt2020nc"
		}, ColorPQ, 0},
		{"incomplete PQ", func(v *Stream) {
			v.CodecName, v.PixelFormat, v.Primaries, v.Transfer, v.Matrix, v.Range = "hevc", "yuv420p10le", "bt2020", "smpte2084", "bt2020nc", ""
		}, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := taggedSDR()
			tc.edit(&v)
			got, err := v.ClassifySourceColor()
			if tc.want == "" {
				if err == nil {
					t.Fatalf("ambiguous source accepted: %+v", got)
				}
				return
			}
			if err != nil || got.Family != tc.want || got.Defaults != tc.defaults || got.Validate() != nil {
				t.Fatalf("classification: %+v %v", got, err)
			}
			if tc.want != ColorHLG && v.ValidateHLG() == nil {
				t.Fatal("HLG renderer contract relaxed")
			}
		})
	}
}

func TestSourceColorDescriptorRejectsForgedDefaults(t *testing.T) {
	v := SourceColor{Family: ColorSDR709, Codec: "h264", PixelFormat: "yuv420p", Range: "tv", Chroma: "left", Defaults: DefaultRange}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	v.Range = "pc"
	if v.Validate() == nil {
		t.Fatal("inferred full range accepted")
	}
	v.Range, v.Defaults = "tv", 128
	if v.Validate() == nil {
		t.Fatal("unknown inference flag accepted")
	}
}
