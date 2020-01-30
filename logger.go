package main

import "log"

type Logger struct {
	indent int
}

func NewLogger() *Logger {
	m := new(Logger)
	m.indent = 0
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
		r += "  "
	}
	return r
}

func (a *Logger) Trace(msg string, v ...interface{}) {
	log.Printf("[TRACE] "+a.spaces()+msg, v...)
}

func (a *Logger) Debug(msg string, v ...interface{}) {
	log.Printf("[DEBUG] "+a.spaces()+msg, v...)
}

func (a *Logger) Info(msg string, v ...interface{}) {
	log.Printf("[INFO] "+a.spaces()+msg, v...)
}
func (a *Logger) Warn(msg string, v ...interface{}) {
	log.Printf("[WARN] "+a.spaces()+msg, v...)
}

func (a *Logger) Error(msg string, v ...interface{}) {
	log.Printf("[ERROR] "+a.spaces()+msg, v...)
}
