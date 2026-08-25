// Package ports 声明 customs-compliance 应用层与外界的边界。这些只是接口：适配器仍
// 阻断在 Bento 持久化闸门之后（ADR-0017），今天唯一的实现是测试替身。
package ports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

type Clock interface {
	Now() time.Time
}

// ExternalResultKey 是外部监管结果接收的幂等键：同一来源响应身份重复到达返回原结果；
// 同一身份不同内容形成冲突。
type ExternalResultKey struct {
	TenantID domain.TenantID
	SourceID string
}

// ExternalResultRecord 是一次外部结果接收越过提交边界留下的东西。归属不上原提交的
// 响应以 Unattributable 留存原始语义与其声称的版本——留存不猜（CONTEXT 硬句 187：
// 不得据此猜测提交、补造缺失层次或按最后到达直接改变当前判断）；同层冲突以
// LayerConflict 标记，双方事实都在库里，不选边。
type ExternalResultRecord struct {
	Key            ExternalResultKey
	ContentDigest  string
	Result         domain.ExternalResult
	Unattributable bool
	RawSemantics   string
	ClaimedVersion string
	LayerConflict  bool
	RecordedAt     time.Time
}

type ExternalResultSaveOutcome uint8

const (
	ExternalResultSaveOutcomeInvalid ExternalResultSaveOutcome = iota
	ExternalResultSaved
	ExternalResultAlreadyRecorded
)

// ExternalResultStore 按幂等键找回并保存接收记录（写入代数同 ADR-0031），并按提交
// 版本读回同一提交已保存的各层事实供同层一致性比对。
type ExternalResultStore interface {
	FindByKey(ctx context.Context, key ExternalResultKey) (ExternalResultRecord, bool, error)
	Save(ctx context.Context, record ExternalResultRecord) (ExternalResultSaveOutcome, error)
	LoadForSubmission(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.SubmissionVersionID,
	) ([]domain.ExternalResult, error)
}

// SubmissionIndex 按版本找回原提交是否存在。found=false 表示响应声称的提交在本系统
// 没有对应版本——归属不上，留存不猜。
type SubmissionIndex interface {
	FindSubmission(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.SubmissionVersionID,
	) (bool, error)
}

// InterpretationRuleView 按（租户，结果层，适用辖区）在评估时点上解析该层外部响应
// 适用的解释规则版本（选择侧，ADR-0070 决定一/二）。登记册按法定生效区间半开解析：
// evaluatedAt 取业务发生或适用时间，绝不取消息到达或系统当前时间（CONTEXT 硬句 191）。
// found=false 表示该辖区该层在该时点没有已登记的规则版本——实例半边未提供时解释停在
// 未决，不用默认口径猜测监管语义，也不拿当前指针兜底。
type InterpretationRuleView interface {
	LoadInterpretationRule(
		ctx context.Context,
		tenant domain.TenantID,
		layer domain.ResultLayer,
		jurisdiction domain.RegulatoryJurisdictionReference,
		evaluatedAt time.Time,
	) (domain.InterpretationRuleReference, bool, error)
}

// ExecutionFactView 按监管决定的范围读回执行方已形成的物理执行事实（NO/TF 拥有，
// 这里只读引用参与核对）。空清单是如实答案——决定推导不出执行，没有事实就是证据
// 不足；依赖调不通作为错误返回。
type ExecutionFactView interface {
	LoadExecutionFacts(
		ctx context.Context,
		tenant domain.TenantID,
		decision domain.RegulatoryDecisionID,
	) ([]domain.ExecutionFact, error)
}

// VerificationKey 是处置执行核对的幂等键：同一决定加同一事实集指纹只出一版核对——
// 事实集变化（新执行事实到达）自然换指纹换版。
type VerificationKey struct {
	TenantID domain.TenantID
	Decision domain.RegulatoryDecisionID
	Digest   string
}

type VerificationSaveOutcome uint8

const (
	VerificationSaveOutcomeInvalid VerificationSaveOutcome = iota
	VerificationSaved
	VerificationAlreadyRecorded
)

// DispositionVerificationStore 按幂等键找回并保存核对判断（写入代数同 ADR-0031）。
type DispositionVerificationStore interface {
	FindByKey(ctx context.Context, key VerificationKey) (domain.DispositionVerification, bool, error)
	Save(
		ctx context.Context,
		key VerificationKey,
		verification domain.DispositionVerification,
	) (VerificationSaveOutcome, error)
}

