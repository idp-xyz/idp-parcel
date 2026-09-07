package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是「资料修订阶段」判断要读的口（PS CONTEXT 词条；ADR-0118）。六格事实分三处出：
// 本上下文自有的三项走既有登记册的读半边（下面两个窄口 + LabelTransactionsByParcelView），
// 关务三格与装袋一格是跨上下文输入，各立一个消费侧读口，适配器落在本上下文的 adapters/
// 下（ADR-0025 消费方侧）。读口只交事实的三态，不交阶段：阶段是本上下文的判断，邻接上下文
// 只出它拥有的那几件事实。

// ResponsibilityStartView 是 IntakeAdoptionStore 的读半边：该包裹当前有没有责任起点（有效
// 网络收寄采用的链尾）。阶段判断只需要「在不在」，不需要写口，窄口让判断不必拿着 Save。
type ResponsibilityStartView interface {
	FindResponsibilityStart(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (IntakeAdoptionRecord, bool, error)
}

// CurrentFinalView 是 FinalOutcomeStore 的读半边：该包裹当前有没有有效终局服务结果。
type CurrentFinalView interface {
	FindCurrentFinal(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (FinalOutcomeRecord, bool, error)
}

// CustomsStageFacts 是关务为一个包裹出的三格事实，对应 `UC-PS-002` 阶段表后三行里关务
// 拥有的那半：申报资料形成中尚未提交、已提交、案件已关闭。三格分开交而不是交一个「关务
// 阶段」：哪一格压过哪一格是本上下文的判断（domain.JudgeAmendmentStage），关务只说自己
// 有没有形成、有没有提交、有没有关闭。
//
// 每格三态。`不知道`是读面没接上或关务对这个包裹答不出时的如实值，判断据以停在未决；把它
// 折成`不在`会让一个没接读面的关务把每个包裹都判成「尚未申报」。
type CustomsStageFacts struct {
	DataForming domain.StageFact
	Submitted   domain.StageFact
	CaseClosed  domain.StageFact
}

// CustomsStageView 读关务为一个包裹出的阶段事实（customs-compliance → parcel-shipment 的
// 消费缝，ADR-0118）。
//
// 依赖调不通作为错误返回；读面存在而关务对该包裹没有任何案件或单元，三格都是`不在`；读面
// 不存在或尚未接上，三格都是`不知道`——那不是错误，是一个如实的「答不出」，重试改不了它。
// 今天 customs-compliance 没有按包裹键的读面（案件键是管辖 × 方向 × 程序 × 义务范围，申报
// 单元虽记成员却没有反查口），本口的生产实现因此只能答`不知道`；缺口归 CC 立票，本上下文
// 不替它加。
type CustomsStageView interface {
	LoadCustomsStageFacts(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (CustomsStageFacts, error)
}

// ConsolidationStageView 读节点作业为一个包裹出的装袋事实（node-operations → parcel-shipment
// 的消费缝，ADR-0118）：该包裹此刻有没有装入集运单元。它与本上下文自有的面单交易结果并成
// 「已制签或已装袋」那一格。
//
// 三态语义同 CustomsStageView。今天 node-operations 的 ContainmentIndex 按作业实物
// （HandlingUnitID）答直接父级，而正式包裹与作业实物的关联属版本化关联、不由本上下文推断，
// 所以生产实现同样只能答`不知道`；缺口归 NO 立票。
type ConsolidationStageView interface {
	LoadBaggingFact(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.StageFact, error)
}
