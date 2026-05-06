package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/sirupsen/logrus"
)

func InitLogger(cfg *config.LoggingConfig) error {
	if cfg == nil {
		cfg = &config.LoggingConfig{
			Level:  "info",
			Format: "console",
			Output: "stdout",
		}
	}

	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)

	switch cfg.Format {
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	default:
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}

	var output io.Writer
	switch cfg.Output {
	case "stdout":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:
		if cfg.Output != "" {
			dir := filepath.Dir(cfg.Output)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create log directory: %w", err)
			}
			file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}
			output = file
		} else {
			output = os.Stdout
		}
	}

	logrus.SetOutput(output)

	logrus.WithFields(logrus.Fields{
		"level":  level.String(),
		"format": cfg.Format,
		"output": cfg.Output,
	}).Info("Logger initialized")

	return nil
}
