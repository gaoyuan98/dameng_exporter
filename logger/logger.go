package logger

import (
	"dameng_exporter/config"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger *zap.SugaredLogger // 全局日志记录器实例

// getLogLevel 根据配置的日志级别字符串返回对应的zapcore.Level
func getLogLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// InitLogger 初始化并配置全局日志记录器
func InitLogger() {
	currentTime := time.Now().Format("2006-01-02")                           // 格式化为 "YYYYMMDD_HHmmss"
	logFileName := fmt.Sprintf("./logs/dameng_exporter_%s.log", currentTime) // 日志文件路径

	// 配置日志切割器
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logFileName,                      // 日志文件路径
		MaxSize:    config.Global.GetLogMaxSize(),    // 使用配置的日志文件大小
		MaxBackups: config.Global.GetLogMaxBackups(), // 使用配置的备份数量
		MaxAge:     config.Global.GetLogMaxAge(),     // 使用配置的保留天数
		Compress:   true,                             // 启用日志文件压缩
	}

	// 创建日志写入目标
	fileWriter := zapcore.AddSync(lumberjackLogger)
	consoleWriter := zapcore.AddSync(os.Stdout)

	// 创建紧凑的编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder, // 日志级别带颜色输出
		EncodeTime:     customTimeEncoder,                // ISO8601 时间格式
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder, // 简短调用者信息
	}

	// 创建文件和控制台编码器
	fileEncoder := zapcore.NewJSONEncoder(encoderConfig)
	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)

	// 获取配置的日志级别
	logLevel := getLogLevel(config.Global.GetLogLevel())

	// 创建 zap 核心
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, zapcore.NewMultiWriteSyncer(fileWriter), logLevel),
		zapcore.NewCore(consoleEncoder, zapcore.NewMultiWriteSyncer(consoleWriter), logLevel),
	)

	// 创建日志记录器实例，并添加调用信息和堆栈跟踪
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	Logger = logger.Sugar()
}

func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05"))
}

// Sync 确保所有缓冲日志条目在程序退出前被刷新
func Sync() {
	if Logger != nil {
		_ = Logger.Sync() // 刷新日志，确保日志被完整输出
	}
}
