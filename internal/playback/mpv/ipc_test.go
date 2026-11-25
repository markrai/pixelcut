package mpv

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPreviewIPCCommand(t *testing.T) {
	var output bytes.Buffer
	p := NewIPC(&output, strings.NewReader("{\"event\":\"playback-restart\"}\n{\"request_id\":99,\"error\":\"success\"}\n{\"request_id\":1,\"error\":\"success\"}\n{\"request_id\":2,\"error\":\"property unavailable\"}\n"))
	if _, err := p.Call("seek", 4.25, "absolute+exact"); err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Command   []any  `json:"command"`
		RequestID uint64 `json:"request_id"`
	}
	if err := json.Unmarshal(output.Bytes(), &sent); err != nil || sent.RequestID != 1 || len(sent.Command) != 3 || sent.Command[0] != "seek" || sent.Command[1] != 4.25 || sent.Command[2] != "absolute+exact" {
		t.Fatalf("invalid seek command %q: %v", output.String(), err)
	}
	if _, err := p.Call("get_property", "time-pos"); err == nil {
		t.Fatal("mpv error ignored")
	}
}