// VerificationHandoffIntent 把核对结论交给适用下游（案件关闭核对与 VE 的处置协调
// 消费它）。意图由核对键认领，重放重发同一份（ADR-0043）。
type VerificationHandoffIntent struct {
	Key          VerificationKey
	Verification domain.DispositionVerification
}

// VerificationHandoff 把核对结论写入 Outbox（`OutboxVerificationHandoff`）。信封 ID
// 由核对幂等键认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type VerificationHandoff interface {
	HandOffVerification(ctx context.Context, intent VerificationHandoffIntent) error
}

// ObligationInventoryView 按案件与业务截点盘出全部适用义务的关闭依据项。义务目录
// 与逐项状态来自监管程序与案内事实（实例半边）；configured=false 即该程序的义务
// 目录还没登记——未决，不是「没有义务所以可关」。
type ObligationInventoryView interface {
	LoadObligationItems(
		ctx context.Context,
		tenant domain.TenantID,
		caseRef domain.CustomsCaseID,
		cutoffAt time.Time,
	) ([]domain.ClosureObligationItem, bool, error)
}

type CaseClosureSaveOutcome uint8

const (
	CaseClosureSaveOutcomeInvalid CaseClosureSaveOutcome = iota
	CaseClosureSaved
	CaseClosureAlreadyRecorded
)

// CaseClosureStore 按案件引用找回并保存关闭记录——单个案件不存在部分关闭，一案
// 至多一份关闭记录（重开追加在记录内）。
type CaseClosureStore interface {
	FindByCase(
		ctx context.Context,
		tenant domain.TenantID,
		caseRef domain.CustomsCaseID,
	) (*domain.CustomsCaseClosure, bool, error)
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		closure *domain.CustomsCaseClosure,
	) (CaseClosureSaveOutcome, error)
}

// CaseClosureHandoffIntent 把关闭决定交给适用下游（VE 的案件视图与治理审计消费它）。
type CaseClosureHandoffIntent struct {
	TenantID domain.TenantID
	Closure  *domain.CustomsCaseClosure
}

// CaseClosureHandoff 把关闭决定写入 Outbox（`OutboxCaseClosureHandoff`）。信封 ID
// 由租户加案件引用认领（案件引用跨租户不唯一），入队由 outboxintent.EnqueueOnce
// 承担；重放重发同一份（ADR-0043）。
type CaseClosureHandoff interface {
	HandOffClosure(ctx context.Context, intent CaseClosureHandoffIntent) error
}

type RestrictionSaveOutcome uint8

const (
	RestrictionSaveOutcomeInvalid RestrictionSaveOutcome = iota
	RestrictionSaved
	RestrictionAlreadyRecorded
)

// RestrictionStore 保存监管限制并按范围盘出参与准入判断的限制。Save 的写入代数只
// 管建立（同 ID 重复建立交回 AlreadyRecorded）；解除是同一限制的状态推进，走 Update
// ——限制身份不变，谁先解除成功谁算，重复解除由领域的不再有效拦。
type RestrictionStore interface {
	FindByID(ctx context.Context, tenant domain.TenantID, id domain.RestrictionID) (domain.RegulatoryRestriction, bool, error)
	ListByScope(ctx context.Context, tenant domain.TenantID, scope domain.DecisionScopeReference) ([]domain.RegulatoryRestriction, error)
	Save(ctx context.Context, tenant domain.TenantID, restriction domain.RegulatoryRestriction) (RestrictionSaveOutcome, error)
	Update(ctx context.Context, tenant domain.TenantID, restriction domain.RegulatoryRestriction) error
}

// RestrictionHandoffIntent 把限制的建立与解除交给适用下游（TF/NO 的门禁执行方消费
// ——它们只执行不豁免）。意图由限制标识认领，重放重发同一份（ADR-0043）。
type RestrictionHandoffIntent struct {
	TenantID    domain.TenantID
	Restriction domain.RegulatoryRestriction
}

// RestrictionHandoff 把限制写入 Outbox（`OutboxRestrictionHandoff`）。信封 ID 由限制
// 标识认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type RestrictionHandoff interface {
	HandOffRestriction(ctx context.Context, intent RestrictionHandoffIntent) error
}

