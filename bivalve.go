package bivalve

// BIVALVE -- the most common organism for ecosystem biomarker monitoring

// requirements
// -- possibilty for multiple files (log and httpaccess) (or can log go to stderr and httpaccess go to stdout?)
// -- single interface for logging or command line?
// this requires being able to control the entire format
// -- debug, info, and error log levels
// -- config from args, env, or explicit

// https://dave.cheney.net/2015/11/05/lets-talk-about-logging
// excellent article about using less logging levels.

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

	"github.com/golang/glog"
)

var (
	valvelog *log.Logger
)

// LogConfig is the struct for passing in logging configuration that we care about.
// 	If not set, configuration will be pulled from command line flags or environment variables
type LogConfig struct {
	Output   string `toml:"output"`
	Level    string `toml:"level"`
	Filename string `toml:"filename"`
}

const (
	configOutputKey   = "BIVALVE_OUTPUT"
	configLevelKey    = "BIVALVE_LEVEL"
	configFilenameKey = "BIVALVE_FILENAME"
	// ApacheFormatPattern is the default format used for apache access logs
	ApacheFormatPattern = "%s - - [%s] \"%s %d %d\" %f\n"
)

type webLoggingHandler struct {
	handler http.Handler
}

func init() {
	// pull configuration from config/log.config by default
	conf := &LogConfig{}
	conf.Output = *flag.String(configOutputKey, getEnvConfigValueOr(configOutputKey, "stdout").(string), "where to output logs; 'stdout', 'file', or 'both'.")
	conf.Level = *flag.String(configLevelKey, getEnvConfigValueOr(configLevelKey, "info").(string), "log level; 'debug', 'info', 'error'")
	conf.Filename = *flag.String(configFilenameKey, getEnvConfigValueOr(configFilenameKey, "bivalve.log").(string), "log filename")

	Configure(conf)
}

// Configure will set the logger instance configuration should an application want to explictly set the configuration
func Configure(conf *LogConfig) {
	switch conf.Output {
	case "stdout":
		flag.Set("logtostderr", "1")
	case "file":
		flag.Set("log_dir", conf.Filename)
		glog.Warningf("Setting output file to %s", conf.Filename)
	}

	switch conf.Level {
	case "debug":
		flag.Set("stderrthreshold", "INFO")
		flag.Set("v", "4")
	case "error":
		flag.Set("stderrthreshold", "ERROR")
		flag.Set("v", "1")
	default:
		flag.Set("stderrthreshold", "INFO")
		flag.Set("v", "2")
	}
	flag.Parse()
}

func Info(args ...interface{}) {
	glog.V(2).Info(args...)
}

func Infof(s string, args ...interface{}) {
	file, line := header()
	fmt.Printf("%s:%d", file, line)
	glog.V(2).Infof(s, args...)
}

func Debug(args ...interface{}) {
	glog.V(4).Info(args...)
}

func Debugf(s string, args ...interface{}) {
	glog.V(4).Infof(s, args...)
}

func Error(args ...interface{}) {
	glog.Error(args...)
}

func Errorf(s string, args ...interface{}) {
	glog.Errorf(s, args...)
}

func RequestLogHandler(h http.Handler) http.Handler {

	return webLoggingHandler{handler: h}
}

// TODO add httpstatus, add response size
func (h webLoggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	start := time.Now()

	h.handler.ServeHTTP(w, r)

	endingTime := time.Now().UTC()

	//		status := fn.status
	//		if status == 0 {
	//			status = 200
	//		}
	type ApacheLogRecord struct {
		ip                    string
		time                  time.Time
		method, uri, protocol string
		status                int
		responseBytes         int64
		elapsedTime           time.Duration
	}

	record := &ApacheLogRecord{
		ip:          r.RemoteAddr,
		time:        time.Time{},
		method:      r.Method,
		uri:         r.RequestURI,
		protocol:    r.Proto,
		status:      http.StatusOK,
		elapsedTime: endingTime.Sub(start),
	}
	timeFormatted := record.time.Format("02/Jan/2006 03:04:05")
	requestLine := fmt.Sprintf("%s %s %s", record.method, record.uri, record.protocol)
	glog.Info(ApacheFormatPattern, record.ip, timeFormatted, requestLine, record.status, record.responseBytes,
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

func header() (string, int) {
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
	return file, line
}
