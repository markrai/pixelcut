package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/markrai/pixelcut/internal/mediacache"
	"github.com/markrai/pixelcut/internal/waveform"
)

func cacheRequest(c *mediacache.Cache, path, kind, tool, params string) (mediacache.Source, mediacache.Key, bool) {
	if c == nil {
		return mediacache.Source{}, mediacache.Key{}, false
	}
	s, err := mediacache.Identify(path)
	if err != nil {
		return s, mediacache.Key{}, false
	}
	identity := mediacache.ToolIdentity(tool)
	if identity == "" {
		return s, mediacache.Key{}, false
	}
	return s, mediacache.MakeKey(s, kind, identity, params), true
}

func unchanged(path string, s mediacache.Source) bool {
	now, err := mediacache.Identify(path)
	return err == nil && now == s
}

func (e Engine) CachedThumbnail(ctx context.Context, c *mediacache.Cache, path string, tick int64, w, h int) ([]byte, error) {
	s, key, ok := cacheRequest(c, path, "thumbnail", e.FFmpeg, fmt.Sprintf("source-color-v2:%d:%d:%d", tick, w, h))
	if ok && tick >= 0 && w >= 2 && h >= 2 && w <= 256 && h <= 256 && w%2 == 0 && h%2 == 0 {
		if b, hit := c.Get(key); hit && len(b) == w*h*4 && unchanged(path, s) {
			return b, nil
		}
	}
	b, err := e.Thumbnail(ctx, path, float64(tick)/1000000, w, h)
	if err == nil && ok && ctx.Err() == nil && unchanged(path, s) {
		_ = c.Put(key, b)
	}
	return b, err
}

func (e Engine) CachedWaveform(ctx context.Context, c *mediacache.Cache, path string, duration int64) (*waveform.Peaks, error) {
	s, key, ok := cacheRequest(c, path, "waveform", e.FFmpeg, fmt.Sprint(duration))
	if ok {
		if b, hit := c.Get(key); hit {
			if p, err := waveform.DecodePeaks(b, duration); err == nil && unchanged(path, s) {
				return p, nil
			}
		}
	}
	p, err := e.Waveform(ctx, path, duration)
	if err == nil && ok && ctx.Err() == nil && unchanged(path, s) {
		if b, err := p.MarshalBinary(); err == nil {
			_ = c.Put(key, b)
		}
	}
	return p, err
}

func (e Engine) CachedFrames(ctx context.Context, c *mediacache.Cache, path string) (*FrameIndex, error) {
	s, key, ok := cacheRequest(c, path, "frames", e.FFprobe, "")
	if ok {
		if b, hit := c.Get(key); hit {
			if f, err := DecodeFrameIndex(b); err == nil && unchanged(path, s) {
				return f, nil
			}
		}
	}
	f, err := e.Frames(ctx, path)
	if err == nil && ok && ctx.Err() == nil && unchanged(path, s) {
		if b, err := f.MarshalBinary(); err == nil {
			_ = c.Put(key, b)
		}
	}
	return f, err
}

const importMetadataCacheContract = "source-color-v2"

func (e Engine) CachedImportReport(ctx context.Context, c *mediacache.Cache, path string) (Report, error) {
	s, key, ok := cacheRequest(c, path, "metadata", e.FFprobe, importMetadataCacheContract)
	if ok {
		if b, hit := c.Get(key); hit {
			var r Report
			if err := json.Unmarshal(b, &r); err == nil && len(r.Streams) > 0 && unchanged(path, s) {
				return r, nil
			}
		}
	}
	r, err := e.Inspect(ctx, path)
	if err != nil {
		return r, err
	}
	if ok && !unchanged(path, s) {
		return Report{}, errors.New("source changed during media inspection")
	}
	r.Format = nil
	for i := range r.Streams {
		stream := &r.Streams[i]
		tags := stream.Tags
		stream.Tags = nil
		if rotate, ok := tags["rotate"]; ok {
			stream.Tags = map[string]string{"rotate": rotate}
		}
		if stream.CodecType == "data" && tags["handler_name"] == "MetaHandle" {
			if stream.Tags == nil {
				stream.Tags = make(map[string]string)
			}
			stream.Tags["handler_name"] = "MetaHandle"
		}
	}
	if ok && ctx.Err() == nil {
		if b, err := json.Marshal(r); err == nil {
			_ = c.Put(key, b)
		}
	}
	return r, nil
}