// GateConditionView 按（范围+动作+边界）盘出参与门禁核对的前置条件逐项判断。前置
// 条件目录与逐项判断来自监管程序与案内事实（实例半边）；configured=false 即目录未
// 登记——未决，没有清单的门禁判断无从复核；空清单是「此动作在此边界不受门禁」的
// 如实答案，与未登记分开。
type GateConditionView interface {
	LoadPreconditionFindings(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.DecisionScopeReference,
		action domain.GuardedAction,
		boundary domain.CustomsProcedureReference,
	) ([]domain.PreconditionFinding, bool, error)
}

// GateVerificationKey 是门禁核对的幂等键：判断身份三维（范围+动作+边界）加逐项判断
// 指纹——条件状态变化自然换指纹换版，同一状态重复核对不出第二版。
type GateVerificationKey struct {
	TenantID domain.TenantID
	Scope    domain.DecisionScopeReference
	Action   domain.GuardedAction
	Boundary domain.CustomsProcedureReference
	Digest   string
}

// FindingsDigest 是逐项判断的稳定指纹：按前置条件引用排序后连状态拼接。
func FindingsDigest(findings []domain.PreconditionFinding) string {
	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		lines = append(lines, finding.Precondition.String()+"="+strconv.Itoa(int(finding.State)))
	}
	sort.Strings(lines)
	digest := sha256.Sum256([]byte(strings.Join(lines, "\x00")))
	return hex.EncodeToString(digest[:])
}

type GateVerificationSaveOutcome uint8

const (
	GateVerificationSaveOutcomeInvalid GateVerificationSaveOutcome = iota
	GateVerificationSaved
	GateVerificationAlreadyRecorded
)

// GateVerificationStore 按幂等键找回并保存门禁核对（写入代数同 ADR-0031）。
type GateVerificationStore interface {
	FindByKey(ctx context.Context, key GateVerificationKey) (domain.ReleaseGateVerification, bool, error)
	Save(
		ctx context.Context,
		key GateVerificationKey,
		gate domain.ReleaseGateVerification,
	) (GateVerificationSaveOutcome, error)
}

// GateVerificationHandoffIntent 把门禁核对交给适用下游（TF/NO 的动作执行方消费——
// 门禁满足不生成放行，放行结果仍由外部事实接收）。意图由门禁幂等键认领，重放重发
// 同一份（ADR-0043）。
type GateVerificationHandoffIntent struct {
	Key  GateVerificationKey
	Gate domain.ReleaseGateVerification
}

// GateVerificationHandoff 把门禁核对写入 Outbox（`OutboxGateVerificationHandoff`）。
// 信封 ID 由门禁幂等键认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份
// （ADR-0043）。
type GateVerificationHandoff interface {
	HandOffGate(ctx context.Context, intent GateVerificationHandoffIntent) error
}

// FollowUpTargetKey 是后续申报动作目标的幂等键：同一触发依据对同一提交版本的同类
// 动作只立一个目标——触发依据换了（新监管要求）或版本换了自然换键。
type FollowUpTargetKey struct {
	TenantID domain.TenantID
	Trigger  domain.FollowUpTriggerReference
	Version  domain.SubmissionVersionID
	Kind     domain.FollowUpActionKind
}

type FollowUpSaveOutcome uint8

const (
	FollowUpSaveOutcomeInvalid FollowUpSaveOutcome = iota
	FollowUpSaved
	FollowUpAlreadyRecorded
)

// FollowUpStore 保存后续动作目标与替代关系。替代关系按目标键定位——一个重报目标
// 至多一份替代关系；生效是同一关系的状态推进，走 UpdateRelation。
type FollowUpStore interface {
	FindTarget(ctx context.Context, key FollowUpTargetKey) (domain.FollowUpTarget, bool, error)
	SaveTarget(ctx context.Context, key FollowUpTargetKey, target domain.FollowUpTarget) (FollowUpSaveOutcome, error)
	FindRelation(ctx context.Context, key FollowUpTargetKey) (domain.ReplacementRelation, bool, error)
	SaveRelation(ctx context.Context, key FollowUpTargetKey, relation domain.ReplacementRelation) (FollowUpSaveOutcome, error)
	UpdateRelation(ctx context.Context, key FollowUpTargetKey, relation domain.ReplacementRelation) error
}

