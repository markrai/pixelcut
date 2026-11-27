package media

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStaticHDRSideData(t *testing.T) {
	var data []map[string]any
	if err := json.Unmarshal([]byte(`[{"side_data_type":"Mastering display metadata","red_x":"34000/50000","red_y":"16000/50000","green_x":"13248/50000","green_y":"34500/50000","blue_x":"7500/50000","blue_y":"3000/50000","white_point_x":"15634/50000","white_point_y":"16450/50000","min_luminance":"50/10000","max_luminance":"10000000/10000"},{"side_data_type":"Content light level metadata","max_content":0,"max_average":0}]`), &data); err != nil {
		t.Fatal(err)
	}
	matrix := map[string]any{"side_data_type": "Display Matrix", "rotation": float64(-90), "displaymatrix": "00000000: 0 65536 0\n00000001: -65536 0 0\n00000002: 0 0 1073741824"}
	for _, tc := range []struct {
		name string
		side []map[string]any
		rot  int
	}{
		{"none", nil, 0},
		{"static", data, 0},
		{"mastering-only", data[:1], 0},
		{"light-only", data[1:], 0},
		{"matrix-only", []map[string]any{matrix}, -90},
		{"mixed", []map[string]any{data[0], matrix, data[1]}, -90},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := sampleStream()
			v.SideData = tc.side
			if err := v.ValidateHLG(); err != nil {
				t.Fatal(err)
			}
			rot, err := v.DisplayRotation()
			if err != nil || rot != tc.rot {
				t.Fatalf("rotation %d: %v", rot, err)
			}
			g, err := v.DisplayGeometry()
			if err != nil || (rot == -90 && (g.Width != v.Height || g.Height != v.Width)) || (rot == 0 && (g.Width != v.Width || g.Height != v.Height)) {
				t.Fatalf("display geometry: %+v %v", g, err)
			}
			if v.HasStaticHDRMetadata() != (tc.name != "none" && tc.name != "matrix-only") {
				t.Fatal("static metadata classification mismatch")
			}
		})
	}
	for _, tc := range []struct {
		name string
		side []map[string]any
		want string
	}{
		{"unknown", []map[string]any{data[0], {"side_data_type": "DOVI configuration record"}}, "DOVI configuration record"},
		{"duplicate-matrix", []map[string]any{matrix, matrix}, "duplicate"},
		{"duplicate-static", []map[string]any{data[0], data[0]}, "duplicate"},
		{"missing-type", []map[string]any{{"rotation": float64(90)}}, "missing type"},
		{"bad-matrix", []map[string]any{data[0], {"side_data_type": "Display Matrix", "rotation": float64(90)}}, "incomplete display matrix"},
		{"rotation-on-static", []map[string]any{{"side_data_type": "Content light level metadata", "rotation": float64(90)}}, "unexpected rotation"},
		{"matrix-on-static", []map[string]any{{"side_data_type": "Mastering display metadata", "displaymatrix": matrix["displaymatrix"]}}, "unexpected display matrix"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := sampleStream()
			v.SideData = tc.side
			if err := v.ValidateHLG(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q rejection: %v", tc.want, err)
			}
		})
	}
	v := sampleStream()
	v.SideData = data
	v.Transfer = "smpte2084"
	if err := v.ValidateHLG(); err == nil {
		t.Fatal("static metadata authorized non-HLG source")
	}
	v.Transfer = "arib-std-b67"
	v.Tags = map[string]string{"rotate": "90"}
	if err := v.ValidateHLG(); err == nil {
		t.Fatal("static metadata authorized legacy rotation")
	}
}
