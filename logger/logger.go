package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	zlog "github.com/rs/zerolog"
)

var log zlog.Logger

var once sync.Once

func InitWithLevel(level zlog.Level) {
	once.Do(func() {
		var output io.Writer = zlog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
		log = zlog.New(output).Level(level).With().Timestamp().Logger()
	})
}

func Log() *zlog.Logger {
	return &log
}

func Debug(msg string) {
	log.Debug().Msg(msg)
}

func Debugf(format string, v ...interface{}) {
	log.Debug().Msgf(format, v...)
}

func Info(msg string) {
	log.Info().Msg(msg)
}

func Infof(format string, v ...interface{}) {
	log.Info().Msgf(format, v...)
}

func Warn(msg string) {
	log.Warn().Msg(msg)
}

func Warnf(format string, v ...interface{}) {
	log.Warn().Msgf(format, v...)
}

func Error(msg string) {
	log.Error().Msg(msg)
}

func Errorf(format string, v ...interface{}) {
	log.Error().Msgf(format, v...)
}

func Fatal(msg string) {
	log.Fatal().Msg(msg)
}

func Fatalf(format string, v ...interface{}) {
	log.Fatal().Msg(fmt.Sprintf(format, v...))
}

func ErrMsg(e error, msg string) {
	log.Err(e).Msg(msg)
}

func Err(e error) {
	log.Err(e).Msg("")
}

func ErrMsgf(e error, format string, v ...interface{}) {
	log.Err(e).Msgf(format, v...)
}

func Request(method string, ip string, path string, status int, message string) {
	level := zlog.DebugLevel
	if status >= 400 && status < 500 {
		level = zlog.WarnLevel
	} else if status >= 500 {
		level = zlog.ErrorLevel
	}
	log.WithLevel(level).Str("ip", ip).Int("status", status).Str("path", path).Str("method", method).Msg(message)
}
