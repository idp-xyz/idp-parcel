// Package ports 声明 customs-compliance 应用层与外界的边界。这些只是接口：适配器仍
// 阻断在 Bento 持久化闸门之后（ADR-0017），今天唯一的实现是测试替身。
package ports

import (
	"context"
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

// SubmissionAuthorityView 取申报单元的提交授权。与就绪判断是两条轨：双有效才成版，
// 任一缺席都不得以另一个顶替（CONTEXT 244）。found=false 表示授权未配置。
type SubmissionAuthorityView interface {
	LoadSubmissionAuthority(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.DeclarationUnitID,
	) (domain.SubmissionAuthorityReference, bool, error)
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
