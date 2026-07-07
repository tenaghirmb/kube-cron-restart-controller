package cronutils

import (
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

var (
	parser     cron.Parser
	parserOnce sync.Once
)

// Get5FieldParser 返回一个线程安全的标准 5 位 Cron 解析器 (分 时 日 月 周)
// 外部调用者无法修改此解析器实例，类似于常量
func Get5FieldParser() cron.Parser {
	parserOnce.Do(func() {
		parser = cron.NewParser(
			cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		)
	})
	return parser
}

func ValidateTimezone(tz string) bool {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return false
	}

	_, err := time.LoadLocation(tz)
	if err != nil {
		return false
	}

	return true
}
