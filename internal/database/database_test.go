package database

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/util"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGormLoggerTraceDoesNotLogRecordNotFoundAsExecutionFailure(t *testing.T) {
	if err := util.InitLogger(&util.LoggerConfig{Level: "debug", Format: "console", Output: "stdout"}); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	var output bytes.Buffer
	util.GetLogger(util.LogTypeSQL).SetOutput(&output)

	l := &gormLogger{LogLevel: logger.Error}
	l.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT * FROM users WHERE username = ?", 0
	}, gorm.ErrRecordNotFound)

	if strings.Contains(output.String(), "SQL execution failed") {
		t.Fatalf("record not found should not be logged as execution failure: %s", output.String())
	}
}