// FollowUpHandoffIntent 把目标形成与替代生效交给适用下游（申报执行方消费目标，VE
// 与案件视图消费生效）。意图由后续动作目标键认领，重放重发同一份（ADR-0043）。
type FollowUpHandoffIntent struct {
	Key      FollowUpTargetKey
	Target   domain.FollowUpTarget
	Relation *domain.ReplacementRelation
}

// FollowUpHandoff 把后续动作目标写入 Outbox（`OutboxFollowUpHandoff`）。信封 ID 由
// 后续动作目标键认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份
// （ADR-0043）。
type FollowUpHandoff interface {
	HandOffFollowUp(ctx context.Context, intent FollowUpHandoffIntent) error
}

// ManifestCandidateView 按监管程序、方向与范围盘出可供关联的申报单元候选。空清单
// 是「没有能匹配的」如实答案（引用保持待关联），读不回是依赖故障。
type ManifestCandidateView interface {
	LoadAssociationCandidates(
		ctx context.Context,
		tenant domain.TenantID,
		procedure domain.CustomsProcedureReference,
		direction domain.ManifestDirection,
	) ([]domain.AssociationCandidate, error)
}

type ManifestSaveOutcome uint8

const (
	ManifestSaveOutcomeInvalid ManifestSaveOutcome = iota
	ManifestSaved
	ManifestAlreadyRecorded
)

// ManifestStore 按外部舱单身份保存受控引用——一舱单一当前引用；来源版本推进走
// Update（原引用与历史关联由领域在新引用内保留）。
type ManifestStore interface {
	FindByManifest(
		ctx context.Context,
		tenant domain.TenantID,
		manifest domain.ExternalManifestID,
	) (domain.ExternalManifestReference, bool, error)
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		reference domain.ExternalManifestReference,
	) (ManifestSaveOutcome, error)
	Update(
		ctx context.Context,
		tenant domain.TenantID,
		reference domain.ExternalManifestReference,
	) error
}

// ManifestHandoffIntent 把舱单引用的接受、关联与版本推进交给适用下游（案件视图与
// 申报链消费）。意图由舱单身份认领，重放重发同一份（ADR-0043）。
type ManifestHandoffIntent struct {
	TenantID  domain.TenantID
	Reference domain.ExternalManifestReference
}

// ManifestHandoff 把舱单引用写入 Outbox（`OutboxManifestHandoff`）。信封 ID 由舱单
// 身份认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type ManifestHandoff interface {
	HandOffManifest(ctx context.Context, intent ManifestHandoffIntent) error
}

// CaseRequirementJudgment 是服务产品与运营责任对「此监管范围要不要建案」的判断。
type CaseRequirementJudgment struct {
	Required bool
	Basis    string
}

// CaseRequirementView 判断当前服务责任是否要求为该范围建立关务案件。规则目录属
// 产品与合同实例；configured=false 即规则未登记——未决，不是「不要求」。
type CaseRequirementView interface {
	JudgeCaseRequirement(
		ctx context.Context,
		tenant domain.TenantID,
		jurisdiction domain.RegulatoryJurisdictionReference,
		direction domain.ManifestDirection,
		procedure domain.CustomsProcedureReference,
	) (CaseRequirementJudgment, bool, error)
}

// CaseRequirementRegistry 是 CaseRequirementView 的写口半边（第六本册子，挡的是
// establish_customs_case 那堵 EstablishCaseUndecided 墙，不在 W13 的两堵之内）。
// 判断内容对两个取值都必须带依据——「答否」与「没答」的续办动作完全不同，写口不得
// 引入任何把未登记折成「不要求」的路径。写入代数与其余五本同（ADR-0031，不 UPSERT）。
type CaseRequirementRegistry interface {
	RegisterCaseRequirementRule(
		ctx context.Context,
		tenant domain.TenantID,
		jurisdiction domain.RegulatoryJurisdictionReference,
		direction domain.ManifestDirection,
		procedure domain.CustomsProcedureReference,
		judgment CaseRequirementJudgment,
	) (CaseConfigurationSaveOutcome, error)
}

