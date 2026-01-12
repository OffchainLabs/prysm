package logs

import (
	"io"

	"github.com/sirupsen/logrus"
)

type WriterHook struct {
	AllowedLevels []logrus.Level
	Writer        io.Writer
	Formatter     logrus.Formatter
}

func (hook *WriterHook) Levels() []logrus.Level {
	if hook.AllowedLevels == nil || len(hook.AllowedLevels) == 0 {
		return logrus.AllLevels
	}
	return hook.AllowedLevels
}

func (hook *WriterHook) Fire(entry *logrus.Entry) error {
	line, err := hook.Formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = hook.Writer.Write(line)
	return err
}
