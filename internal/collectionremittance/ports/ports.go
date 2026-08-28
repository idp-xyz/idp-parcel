// Package ports 是代收与清分对外的端口面：写口把登记与记账放进存储，点读口把已在册
// 的内容交回给编排做冲突判定与余额守卫。
//
// 端口按聚合族分三对，不合成一个大接口：代收面（指令、事实、差异事项）、分户账面
// （开立与记账）、回汇面（批次）。合成之后任何一族换形状都要动另外两族的实现。
//
// 本文件只放**点读**口：按键取一行、按分户账键取余额，伺候的是判断。管理台上列两册
// 那类 `catalogue_read` 读口是另一个调用面（先例：关务的 PortsPathsView 点读与
// PortsPathsCatalogueRead 上列分开两个接口），由读面那一侧追加，不拓宽这里的点读口。
package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// SaveOutcome 是本上下文全部登记册的写入代数。只有两格（ADR-0031）：`已登记`是业务
// 答案不是错误。**没有覆盖格是有意的**——写口一律不做 UPSERT，同键已在册就交回
// `已登记`，内容是否一致由编排读回既有登记自己比。把比对放在编排而不是 SQL 里，是
// 为了让「重放同一份」与「换了内容」这两件事在用例结果上分得开。
type SaveOutcome uint8

const (
	SaveOutcomeInvalid SaveOutcome = iota
	SaveRegistered
	SaveAlreadyRegistered
)

// CollectionRegistry 是代收面三本册子的写口：代收指令、代收事实与差异事项。
//
// 三者都只追加，没有更新方法。代收事实的更正走「追加新事实并关联原事实」，差异事项
// 的更正同理；指令内容变化走登记新指令。写口不提供任何改写既有行的路径，是因为已依
// 它们落账的记账不会跟着回退——改了上游，账面就凭空指向一份不存在过的依据。
type CollectionRegistry interface {
	RegisterInstruction(
		ctx context.Context,
		tenant domain.TenantID,
		instruction domain.CollectionInstruction,
	) (SaveOutcome, error)
	AcceptCollectionFact(
		ctx context.Context,
		tenant domain.TenantID,
		fact domain.CollectionFact,
	) (SaveOutcome, error)
	RegisterDiscrepancyItem(
		ctx context.Context,
		tenant domain.TenantID,
		item domain.DiscrepancyItem,
	) (SaveOutcome, error)
}

// CollectionView 按键读回代收面三本册子的一行。found=false 即该键未登记——实例半边
// 未提供时停在未决，不拿零值或空引用兜底。
type CollectionView interface {
	LoadInstruction(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.CollectionInstructionID,
	) (domain.CollectionInstruction, bool, error)
	LoadCollectionFact(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.CollectionFactID,
	) (domain.CollectionFact, bool, error)
	LoadDiscrepancyItem(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.DiscrepancyItemID,
	) (domain.DiscrepancyItem, bool, error)
}

// SubledgerRegistry 是分户账面的写口：开立与记账。
//
// 开立与记账分两个方法，同关务义务的目录/明细：「已开立但无记账」与「未开立」是相反
// 的答案，登记方必须能单独表达前者。
//
// AppendPosting **只追加**。这里没有更新也没有删除方法，不是省略——写错走反向记账
// 冲正，原记账原样留在账上；给一个改写入口，分配守恒就不再由结构交付了。
type SubledgerRegistry interface {
	OpenSubledger(
		ctx context.Context,
		tenant domain.TenantID,
		ledger domain.Subledger,
	) (SaveOutcome, error)
	AppendPosting(
		ctx context.Context,
		tenant domain.TenantID,
		posting domain.SubledgerPosting,
	) (SaveOutcome, error)
}

// SubledgerView 读分户账的开立面与派生余额。
//
// 两个方法答的是两件事：LoadSubledger 答「这本账开没开立」，LoadBalance 答「账上各
// 位置现在有多少」。零记账的账在后者上交回一个各位置皆零、笔数为零的余额，而不是
// found=false——「已开立且无本金」与「未开立」的区别就落在这里，合成一格之后读面
// 只能用查不到冒充无本金。
type SubledgerView interface {
	LoadSubledger(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.SubledgerKey,
	) (domain.Subledger, bool, error)
	LoadBalance(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.SubledgerKey,
	) (domain.SubledgerBalance, bool, error)
	LoadPosting(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.PostingID,
	) (domain.SubledgerPosting, bool, error)
}

// RemittanceRegistry 是回汇面的写口：形成批次与交出汇付主张。
//
// 交出主张是**状态推进**而不是另一次登记，所以它单独一个方法：合成进 FormBatch 会让
// 「重新形成」有机会顶掉已交出批次的归集截点。写口只允许 `已归集` → `已交出`，没有
// 反向路径；键、币种、截点与形成时刻在 SQL 上也没有改写入口。
type RemittanceRegistry interface {
	FormBatch(
		ctx context.Context,
		tenant domain.TenantID,
		batch domain.RemittanceBatch,
	) (SaveOutcome, error)
	HandOverBatchForPayment(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.RemittanceBatchID,
	) (HandOverOutcome, error)
}

// HandOverOutcome 是交出汇付主张的封闭三格。`已交出`与`已在已交出态`分开，是因为
// 两者的处置不同：前者是本次推进，后者说明有人已经交过——意图达成，但不该被记成
// 一次新的交出。`无对象`是批次不在册，那要改请求而不是重试。
type HandOverOutcome uint8

const (
	HandOverOutcomeInvalid HandOverOutcome = iota
	HandedOver
	AlreadyHandedOver
	HandOverTargetMissing
)

// RemittanceView 按批次标识读回批次，含状态。状态必须读得回来——汇付记账要先确认
// 目标批次仍在`已归集`态才能引用它，读回一个不带状态的批次等于把那道门拆了。
type RemittanceView interface {
	LoadBatch(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.RemittanceBatchID,
	) (domain.RemittanceBatch, bool, error)
}
