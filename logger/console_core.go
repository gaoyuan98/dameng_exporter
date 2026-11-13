package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewConsoleCore renders structured events for people. Each Write emits the
// summary and its details together; all derived cores share the locked sink.
func NewConsoleCore(out zapcore.WriteSyncer, level zapcore.LevelEnabler) zapcore.Core {
	return &consoleCore{out: zapcore.Lock(out), level: level}
}

type consoleCore struct {
	out     zapcore.WriteSyncer
	level   zapcore.LevelEnabler
	context []zapcore.Field
}

func (c *consoleCore) Enabled(l zapcore.Level) bool { return c.level.Enabled(l) }
func (c *consoleCore) With(fields []zapcore.Field) zapcore.Core {
	next := *c
	next.context = append([]zapcore.Field(nil), c.context...)
	// Resolve object/array marshalers at With time, as zap's built-in cores do.
	for _, f := range fields {
		if f.Type == zapcore.NamespaceType {
			next.context = append(next.context, f)
			continue
		}
		enc := zapcore.NewMapObjectEncoder()
		f.AddTo(enc)
		keys := make([]string, 0, len(enc.Fields))
		for k := range enc.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			next.context = append(next.context, zap.Any(k, snapshotConsoleValue(enc.Fields[k])))
		}
	}
	return &next
}

// With must capture mutable reflected values now, rather than read them on a
// later Write. Keep scalar types; JSON snapshots freeze maps, slices and objects.
func snapshotConsoleValue(v interface{}) interface{} {
	switch v.(type) {
	case nil, string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64, complex64, complex128:
		return v
	}
	data, err := json.Marshal(v)
	if err != nil {
		return plainValue(v)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var frozen interface{}
	if err := decoder.Decode(&frozen); err != nil {
		return plainValue(v)
	}
	return frozen
}

func (c *consoleCore) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) {
		return ce.AddCore(e, c)
	}
	return ce
}
func (c *consoleCore) Write(e zapcore.Entry, fields []zapcore.Field) error {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range c.context {
		f.AddTo(enc)
	}
	for _, f := range fields {
		f.AddTo(enc)
	}
	data := []byte(formatConsole(e, enc.Fields))
	n, err := c.out.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil && e.Level > zapcore.ErrorLevel {
		// Match zap's core: panic/fatal entries flush before control can exit.
		_ = c.Sync()
	}
	return err
}
func (c *consoleCore) Sync() error { return c.out.Sync() }
