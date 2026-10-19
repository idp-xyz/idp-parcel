package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
)

// intakeImporter 是收寄子命令的全部依赖。事务由本层给出（收寄库写口无事务即拒），一行
// 一笔——部分成功是本口的常态，整批一笔事务会让一行的冲突回滚掉其他行已合法形成的
// 事实（UC-NO-002「部分收寄不回滚其他实物已合法形成的事实」）。
type intakeImporter struct {
	handler    *application.ReceiveDeliveredUnitHandler
	transactor bentoapp.Transactor
}

// buildIntakeImporter 装配真实收寄链：收寄库、收寄结果版本签发、Outbox 意图交付走 NO
// 真库口，与 parcel-api 的 buildReceptionOrchestration 同一套件。身份核对由参数给出：生产传
// parcelIdentityViewOf 装的 PS 关联册读口，测试可传能解析的替身直证关联收寄那一格。
//
// 跨上下文读口须经 savepoint 隔离：读口跟着走本行那一笔事务，一条 SQL 级读错误会把事务打进中止态、
// 连带作废收寄的写。服务结果读口由 parcelServiceResultViewOf 包好（ADR-0186）；身份核对读口不包——
// 它读不动本就不形成答案，整笔回滚、本行记未决正是要的结果。
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
	// 服务结果读口（票 operator-channel/25）：身份唯一成立的行向 PS 取取消与当前终局，落成标记。
	serviceResults, err := parcelServiceResultViewOf(db)
	if err != nil {
		return intakeImporter{}, err
	}
	handler, err := application.NewReceiveDeliveredUnitHandler(application.ReceiveDeliveredUnitDeps{
		Identity:       identity,
		ServiceResults: serviceResults,
		Receptions:     receptions,
		Versions:       versions,
		Downstream:     downstream,
		Clock:          clock,
	})
	if err != nil {
		return intakeImporter{}, fmt.Errorf("收寄编排：%w", err)
	}
	return intakeImporter{
		handler:    handler,
		transactor: db.Transactor(),
	}, nil
}

// parcelIdentityViewOf 装生产的身份核对读口：PS 的外部标识关联册经 NO 的防腐适配器读（ADR-0184 决定五），
// 与 parcel-api 的 buildParcelIdentityView 同一套件。
func parcelIdentityViewOf(db *bentopg.DB) (ports.ParcelIdentityView, error) {
	identifiers, err := pspostgres.NewExternalIdentifiers(db)
	if err != nil {
		return nil, fmt.Errorf("外部标识关联册：%w", err)
	}
	view, err := noparcel.NewParcelIdentityView(identifiers)
	if err != nil {
		return nil, fmt.Errorf("身份核对读口：%w", err)
	}
	return view, nil
}

// parcelServiceResultViewOf 装生产的服务结果读口：PS 的取消决定册与终局册经 NO 的防腐适配器读（票 operator-channel/25），
// 外面包一层 savepoint，读失败不连带本行收寄的写（ADR-0186）；ctx 不在事务里时直通。与 parcel-api 的
// buildParcelServiceResultView 同一套件；收寄与集运方向依据两处都经这里装，同一只读口。
func parcelServiceResultViewOf(db *bentopg.DB) (ports.ParcelServiceResultView, error) {
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		return nil, fmt.Errorf("取消决定册：%w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("终局册：%w", err)
	}
	view, err := noparcel.NewServiceResultView(cancellations, finals)
	if err != nil {
		return nil, fmt.Errorf("服务结果读口：%w", err)
	}
	isolated, err := nopostgres.NewSavepointedServiceResultView(db, view)
	if err != nil {
		return nil, fmt.Errorf("服务结果读口 savepoint：%w", err)
	}
	return isolated, nil
}

// executeIntake 逐行推进、打报告、算退出码。
func executeIntake(ctx context.Context, batch intakeBatch, importer intakeImporter, out io.Writer) int {
	results := importIntake(ctx, batch, importer)
	writeImportReport(out, batch.BatchRef, intakeTemplateVersion, results)
	return exitCodeFor(results)
}

// importIntake 逐行推进：一行一笔事务 → 交编排 → 应用结果译成去向。编排返回 Go 错误
// 时整笔回滚：命令本身被拒的行记被拒（重跑同一份文件不会变好），其余记未决；其他行照常继续。
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
		if errors.Is(err, application.ErrReceptionCommandRejected) {
			results = append(results, rowResult{
				Line: row.Line, FactRef: row.FactRef,
				Outcome:     "COMMAND_REJECTED",
				Disposition: rowRejected,
				Detail:      fmt.Sprintf("收寄命令被拒，未落库：%v", err),
			})
			continue
		}
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
//   - 待确认不带记录 → 未决：编排不再交回这一格，出现即实现缺陷；
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
			// 编排对没形成答案的尝试一律交回 error（ADR-0186），这一格不再有业务含义；出现即实现缺陷，按未决让人看。
			answer.Disposition = rowUndecided
			answer.Detail = "实现缺陷：待确认却没有记录，未落库"
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
	switch record.ServiceResultCheck {
	case ports.ServiceResultsChecked:
		if len(record.ServiceMarkers) == 0 {
			parts = append(parts, "服务结果已查：没有取消与终局")
		} else {
			parts = append(parts, "服务结果已查：标记 "+strings.Join(record.ServiceMarkers, "、"))
		}
	case ports.ServiceResultsUnreadable:
		parts = append(parts, "服务结果读不动：标记缺席不代表没有取消或终局")
	}
	if record.IdentityConflict {
		parts = append(parts, fmt.Sprintf("身份冲突（%d 个候选），方向性作业暂停", len(record.Candidates)))
	}
	if record.RefusalReason != "" {
		parts = append(parts, "拒收原因 "+record.RefusalReason)
	}
	return strings.Join(parts, "，")
}
