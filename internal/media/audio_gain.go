package media

import (
	"errors"
	"fmt"
)

const MaxAudioGainPercent = 400

func previewGainArgs(gain int, silent bool) ([]string, error) {
	if gain < 0 || gain > MaxAudioGainPercent {
		return nil, errors.New("invalid preview audio gain")
	}
	if silent || gain == 0 {
		return []string{"--aid=no"}, nil
	}
	if gain == 100 {
		return nil, nil
	}
	return []string{fmt.Sprintf("--af=lavfi=[volume=%.2f:precision=double]", float64(gain)/100)}, nil
}
