// Package ports 是代收与清分对外的端口面：写口把登记与记账放进存储，点读口把已在册
// 的内容交回给编排做冲突判定与余额守卫。
//
// 端口按聚合族分三对，不合成一个大接口：代收面（指令、事实、差异事项、认可的协议抵扣）、分户账面
// （开立与记账）、回汇面（批次）。合成之后任何一族换形状都要动另外两族的实现。
//
// 本文件只放**点读**口：按键取一行、按分户账键取余额、按代收指令取它名下的事实与记账，
// 伺候的是判断。管理台上列两册那类 `catalogue_read` 读口是另一个调用面（先例：关务的
// PortsPathsView 点读与 PortsPathsCatalogueRead 上列分开两个接口），由读面那一侧追加，
// 不拓宽这里的点读口。
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

// CollectionRegistry 是代收面四本册子的写口：代收指令、代收事实、差异事项与认可的
// 协议抵扣。
//
// 四者都只追加，没有更新方法。代收事实的更正走「追加新事实并关联原事实」，差异事项
// 的更正同理；指令内容变化走登记新指令；认可的抵扣被更正走新认可新记录，被撤回走冲正
// 已落的记账。写口不提供任何改写既有行的路径，是因为已依它们落账的记账不会跟着回退
// ——改了上游，账面就凭空指向一份不存在过的依据。
//
// RegisterAgreedOffset 的撞键有两种：标识相同，或（认可引用，指令）相同；两者都答
// `已登记`、不覆盖，由编排按标识读回区分重放与冲突。运营企业真实到账同理有两种撞键：标识相同，或（采用事实引用，指令）
// 相同；一条已采用事实对一条指令至多支撑一条真实到账。
//
// LockAdoptedFundsReferences 在环境事务里取（租户，已采用事实引用）的事务级锁，事务结束时自动放。真实到账的登记是
// 「读回此事实名下已有的到账、判合计、再落册」，两路并发引用同一条事实的到账各自读到同一份已有到账、各自判够，落下去
// 合计就超过事实金额。锁序：本锁在 AdoptedFundsView.LockAdoptedFunds 之前取，在分户账锁之外——登记路径不取分户账锁；
// 记账路径不取本锁（移位记账的采用事实门只读此事实名下的到账，写入方的登记已被采用事实的共享锁与更正排开）。
type CollectionRegistry interface {
	LockAdoptedFundsReferences(
		ctx context.Context,
		tenant domain.TenantID,
		reference domain.AdoptedFundsReference,
	) error
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
	RegisterAgreedOffset(
		ctx context.Context,
		tenant domain.TenantID,
		offset domain.AgreedOffset,
	) (SaveOutcome, error)
}

// CollectionView 按键读回代收面四本册子的一行。found=false 即该键未登记——实例半边
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
	LoadAgreedOffset(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.AgreedOffsetID,
	) (domain.AgreedOffset, bool, error)
	// LoadInstructionFacts 交回该指令名下的全部代收事实，空切片即一条也没登记。
	LoadInstructionFacts(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.CollectionInstructionID,
	) ([]domain.CollectionFact, error)
	// LoadAdoptedFundsReferences 交回全部引用这条已采用事实的运营企业真实到账（跨指令），按事实标识排序，空切片即
	// 还没有。已采用事实的合计、每指令至多一条都要读这一组才判得了（domain.AdmitAdoptedFundsReference）。
	LoadAdoptedFundsReferences(
		ctx context.Context,
		tenant domain.TenantID,
		reference domain.AdoptedFundsReference,
	) ([]domain.CollectionFact, error)
}

// AdoptedFundsView 是登记与移位运营企业真实到账时读 settlement-accounting 已采用事实的口（ADR-0330）。代收与清分
// 不读结算的表：这一口由适配器翻译结算的只读口，只交回「已采用、且在结算一侧已被整条认作代收本金」的事实此刻的链头版本与
// 金额。没采用、采用了却没认作代收本金、别的租户的同引用，对调用方是同一件事——先去结算把它采用并认作——所以都答
// found=false（ADR-0029：恢复动作相同则同格）。
//
// LockAdoptedFunds 在调用方的事务里先对结算一侧的事实身份行取共享锁再读：读到的金额在调用方事务提交之前不会被结算一侧的
// 外部更正改掉，调用方据此判的合计到落册那一刻仍成立。无环境事务即拒；读不动答 error（未决），不折成 found=false。
type AdoptedFundsView interface {
	LockAdoptedFunds(
		ctx context.Context,
		tenant domain.TenantID,
		reference domain.AdoptedFundsReference,
	) (domain.AdoptedFunds, bool, error)
}

// SubledgerRegistry 是分户账面的写口：开立与记账。
//
// 开立与记账分两个方法，同关务义务的目录/明细：「已开立但无记账」与「未开立」是相反
// 的答案，登记方必须能单独表达前者。
//
// AppendPosting **只追加**。这里没有更新也没有删除方法，不是省略——写错走反向记账
// 冲正，原记账原样留在账上；给一个改写入口，分配守恒就不再由结构交付了。
//
// LockLedger 在环境事务里取这本账的事务级锁，事务结束时自动放。余额守卫与按指令的
// 上限都是「读回账面再判」：两笔并发的记账各自读到同一份账面、各自判够，落下去就透支。
// 它放在写口而不是读口，是因为它只在有环境事务时才有意义——读口在事务外也能读。
type SubledgerRegistry interface {
	LockLedger(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.SubledgerKey,
	) error
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
	// LoadCorrectionOf 交回冲正了指定记账的那一笔。一笔记账至多被冲正一次，所以至多
	// 一笔；重复到达的冲正要在余额守卫之前凭它判出冲突，否则同账别的指令的余额够扣
	// 时，第二笔冲正会多拿别人的钱。
	LoadCorrectionOf(
		ctx context.Context,
		tenant domain.TenantID,
		original domain.PostingID,
	) (domain.SubledgerPosting, bool, error)
	// LoadInstructionPostings 交回依据追溯到该指令的全部记账：凭它的代收事实、凭它的
	// 差异事项、凭它的认可抵扣、凭它清分的，以及沿冲正链冲正这些记账的。按指令判上限
	// （ADR-0271）要的正是这一组；冲正不跟着追溯，冲正回`渠道在途`的短款就还不回它的
	// 指令。
	LoadInstructionPostings(
		ctx context.Context,
		tenant domain.TenantID,
		instruction domain.CollectionInstruction,
	) ([]domain.SubledgerPosting, error)
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
