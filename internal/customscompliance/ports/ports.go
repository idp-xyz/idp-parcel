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

// InterpretationRuleView 取该层外部响应的解释规则配置。found=false 表示解释规则未
// 配置——实例半边未提供时解释停在未决，不用默认口径猜测监管语义。
type InterpretationRuleView interface {
	LoadInterpretationRule(
		ctx context.Context,
		tenant domain.TenantID,
		layer domain.ResultLayer,
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

// VerificationHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017
// 的 Bento/Outbox 闸门。
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
		caseRef string,
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
		caseRef string,
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

// CaseClosureHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017
// 的 Bento/Outbox 闸门。
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
// ——它们只执行不豁免）。
type RestrictionHandoffIntent struct {
	TenantID    domain.TenantID
	Restriction domain.RegulatoryRestriction
}

// RestrictionHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017
// 的 Bento/Outbox 闸门。
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
// 门禁满足不生成放行，放行结果仍由外部事实接收）。
type GateVerificationHandoffIntent struct {
	Key  GateVerificationKey
	Gate domain.ReleaseGateVerification
}

// GateVerificationHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于
// ADR-0017 的 Bento/Outbox 闸门。
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
// 与案件视图消费生效）。
type FollowUpHandoffIntent struct {
	Key      FollowUpTargetKey
	Target   domain.FollowUpTarget
	Relation *domain.ReplacementRelation
}

// FollowUpHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017 的
// Bento/Outbox 闸门。
type FollowUpHandoff interface {
	HandOffFollowUp(ctx context.Context, intent FollowUpHandoffIntent) error
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
}

type DeclarationSubmissionSaveOutcome uint8

const (
	DeclarationSubmissionSaveOutcomeInvalid DeclarationSubmissionSaveOutcome = iota
	DeclarationSubmissionSaved
	DeclarationSubmissionAlreadyRecorded
)

// DeclarationSubmissionStore 按幂等键找回并保存提交申报（写入代数同 ADR-0031）。
type DeclarationSubmissionStore interface {
	FindByKey(ctx context.Context, key DeclarationSubmissionKey) (DeclarationSubmissionRecord, bool, error)
	Save(ctx context.Context, record DeclarationSubmissionRecord) (DeclarationSubmissionSaveOutcome, error)
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

// DeclarationVersionFactory 签发提交版本标识。
type DeclarationVersionFactory interface {
	NextSubmissionVersion(ctx context.Context) (domain.SubmissionVersionID, error)
}

// DeclarationSubmissionHandoffIntent 把已固定的提交版本交给发送通道与外部结果核对
// 消费。意图由幂等键认领，重放重发同一份（ADR-0043 同款纪律）。
type DeclarationSubmissionHandoffIntent struct {
	Record DeclarationSubmissionRecord
}

// DeclarationSubmissionHandoff 今天没有实现，唯一实现是测试替身。
type DeclarationSubmissionHandoff interface {
	HandOffDeclarationSubmission(ctx context.Context, intent DeclarationSubmissionHandoffIntent) error
}
