package pgtest

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// TimingVariable 指向一份 TSV 文件；设了就把每次 Pool 的四段耗时追加进去，不设则
// 零行为变化。它只为量夹具费存在：隔离粒度不能再加，能省的只有每用例重复付的那份
// 固定开销，而省之前得先知道它占多少——数字写进票面，不凭推。
//
// 每行七列，制表符分隔，无表头（多个测试进程可能同时追加，表头没有可靠的写入时机）：
//
//	pid  用例名  建库ms  迁移ms  用例正文ms  删库ms  合计ms
//
// 「用例正文」从 Pool 返回量到 Pool 注册的清理函数开始运行；用例自己注册的清理比它
// 晚注册、先运行，因此算在正文里。「迁移」是该用例为迁移等的时间：逐用例迁移时每行
// 都有，模板库模式下只有触发建模板的那一个用例付。
const TimingVariable = "IDP_PARCEL_PGTEST_TIMING"

// phases 是一次 Pool 调用的四段耗时。
type phases struct {
	create  time.Duration
	migrate time.Duration
	body    time.Duration
	drop    time.Duration
}

var (
	timingMu   sync.Mutex
	timingFile *os.File
)

// recordTiming 把一次 Pool 的四段追加进 TimingVariable 指向的文件。写不进去只
// 报 t.Logf 不 Fail：打点是旁路，不能让一次量测把被量的用例弄红。
func recordTiming(t *testing.T, p phases) {
	t.Helper()

	path := os.Getenv(TimingVariable)
	if path == "" {
		return
	}

	timingMu.Lock()
	defer timingMu.Unlock()

	if timingFile == nil {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Logf("pgtest 打点文件打不开，本用例耗时未记录：%v", err)
			return
		}
		timingFile = f
	}

	total := p.create + p.migrate + p.body + p.drop
	line := fmt.Sprintf("%d\t%s\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\n",
		os.Getpid(), t.Name(),
		ms(p.create), ms(p.migrate), ms(p.body), ms(p.drop), ms(total))
	if _, err := timingFile.WriteString(line); err != nil {
		t.Logf("pgtest 打点写入失败，本用例耗时未记录：%v", err)
	}
}

func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
