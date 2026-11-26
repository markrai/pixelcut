package media

import (
	"errors"
	"fmt"
	"strings"
)

const BackendCUDA Backend = "cuda"

const cudaDownloadFilter = "hwdownload,format=p010le,format=yuv420p10le"

func ParseDecodeBackend(value string) (Backend, error) {
	switch Backend(value) {
	case "", BackendCPU:
		return BackendCPU, nil
	case BackendAuto:
		return BackendAuto, nil
	case BackendCUDA:
		return BackendCUDA, nil
	default:
		return "", fmt.Errorf("unknown decode backend %q (use cpu, auto or cuda)", value)
	}
}

func (e Engine) DecodeBackendName() string {
	if e.DecodeBackend == BackendCUDA {
		return "CUDA (experimental)"
	}
	return "CPU"
}

func validateCUDADecodeSource(v Stream) error {
	if err := v.ValidateHLG(); err != nil {
		return err
	}
	if v.CodecName != "hevc" || v.Profile != "Main 10" || v.Width > 8192 || v.Height > 8192 || int64(v.Width)*int64(v.Height) > 33554432 {
		return errors.New("CUDA decode requires bounded HEVC Main 10 HLG source")
	}
	return nil
}

func cudaDecodeInput() []string {
	return []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda", "-c:v", "hevc"}
}

func (e Engine) VideoDecodePipeline(v Stream, graph string, displayRotation bool) ([]string, string, error) {
	backend, err := ParseDecodeBackend(string(e.DecodeBackend))
	if err != nil {
		return nil, "", err
	}
	if backend != BackendCUDA {
		return nil, graph, nil
	}
	if err := validateCUDADecodeSource(v); err != nil {
		return nil, "", err
	}
	args := append([]string{"-noautorotate"}, cudaDecodeInput()...)
	filters := []string{cudaDownloadFilter}
	if displayRotation {
		rotation, err := v.DisplayRotation()
		if err != nil {
			return nil, "", err
		}
		switch rotation {
		case -90:
			filters = append(filters, "transpose=clock")
		case 90:
			filters = append(filters, "transpose=cclock")
		case 180:
			filters = append(filters, "hflip", "vflip")
		}
	}
	if graph != "" {
		filters = append(filters, graph)
	}
	return args, strings.Join(filters, ","), nil
}
