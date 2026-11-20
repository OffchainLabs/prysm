// Package logrus is a minimal stub of github.com/sirupsen/logrus for analyzer tests.
package logrus

type Fields map[string]any

type Entry struct{}

func WithField(key string, value any) *Entry            { return &Entry{} }
func WithFields(fields Fields) *Entry                   { return &Entry{} }
func Info(args ...any)                                  {}
func Infof(format string, args ...any)                  {}
func (e *Entry) WithField(key string, value any) *Entry { return e }
func (e *Entry) WithFields(fields Fields) *Entry        { return e }
func (e *Entry) WithError(err error) *Entry             { return e }
func (e *Entry) Debug(args ...any)                      {}
func (e *Entry) Info(args ...any)                       {}
func (e *Entry) Warn(args ...any)                       {}
func (e *Entry) Error(args ...any)                      {}
func (e *Entry) Println(args ...any)                    {}
func (e *Entry) Debugf(format string, args ...any)      {}
func (e *Entry) Infof(format string, args ...any)       {}
func (e *Entry) Warnf(format string, args ...any)       {}
func (e *Entry) Errorf(format string, args ...any)      {}