// CustomsCaseKey 是关务案件的身份键：固定监管范围四维——同一法律行为一案；同一
// 包裹进入另一独立监管程序自然换键（一包裹可关联多个彼此独立的案件）。
type CustomsCaseKey struct {
	TenantID     domain.TenantID
	Jurisdiction domain.RegulatoryJurisdictionReference
	Direction    domain.ManifestDirection
	Procedure    domain.CustomsProcedureReference
	Obligation   domain.ObligationScopeReference
}

type CustomsCaseSaveOutcome uint8

const (
	CustomsCaseSaveOutcomeInvalid CustomsCaseSaveOutcome = iota
	CustomsCaseSaved
	CustomsCaseAlreadyRecorded
)

// CustomsCaseStore 按身份键找回并保存案件（写入代数同 ADR-0031）。FindByID 是按铸造
// 标识的反查读口（ADR-0073 决定五）：提交链写入前核案件存在靠它——库侧
// customs_case_id_unique 唯一约束现成，范围键的职责收敛为建案幂等（ADR-0069 决定三）。
type CustomsCaseStore interface {
	FindByKey(ctx context.Context, key CustomsCaseKey) (domain.CustomsCase, bool, error)
	FindByID(ctx context.Context, tenant domain.TenantID, id domain.CustomsCaseID) (domain.CustomsCase, bool, error)
	Save(ctx context.Context, key CustomsCaseKey, customsCase domain.CustomsCase) (CustomsCaseSaveOutcome, error)
}

// CaseIdentityFactory 为新案件签发标识。
type CaseIdentityFactory interface {
	MintCaseID(ctx context.Context) (domain.CustomsCaseID, error)
}

// CustomsCaseHandoffIntent 把案件建立交给适用下游（申报链与 VE 案件视图消费）。意图
// 由案件键认领，重放重发同一份（ADR-0043）。
type CustomsCaseHandoffIntent struct {
	Key  CustomsCaseKey
	Case domain.CustomsCase
}

// CustomsCaseHandoff 把案件建立写入 Outbox（`OutboxCustomsCaseHandoff`）。信封 ID 由
// 案件键认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type CustomsCaseHandoff interface {
	HandOffCase(ctx context.Context, intent CustomsCaseHandoffIntent) error
}

// ExternalResultHandoffIntent 把已提交的接收记录交给判断与核对消费。意图由幂等键
// 认领，重放重发同一份（ADR-0043 同款纪律）；归属不上的留存记录没有可供判断消费的
// 监管事实，不产生意图。
type ExternalResultHandoffIntent struct {
	Record ExternalResultRecord
}

// ExternalResultHandoff 今天没有实现，唯一实现是测试替身。
type ExternalResultHandoff interface {
	HandOffExternalResult(ctx context.Context, intent ExternalResultHandoffIntent) error
}

type DeclarationUnitSaveOutcome uint8

const (
	DeclarationUnitSaveOutcomeInvalid DeclarationUnitSaveOutcome = iota
	DeclarationUnitSaved
	DeclarationUnitAlreadyRecorded
)

// DeclarationUnitStore 是申报单元的持久化本体（ADR-0073 决定一：CONTEXT 要求「独立
// 身份和可追溯组成」，jsonb 快照给不出独立身份）。Save 只建立、无更新路径——「同一
// 单元的案件维不得变更」由此在结构上承载（决定二）：同键已在册答`已有记录`，内容是否
// 一致由编排读回自己比，换案件即换（替代）单元。FindByID 供提交链取回单元身份与案件
// 维（重放一致性核对与意图载荷取数）；「案件→单元集」的反向查询按表上案件列带索引
// 查询即得（决定三），今天没有消费方，端口不预设方法。
type DeclarationUnitStore interface {
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.DeclarationUnit,
		formedAt time.Time,
	) (DeclarationUnitSaveOutcome, error)
	FindByID(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.DeclarationUnitID,
	) (domain.DeclarationUnit, bool, error)
}

// DeclarationSubmissionKey 是提交申报的幂等键：同一逻辑申报目标（租户+申报单元+监管
// 程序）重复提交返回原版本，不重复形成（CONTEXT 硬句 168：首次实际发送前形成不可
// 覆盖版本）。
type DeclarationSubmissionKey struct {
	TenantID  domain.TenantID
	Unit      domain.DeclarationUnitID
	Procedure domain.CustomsProcedureReference
}

