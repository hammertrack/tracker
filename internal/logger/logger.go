package logger

import (
	"fmt"
	"time"

	"github.com/hammertrack/tracker/internal/color"
	"github.com/hammertrack/tracker/internal/utils"
)

type CustomLogger struct{}

func (writer CustomLogger) Write(bytes []byte) (int, error) {
	now := time.Now().Format(time.RFC3339)
	return fmt.Printf("[%s] ► %s",
		color.String(color.Yellow, now), color.String(color.Green, utils.ByteToStr(bytes)),
	)
}

func New() *CustomLogger {
	return new(CustomLogger)
}
