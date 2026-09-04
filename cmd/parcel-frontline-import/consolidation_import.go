package main

import (
	"context"
	"fmt"
	"io"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// consolidationImporter 是集运子命令的全部依赖。事务由本层给出（单元库与来源事实登记的
// 写口无事务即拒），一行一笔——理由同收寄口，这里还多一层：集运行之间有先后依赖（开启
// 在前、移入在后、封装再后），一行被拒不该连累它前面已合法形成的作业事实。
//
// 与收寄口不同，集运六口没有身份核对那条缝：成员是作业实物号，不经 PS 侧解析。因此没有
// 「显式未配置」要在开工前打印，样例九行今天就能全部落地。
type consolidationImporter struct {
	handler    *application.ConsolidateParcelsHandler
	transactor bentoapp.Transactor
}

// buildConsolidationImporter 装配真实集运链：单元库兼容纳索引、来源事实登记、封装快照意图
// 经 Outbox 交付，全部走 NO 真库口。本口是 ConsolidateParcelsHandler 今天唯一的生产装配点
// ——一线作业客户端（ADR-0021）尚未开工，parcel-api 也没有集运端点。
func buildConsolidationImporter(db *bentopg.DB, clock ports.Clock) (consolidationImporter, error) {
	units, err := nopostgres.NewConsolidationUnits(db)
	if err != nil {
		return consolidationImporter{}, fmt.Errorf("集运单元库：%w", err)
	}
	facts, err := nopostgres.NewConsolidationFacts(db)
	if err != nil {
		return consolidationImporter{}, fmt.Errorf("集运作业事实登记：%w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return consolidationImporter{}, fmt.Errorf("outbox 存储：%w", err)
	}
	downstream, err := nopostgres.NewOutboxSealedSnapshotHandoff(db, store, clock)
	if err != nil {
		return consolidationImporter{}, fmt.Errorf("封装快照意图交付：%w", err)
	}
	handler := application.NewConsolidateParcelsHandler(application.ConsolidateParcelsDeps{
		Store:       units,
		Facts:       facts,
		Containment: units,
		Downstream:  downstream,
		Clock:       clock,
	})
	return consolidationImporter{handler: handler, transactor: db.Transactor()}, nil
}

// executeConsolidation 逐行推进、打报告、算退出码。
func executeConsolidation(ctx context.Context, batch consolidationBatch, importer consolidationImporter, out io.Writer) int {
	results := importConsolidation(ctx, batch, importer)
	writeImportReport(out, batch.BatchRef, consolidationTemplateVersion, results)
	return exitCodeFor(results)
}

// importConsolidation 逐行推进：一行一笔事务 → 按命令类型交对应的集运口 → 应用结果译成
// 去向。编排返回 Go 错误时整笔回滚，该行记未决；其他行照常继续。
func importConsolidation(ctx context.Context, batch consolidationBatch, importer consolidationImporter) []rowResult {
	results := make([]rowResult, 0, len(batch.Rows))
	for _, row := range batch.Rows {
		var handled application.ConsolidationResult
		err := importer.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			result, submitErr := submitConsolidation(txCtx, importer.handler, row.Command)
			if submitErr != nil {
				return submitErr
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
		results = append(results, consolidationRowDisposition(row, handled))
	}
	return results
}

// submitConsolidation 按命令类型分派到六口之一。默认分支是实现坏了而不是模板填错——译装
// 只会造这六种类型；走到那里按 Go 错误回滚，该行记未决让人看。
func submitConsolidation(
	ctx context.Context,
	handler *application.ConsolidateParcelsHandler,
	command any,
) (application.ConsolidationResult, error) {
	switch command := command.(type) {
	case application.OpenUnitCommand:
		return handler.Open(ctx, command)
	case application.AddMemberCommand:
		return handler.AddMember(ctx, command)
	case application.RemoveMemberCommand:
		return handler.RemoveMember(ctx, command)
	case application.SealUnitCommand:
		return handler.Seal(ctx, command)
	case application.UnsealUnitCommand:
		return handler.Unseal(ctx, command)
	case application.CloseUnitCommand:
		return handler.Close(ctx, command)
	default:
		return application.ConsolidationResult{}, fmt.Errorf("parcel-frontline-import: 未知集运命令类型 %T", command)
	}
}

// consolidationRowDisposition 把集运六口的结果译成四格去向，不增不减：
//   - 已开启 / 已移入 / 已移出 / 已封装 / 已开封 / 已关闭 → 已落地：作业事实已越过提交边界；
//   - 已有结果 → 重放：同一来源身份同一内容，原结果原样；
//   - 单元已在册 / 成员已在本单元 → 重放：别的来源先做了同一件事，本行没有作业发生、不留
//     来源事实，重跑答同一句——对内勤而言与重放要做的事相同：不用管；
//   - 成员在别的单元 / 单元不在册 / 未受理 / 来源冲突 → 被拒：四种都是重跑同一文件不会变的
//     答案，要人核对纸单——抄错了单元、行序颠倒、还是这一步在当前三相下本就做不得；
//   - 未决 → 未决：依赖故障，什么都没落库，重跑续办。
func consolidationRowDisposition(row consolidationRow, result application.ConsolidationResult) rowResult {
	outcome := result.Outcome()
	answer := rowResult{Line: row.Line, FactRef: row.FactRef, Outcome: outcome.String()}
	unit, hasUnit := result.Unit()

	switch outcome {
	case application.UnitOpened, application.MemberAdded, application.MemberRemoved,
		application.UnitSealedRecorded, application.UnitUnsealed, application.UnitClosedRecorded:
		answer.Disposition = rowLanded
		answer.Detail = describeUnit(unit, hasUnit)
	case application.ConsolidationExistingResult:
		answer.Disposition = rowReplayed
		answer.Detail = "原结果原样；" + describeUnit(unit, hasUnit)
	case application.UnitExisting:
		answer.Disposition = rowReplayed
		answer.Detail = "单元已由别的来源开启，本行未留来源事实；" + describeUnit(unit, hasUnit)
	case application.MemberAlreadyContained:
		answer.Disposition = rowReplayed
		answer.Detail = "成员已由别的来源移入本单元，本行未留来源事实；" + describeUnit(unit, hasUnit)
	case application.MemberElsewhereContained:
		answer.Disposition = rowRejected
		answer.Detail = "成员此刻在别的单元 " + result.Elsewhere().String() +
			" 里——同一时点最多一个直接物理父级；先从那里移出（" + domain.RemoveMemberAction.String() + "），不是重试"
	case application.UnitNotFound:
		answer.Disposition = rowRejected
		answer.Detail = "单元不在册——核对 " + columnUnitRef + "，或把 " + domain.OpenUnitAction.String() + " 行补在本行之前"
	case application.ConsolidationNotAccepted:
		answer.Disposition = rowRejected
		answer.Detail = "这一步在单元当前三相下做不得（如封装态移入或关闭、未封装开封、空单元封装、成员未清空且无处置依据关闭、重复移入或移出不在册的成员）——核对动作与行序"
	case application.ConsolidationSourceConflict:
		answer.Disposition = rowRejected
		answer.Detail = "同一行事实号已在册且内容不同——绝不覆盖；核对纸单：若是另一件事实请换行事实号"
	case application.ConsolidationUndecided:
		answer.Disposition = rowUndecided
		answer.Detail = "未落库；依赖未答，重跑同一文件续办"
	default:
		// 编排交回一个它自己都不认识的格是实现坏了，不是业务答案；按未决让人看。
		answer.Outcome = fmt.Sprintf("UNKNOWN(%d)", outcome)
		answer.Disposition = rowUndecided
		answer.Detail = "未知应用结果"
	}
	if handoff := result.HandoffReference(); handoff != "" {
		answer.Detail += "；封装快照意图未交出，续办引用 " + handoff
	}
	return answer
}

// describeUnit 给报告一句单元现状：三相、成员数、快照数——运维对纸单时看得出这一行推到了哪。
func describeUnit(unit *domain.ConsolidationUnit, present bool) string {
	if !present {
		return ""
	}
	phase := "开放"
	switch {
	case unit.Closed():
		phase = "已关闭"
	case unit.Sealed():
		phase = "已封装"
	}
	return fmt.Sprintf("单元 %s %s，成员 %d，快照 %d", unit.ID(), phase, len(unit.Members()), len(unit.Snapshots()))
}
