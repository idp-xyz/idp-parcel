package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

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

// buildIntakeImporter 装配真实收寄链：收寄库、收寄结果版本签发、Outbox 意图交付走 NO
// 真库口，与 parcel-api 的 buildReceptionOrchestration 同一套件。身份核对缝由参数给出：
// 生产传显式未配置替身，测试传能解析的替身证整条链会落库——也就是 PS 侧提供方到位后
// 唯一要换的那一格。
func buildIntakeImporter(db *bentopg.DB, clock ports.Clock, identity ports.ParcelIdentityView) (intakeImporter, error) {
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return intakeImporter{}, fmt.Errorf("收寄库：%w", err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		return intakeImporter{}, fmt.Errorf("收寄结果版本签发：%w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return intakeImporter{}, fmt.Errorf("outbox 存储：%w", err)
	}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, store, clock)
	if err != nil {
		return intakeImporter{}, fmt.Errorf("收寄意图交付：%w", err)
	}
	_, unconfigured := identity.(unconfiguredParcelIdentityView)
	handler := application.NewReceiveDeliveredUnitHandler(application.ReceiveDeliveredUnitDeps{
		Identity:   identity,
		Receptions: receptions,
		Versions:   versions,
		Downstream: downstream,
		Clock:      clock,
	})
	return intakeImporter{
		handler:            handler,
		transactor:         db.Transactor(),
		identityConfigured: !unconfigured,
	}, nil
}

// executeIntake 开工前先把身份核对缝的状态说清，再逐行推进、打报告、算退出码。
func executeIntake(ctx context.Context, batch intakeBatch, importer intakeImporter, out io.Writer) int {
	if !importer.identityConfigured {
		fmt.Fprintf(out, "注意：身份核对缝未配置（.scratch/ps-external-mark-relations/01）——%s 行将答 RECEPTION_UNDECIDED 且不落库；%s / %s 行不经身份核对，照常落库\n",
			claimReceived, claimRefused, claimScanOnly)
	}
	results := importIntake(ctx, batch, importer)
	writeImportReport(out, batch.BatchRef, intakeTemplateVersion, results)
	return exitCodeFor(results)
}

// importIntake 逐行推进：一行一笔事务 → 交编排 → 应用结果译成去向。编排返回 Go 错误
// 时整笔回滚，该行记未决；其他行照常继续。
func importIntake(ctx context.Context, batch intakeBatch, importer intakeImporter) []rowResult {
	results := make([]rowResult, 0, len(batch.Rows))
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
			results = append(results, rowResult{
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
func intakeRowDisposition(row intakeRow, result application.ReceiveDeliveredUnitResult) rowResult {
	outcome := result.Outcome()
	answer := rowResult{Line: row.Line, FactRef: row.FactRef, Outcome: outcome.String()}
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
