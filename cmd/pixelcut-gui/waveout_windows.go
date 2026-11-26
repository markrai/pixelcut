package main

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/markrai/pixelcut/internal/composition"
	"github.com/markrai/pixelcut/internal/media"
	"github.com/markrai/pixelcut/internal/playback"
)

const waveQueueLimit = 3
const waveHeaderDone = 0x00000001

var winmm = syscall.NewLazyDLL("winmm.dll")
var waveDeviceFault atomic.Bool
var waveCopy = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlMoveMemory")

type waveFormat struct {
	Tag, Channels         uint16
	Rate, BytesPerSecond  uint32
	BlockAlign, Bits, Ext uint16
}

type waveHeader struct {
	Data                  uintptr
	Length, BytesRecorded uint32
	User                  uintptr
	Flags, Loops          uint32
	Next, Reserved        uintptr
}

type queuedWave struct {
	header uintptr
	end    int64
	levels media.AudioLevels
}

func waveCall(name string, args ...uintptr) error {
	code, _, _ := winmm.NewProc(name).Call(args...)
	if code != 0 {
		return fmt.Errorf("%s: Windows multimedia error %d", name, code)
	}
	return nil
}

func freeWave(address uintptr) {
	kernel.NewProc("GlobalFree").Call(address)
}

func queueWave(handle uintptr, block composition.PCMBlock) (queuedWave, error) {
	if block.Frames < 1 || block.Frames > media.PCMBlockFrames || len(block.Samples) != block.Frames*media.PCMChannels {
		return queuedWave{}, fmt.Errorf("invalid audio output buffer")
	}
	size := unsafe.Sizeof(waveHeader{})
	length := uintptr(len(block.Samples) * 2)
	address, _, _ := kernel.NewProc("GlobalAlloc").Call(0x40, size+length)
	if address == 0 {
		return queuedWave{}, fmt.Errorf("audio output buffer allocation failed")
	}
	header := waveHeader{Data: address + size, Length: uint32(length)}
	waveCopy.Call(address, uintptr(unsafe.Pointer(&header)), size)
	waveCopy.Call(header.Data, uintptr(unsafe.Pointer(&block.Samples[0])), length)
	runtime.KeepAlive(&header)
	runtime.KeepAlive(block.Samples)
	if err := waveCall("waveOutPrepareHeader", handle, address, size); err != nil {
		freeWave(address)
		return queuedWave{}, err
	}
	if err := waveCall("waveOutWrite", handle, address, size); err != nil {
		if waveCall("waveOutUnprepareHeader", handle, address, size) == nil {
			freeWave(address)
			return queuedWave{}, err
		}
		return queuedWave{header: address, end: block.Offset + int64(block.Frames), levels: block.Levels}, err
	}
	return queuedWave{header: address, end: block.Offset + int64(block.Frames), levels: block.Levels}, nil
}

func waveDone(address uintptr) bool {
	var h waveHeader
	waveCopy.Call(uintptr(unsafe.Pointer(&h)), address, unsafe.Sizeof(h))
	runtime.KeepAlive(&h)
	return h.Flags&waveHeaderDone != 0
}

func releaseWave(handle uintptr, q queuedWave) error {
	if err := waveCall("waveOutUnprepareHeader", handle, q.header, unsafe.Sizeof(waveHeader{})); err != nil {
		return err
	}
	freeWave(q.header)
	return nil
}

func playPCM(ctx context.Context, clock *playback.SampleClock, blocks <-chan composition.PCMBlock, ready <-chan struct{}, metrics *playback.PreviewMetrics) (err error) {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ready:
	}
	format := waveFormat{Tag: 1, Channels: media.PCMChannels, Rate: media.PCMRate, BytesPerSecond: media.PCMRate * media.PCMChannels * 2, BlockAlign: media.PCMChannels * 2, Bits: 16}
	if waveDeviceFault.Load() {
		return fmt.Errorf("audio device cleanup previously failed; restart PixelCut")
	}
	var handle uintptr
	code, _, _ := winmm.NewProc("waveOutOpen").Call(uintptr(unsafe.Pointer(&handle)), 0xffffffff, uintptr(unsafe.Pointer(&format)), 0, 0, 0)
	runtime.KeepAlive(&format)
	runtime.KeepAlive(&handle)
	if code != 0 {
		return fmt.Errorf("waveOutOpen: Windows multimedia error %d", code)
	}
	var queued []queuedWave
	defer func() {
		if resetErr := waveCall("waveOutReset", handle); resetErr != nil {
			waveDeviceFault.Store(true)
			if err == nil {
				err = resetErr
			}
		}
		for _, q := range queued {
			if releaseErr := releaseWave(handle, q); releaseErr != nil {
				waveDeviceFault.Store(true)
				if err == nil {
					err = releaseErr
				}
			}
		}
		if closeErr := waveCall("waveOutClose", handle); closeErr != nil {
			waveDeviceFault.Store(true)
			if err == nil {
				err = closeErr
			}
		}
	}()
	if err = waveCall("waveOutPause", handle); err != nil {
		return err
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	var submitted int64
	started, closed, starved := false, false, false
	lastProgress := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		for len(queued) > 0 && waveDone(queued[0].header) {
			q := queued[0]
			if err := releaseWave(handle, q); err != nil {
				return err
			}
			queued = queued[1:]
			lastProgress = time.Now()
			if !clock.Confirm(q.end) {
				return fmt.Errorf("invalid audio completion position")
			}
			if metrics != nil {
				metrics.ConfirmAudio(q.levels)
			}
		}
		empty := started && !closed && len(queued) == 0 && submitted < clock.Total()
		if empty && !starved && metrics != nil {
			metrics.Starved()
		}
		starved = empty
		if closed && len(queued) == 0 {
			if submitted != clock.Total() || clock.Played() != clock.Total() {
				return fmt.Errorf("audio ended before project duration")
			}
			return nil
		}
		if !started && (len(queued) == waveQueueLimit || closed) {
			if err := waveCall("waveOutRestart", handle); err != nil {
				return err
			}
			started = true
			lastProgress = time.Now()
		}
		if started && len(queued) > 0 && time.Since(lastProgress) > 5*time.Second {
			return fmt.Errorf("audio device stopped completing buffers")
		}
		var input <-chan composition.PCMBlock
		if !closed && len(queued) < waveQueueLimit {
			input = blocks
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		case block, ok := <-input:
			if !ok {
				closed = true
				continue
			}
			if block.Offset != submitted || int64(block.Frames) > clock.Total()-submitted {
				return fmt.Errorf("out-of-order audio output buffer")
			}
			q, err := queueWave(handle, block)
			if q.header != 0 {
				if len(queued) == 0 {
					lastProgress = time.Now()
				}
				queued = append(queued, q)
			}
			if err != nil {
				return err
			}
			submitted += int64(block.Frames)
		}
	}
}
