package media

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestCUDAScaleCheckPipeline(t *testing.T) {
	size := scaleCheckSize{32, 24}
	args, err := hevcFrameCheckArgs("source.mp4", 2, 4, true, &size)
	if err != nil || !slices.Contains(args, "-hwaccel_output_format") || !slices.Contains(args, "cuda") || slices.Contains(args, "-ss") {
		t.Fatalf("invalid CUDA scale arguments: %v %v", args, err)
	}
	graph := "scale_cuda=w=32:h=24:format=p010le:interp_algo=nearest:passthrough=0,hwdownload,format=p010le,format=yuv420p10le,select=between(n\\,2\\,5)"
	if !slices.Contains(args, graph) {
		t.Fatalf("scale must precede hardware download: %v", args)
	}
	args, err = hevcFrameCheckArgs("source.mp4", 2, 4, false, &size)
	if err != nil || !slices.Contains(args, "none") || slices.Contains(args, "cuda") || !slices.Contains(args, "format=yuv420p10le,scale=w=32:h=24:flags=neighbor,format=yuv420p10le,select=between(n\\,2\\,5)") {
		t.Fatalf("invalid CPU scale arguments: %v %v", args, err)
	}
}

func TestCUDAScaleCheckRejectsInvalidSize(t *testing.T) {
	for _, size := range []scaleCheckSize{{0, 24}, {-2, 24}, {31, 24}, {32, 25}, {8194, 2}, {8192, 8192}} {
		r, err := Default().CheckCUDAScale(context.Background(), "missing.mp4", 0, 4, size.width, size.height)
		if err == nil || r.Status != "failed" || !strings.Contains(r.Failure, "output dimensions") || r.DownloadBytesPerFrame != 0 {
			t.Fatalf("invalid size accepted: %+v %v", r, err)
		}
	}
	for _, size := range []scaleCheckSize{{2, 2}, {960, 540}, {8192, 4096}} {
		if err := size.validate(); err != nil {
			t.Fatal(err)
		}
	}
}
