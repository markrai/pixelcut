package main

import (
	"testing"
	"unsafe"

	"github.com/markrai/pixelcut/internal/composition"
)

func TestWaveOutNativeLayoutAndInvalidBuffer(t *testing.T) {
	var h waveHeader
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(h) != 48 || unsafe.Offsetof(h.Length) != 8 || unsafe.Offsetof(h.Flags) != 24 || unsafe.Offsetof(h.Next) != 32) {
		t.Fatal("incorrect Win64 WAVEHDR layout")
	}
	var f waveFormat
	if unsafe.Offsetof(f.Rate) != 4 || unsafe.Offsetof(f.BlockAlign) != 12 || unsafe.Offsetof(f.Ext) != 16 {
		t.Fatal("incorrect WAVEFORMATEX layout")
	}
	if _, err := queueWave(0, composition.PCMBlock{Frames: 1}); err == nil {
		t.Fatal("invalid native buffer accepted")
	}
}
