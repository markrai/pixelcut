package mediacache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	MiB          int64 = 1 << 20
	DefaultLimit       = 1024 * MiB
	MaxEntry           = 16 * MiB
	MaxEntries         = 32768
)

type Source struct {
	Path     string
	Size     int64
	Modified int64
}

func Identify(path string) (Source, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return Source{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Source{}, err
	}
	if !info.Mode().IsRegular() {
		return Source{}, errors.New("source is not a regular file")
	}
	return Source{filepath.Clean(path), info.Size(), info.ModTime().UnixNano()}, nil
}

func ToolIdentity(tool string) string {
	path, err := exec.LookPath(tool)
	if err != nil {
		return ""
	}
	s, err := Identify(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", s.Path, s.Size, s.Modified)
}

type Key [32]byte

func MakeKey(source Source, kind, tool, parameters string) Key {
	b, _ := json.Marshal(struct {
		Version                int
		Source                 Source
		Kind, Tool, Parameters string
	}{1, source, kind, tool, parameters})
	return sha256.Sum256(b)
}

func (k Key) name() string { return hex.EncodeToString(k[:]) + ".pcache" }

type entry struct {
	size int64
	used time.Time
}

type Cache struct {
	mu           sync.Mutex
	dir          string
	limit, total int64
	entries      map[string]entry
}

type Stats struct {
	Bytes, Limit int64
	Entries      int
}

func ValidLimit(n int64) bool { return n == 256*MiB || n == DefaultLimit || n == 4096*MiB }

func Open(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("invalid media cache directory")
	}
	c := &Cache{dir: dir, limit: DefaultLimit, entries: map[string]entry{}}
	f, err := os.Open(filepath.Join(dir, "limit.json"))
	if err == nil {
		b, e := io.ReadAll(io.LimitReader(f, 128))
		f.Close()
		var limit int64
		if e == nil && json.Unmarshal(b, &limit) == nil && ValidLimit(limit) {
			c.limit = limit
		}
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		name := file.Name()
		if suffix, ok := strings.CutPrefix(name, ".cache-write-"); ok && suffix != "" && strings.Trim(suffix, "0123456789") == "" {
			if info, err := file.Info(); err == nil && info.Mode().IsRegular() {
				_ = os.Remove(filepath.Join(dir, name))
			}
			continue
		}
		if !entryName(name) {
			continue
		}
		info, err := file.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		c.entries[name] = entry{info.Size(), info.ModTime()}
		c.total += info.Size()
	}
	if err = c.prune(0, 0); err != nil {
		return nil, err
	}
	return c, nil
}

func entryName(name string) bool {
	if len(name) != 64+len(".pcache") || !strings.HasSuffix(name, ".pcache") {
		return false
	}
	_, err := hex.DecodeString(name[:64])
	return err == nil && name[:64] == strings.ToLower(name[:64])
}

func (c *Cache) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Stats{c.total, c.limit, len(c.entries)}
}

func (c *Cache) remove(name string) error {
	if !entryName(name) {
		return errors.New("invalid cache entry name")
	}
	if err := os.Remove(filepath.Join(c.dir, name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if v, ok := c.entries[name]; ok {
		c.total -= v.size
		delete(c.entries, name)
	}
	return nil
}

func (c *Cache) prune(extra int64, count int) error {
	if c.total+extra <= c.limit && len(c.entries)+count <= MaxEntries {
		return nil
	}
	names := make([]string, 0, len(c.entries))
	for name := range c.entries {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := c.entries[names[i]], c.entries[names[j]]
		if a.used.Equal(b.used) {
			return names[i] < names[j]
		}
		return a.used.Before(b.used)
	})
	for _, name := range names {
		if c.total+extra <= c.limit && len(c.entries)+count <= MaxEntries {
			break
		}
		if err := c.remove(name); err != nil {
			return err
		}
	}
	return nil
}

func (c *Cache) Get(key Key) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	name := key.name()
	path := filepath.Join(c.dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			_ = c.remove(name)
		}
		return nil, false
	}
	if !info.Mode().IsRegular() {
		return nil, false
	}
	if info.Size() < 64 || info.Size() > MaxEntry+64 {
		_ = c.remove(name)
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxEntry+65))
	f.Close()
	if err != nil || len(b) < 64 || int64(len(b)) > MaxEntry+64 {
		_ = c.remove(name)
		return nil, false
	}
	h := sha256.Sum256(b[64:])
	if !bytes.Equal(b[:32], key[:]) || !bytes.Equal(b[32:64], h[:]) {
		_ = c.remove(name)
		return nil, false
	}
	now := time.Now()
	if old, ok := c.entries[name]; ok {
		c.total -= old.size
	}
	c.entries[name] = entry{int64(len(b)), now}
	c.total += int64(len(b))
	_ = os.Chtimes(path, now, now)
	_ = c.prune(0, 0)
	return b[64:], true
}

func stage(dir string, b []byte) (string, error) {
	f, err := os.CreateTemp(dir, ".cache-write-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

func (c *Cache) Put(key Key, payload []byte) error {
	if c == nil {
		return nil
	}
	if len(payload) == 0 || int64(len(payload)) > MaxEntry {
		return errors.New("invalid cache payload size")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	name := key.name()
	path := filepath.Join(c.dir, name)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("invalid cache destination")
		}
		if err = c.remove(name); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	size := int64(len(payload)) + 64
	if size > c.limit {
		return errors.New("cache payload exceeds budget")
	}
	if err := c.prune(size, 1); err != nil {
		return err
	}
	h := sha256.Sum256(payload)
	b := make([]byte, 0, size)
	b = append(b, key[:]...)
	b = append(b, h[:]...)
	b = append(b, payload...)
	temp, err := stage(c.dir, b)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	if err = os.Rename(temp, path); err != nil {
		return err
	}
	c.entries[name] = entry{size, time.Now()}
	c.total += size
	return nil
}

func (c *Cache) SetLimit(limit int64) error {
	if !ValidLimit(limit) {
		return errors.New("unsupported cache size")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.limit
	c.limit = limit
	if err := c.prune(0, 0); err != nil {
		c.limit = old
		return err
	}
	b, _ := json.Marshal(limit)
	temp, err := stage(c.dir, b)
	if err != nil {
		c.limit = old
		return err
	}
	defer os.Remove(temp)
	if err = os.Rename(temp, filepath.Join(c.dir, "limit.json")); err != nil {
		c.limit = old
		return err
	}
	return nil
}

func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var first error
	for name := range c.entries {
		if err := c.remove(name); err != nil && first == nil {
			first = err
		}
	}
	return first
}
