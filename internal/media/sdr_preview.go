package media

import (
	"errors"
	"fmt"
)

func sdrPreviewGraph(color SourceColor) (string, error) {
	if color.Family != ColorSDR709 || color.Validate() != nil {
		return "", errors.New("invalid SDR preview color descriptor")
	}
	horizontal := 0
	if color.Chroma == "center" {
		horizontal = 128
	}
	return fmt.Sprintf("scale=in_color_matrix=bt709:in_range=%s:out_range=pc:in_h_chr_pos=%d:in_v_chr_pos=128:flags=lanczos+accurate_rnd+full_chroma_int,format=bgra", color.Range, horizontal), nil
}

func previewEngineForSource(engine Engine, color SourceColor) Engine {
	if color.Family == ColorSDR709 {
		engine.DecodeBackend = BackendCPU
	}
	return engine
}