// DeclarationSubmissionRecord 是一次提交申报越过提交边界留下的东西：不可覆盖的提交
// 版本与首次发送尝试。
type DeclarationSubmissionRecord struct {
	Key           DeclarationSubmissionKey
	ContentDigest string
	Version       domain.CustomsSubmissionVersion
	Attempt       domain.SubmissionAttempt
	RecordedAt    time.Time
	// CorrectedFrom 指名被本版本更正的前一版（原案内更正/补充，CONTEXT 硬句 169）；
	// 零值即首版。替代关系由源上下文随更正一并给出（VE CONTEXT「来源事实替代关系」
	// 的所有权句），这一格就是它的来处——下游消费按它登记替代，不自行推断谁更正了谁。
	CorrectedFrom domain.SubmissionVersionID
}

type DeclarationSubmissionSaveOutcome uint8

const (
	DeclarationSubmissionSaveOutcomeInvalid DeclarationSubmissionSaveOutcome = iota
	DeclarationSubmissionSaved
	DeclarationSubmissionAlreadyRecorded
)

// DeclarationCorrectionSaveOutcome 是原案内更正写入的封闭两格。`当前版已被换`不是
// 错误——并发更正先落或迟到重放都会撞上它，调用方读回当前版再按内容分格作答；没有
// 覆盖格是有意的：更正只允许接在当前版之后，接旧版等于把版本链改写成树。
type DeclarationCorrectionSaveOutcome uint8

const (
	DeclarationCorrectionSaveOutcomeInvalid DeclarationCorrectionSaveOutcome = iota
	DeclarationCorrectionSaved
	DeclarationCorrectionCurrentMoved
)

// DeclarationSubmissionStore 按幂等键找回并保存提交申报（写入代数同 ADR-0031）。
// FindByKey 交回当前版；FindByVersion 按版本读回留存版本（原案内更正后原版本永久
// 保留，CONTEXT 硬句 169——下游按信封宣告的版本取数，不受当前版推进影响）。
// SaveCorrection 在同一事务里把 CorrectedFrom 指名的当前版转为非当前并落新版本行，
// 前版内容一列不改。
type DeclarationSubmissionStore interface {
	FindByKey(ctx context.Context, key DeclarationSubmissionKey) (DeclarationSubmissionRecord, bool, error)
	FindByVersion(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.SubmissionVersionID,
	) (DeclarationSubmissionRecord, bool, error)
	Save(ctx context.Context, record DeclarationSubmissionRecord) (DeclarationSubmissionSaveOutcome, error)
	SaveCorrection(ctx context.Context, record DeclarationSubmissionRecord) (DeclarationCorrectionSaveOutcome, error)
}

// ReadinessView 取申报单元的就绪判断。found=false 表示资格目录/就绪规则未配置——
// 实例半边未提供时停在未决；found=true 而判断已失效即`不再就绪`，由调用方按业务
// 结果分格（就绪与授权分别形成和失效，CONTEXT 244）。
type ReadinessView interface {
	LoadReadiness(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.DeclarationUnitID,
	) (domain.ReadinessJudgment, bool, error)
}

// SubmissionAuthorityView 取申报单元的提交授权判断。与就绪读口同形三态：found=false
// 表示授权未配置（实例半边）；found=true 而判断已失效即`授权已失效`——分别形成和
// 失效的那半边在形状上有格可表，适配器不必把失效谎报成未配置或仍有效（CONTEXT 244）。
type SubmissionAuthorityView interface {
	LoadSubmissionAuthority(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.DeclarationUnitID,
	) (domain.SubmissionAuthorization, bool, error)
}

// CaseConfigurationSaveOutcome 是案件配置登记册的写入代数。只有两格，与本上下文其余
// 写口一致（ADR-0031）：`已登记`是业务答案不是错误。**没有覆盖格是有意的**——登记册
// 的写入方一律不做 UPSERT，同键已在册就交回`已登记`，内容是否一致由编排读回既有登记
// 自己比（同 SubmitDeclarationHandler 先 FindByKey 再比指纹那条路）。把比对放在编排
// 而不是 SQL 里，是为了让「重放同一份」与「换了内容」这两件事在用例结果上分得开。
type CaseConfigurationSaveOutcome uint8

const (
	CaseConfigurationSaveOutcomeInvalid CaseConfigurationSaveOutcome = iota
	CaseConfigurationRegistered
	CaseConfigurationAlreadyRegistered
)

