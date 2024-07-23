package logger

import (
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"time"
)

var Logger *zap.SugaredLogger // 全局日志记录器实例

// InitLogger 初始化并配置全局日志记录器
func InitLogger() {
	currentTime := time.Now().Format("2006-01-02")                           // 格式化为 "YYYYMMDD_HHmmss"
	logFileName := fmt.Sprintf("./logs/dameng_exporter_%s.log", currentTime) // 日志文件路径

	// 配置日志切割器
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logFileName, // 日志文件路径
		MaxSize:    10,          // 每个日志文件最大为 10 MB
		MaxBackups: 3,           // 保留最近的 3 个日志文件备份
		MaxAge:     31,          // 保留 28 天的日志文件
		Compress:   true,        // 启用日志文件压缩
	}

	// 创建日志写入目标
	fileWriter := zapcore.AddSync(lumberjackLogger)
	consoleWriter := zapcore.AddSync(zapcore.Lock(os.Stdout))

	// 创建编码器配置
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"                   // 时间键名称
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // 时间编码器，使用 ISO8601 时间格式

	// 创建文件和控制台编码器
	fileEncoder := zapcore.NewJSONEncoder(encoderConfig)
	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)

	// 创建 zap 核心
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, fileWriter, zapcore.DebugLevel),
		zapcore.NewCore(consoleEncoder, consoleWriter, zapcore.DebugLevel),
	)

	//logger := zap.New(core, zap.AddCaller()) // 创建日志记录器实例
	// 创建日志记录器实例，并添加调用信息和堆栈跟踪
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	Logger = logger.Sugar()
}

// Sync 确保所有缓冲日志条目在程序退出前被刷新
func Sync() {
	if Logger != nil {
		Logger.Sync() // 刷新日志
	}
}
