package log

import (
	"fmt"
	"io"
	stdlog "log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Logger interface {
	Log(level string, keyvals ...interface{}) error
}

type stdLogger struct {
	mu      sync.Mutex
	base    *stdlog.Logger
	keyvals []interface{}
}

func NewStdLogger(w io.Writer) Logger {
	if w == nil {
		w = os.Stdout
	}
	return &stdLogger{base: stdlog.New(w, "", stdlog.LstdFlags)}
}

func (l *stdLogger) Log(level string, keyvals ...interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	merged := append([]interface{}{"level", level}, l.keyvals...)
	merged = append(merged, keyvals...)

	parts := make([]string, 0, len(merged)/2)
	for i := 0; i < len(merged); i += 2 {
		key := fmt.Sprint(merged[i])
		var value interface{}
		if i+1 < len(merged) {
			value = resolveValue(merged[i+1])
		}
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	l.base.Println(strings.Join(parts, " "))
	return nil
}

func resolveValue(v interface{}) interface{} {
	if valuer, ok := v.(Valuer); ok {
		return valuer()
	}
	return v
}

type Valuer func() interface{}

func With(logger Logger, keyvals ...interface{}) Logger {
	if len(keyvals)%2 != 0 {
		keyvals = append(keyvals, "")
	}
	if l, ok := logger.(*stdLogger); ok {
		copyVals := append([]interface{}{}, l.keyvals...)
		copyVals = append(copyVals, keyvals...)
		return &stdLogger{base: l.base, keyvals: copyVals}
	}
	return logger
}

var DefaultTimestamp Valuer = func() interface{} {
	return time.Now().UTC().Format(time.RFC3339)
}

var DefaultCaller Valuer = func() interface{} {
	if _, file, line, ok := runtime.Caller(2); ok {
		return fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}
	return "unknown:0"
}

type Helper struct {
	Logger Logger
}

func NewHelper(logger Logger) *Helper {
	return &Helper{Logger: logger}
}

func (h *Helper) log(level string, msg interface{}) {
	if h == nil || h.Logger == nil {
		return
	}
	_ = h.Logger.Log(level, "msg", msg)
}

func (h *Helper) Error(args ...interface{}) {
	h.log("ERROR", fmt.Sprint(args...))
}

func (h *Helper) Errorf(format string, args ...interface{}) {
	h.log("ERROR", fmt.Sprintf(format, args...))
}

func (h *Helper) Fatalf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	h.log("FATAL", msg)
	os.Exit(1)
}
