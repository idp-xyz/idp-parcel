// Package ports 声明 settlement-accounting 自有的语义边界。这些只是接口：它们的
// PostgreSQL 适配器仍阻断在 Bento 持久化闸门之后（ADR-0017），今天唯一的实现是测试替身。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// PreAcceptanceControlPolicyView 取商业侧对「这个范围要不要接受前财务控制」的回答。
//
// 本上下文只消费它，绝不自行推导：CONTEXT 把接受前财务控制策略的版本生命周期判给
// `party-commercial`，也明写不适用依据由那边提供。在这里判一次就成了第二处定义。
//
// 依赖调不通要作为错误返回。把它读成「不要求控制」正是 CONTEXT 禁止的「默认信用通过」，
// 一次商业侧故障会因此变成一个看起来通过了的接受前控制。
//
// 三格（ADR-0054）：
//
//   - found=true + policy：商业侧登记过本范围的控制策略，答案在 policy 里（要求带方式与
//     采用政策，不要求带商业不适用依据）。
//   - found=false：**未登记**。`PAR-COM-15` 是待提供的实例参数，首发没有租户时这是唯一
//     走得到的真实分支。消费方据以停在自己的未决格，不得当成`无控制`——后者是合同已经
//     说过的终局答案，据它可以放行接受判断，而没人说过话时放行就是默认信用通过。
//   - error：调不通，等重试。
//
// 第三格不是可有可无的形状之争：少了它，适配器交回零值加 nil 就会在编排里落成一个带空
// 依据的`无控制`，而两格代数里没有任何地方能把那一格拦下来。
//
// 键形（sa-preacceptance-policy-view/01 裁决）：除作用域外还收一格**商业解析回指**。
// 作用域是资金维（法人/账户/币种），而商业侧的控制声明按客户合同版本键入——两者不同维，
// 少了回指，实现者只能自己从作用域反查合同，那正是「本上下文只消费不自行推导」禁的事，
// 且反查目录会成为账户映射的第二处定义。回指由调用方从同一次解析回显带来（施加路径的
// 作用域本就派生自它），实现凭（租户+回指）取商业侧已固定的闭包再读声明。
//
// 回指是必备入参，不是可选：空回指下没有任何诚实答案——它既不是「未登记」（没人问过，
// 谈不上登记与否）也不是「调不通」。调用方给不出回指时，命令在编排的受理判断处就该停住。
type PreAcceptanceControlPolicyView interface {
	LoadControlPolicy(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		resolution domain.CommercialResolutionReference,
	) (domain.PreAcceptanceControlPolicy, bool, error)
}