// ReadinessRegistry 是 ReadinessView 的写口半边。就绪判断的内容属实例半边，但**放进
// 库里的那条受控路径属机制半边**——没有它，租户上线时这本册子今天没处配（W13）。
//
// 登记与撤销分成两个方法，不合成一个 Save：撤销是同一判断的状态推进而不是另一次登记
// （撤销不是删除，原依据与形成时间原样留在行内），合成一个写口就会让「重新登记」有机会
// 顶掉已撤销那一行的原依据。
type ReadinessRegistry interface {
	RegisterReadiness(
		ctx context.Context,
		tenant domain.TenantID,
		judgment domain.ReadinessJudgment,
	) (CaseConfigurationSaveOutcome, error)
	RevokeReadiness(
		ctx context.Context,
		tenant domain.TenantID,
		judgment domain.ReadinessJudgment,
	) error
}

// SubmissionAuthorityRegistry 是 SubmissionAuthorityView 的写口半边。与就绪分表分口
// ——两条轨分别形成和失效（CONTEXT 244），一个写口写两张表就等于让它们同生同灭。
//
// 同 SubmissionAuthorityView 的告诫：**这本册子不是接入认证**。这里登记的是「这个申报
// 单元有没有有效的提交授权依据」，不是「这个请求来自哪个租户」——租户是入参。
type SubmissionAuthorityRegistry interface {
	GrantSubmissionAuthority(
		ctx context.Context,
		tenant domain.TenantID,
		authorization domain.SubmissionAuthorization,
	) (CaseConfigurationSaveOutcome, error)
	RevokeSubmissionAuthority(
		ctx context.Context,
		tenant domain.TenantID,
		authorization domain.SubmissionAuthorization,
	) error
}

// InterpretationRuleRegistry 是 InterpretationRuleView 的写口半边。登记面按
// （租户，结果层，适用辖区，法定生效区间起）立键（ADR-0070 问一甲）。
//
// 终点不是登记输入：每个版本以开放区间进册，**后继版本登记时前版终点落定为后继起点**
// ——那是换版的唯一路径，与就绪/授权的撤销同款（状态推进，原规则与起点原样留在行内），
// 不是覆盖。W13 的不可覆盖语义在多版本形状下保持：同全键重放交回`已登记`由编排比对，
// 同键异 rule_ref 是冲突；起点早于既有开放版或撞进已闭合区间的登记同样只会交回
// `已登记`，读回比不上即冲突——历史区间是已记录的选择依据，不接受追改。登记因此
// 按生效起点升序进行。
type InterpretationRuleRegistry interface {
	RegisterInterpretationRule(
		ctx context.Context,
		tenant domain.TenantID,
		layer domain.ResultLayer,
		jurisdiction domain.RegulatoryJurisdictionReference,
		rule domain.InterpretationRuleReference,
		appliesFrom time.Time,
	) (CaseConfigurationSaveOutcome, error)
}

// ObligationRegistration 是一项关闭义务的登记内容：义务项加它的适用区间。区间必须
// 随项给出——盘点按业务截点进行（LoadObligationItems 走半开区间），没有区间的义务项
// 任何一次截点都盘不进来，等于登记了却永远不参与关闭判断。AppliesUntil 零值表示尚无
// 终点，不是「已失效」。
type ObligationRegistration struct {
	Item         domain.ClosureObligationItem
	AppliesFrom  time.Time
	AppliesUntil time.Time
}

// ObligationInventoryRegistry 是 ObligationInventoryView 的写口半边。
//
// 目录与明细分两个方法，和读口分两张表同一个理由：目录在场与明细行数是两个独立信号。
// 只登明细不登目录，读口答`未配置`；只登目录不登明细，读口答「已登记且本截点空清单」
// ——后者是「此案在此截点无适用义务」的如实答案，登记方必须能单独表达它。
type ObligationInventoryRegistry interface {
	RegisterObligationCatalog(
		ctx context.Context,
		tenant domain.TenantID,
		caseRef domain.CustomsCaseID,
		registeredAt time.Time,
	) (CaseConfigurationSaveOutcome, error)
	RegisterObligationItem(
		ctx context.Context,
		tenant domain.TenantID,
		caseRef domain.CustomsCaseID,
		registration ObligationRegistration,
	) (CaseConfigurationSaveOutcome, error)
}

