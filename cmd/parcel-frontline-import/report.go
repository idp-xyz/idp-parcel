package main

import (
	"fmt"
	"io"
)

// rowDisposition 是一行导入的四格去向。它不是应用结果的别名——应用结果说的是业务判断
// （收寄形成 / 待识别 / 未形成 / 待确认…），这里说的是「这一行要不要人再管」：落地与重放
// 不用管，被拒要改文件，未决要重跑。两个子命令共用同一套去向，退出码才能只算一遍。
type rowDisposition uint8

const (
	rowDispositionInvalid rowDisposition = iota
	rowLanded
	rowReplayed
	rowRejected
	rowUndecided
)

func (disposition rowDisposition) String() string {
	switch disposition {
	case rowLanded:
		return "已落地"
	case rowReplayed:
		return "重放"
	case rowRejected:
		return "被拒"
	case rowUndecided:
		return "未决"
	default:
		return ""
	}
}

// rowResult 是一行的逐行结果：模板行号与行事实号给内勤对表，应用结果原名给运维对照
// 用例结果契约，去向决定退出码。
type rowResult struct {
	Line        int
	FactRef     string
	Outcome     string
	Disposition rowDisposition
	Detail      string
}

// writeImportReport 打印逐行结果与汇总。逐行一行、汇总一行，格式稳定——运维拿它对
// 纸单，不拿它做机器解析（要机器解析的下游今天不存在，不预建）。
func writeImportReport(out io.Writer, batchRef, templateVersion string, results []rowResult) {
	counts := map[rowDisposition]int{}
	for _, result := range results {
		counts[result.Disposition]++
		fmt.Fprintf(out, "第 %d 行 factRef=%s → %s [%s] %s\n",
			result.Line, result.FactRef, result.Outcome, result.Disposition, result.Detail)
	}
	fmt.Fprintf(out, "批次 %s（模板 %s）：%d 行，已落地 %d，重放 %d，被拒 %d，未决 %d\n",
		batchRef, templateVersion, len(results),
		counts[rowLanded], counts[rowReplayed], counts[rowRejected], counts[rowUndecided])
}

// exitCodeFor 由去向汇总退出码。未决压过被拒：只要还有未决，「重跑同一文件」就仍然是
// 必要动作（已落地的行重跑答重放，无副作用）；重跑收敛后剩下的被拒才轮到改文件。
func exitCodeFor(results []rowResult) int {
	code := exitLanded
	for _, result := range results {
		switch result.Disposition {
		case rowUndecided:
			return exitUndecided
		case rowRejected:
			code = exitRejected
		}
	}
	return code
}
