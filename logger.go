package main

import "log"

type Logger struct {
	indent int
	level  LogLevel
}

type LogLevel int

const (
	LogLevel_TRACE LogLevel = iota
	LogLevel_DEBUG
	LogLevel_INFO
	LogLevel_WARN
	LogLevel_ERROR
)

func NewLogger(level LogLevel) *Logger {
	m := new(Logger)
	m.indent = 0
	m.level = level
	return m
}

func (a *Logger) Indent() {
	a.indent = a.indent + 1
}

func (a *Logger) Dedent() {
	a.indent = a.indent - 1
}

func (a *Logger) spaces() string {
	r := ""
	for i := 0; i < a.indent; i++ {
		r += " "
	}
	return r
}

func (a *Logger) Trace(msg string, v ...interface{}) {
	if a.level <= LogLevel_TRACE {
		log.Printf("[TRACE] "+a.spaces()+msg, v...)
	}
}

func (a *Logger) Debug(msg string, v ...interface{}) {
	if a.level <= LogLevel_DEBUG {
		log.Printf("[DEBUG] "+a.spaces()+msg, v...)
	}
}

func (a *Logger) Info(msg string, v ...interface{}) {
	if a.level <= LogLevel_INFO {
		log.Printf("[INFO] "+a.spaces()+msg, v...)
	}
}
func (a *Logger) Warn(msg string, v ...interface{}) {
	if a.level <= LogLevel_WARN {
		log.Printf("[WARN] "+a.spaces()+msg, v...)
	}
}

func (a *Logger) Error(msg string, v ...interface{}) {
	if a.level <= LogLevel_ERROR {
		log.Printf("[ERROR] "+a.spaces()+msg, v...)
	}
}