// GateConditionRegistry 是 GateConditionView 的写口半边。目录与逐项判断分两个方法，
// 同义务盘点的理由；这里那一格尤其要紧——目录登记了却空清单是「此动作在此边界本就
// 不受门禁」，而目录未登记是未决，登记方必须分得开，否则就是用「查不到」冒充「不受管」。
type GateConditionRegistry interface {
	RegisterGateCatalog(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.DecisionScopeReference,
		action domain.GuardedAction,
		boundary domain.CustomsProcedureReference,
		registeredAt time.Time,
	) (CaseConfigurationSaveOutcome, error)
	RegisterGateFinding(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.DecisionScopeReference,
		action domain.GuardedAction,
		boundary domain.CustomsProcedureReference,
		finding domain.PreconditionFinding,
	) (CaseConfigurationSaveOutcome, error)
}

// DeclarationVersionFactory 签发提交版本标识。
type DeclarationVersionFactory interface {
	NextSubmissionVersion(ctx context.Context) (domain.SubmissionVersionID, error)
}

// DeclarationSubmissionHandoffIntent 把已固定的提交版本交给发送通道与外部结果核对
// 消费。意图由幂等键认领，重放重发同一份（ADR-0043 同款纪律）。Case 是单元所属案件
// （载荷补案件引用、不进分区键，ADR-0069 决定四/ADR-0073 决定五）；提交行不存案件列
// ——单元表那一列是这条边唯一的存储（决定三），意图从编排在手的单元身份取。
type DeclarationSubmissionHandoffIntent struct {
	Record DeclarationSubmissionRecord
	Case   domain.CustomsCaseID
}

// DeclarationSubmissionHandoff 把提交版本写入 Outbox（`OutboxDeclarationSubmissionHandoff`）。
// 信封 ID 由幂等键（租户+申报单元+监管程序）认领，入队由 outboxintent.EnqueueOnce 承担。
type DeclarationSubmissionHandoff interface {
	HandOffDeclarationSubmission(ctx context.Context, intent DeclarationSubmissionHandoffIntent) error
}

// CaseRequirementRuleEntry 是建案要求规则登记册的一行：监管范围三维与判断内容。依据
// 随行透出——「不要求」也是有依据的答案，上列时藏掉依据就分不出它与「没登记」。
type CaseRequirementRuleEntry struct {
	Jurisdiction domain.RegulatoryJurisdictionReference
	Direction    domain.ManifestDirection
	Procedure    domain.CustomsProcedureReference
	Judgment     CaseRequirementJudgment
}

// InterpretationRuleEntry 是解释规则登记册的一行：选择键三维、法定生效区间与规则引用
// （ADR-0070 问一甲的登记面形状）。AppliesUntil 零值即尚无终点（开放版），与
// ObligationRegistration 同约定——终点不是登记输入，它在后继版本登记时落定。
type InterpretationRuleEntry struct {
	Layer        domain.ResultLayer
	Jurisdiction domain.RegulatoryJurisdictionReference
	Rule         domain.InterpretationRuleReference
	AppliesFrom  time.Time
	AppliesUntil time.Time
}

// RuleCatalogueRead 是合规规则库的伴生列表读口（ADR-0077 Decision 一/五）：管理台
// 主数据页上列两本规则登记册——建案要求规则与解释规则。上列范围按词汇对照裁定：
// 这两本按监管维度立键、登记的是规则内容（ADR-0070 称两者同为关务规则）；案件配置里
// 就绪/提交授权/关闭义务/门禁条件四本按申报单元、案件或决定范围立键，是案件处理的
// 运行态，不属规则库页，不在本读口。
//
// 查阅不触发判断、决定或披露——它接存储读面，不接应用编排（分界句沿
// /shipment-request-views 先例）。不拓宽既有写口与判断读口：扩既有接口会拆全部测试
// 替身，伴生读口另立（ADR-0077 Decision 五）。租户维在方法签名上；limit 必须为正，
// 页大小由接入面按渠道契约裁决，读口只拒绝无意义的取值。
type RuleCatalogueRead interface {
	ListCaseRequirementRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]CaseRequirementRuleEntry, error)
	ListInterpretationRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]InterpretationRuleEntry, error)
}
