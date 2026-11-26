package media

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestPreviewPlaneAccumulator(t *testing.T) {
	a := previewPlaneAccumulator{width: 2, height: 2}
	c, g := make([]byte, 12), make([]byte, 12)
	for i := 0; i < len(c); i += 2 {
		binary.LittleEndian.PutUint16(c[i:], 512)
		binary.LittleEndian.PutUint16(g[i:], 512)
	}
	if err := a.add(c, c); err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint16(g[0:], 520)
	binary.LittleEndian.PutUint16(g[8:], 500)
	if err := a.add(c, g); err != nil {
		t.Fatal(err)
	}
	y, u, v := a.planes[0].q, a.planes[1].q, a.planes[2].q
	if a.exactFrames != 1 || y.Samples != 8 || y.DifferentSamples != 1 || y.MaxDelta != 8 || y.MeanAbsoluteDelta != 1 ||
		u.Samples != 2 || u.MaxDelta != 12 || u.MeanAbsoluteDelta != 6 || v.Samples != 2 || v.DifferentSamples != 0 || v.MinFramePSNRDB != 100 {
		t.Fatalf("incorrect plane statistics: Y=%+v U=%+v V=%+v", y, u, v)
	}
	binary.LittleEndian.PutUint16(g[10:], 1024)
	if err := a.add(c, g); err == nil {
		t.Fatal("invalid planar sample accepted")
	}
}

func TestPreviewStageGraphBoundaries(t *testing.T) {
	v := (ExportContract{Canvas: CanvasSize{1920, 1080}, Color: "hlg", FPS: 30}).ExpectedVideo()
	v.Profile = "Main 10"
	req := StillRequest{Path: "source.mp4", Width: 1920, Height: 1080, Canvas: CanvasSize{1920, 1080}, OutputWidth: 960, OutputHeight: 540}
	g, err := buildPreviewStageGraphs(v, req, 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, graph := range []string{g.reference, g.cpuOrder, g.cuda} {
		if strings.Contains(graph, "tonemap") || !strings.HasSuffix(graph, "format=yuv420p10le") || !strings.Contains(graph, "select=between(n\\,2\\,5)") {
			t.Fatalf("invalid pre-tone-map graph: %q", graph)
		}
	}
	if !strings.HasPrefix(g.cuda, "scale_cuda=w=960:h=540:format=p010le:interp_algo=lanczos:passthrough=0,"+cudaDownloadFilter) || strings.Contains(g.reference, "scale_cuda") || strings.Contains(g.cpuOrder, "scale_cuda") {
		t.Fatalf("invalid control/candidate graphs: %+v", g)
	}
}
