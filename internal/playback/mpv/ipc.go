package mpv

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type ipcReply struct {
	RequestID uint64          `json:"request_id"`
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
}

type IPC struct {
	file   io.Writer
	reader *bufio.Scanner
	id     uint64
}

func (p *IPC) Call(command ...any) (json.RawMessage, error) {
	p.id++
	b, err := json.Marshal(map[string]any{"command": command, "request_id": p.id})
	if err != nil {
		return nil, err
	}
	if _, err = p.file.Write(append(b, '\n')); err != nil {
		return nil, err
	}
	for p.reader.Scan() {
		var r ipcReply
		if json.Unmarshal(p.reader.Bytes(), &r) != nil || r.RequestID != p.id {
			continue
		}
		if r.Error != "success" {
			return nil, fmt.Errorf("mpv: %s", r.Error)
		}
		return r.Data, nil
	}
	if err = p.reader.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("mpv IPC closed")
}

func (p *IPC) Property(name string, value any) bool {
	b, err := p.Call("get_property", name)
	return err == nil && len(b) > 0 && string(b) != "null" && json.Unmarshal(b, value) == nil
}

func NewIPC(w io.Writer, r io.Reader) *IPC {
	return &IPC{file: w, reader: bufio.NewScanner(r)}
}
