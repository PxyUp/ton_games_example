package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger interface {
	Infof(msg string, fields ...any)
	Info(msg string)
	Infow(msg string, fields ...string)
	Debug(msg string)
	Debugw(msg string, fields ...string)
	Debugf(msg string, fields ...any)
	Error(msg string)
	Errorf(msg string, fields ...any)
	Errorw(msg string, fields ...string)
	With(fields ...string) Logger
}

type null struct {
}

func (n null) With(fields ...string) Logger {
	return null{}
}

func (n null) Infof(msg string, fields ...any) {
}

func (n null) Info(msg string) {
}

func (n null) Infow(msg string, fields ...string) {
}

func (n null) Debugf(msg string, fields ...any) {
}

func (n null) Debug(msg string) {
}

func (n null) Debugw(msg string, fields ...string) {
}

func (n null) Error(msg string) {
}

func (n null) Errorf(msg string, fields ...any) {
}

func (n null) Errorw(msg string, fields ...string) {
}

type zapLogger struct {
	logger *zap.Logger
}

func (z *zapLogger) Debug(msg string) {
	z.logger.Debug(msg)
}

func (z *zapLogger) Debugw(msg string, fields ...string) {
	z.logger.With(makeFields(fields)...).Debug(msg)
}

func (z *zapLogger) Debugf(msg string, fields ...any) {
	z.logger.Debug(fmt.Sprintf(msg, fields...))
}

func makeFields(fields []string) []zap.Field {
	zapField := make([]zap.Field, len(fields)/2)

	for i := 0; i < len(fields); i += 2 {
		zapField[i/2] = zap.String(fields[i], fields[i+1])
	}

	return zapField
}

func (z *zapLogger) Infof(msg string, fields ...any) {
	z.logger.Info(fmt.Sprintf(msg, fields...))
}

func (z *zapLogger) With(fields ...string) Logger {
	return &zapLogger{
		logger: z.logger.With(makeFields(fields)...),
	}
}

func (z *zapLogger) Info(msg string) {
	z.logger.Info(msg)
}

func (z *zapLogger) Infow(msg string, fields ...string) {
	z.logger.With(makeFields(fields)...).Info(msg)
}

func (z *zapLogger) Error(msg string) {
	z.logger.Info(msg)
}

func (z *zapLogger) Errorf(msg string, fields ...any) {
	z.logger.Error(fmt.Sprintf(msg, fields...))
}

func (z *zapLogger) Errorw(msg string, fields ...string) {
	z.logger.With(makeFields(fields)...).Error(msg)
}

func NewLogger(lvl string) *zapLogger {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.RFC3339TimeEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(config)
	defaultLogLevel := zapcore.DebugLevel
	if lvl != "" {
		_ = defaultLogLevel.UnmarshalText([]byte(lvl))
	}

	return &zapLogger{
		logger: zap.New(zapcore.NewTee(zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), defaultLogLevel)), zap.AddCaller()),
	}
}
