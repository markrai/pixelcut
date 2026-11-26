package media

import "testing"

func TestProjectExportContractRejectsMislabeledOrIncorrectOutput(t *testing.T) {
	c := ExportContract{Color: "hlg", Canvas: CanvasSize{Width: 1920, Height: 1080}, FPS: 30, Audio: true}
	v := c.ExpectedVideo()
	v.AvgFrameRate = "30/1"
	a := Stream{CodecType: "audio", CodecName: "aac", SampleRate: "48000", Channels: 2}
	r := Report{Streams: []Stream{v, a}}
	if err := c.VerifyOutput(r); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Stream){
		func(s *Stream) { s.PixelFormat = "yuv420p" },
		func(s *Stream) { s.Transfer = "bt709" },
		func(s *Stream) { s.Range = "pc" },
		func(s *Stream) { s.ChromaLocation = "center" },
		func(s *Stream) { s.AvgFrameRate = "25/1" },
		func(s *Stream) { s.Width = 1280 },
		func(s *Stream) { s.CodecTag = "hev1" },
	} {
		r.Streams[0] = v
		mutate(&r.Streams[0])
		if err := c.VerifyOutput(r); err == nil {
			t.Fatal("incorrect video contract accepted")
		}
	}
	r.Streams[0] = v
	r.Streams[1].Channels = 1
	if err := c.VerifyOutput(r); err == nil {
		t.Fatal("incorrect audio layout accepted")
	}
	r.Streams = r.Streams[:1]
	if err := c.VerifyOutput(r); err == nil {
		t.Fatal("missing audio accepted")
	}
	c.Audio = false
	if err := c.VerifyOutput(r); err != nil {
		t.Fatal(err)
	}
}
