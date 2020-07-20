package logs

import (
	"os"
	"time"
	"encoding/json"
	"fmt"
	"strings"
)



type fileWriter struct {
	lg  *logWriter
	FileName string    `json:"filename"`
	Level int			`json:"level"`
	Colorful bool  		`json:"color"`
}



func NewFile() Logger {
	file,err := os.OpenFile("default.log",os.O_APPEND|os.O_WRONLY|os.O_CREATE,0644)
	if err != nil {
		fmt.Println(err)
	}
	return &fileWriter{
		lg : newLogWriter(file),
		FileName: "default.log",
		Level: LevelDebug,
		Colorful: true,
	}

}

func (f *fileWriter) Init(jsonConfig string) error {
	if len(jsonConfig) == 0 {
		return nil
	}

	err := json.Unmarshal([]byte(jsonConfig), f)
	if err != nil {
		return err
	}

	logfile ,err := os.OpenFile(f.FileName,os.O_APPEND|os.O_WRONLY|os.O_CREATE,0644)
	if err != nil {
		return err
	}
	f.lg = newLogWriter(logfile)
	return nil
}

// WriteMsg write message in console.
func (f *fileWriter) WriteMsg(when time.Time, msg string, level int) error {
	if level > f.Level {
		return nil
	}
	if f.Colorful {
		msg = strings.Replace(msg, levelPrefix[level], colors[level](levelPrefix[level]), 1)
	}
	f.lg.writeln(when, msg)
	return nil
}

// Destroy implementing method. empty.
func (f *fileWriter) Destroy() {
	
}

// Flush implementing method. empty.
func (f *fileWriter) Flush() {

}

func init() {
	Register(AdapterFile, NewFile)
}


