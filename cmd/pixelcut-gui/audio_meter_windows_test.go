package main

import "testing"

func TestAudioMeterWidth(t *testing.T) {
	if meterWidth(0, 100) != 0 || meterWidth(32, 100) != 0 || meterWidth(32768, 100) != 100 || meterWidth(16384, 100) != 90 || meterWidth(32768, 0) != 0 {
		t.Fatal("incorrect logarithmic meter mapping")
	}
}
