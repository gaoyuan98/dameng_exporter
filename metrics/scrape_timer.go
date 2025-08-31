package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// ScrapeTimer 用于跟踪Prometheus scrape的时间统计
type ScrapeTimer struct {
	mu              sync.RWMutex
	scrapeStartTime time.Time // 当前scrape的开始时间
	scrapeID        int64     // 当前scrape ID
}

var (
	// 全局scrape计时器实例
	globalScrapeTimer = &ScrapeTimer{
		scrapeStartTime: time.Now(),
		scrapeID:        0,
	}

	// scrape计数器
	scrapeCounter int64 = 0
)

// NewScrape 标记新的scrape开始
func NewScrape() {
	globalScrapeTimer.mu.Lock()
	defer globalScrapeTimer.mu.Unlock()

	atomic.AddInt64(&scrapeCounter, 1)
	globalScrapeTimer.scrapeStartTime = time.Now()
	globalScrapeTimer.scrapeID = scrapeCounter
}

// GetElapsedTime 获取从scrape开始到现在的经过时间
func GetElapsedTime() time.Duration {
	globalScrapeTimer.mu.RLock()
	defer globalScrapeTimer.mu.RUnlock()

	return time.Since(globalScrapeTimer.scrapeStartTime)
}
