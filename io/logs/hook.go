package logs

import (
	"io"

	"github.com/sirupsen/logrus"
)

type HookIdentifier string

type WriterHook struct {
	AllowedLevels []logrus.Level
	Writer        io.Writer
	Formatter     logrus.Formatter
	Identifier    HookIdentifier
}

func (hook *WriterHook) Levels() []logrus.Level {
	if len(hook.AllowedLevels) == 0 {
		return logrus.AllLevels
	}
	return hook.AllowedLevels
}

func (hook *WriterHook) Fire(entry *logrus.Entry) error {
	val, ok := entry.Data[LogTargetField]
	if ok && val != hook.Identifier {
		return nil
	}

	line, err := hook.Formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = hook.Writer.Write(line)
	return err
}
