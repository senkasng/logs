package logs

import (
	"sync"
	"time"
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"io"
)

// 4个log 级别
const (
	LevelError = iota
	LevelWarning
	LevelInfo
	LevelDebug
)

// 2个log的输出方式，支持文件和控制台
const (
	AdapterConsole   = "console"
	AdapterFile      = "file"
)

const levelLoggerImpl = -1


//Logger 接口的定义，包括初始化，写log方式，销毁和刷新
type Logger interface {
	Init(config string) error
	WriteMsg(when time.Time, msg string, level int) error
	Destroy()
	Flush()
}

// 类型别名，为了获取到实现Logger的类型，如consoleLogger 或者 fileLogger
type newLoggerFunc func() Logger


var levelPrefix = [LevelDebug + 1]string{"[E]", "[W]", "[I]", "[D]"}

// 接口池，实现了Logger 接口的接口池
var adapters = make(map[string]newLoggerFunc)

// Register 函数实现了 套接层向接口池 adapters的注册
func Register(name string, log newLoggerFunc) {
	if log == nil {
		panic("logs: Register provide is nil")
	}
	if _, dup := adapters[name]; dup {
		panic("logs: Register called twice for provider " + name)
	}
	adapters[name] = log
}


// 整个app log 的结构体,可以包括多个实例化的Logger 类型
type AppLogger struct {
	lock                sync.Mutex
	level               int
	init                bool
	enableFuncCallDepth bool
	loggerFuncCallDepth int
	asynchronous        bool
	prefix              string
	msgChanLen          int64
	msgChan             chan *logMsg
	signalChan          chan string
	wg                  sync.WaitGroup
	outputs             []*nameLogger
}


const defaultAsyncMsgLen = 1e2

type nameLogger struct {
	Logger
	name string
}

//log的具体内容，包括级别，信息和时间
type logMsg struct {
	level int
	msg   string
	when  time.Time
}

//协程池
var logMsgPool *sync.Pool



//实例化APPLogger 
func NewAppLogger(channelLens ...int64) *AppLogger {
	al := new(AppLogger)
	al.level = LevelDebug
	al.loggerFuncCallDepth = 2
	al.msgChanLen = append(channelLens, 0)[0]
	if al.msgChanLen <= 0 {
		al.msgChanLen = defaultAsyncMsgLen
	}
	al.signalChan = make(chan string, 1)
	al.setLogger(AdapterConsole)
	return al
}

//异步发送log的方法
func (al *AppLogger) Async(msgLen ...int64) *AppLogger {
	al.lock.Lock()
	defer al.lock.Unlock()
	if al.asynchronous {
		return al
	}
	al.asynchronous = true
	if len(msgLen) > 0 && msgLen[0] > 0 {
		al.msgChanLen = msgLen[0]
	}
	al.msgChan = make(chan *logMsg, al.msgChanLen)
	logMsgPool = &sync.Pool{
		New: func() interface{} {
			return &logMsg{}
		},
	}
	al.wg.Add(1)
	go al.startLogger()
	return al
}

//Logger实例和其配置添加到APPLogger
func (al *AppLogger) setLogger(adapterName string, configs ...string) error {
	config := append(configs, "{}")[0]
	for _, l := range al.outputs {
		if l.name == adapterName {
			return fmt.Errorf("logs: duplicate adaptername %q (you have set this logger before)", adapterName)
		}
	}

	logAdapter, ok := adapters[adapterName]
	if !ok {
		return fmt.Errorf("logs: unknown adaptername %q (forgotten Register?)", adapterName)
	}

	lg := logAdapter()
	err := lg.Init(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "logs.BeeLogger.SetLogger: "+err.Error())
		return err
	}
	al.outputs = append(al.outputs, &nameLogger{name: adapterName, Logger: lg})
	return nil
}

// 异步启动 logget
func (al *AppLogger) startLogger() {
	gameOver := false
	for {
		select {
		case bm := <-al.msgChan:
			al.writeToLoggers(bm.when, bm.msg, bm.level)
			logMsgPool.Put(bm)
		case sg := <-al.signalChan:
			// Now should only send "flush" or "close" to bl.signalChan
			al.flush()
			if sg == "close" {
				for _, l := range al.outputs {
					l.Destroy()
				}
				al.outputs = nil
				gameOver = true
			}
			al.wg.Done()
		}
		if gameOver {
			break
		}
	}
}