// OperationalBalanceView 取一个结算作用域当前的运营结算余额。按作用域取而不按账户取，
// 是因为责任法人、结算账户与币种三者共同决定一份余额的归属——少一维就可能拿另一个作用域
// 的钱来冻这一笔。
type OperationalBalanceView interface {
	LoadBalance(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (domain.OperationalBalance, error)
}

// FreezeLedgerRepository 按结算作用域取回只增不删的冻结登记册。幂等与请求冲突都由登记册
// 自己判定，所以整册取回而不是按请求逐条查：判定「同一请求身份是否携带了不同内容」需要它
// 原先记下的摘要，逐条查回来的单条冻结带不出那个。
type FreezeLedgerRepository interface {
	LoadForScope(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (*domain.FreezeLedger, error)
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		ledger *domain.FreezeLedger,
	) error
}

// CreditStandingView 取一个账期作用域当前的信用状况（额度、已占用暴露、是否逾期）。
// 与运营余额分开取：SET-03 明写预付冻结不与同一客户账期范围共用余额、额度（ADR-0047）。
// 依赖调不通要作为错误返回，读成「有额度」同样是被禁止的默认信用通过。
type CreditStandingView interface {
	LoadCreditStanding(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (domain.CreditStanding, error)
}

// CreditExposureLedgerRepository 按结算作用域取回只增不删的信用暴露登记册。整册取回的
// 理由与冻结登记册一字不差；它是另一本账，与冻结账本互不借用。
type CreditExposureLedgerRepository interface {
	LoadForScope(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (*domain.CreditExposureLedger, error)
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		ledger *domain.CreditExposureLedger,
	) error
}

type Clock interface {
	Now() time.Time
}

// BillReceptionKey 是供应商账单接收的幂等键：相同供应商、账期、主张身份和版本重复
// 到达返回原结果；同一身份内容变化形成版本冲突（UC-SA-004 一致性）。
type BillReceptionKey struct {
	TenantID domain.TenantID
	Claim    domain.BillClaimID
	Version  domain.BillClaimVersion
}

// BillReceptionRecord 是一次账单接收越过提交边界留下的东西：主张本体与逐行匹配。
// 这里刻意没有任何审核应付字段——匹配完成不是应付，审核通过才是（分步），应付由
// 审核步骤依据本记录另行形成。
type BillReceptionRecord struct {
	Key                      BillReceptionKey
	ContentDigest            string
	Claim                    domain.SupplierBillClaim
	Matches                  []domain.BillLineMatch
	AuditAuthorityConfigured bool
	RecordedAt               time.Time
}

type BillSaveOutcome uint8

const (
	BillSaveOutcomeInvalid BillSaveOutcome = iota
	BillSaved
	BillAlreadyRecorded
)

// BillReceptionStore 按幂等键找回并保存账单接收（写入代数同 ADR-0031：并发落败读回
// 赢家，不覆盖）。
type BillReceptionStore interface {
	FindByKey(ctx context.Context, key BillReceptionKey) (BillReceptionRecord, bool, error)
	Save(ctx context.Context, record BillReceptionRecord) (BillSaveOutcome, error)
}

// ExpectedCostView 按版本取回供应商预期成本供逐行匹配引用。found=false 表示该版本
// 不存在——指错版本是提交矛盾，不是等谁。
type ExpectedCostView interface {
	LoadExpectedCost(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.SupplierCostVersionID,
	) (domain.SupplierExpectedCost, bool, error)
}

// SupplierAuditAuthorityView 取该供应商/责任法人范围的审核授权配置。found=false 表示
// 授权未配置——实例半边未提供时审核停在未决，不默认放行也不虚构授权人（UC-SA-004
// 「无授权不得人工接受或拒绝」）。
type SupplierAuditAuthorityView interface {
	LoadSupplierAuditAuthority(
		ctx context.Context,
		tenant domain.TenantID,
		supplier domain.SupplierPartyReference,
		legalEntity domain.LegalEntityReference,
	) (domain.AuditorReference, bool, error)
}

// SupplierBillHandoffIntent 把已提交的接收记录交给审核与对账消费。意图由幂等键认领，
// 重放重发同一份（ADR-0043 同款纪律）。
type SupplierBillHandoffIntent struct {
	Record BillReceptionRecord
}

// SupplierBillHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017
// 的 Bento/Outbox 闸门。
type SupplierBillHandoff interface {
	HandOffSupplierBill(ctx context.Context, intent SupplierBillHandoffIntent) error
}

// ConfirmationCondition 是费用类型确认条件的核对应答：Met 时携带确认依据（交付确认、
// 里程碑达成……），未满足时携带缺口引用——缺口是续办入口，不是拒绝理由。
type ConfirmationCondition struct {
	Met   bool
	Basis domain.ConfirmationBasisReference
	Gap   string
}

// ConfirmationConditionView 取该费用类型的确认条件核对结果。found=false 表示确认
// 条件目录未配置——实例半边未提供时确认停在未决，不默认转正（UC-SA-002「费用已确认：
// 确认条件已满足」，条件本身是实例参数）。
type ConfirmationConditionView interface {
	LoadConfirmationCondition(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerChargeID,
		feeItem domain.FeeItemReference,
	) (ConfirmationCondition, bool, error)
}

type ChargeSaveOutcome uint8

const (
	ChargeSaveOutcomeInvalid ChargeSaveOutcome = iota
	ChargeSaved
	ChargeAlreadyConfirmed
)

// CustomerChargeStore 按标识找回并保存客户费用（写入代数同 ADR-0031：并发二确落败
// 读回赢家，不覆盖）。
type CustomerChargeStore interface {
	FindByID(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.CustomerChargeID,
	) (domain.CustomerCharge, bool, error)
	SaveConfirmed(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerCharge,
	) (ChargeSaveOutcome, error)
}

// ChargeConfirmationHandoffIntent 把已确认的费用交给对账单纳入消费（UC-SA-003 只
// 纳已确认费用）。意图由费用标识认领，重放重发同一份（ADR-0043 同款纪律）。
type ChargeConfirmationHandoffIntent struct {
	TenantID domain.TenantID
	Charge   domain.CustomerCharge
}

// ChargeConfirmationHandoff 今天没有实现，唯一实现是测试替身。
type ChargeConfirmationHandoff interface {
	HandOffChargeConfirmation(ctx context.Context, intent ChargeConfirmationHandoffIntent) error
}

// AdvanceAssessmentKey 是实际代垫评估的幂等键：同一评估标识只登一次。
type AdvanceAssessmentKey struct {
	TenantID   domain.TenantID
	Assessment domain.AdvanceAssessmentID
}

// AdvanceAssessmentRecord 是一次代垫评估越过提交边界留下的东西。
type AdvanceAssessmentRecord struct {
	Key           AdvanceAssessmentKey
	ContentDigest string
	Assessment    domain.ActualAdvanceAssessment
	RecordedAt    time.Time
}

type AdvanceAssessmentSaveOutcome uint8

const (
	AdvanceAssessmentSaveOutcomeInvalid AdvanceAssessmentSaveOutcome = iota
	AdvanceAssessmentSaved
	AdvanceAssessmentAlreadyRecorded
)

// AdvanceAssessmentStore 按幂等键找回并保存代垫评估（写入代数同 ADR-0031）。
type AdvanceAssessmentStore interface {
	FindByKey(ctx context.Context, key AdvanceAssessmentKey) (AdvanceAssessmentRecord, bool, error)
	Save(ctx context.Context, record AdvanceAssessmentRecord) (AdvanceAssessmentSaveOutcome, error)
}

// AdvanceRecoveryKey 是客户代垫回收的幂等键。
type AdvanceRecoveryKey struct {
	TenantID domain.TenantID
	Recovery domain.AdvanceRecoveryID
}

// AdvanceRecoveryRecord 是一次回收形成越过提交边界留下的东西。
type AdvanceRecoveryRecord struct {
	Key           AdvanceRecoveryKey
	ContentDigest string
	Recovery      domain.CustomerAdvanceRecovery
	RecordedAt    time.Time
}

type AdvanceRecoverySaveOutcome uint8

const (
	AdvanceRecoverySaveOutcomeInvalid AdvanceRecoverySaveOutcome = iota
	AdvanceRecoverySaved
	AdvanceRecoveryAlreadyFormed
)

// AdvanceRecoveryStore 按幂等键找回并保存客户代垫回收（写入代数同 ADR-0031）。
type AdvanceRecoveryStore interface {
	FindByKey(ctx context.Context, key AdvanceRecoveryKey) (AdvanceRecoveryRecord, bool, error)
	Save(ctx context.Context, record AdvanceRecoveryRecord) (AdvanceRecoverySaveOutcome, error)
}

// RecoveryAdjustmentKey 是回收调整的幂等键。
type RecoveryAdjustmentKey struct {
	TenantID   domain.TenantID
	Adjustment domain.RecoveryAdjustmentID
}

// RecoveryAdjustmentRecord 是一次回收调整越过提交边界留下的东西。
type RecoveryAdjustmentRecord struct {
	Key           RecoveryAdjustmentKey
	ContentDigest string
	Adjustment    domain.RecoveryAdjustment
	RecordedAt    time.Time
}

type RecoveryAdjustmentSaveOutcome uint8

const (
	RecoveryAdjustmentSaveOutcomeInvalid RecoveryAdjustmentSaveOutcome = iota
	RecoveryAdjustmentSaved
	RecoveryAdjustmentAlreadyFormed
)

// RecoveryAdjustmentStore 按幂等键找回并保存回收调整（写入代数同 ADR-0031）。
type RecoveryAdjustmentStore interface {
	FindByKey(ctx context.Context, key RecoveryAdjustmentKey) (RecoveryAdjustmentRecord, bool, error)
	Save(ctx context.Context, record RecoveryAdjustmentRecord) (RecoveryAdjustmentSaveOutcome, error)
}

// ContractResponsibilityView 取客户对该税费义务的合同责任依据。found=false 表示合同
// 责任目录未配置——回收需要合同依据（实例半边），未配置停在未决，不默认可回收。
type ContractResponsibilityView interface {
	LoadContractResponsibility(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.RecoveryCustomerReference,
		obligation domain.TaxObligationReference,
	) (domain.ContractResponsibilityReference, bool, error)
}

// AdvanceRecoveryIntent 把已形成的回收/调整交给对账单纳入消费（UC-SA-003 上游）。
// 意图由幂等键认领，重放重发同一份（ADR-0043）。
type AdvanceRecoveryIntent struct {
	Recovery   AdvanceRecoveryRecord
	Adjustment RecoveryAdjustmentRecord
}

// AdvanceRecoveryHandoff 今天没有实现，唯一实现是测试替身。
type AdvanceRecoveryHandoff interface {
	HandOffAdvanceRecovery(ctx context.Context, intent AdvanceRecoveryIntent) error
}

// StatementKey 是已发布对账单的幂等键：同一单号只发布一次；作废以 Replace 换值，
// 替代单用新单号。
type StatementKey struct {
	TenantID domain.TenantID
	Number   domain.StatementNumber
}

// StatementRecord 是一次对账单发布越过提交边界留下的东西。
type StatementRecord struct {
	Key           StatementKey
	ContentDigest string
	Statement     domain.PublishedStatement
	RecordedAt    time.Time
}

type StatementSaveOutcome uint8

const (
	StatementSaveOutcomeInvalid StatementSaveOutcome = iota
	StatementSaved
	StatementAlreadyPublished
)

// PublishedStatementStore 按幂等键找回并保存已发布对账单（写入代数同 ADR-0031）。
type PublishedStatementStore interface {
	FindByKey(ctx context.Context, key StatementKey) (StatementRecord, bool, error)
	Save(ctx context.Context, record StatementRecord) (StatementSaveOutcome, error)
	Replace(ctx context.Context, record StatementRecord) (bool, error)
}

// InclusionKey 是后续账期纳入的幂等键。
type InclusionKey struct {
	TenantID  domain.TenantID
	Inclusion domain.InclusionReference
}

// InclusionRecord 是一次后续账期纳入越过提交边界留下的东西。
type InclusionRecord struct {
	Key           InclusionKey
	ContentDigest string
	Inclusion     domain.SubsequentInclusion
	RecordedAt    time.Time
}

type InclusionSaveOutcome uint8

const (
	InclusionSaveOutcomeInvalid InclusionSaveOutcome = iota
	InclusionSaved
	InclusionAlreadyRecorded
)

// SubsequentInclusionStore 按幂等键找回并保存后续账期纳入（写入代数同 ADR-0031）。
type SubsequentInclusionStore interface {
	FindByKey(ctx context.Context, key InclusionKey) (InclusionRecord, bool, error)
	Save(ctx context.Context, record InclusionRecord) (InclusionSaveOutcome, error)
}

// DisputeKey 是客户异议的幂等键。
type DisputeKey struct {
	TenantID domain.TenantID
	Dispute  domain.DisputeID
}

// DisputeRecord 是一次异议越过提交边界留下的东西。裁定以 Replace 换值——异议是独立
// 对象，不改写对账单。
type DisputeRecord struct {
	Key           DisputeKey
	ContentDigest string
	Dispute       domain.StatementDispute
	RecordedAt    time.Time
}

type DisputeSaveOutcome uint8

const (
	DisputeSaveOutcomeInvalid DisputeSaveOutcome = iota
	DisputeSaved
	DisputeAlreadyOpened
)

// StatementDisputeStore 按幂等键找回并保存异议（写入代数同 ADR-0031）。
type StatementDisputeStore interface {
	FindByKey(ctx context.Context, key DisputeKey) (DisputeRecord, bool, error)
	Save(ctx context.Context, record DisputeRecord) (DisputeSaveOutcome, error)
	Replace(ctx context.Context, record DisputeRecord) (bool, error)
}

// StatementIntent 把对账单事件交给下游（客户通知、外部资金核销的目标索引）。发布、
// 作废与后续纳入各携其记录，消费方自分；重放重发同一份（ADR-0043）。
type StatementIntent struct {
	Statement StatementRecord
	Inclusion InclusionRecord
}

// StatementHandoff 今天没有实现，唯一实现是测试替身。
type StatementHandoff interface {
	HandOffStatement(ctx context.Context, intent StatementIntent) error
}

// FundsFactKey 是外部资金事实采用的幂等键：事实归银行/支付系统拥有，同一事实引用只
// 采用一次。
type FundsFactKey struct {
	TenantID domain.TenantID
	Fact     domain.FundsFactReference
}

// FundsFactRecord 是一次资金事实采用越过提交边界留下的东西。
type FundsFactRecord struct {
	Key           FundsFactKey
	ContentDigest string
	Fact          domain.ExternalFundsFact
	RecordedAt    time.Time
}

type FundsFactSaveOutcome uint8

const (
	FundsFactSaveOutcomeInvalid FundsFactSaveOutcome = iota
	FundsFactSaved
	FundsFactAlreadyAdopted
)

// ExternalFundsFactStore 按幂等键找回并保存资金事实引用（写入代数同 ADR-0031）。
type ExternalFundsFactStore interface {
	FindByKey(ctx context.Context, key FundsFactKey) (FundsFactRecord, bool, error)
	Save(ctx context.Context, record FundsFactRecord) (FundsFactSaveOutcome, error)
}

// FundsMappingKey 是资金映射的幂等键。
type FundsMappingKey struct {
	TenantID domain.TenantID
	Mapping  domain.MappingReference
}

// FundsMappingRecord 是一次映射越过提交边界留下的东西。
type FundsMappingRecord struct {
	Key           FundsMappingKey
	ContentDigest string
	Mapping       domain.FundsMapping
	RecordedAt    time.Time
}

type FundsMappingSaveOutcome uint8

const (
	FundsMappingSaveOutcomeInvalid FundsMappingSaveOutcome = iota
	FundsMappingSaved
	FundsMappingAlreadyRecorded
)

// FundsMappingStore 按幂等键找回并保存资金映射（写入代数同 ADR-0031）。
type FundsMappingStore interface {
	FindByKey(ctx context.Context, key FundsMappingKey) (FundsMappingRecord, bool, error)
	Save(ctx context.Context, record FundsMappingRecord) (FundsMappingSaveOutcome, error)
}

// SettlementApplicationKey 是核销的幂等键。撤销以 Replace 换值，撤销关系在本体上。
type SettlementApplicationKey struct {
	TenantID    domain.TenantID
	Application domain.ApplicationReference
}

// SettlementApplicationRecord 是一次核销越过提交边界留下的东西。
type SettlementApplicationRecord struct {
	Key           SettlementApplicationKey
	ContentDigest string
	Application   domain.SettlementApplication
	RecordedAt    time.Time
}

type SettlementApplicationSaveOutcome uint8

const (
	SettlementApplicationSaveOutcomeInvalid SettlementApplicationSaveOutcome = iota
	SettlementApplicationSaved
	SettlementApplicationAlreadyApplied
)

// SettlementApplicationStore 按幂等键找回并保存核销（写入代数同 ADR-0031）。
type SettlementApplicationStore interface {
	FindByKey(ctx context.Context, key SettlementApplicationKey) (SettlementApplicationRecord, bool, error)
	Save(ctx context.Context, record SettlementApplicationRecord) (SettlementApplicationSaveOutcome, error)
	Replace(ctx context.Context, record SettlementApplicationRecord) (bool, error)
}

// SettlementApplicationIntent 把核销/撤销交给下游（对账单与应付的已结视图）。重放
// 重发同一份（ADR-0043）。
type SettlementApplicationIntent struct {
	Record SettlementApplicationRecord
}

// SettlementApplicationHandoff 今天没有实现，唯一实现是测试替身。
type SettlementApplicationHandoff interface {
	HandOffSettlementApplication(ctx context.Context, intent SettlementApplicationIntent) error
}

// AllocationRuleView 取来源费用适用的分摊规则版本。found=false 表示分摊规则目录未
// 配置——无规则不分摊、不默认均摊（实例半边，AT-SA-123）。
type AllocationRuleView interface {
	LoadAllocationRule(
		ctx context.Context,
		tenant domain.TenantID,
		source domain.AllocationSourceReference,
	) (domain.AllocationRuleVersionReference, bool, error)
}

// AllocationKey 是成本分摊的幂等键。重分摊以 Replace 换值，版本链在本体上回指。
type AllocationKey struct {
	TenantID   domain.TenantID
	Allocation domain.AllocationID
}

// AllocationRecord 是一次分摊越过提交边界留下的东西。
type AllocationRecord struct {
	Key           AllocationKey
	ContentDigest string
	Allocation    domain.CostAllocation
	RecordedAt    time.Time
}

type AllocationSaveOutcome uint8

const (
	AllocationSaveOutcomeInvalid AllocationSaveOutcome = iota
	AllocationSaved
	AllocationAlreadyFormed
)

// CostAllocationStore 按幂等键找回并保存成本分摊（写入代数同 ADR-0031）。
type CostAllocationStore interface {
	FindByKey(ctx context.Context, key AllocationKey) (AllocationRecord, bool, error)
	Save(ctx context.Context, record AllocationRecord) (AllocationSaveOutcome, error)
	Replace(ctx context.Context, record AllocationRecord) (bool, error)
}

// OperatingResultKey 是经营结果快照的幂等键：同一（口径+账期+基准）一版一登，重派生
// 以 Replace 换值、新版本关联原截点（AT-SA-137）。
type OperatingResultKey struct {
	TenantID domain.TenantID
	Scope    domain.OperatingScopeReference
	Period   domain.BillingPeriodReference
	Basis    domain.OperatingBasis
}

// OperatingResultRecord 是一次指标派生越过提交边界留下的东西。
type OperatingResultRecord struct {
	Key           OperatingResultKey
	ContentDigest string
	Result        domain.OperatingResult
	RecordedAt    time.Time
}

type OperatingResultSaveOutcome uint8

const (
	OperatingResultSaveOutcomeInvalid OperatingResultSaveOutcome = iota
	OperatingResultSaved
	OperatingResultAlreadyDerived
)

// OperatingResultStore 按幂等键找回并保存经营结果（写入代数同 ADR-0031）。
type OperatingResultStore interface {
	FindByKey(ctx context.Context, key OperatingResultKey) (OperatingResultRecord, bool, error)
	Save(ctx context.Context, record OperatingResultRecord) (OperatingResultSaveOutcome, error)
	Replace(ctx context.Context, record OperatingResultRecord) (bool, error)
}

// OperatingIntent 把分摊与指标交给分析消费。重放重发同一份（ADR-0043）。
type OperatingIntent struct {
	Allocation AllocationRecord
	Result     OperatingResultRecord
}

// OperatingHandoff 今天没有实现，唯一实现是测试替身。
type OperatingHandoff interface {
	HandOffOperating(ctx context.Context, intent OperatingIntent) error
}

// ClaimAmountRuleView 取责任结论适用的限额/比例/免赔金额规则版本。found=false 表示
// 金额规则目录未配置——没有规则版本不形成金额（实例半边，AT-SA-147）。
type ClaimAmountRuleView interface {
	LoadClaimAmountRule(
		ctx context.Context,
		tenant domain.TenantID,
		responsibility domain.ResponsibilityConclusionReference,
	) (domain.AmountRuleVersionReference, bool, error)
}

// ClaimAmountKey 是客户方向索赔金额的幂等键。
type ClaimAmountKey struct {
	TenantID domain.TenantID
	Amount   domain.CustomerClaimAmountID
}

// ClaimAmountRecord 是一笔赔付/退款金额越过提交边界留下的东西。
type ClaimAmountRecord struct {
	Key           ClaimAmountKey
	ContentDigest string
	Amount        domain.CustomerClaimAmount
	RecordedAt    time.Time
}

type ClaimAmountSaveOutcome uint8

const (
	ClaimAmountSaveOutcomeInvalid ClaimAmountSaveOutcome = iota
	ClaimAmountSaved
	ClaimAmountAlreadyFormed
)

// CustomerClaimAmountStore 按幂等键找回并保存索赔金额（写入代数同 ADR-0031）。
type CustomerClaimAmountStore interface {
	FindByKey(ctx context.Context, key ClaimAmountKey) (ClaimAmountRecord, bool, error)
	Save(ctx context.Context, record ClaimAmountRecord) (ClaimAmountSaveOutcome, error)
}

// ReceivableKey 是应追偿金额的幂等键。
type ReceivableKey struct {
	TenantID   domain.TenantID
	Receivable domain.RecoveryReceivableID
}

// ReceivableRecord 是一笔应追偿越过提交边界留下的东西。
type ReceivableRecord struct {
	Key           ReceivableKey
	ContentDigest string
	Receivable    domain.RecoveryReceivable
	RecordedAt    time.Time
}

type ReceivableSaveOutcome uint8

const (
	ReceivableSaveOutcomeInvalid ReceivableSaveOutcome = iota
	ReceivableSaved
	ReceivableAlreadyFormed
)

// RecoveryReceivableStore 按幂等键找回并保存应追偿（写入代数同 ADR-0031）。
type RecoveryReceivableStore interface {
	FindByKey(ctx context.Context, key ReceivableKey) (ReceivableRecord, bool, error)
	Save(ctx context.Context, record ReceivableRecord) (ReceivableSaveOutcome, error)
}

// AcknowledgementKey 是追偿认可的幂等键。
type AcknowledgementKey struct {
	TenantID        domain.TenantID
	Acknowledgement domain.AcknowledgementID
}

// AcknowledgementRecord 是一次认可越过提交边界留下的东西。
type AcknowledgementRecord struct {
	Key             AcknowledgementKey
	ContentDigest   string
	Acknowledgement domain.RecoveryAcknowledgement
	RecordedAt      time.Time
}

type AcknowledgementSaveOutcome uint8

const (
	AcknowledgementSaveOutcomeInvalid AcknowledgementSaveOutcome = iota
	AcknowledgementSaved
	AcknowledgementAlreadyRecorded
)

// RecoveryAcknowledgementStore 按幂等键找回并保存认可（写入代数同 ADR-0031）。
type RecoveryAcknowledgementStore interface {
	FindByKey(ctx context.Context, key AcknowledgementKey) (AcknowledgementRecord, bool, error)
	Save(ctx context.Context, record AcknowledgementRecord) (AcknowledgementSaveOutcome, error)
}

// ClaimAdjustmentKey 是索赔金额调整的幂等键。
type ClaimAdjustmentKey struct {
	TenantID   domain.TenantID
	Adjustment domain.ClaimAmountAdjustmentID
}

// ClaimAdjustmentRecord 是一次调整越过提交边界留下的东西。
type ClaimAdjustmentRecord struct {
	Key           ClaimAdjustmentKey
	ContentDigest string
	Adjustment    domain.ClaimAmountAdjustment
	RecordedAt    time.Time
}

type ClaimAdjustmentSaveOutcome uint8

const (
	ClaimAdjustmentSaveOutcomeInvalid ClaimAdjustmentSaveOutcome = iota
	ClaimAdjustmentSaved
	ClaimAdjustmentAlreadyFormed
)

// ClaimAmountAdjustmentStore 按幂等键找回并保存调整（写入代数同 ADR-0031）。
type ClaimAmountAdjustmentStore interface {
	FindByKey(ctx context.Context, key ClaimAdjustmentKey) (ClaimAdjustmentRecord, bool, error)
	Save(ctx context.Context, record ClaimAdjustmentRecord) (ClaimAdjustmentSaveOutcome, error)
}

// ClaimSettlementIntent 把赔付金额与调整交给对账单纳入、把应追偿与认可交给追偿链。
// 各携其记录，消费方自分；重放重发同一份（ADR-0043）。
type ClaimSettlementIntent struct {
	ClaimAmount     ClaimAmountRecord
	Receivable      ReceivableRecord
	Acknowledgement AcknowledgementRecord
	Adjustment      ClaimAdjustmentRecord
}

// ClaimSettlementHandoff 今天没有实现，唯一实现是测试替身。
type ClaimSettlementHandoff interface {
	HandOffClaimSettlement(ctx context.Context, intent ClaimSettlementIntent) error
}
