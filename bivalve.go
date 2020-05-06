package bivalve

// BIVALVE -- the most common organism for ecosystem biomarker monitoring

// requirements
// -- possibilty for multiple files (log and httpaccess) (or can log go to stderr and httpaccess go to stdout?)
// -- single interface for logging or command line?
// ---- this requires being able to control the entire format to include / not include headers
// -- debug, info, and error log levels
// -- config from args, env, or explicit
// -- Display application file/line #'s
// -- wrap the logging implementation to allow for future change

// https://dave.cheney.net/2015/11/05/lets-talk-about-logging
// excellent article about using less logging levels.

// NOT USING GLOG b/c GLOG headers are static
// https://hpc.nih.gov/development/glog.html
// good overview & disambiguation of glog (especially severity vs. VLOG() )
// TL;DR : Severity is the message importance.
// 		While LOG() will 'Show all VLOG(m) messages for m less or equal the value of this flag' VLOG() will log at INFO severity.
//		ie, if v = 4, VLOG(3) will be displayed

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

var (
	valvelog       *log.Logger
	level          int8
	terminalOutput bool
)

// LogConfig is the struct for passing in logging configuration that we care about.
// 	If not set, configuration will be pulled from command line flags or environment variables
type LogConfig struct {
	Output   string `toml:"output"`
	Level    string `toml:"level"`
	Filename string `toml:"filename"`
	// DisplayMinimal will if true display a minimal log output.  If false, a default log line prefix with date, file, line number will be displayed
	DisplayMinimal bool `toml:"displayMinimal"`
	TerminalOutput bool `toml:"displayMinimal"`
}

const (
	configOutputKey         = "BIVALVE_OUTPUT"
	configLevelKey          = "BIVALVE_LEVEL"
	configFilenameKey       = "BIVALVE_FILENAME"
	configDisplayMinimalKey = "BIVALVE_DISPLAY_MINIMAL"

	// ApacheFormatPattern is the default format used for apache access logs
	ApacheFormatPattern = "%s - - [%s] \"%s %d %d\" %f\n"
	debugLevel          = 4
	infoLevel           = 2
	errorLevel          = 1

	ErrorColor = "\033[1;31m%s\033[0m"
	DebugColor = "\033[0;36m%s\033[0m"
)

type webLoggingHandler struct {
	handler http.Handler
}

type statusWriter struct {
	http.ResponseWriter
	status int
	length int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	n, err := w.ResponseWriter.Write(b)
	w.length += n
	return n, err
}

func init() {
	// pull configuration from config/log.config by default
	conf := &LogConfig{}
	conf.Output = *flag.String(configOutputKey, getEnvConfigValueOr(configOutputKey, "stdout").(string), "where to output logs; 'stdout', 'file', or 'both'.")
	conf.Level = *flag.String(configLevelKey, getEnvConfigValueOr(configLevelKey, "info").(string), "log level; 'debug', 'info', 'error'")
	conf.Filename = *flag.String(configFilenameKey, getEnvConfigValueOr(configFilenameKey, "bivalve.log").(string), "log filename")
	conf.DisplayMinimal = *flag.Bool(configDisplayMinimalKey, getEnvConfigValueOr(configDisplayMinimalKey, false).(bool), "log filename")

	Configure(conf)
}

// Configure will set the logger instance configuration should an application want to explictly set the configuration
func Configure(conf *LogConfig) {
	writer := os.Stderr
	switch conf.Output {
	case "file":
		f, err := os.OpenFile(conf.Filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println(err)
		}
		writer = f
		defer f.Close()
	case "stdout":
		writer = os.Stdout

	}

	switch conf.Level {
	case "debug":

		level = debugLevel
	case "error":

		level = errorLevel
	default:

		level = infoLevel
	}

	if conf.TerminalOutput {
		terminalOutput = true
	} else {
		terminalOutput = false
	}

	logflags := log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC | log.Lshortfile
	if conf.DisplayMinimal {
		logflags = 0

	}
	valvelog = log.New(writer, "", logflags)
	Debugf("Log Config set to : %+v", conf)
}

// Info will log a string message
func Info(s string) {
	if level >= infoLevel {
		valvelog.Output(2, s)
	}
}

// Infof will log a formatted string message
func Infof(s string, args ...interface{}) {

	if level >= infoLevel {
		valvelog.Output(2, fmt.Sprintf(s, args...))
	}
}

// Debug will log a string message
func Debug(s string) {
	if level >= debugLevel {
		if terminalOutput {
			s = fmt.Sprintf(DebugColor, s)
		}
		valvelog.Output(2, s)
	}
}

// Debugf will log a formatted string message
func Debugf(s string, args ...interface{}) {
	if level >= debugLevel {
		msg := fmt.Sprintf(s, args...)
		if terminalOutput {
			msg = fmt.Sprintf(DebugColor, msg)
		}
		valvelog.Output(2, msg)
	}
}

// Error will log a string message
func Error(s string) {
	if terminalOutput {
		s = fmt.Sprintf(ErrorColor, s)
	}
	valvelog.Output(2, s)

}

// Errorf will log a formatted string message
func Errorf(s string, args ...interface{}) {
	msg := fmt.Sprintf(s, args...)
	if terminalOutput {
		msg = fmt.Sprintf(ErrorColor, msg)
	}
	valvelog.Output(2, msg)
}

// RequestLogHandler http request log handler
func RequestLogHandler(h http.Handler) http.Handler {

	return webLoggingHandler{handler: h}
}

// TODO add httpstatus, add response size
func (h webLoggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	start := time.Now()
	sw := statusWriter{ResponseWriter: w}

	h.handler.ServeHTTP(&sw, r)

	endingTime := time.Now().UTC()

	type ApacheLogRecord struct {
		ip                    string
		time                  time.Time
		method, uri, protocol string
		status                int
		responseBytes         int64
		elapsedTime           time.Duration
	}

	record := &ApacheLogRecord{
		ip:            r.RemoteAddr,
		time:          time.Time{},
		method:        r.Method,
		uri:           r.RequestURI,
		protocol:      r.Proto,
		status:        sw.status,
		elapsedTime:   endingTime.Sub(start),
		responseBytes: int64(sw.length),
	}
	timeFormatted := record.time.Format("02/Jan/2006 03:04:05")
	requestLine := fmt.Sprintf("%s %s %s", record.method, record.uri, record.protocol)
	Infof(ApacheFormatPattern, record.ip, timeFormatted, requestLine, record.status, record.responseBytes,
		record.elapsedTime.Seconds())

}

// getConfigValue will check if an env variable is set.  If
func getEnvConfigValueOr(envKey string, defaultValue interface{}) interface{} {
	var configValue interface{}
	if env := os.Getenv(envKey); env != "" {
		configValue = env
	} else {
		configValue = defaultValue
	}
	return configValue
}

func header() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "???"
		line = 1
	} else {
		slash := strings.LastIndex(file, "/")
		if slash >= 0 {
			file = file[slash+1:]
		}
	}
	return fmt.Sprintf("[%s:%d]", file, line)
}