func (al *AppLogger) Flush() {
	if al.asynchronous {
		al.signalChan <- "flush"
		al.wg.Wait()
		al.wg.Add(1)
		return
	}
	al.flush()
}



func (al *AppLogger) flush() {
	if al.asynchronous {
		for {
			if len(al.msgChan) > 0 {
				bm := <-al.msgChan
				al.writeToLoggers(bm.when, bm.msg, bm.level)
				logMsgPool.Put(bm)
				continue
			}
			break
		}
	}
	for _, l := range al.outputs {
		l.Flush()
	}
}


//同步写日志函数，logger 实例需要实现 WriteMsg 函数
func (al *AppLogger) writeToLoggers(when time.Time, msg string, level int) {
	for _, l := range al.outputs {
		err := l.WriteMsg(when, msg, level)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to WriteMsg to adapter:%v,error:%v\n", l.name, err)
		}
	}

	
}


//写日志的主要函数，支持同步写和异步写
func (al *AppLogger) writeMsg(logLevel int, msg string, v ...interface{}) error {
	if !al.init {
		al.lock.Lock()
		al.setLogger(AdapterConsole)
		al.lock.Unlock()
	}

	if len(v) > 0 {
		msg = fmt.Sprintf(msg, v...)
		//fmt.Println(msg)
	}

	msg = al.prefix + " " + msg

	when := time.Now()
	if al.enableFuncCallDepth {
		_, file, line, ok := runtime.Caller(al.loggerFuncCallDepth)
		if !ok {
			file = "???"
			line = 0
		}
		_, filename := path.Split(file)
		msg = "[" + filename + ":" + strconv.Itoa(line) + "] " + msg
	}

	//set level info in front of filename info
	if logLevel == levelLoggerImpl {
		// set to emergency to ensure all log will be print out correctly
		logLevel = LevelDebug
	} else {
		msg = levelPrefix[logLevel] + " " + msg
	}

	// 异步写实现
	if al.asynchronous {
		lm := logMsgPool.Get().(*logMsg)
		lm.level = logLevel
		lm.msg = msg
		lm.when = when
		if al.outputs != nil {
			al.msgChan <- lm
		} else {
			logMsgPool.Put(lm)
		}
	} else {
		al.writeToLoggers(when, msg, logLevel)
	}
	return nil
}

func (al *AppLogger) Close() {
	if al.asynchronous {
		al.signalChan <- "close"
		al.wg.Wait()
		close(al.msgChan)
	} else {
		al.flush()
		for _, l := range al.outputs {
			l.Destroy()
		}
		al.outputs = nil
	}
	close(al.signalChan)
}




func (al *AppLogger) Info(format string, v ...interface{}) {
	if LevelInfo > al.level {
		return
	}
	al.writeMsg(LevelInfo, format, v...)
}

func (al *AppLogger) Warn(format string, v ...interface{}) {
	if LevelWarning > al.level {
		return
	}
	al.writeMsg(LevelWarning, format, v...)
}

func (al *AppLogger) Debug(format string, v ...interface{}) {
	if LevelDebug > al.level {
		return
	}
	al.writeMsg(LevelDebug, format, v...)
}


func (al *AppLogger) Error(format string, v ...interface{}) {
	if LevelError > al.level {
		return
	}
	al.writeMsg(LevelError, format, v...)
}


//================================================== 愉快的分割线 =============================

type logWriter struct {
	sync.Mutex
	writer io.Writer
}

func newLogWriter(wr io.Writer) *logWriter {
	return &logWriter{writer: wr}
}

func (lg *logWriter) writeln(when time.Time, msg string) (int, error) {
	lg.Lock()
	h, _, _ := formatTimeHeader(when)
	n, err := lg.writer.Write(append(append(h, msg...), '\n'))
	lg.Unlock()
	return n, err
}

