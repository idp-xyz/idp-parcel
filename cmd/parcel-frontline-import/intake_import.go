package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// rowDisposition 是一行导入的四格去向。它不是应用结果的别名——应用结果说的是业务判断
// （收寄形成 / 待识别 / 未形成 / 待确认…），这里说的是「这一行要不要人再管」：落地与重放
// 不用管，被拒要改文件，未决要重跑。
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

// intakeRowResult 是一行的逐行结果：模板行号与行事实号给内勤对表，应用结果原名给
// 运维对照 UC-NO-002 结果契约，去向决定退出码。
type intakeRowResult struct {
	Line        int
	FactRef     string
	Outcome     string
	Disposition rowDisposition
	Detail      string
}

// intakeImporter 是收寄子命令的全部依赖。事务由本层给出（收寄库写口无事务即拒），一行
// 一笔——部分成功是本口的常态，整批一笔事务会让一行的冲突回滚掉其他行已合法形成的
// 事实（UC-NO-002「部分收寄不回滚其他实物已合法形成的事实」）。
type intakeImporter struct {
	handler    *application.ReceiveDeliveredUnitHandler
	transactor bentoapp.Transactor
	// identityConfigured 为假时身份核对缝接的是显式未配置替身：RECEIVED 行必然答未决且
	// 不落库。本口在开工前把这一点打印出来，而不是让内勤从一列 CONT- 引用里自己猜。
	identityConfigured bool
}

// importIntake 逐行推进：一行一笔事务 → 交编排 → 应用结果译成去向。编排返回 Go 错误
// 时整笔回滚，该行记未决；其他行照常继续。
func importIntake(ctx context.Context, batch intakeBatch, importer intakeImporter) []intakeRowResult {
	results := make([]intakeRowResult, 0, len(batch.Rows))
	for _, row := range batch.Rows {
		var handled application.ReceiveDeliveredUnitResult
		err := importer.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			result, handleErr := importer.handler.Handle(txCtx, row.Command)
			if handleErr != nil {
				return handleErr
			}
			handled = result
			return nil
		})
		if err != nil {
			results = append(results, intakeRowResult{
				Line: row.Line, FactRef: row.FactRef,
				Outcome:     "NO_ANSWER_FORMED",
				Disposition: rowUndecided,
				Detail:      fmt.Sprintf("编排未形成答案，已回滚：%v", err),
			})
			continue
		}
		results = append(results, intakeRowDisposition(row, handled))
	}
	return results
}

// intakeRowDisposition 把 UC-NO-002 的六格结果译成去向，不增不减：
//   - 收寄形成 / 待识别 / 未形成 → 已落地：三格都是越过提交边界的判断（未形成也是一条
//     真实的拒收事实）；
//   - 待确认且带记录 → 已落地：SCAN_ONLY 行的判断本就是「待确认」，记录已提交；
//   - 待确认不带记录 → 未决：依赖故障或身份核对缝不通，什么都没落库，重跑续办；
//   - 已有结果 → 重放：同一来源身份同一内容，原结果原样；
//   - 来源冲突 → 被拒：同一行事实号换了内容，绝不覆盖，人核对纸单再改文件。
func intakeRowDisposition(row intakeRow, result application.ReceiveDeliveredUnitResult) intakeRowResult {
	outcome := result.Outcome()
	answer := intakeRowResult{Line: row.Line, FactRef: row.FactRef, Outcome: outcome.String()}
	record, hasRecord := result.Record()

	switch outcome {
	case application.NodeIntakeFormed, application.UnitPendingIdentification, application.NodeIntakeNotFormed:
		answer.Disposition = rowLanded
		answer.Detail = describeReception(record)
	case application.ReceptionUndecided:
		if hasRecord {
			answer.Disposition = rowLanded
			answer.Detail = "扫描来源已保全，收寄判断待确认，未建立控制"
		} else {
			answer.Disposition = rowUndecided
			answer.Detail = "未落库；续办引用 " + result.ContinuationReference()
		}
	case application.ReceptionExistingResult:
		answer.Disposition = rowReplayed
		answer.Detail = "原结果 " + record.Kind.String() + "；" + describeReception(record)
	case application.ReceptionSourceConflict:
		answer.Disposition = rowRejected
		answer.Detail = "同一行事实号已在册且内容不同——绝不覆盖；核对纸单：若是另一件事实请换行事实号"
	default:
		// 编排交回一个它自己都不认识的格是实现坏了，不是业务答案；按未决让人看。
		answer.Outcome = fmt.Sprintf("UNKNOWN(%d)", outcome)
		answer.Disposition = rowUndecided
		answer.Detail = "未知应用结果"
	}
	if handoff := result.IntakeHandoffReference(); handoff != "" {
		answer.Detail += "；意图未交出（" + handoff + "），重跑同一文件会重发同一份"
	}
	return answer
}

func describeReception(record ports.ReceptionRecord) string {
	var parts []string
	if version := record.Intake.Version().String(); version != "" {
		parts = append(parts, "收寄版本 "+version)
		if association, associated := record.Intake.Association(); associated {
			parts = append(parts, "关联 "+association.String())
		}
	}
	if record.Control.Active() {
		parts = append(parts, "控制依据 "+record.Control.Basis().String())
	}
	if record.IdentityConflict {
		parts = append(parts, fmt.Sprintf("身份冲突（%d 个候选），方向性作业暂停", len(record.Candidates)))
	}
	if record.RefusalReason != "" {
		parts = append(parts, "拒收原因 "+record.RefusalReason)
	}
	return strings.Join(parts, "，")
}

// writeIntakeReport 打印逐行结果与汇总。逐行一行、汇总一行，格式稳定——运维拿它对
// 纸单，不拿它做机器解析（要机器解析的下游今天不存在，不预建）。
func writeIntakeReport(out io.Writer, batch intakeBatch, results []intakeRowResult) {
	counts := map[rowDisposition]int{}
	for _, result := range results {
		counts[result.Disposition]++
		fmt.Fprintf(out, "第 %d 行 factRef=%s → %s [%s] %s\n",
			result.Line, result.FactRef, result.Outcome, result.Disposition, result.Detail)
	}
	fmt.Fprintf(out, "批次 %s（模板 %s）：%d 行，已落地 %d，重放 %d，被拒 %d，未决 %d\n",
		batch.BatchRef, intakeTemplateVersion, len(results),
		counts[rowLanded], counts[rowReplayed], counts[rowRejected], counts[rowUndecided])
}

// exitCodeFor 由去向汇总退出码。未决压过被拒：只要还有未决，「重跑同一文件」就仍然是
// 必要动作（已落地的行重跑答重放，无副作用）；重跑收敛后剩下的被拒才轮到改文件。
func exitCodeFor(results []intakeRowResult) int {
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
