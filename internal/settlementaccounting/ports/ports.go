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
type PreAcceptanceControlPolicyView interface {
	LoadControlPolicy(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (domain.PreAcceptanceControlPolicy, error)
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