const (
	y1  = `0123456789`
	y2  = `0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789`
	y3  = `0000000000111111111122222222223333333333444444444455555555556666666666777777777788888888889999999999`
	y4  = `0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789`
	mo1 = `000000000111`
	mo2 = `123456789012`
	d1  = `0000000001111111111222222222233`
	d2  = `1234567890123456789012345678901`
	h1  = `000000000011111111112222`
	h2  = `012345678901234567890123`
	mi1 = `000000000011111111112222222222333333333344444444445555555555`
	mi2 = `012345678901234567890123456789012345678901234567890123456789`
	s1  = `000000000011111111112222222222333333333344444444445555555555`
	s2  = `012345678901234567890123456789012345678901234567890123456789`
	ns1 = `0123456789`
)

func formatTimeHeader(when time.Time) ([]byte, int, int) {
	y, mo, d := when.Date()
	h, mi, s := when.Clock()
	ns := when.Nanosecond() / 1000000
	//len("2006/01/02 15:04:05.123 ")==24
	var buf [24]byte

	buf[0] = y1[y/1000%10]
	buf[1] = y2[y/100]
	buf[2] = y3[y-y/100*100]
	buf[3] = y4[y-y/100*100]
	buf[4] = '/'
	buf[5] = mo1[mo-1]
	buf[6] = mo2[mo-1]
	buf[7] = '/'
	buf[8] = d1[d-1]
	buf[9] = d2[d-1]
	buf[10] = ' '
	buf[11] = h1[h]
	buf[12] = h2[h]
	buf[13] = ':'
	buf[14] = mi1[mi]
	buf[15] = mi2[mi]
	buf[16] = ':'
	buf[17] = s1[s]
	buf[18] = s2[s]
	buf[19] = '.'
	buf[20] = ns1[ns/100]
	buf[21] = ns1[ns%100/10]
	buf[22] = ns1[ns%10]

	buf[23] = ' '

	return buf[0:], d, h
}

var (
	green   = string([]byte{27, 91, 57, 55, 59, 52, 50, 109})
	white   = string([]byte{27, 91, 57, 48, 59, 52, 55, 109})
	yellow  = string([]byte{27, 91, 57, 55, 59, 52, 51, 109})
	red     = string([]byte{27, 91, 57, 55, 59, 52, 49, 109})
	blue    = string([]byte{27, 91, 57, 55, 59, 52, 52, 109})
	magenta = string([]byte{27, 91, 57, 55, 59, 52, 53, 109})
	cyan    = string([]byte{27, 91, 57, 55, 59, 52, 54, 109})

	w32Green   = string([]byte{27, 91, 52, 50, 109})
	w32White   = string([]byte{27, 91, 52, 55, 109})
	w32Yellow  = string([]byte{27, 91, 52, 51, 109})
	w32Red     = string([]byte{27, 91, 52, 49, 109})
	w32Blue    = string([]byte{27, 91, 52, 52, 109})
	w32Magenta = string([]byte{27, 91, 52, 53, 109})
	w32Cyan    = string([]byte{27, 91, 52, 54, 109})

	reset = string([]byte{27, 91, 48, 109})
)

var once sync.Once
var colorMap map[string]string

func initColor() {
	if runtime.GOOS == "windows" {
		green = w32Green
		white = w32White
		yellow = w32Yellow
		red = w32Red
		blue = w32Blue
		magenta = w32Magenta
		cyan = w32Cyan
	}
	colorMap = map[string]string{
		//by color
		"green":  green,
		"white":  white,
		"yellow": yellow,
		"red":    red,
		//by method
		"GET":     blue,
		"POST":    cyan,
		"PUT":     yellow,
		"DELETE":  red,
		"PATCH":   green,
		"HEAD":    magenta,
		"OPTIONS": white,
	}
}

// ColorByStatus return color by http code
// 2xx return Green
// 3xx return White
// 4xx return Yellow
// 5xx return Red
func ColorByStatus(code int) string {
	once.Do(initColor)
	switch {
	case code >= 200 && code < 300:
		return colorMap["green"]
	case code >= 300 && code < 400:
		return colorMap["white"]
	case code >= 400 && code < 500:
		return colorMap["yellow"]
	default:
		return colorMap["red"]
	}
}

// ColorByMethod return color by http code
func ColorByMethod(method string) string {
	once.Do(initColor)
	if c := colorMap[method]; c != "" {
		return c
	}
	return reset
}

// ResetColor return reset color
func ResetColor() string {
	return reset
}