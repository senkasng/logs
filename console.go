package logs 

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// brush is a color join function
type brush func(string) string

// newBrush return a fix color Brush,\033[文字背景颜色;文字颜色m 你要显示的内容 \033[0m
func newBrush(color string) brush {
	pre := "\033["
	reset := "\033[0m"
	return func(text string) string {
		return pre + color + "m" + text + reset
	}
}

var colors = []brush{
	newBrush("1;31"), // Error              高亮度 red
	newBrush("1;33"), // Warning            yellow
	newBrush("1;32"), // Informational      green
	newBrush("1;37"), // Debug              green
}

// consoleWriter implements LoggerInterface and writes messages to terminal.
type consoleWriter struct {
	lg       *logWriter
	Level    int  `json:"level"`
	Colorful bool `json:"color"` //this filed is useful only when system's terminal supports color
}

// NewConsole create ConsoleWriter returning as LoggerInterface.
func NewConsole() Logger {
	cw := &consoleWriter{
		lg:       newLogWriter(os.Stdout),
		Level:    LevelDebug,
		Colorful: true,
	}
	return cw
}

// Init init console logger.
// jsonConfig like '{"level":LevelTrace}'.
func (c *consoleWriter) Init(jsonConfig string) error {
	if len(jsonConfig) == 0 {
		return nil
	}
	return json.Unmarshal([]byte(jsonConfig), c)
}

// WriteMsg write message in console.
func (c *consoleWriter) WriteMsg(when time.Time, msg string, level int) error {
	if level > c.Level {
		return nil
	}
	if c.Colorful {
		msg = strings.Replace(msg, levelPrefix[level], colors[level](levelPrefix[level]), 1)
	}
	c.lg.writeln(when, msg)
	return nil
}

// Destroy implementing method. empty.
func (c *consoleWriter) Destroy() {

}

// Flush implementing method. empty.
func (c *consoleWriter) Flush() {

}

func init() {
	Register(AdapterConsole, NewConsole)
}