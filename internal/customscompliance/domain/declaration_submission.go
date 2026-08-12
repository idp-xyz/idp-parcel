package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidDeclarationUnit   = errors.New("customs compliance: invalid declaration unit")
	ErrInvalidReadiness         = errors.New("customs compliance: invalid readiness judgment")
	ErrReadinessAlreadyRevoked  = errors.New("customs compliance: the readiness is already revoked")
	ErrInvalidSubmissionVersion = errors.New("customs compliance: invalid submission version")
	ErrInvalidSubmissionAttempt = errors.New("customs compliance: invalid submission attempt")
	ErrUnsafeResend             = errors.New("customs compliance: no safe-resend judgment for this version")
)

// DeclarationUnitID 是申报单元的独立身份——包裹、客户委托、集运单元、总单、运输
// 舱单、监管舱单或班次都不能直接替代它（CONTEXT 硬句 143）。
type DeclarationUnitID struct{ requiredValue }

func NewDeclarationUnitID(value string) (DeclarationUnitID, error) {
	required, err := newRequiredValue("declaration unit ID", value)
	return DeclarationUnitID{required}, err
}

// DeclaredParcelReference 指名申报单元组成中的一个包裹。
type DeclaredParcelReference struct{ requiredValue }

func NewDeclaredParcelReference(value string) (DeclaredParcelReference, error) {
	required, err := newRequiredValue("declared parcel reference", value)
	return DeclaredParcelReference{required}, err
}

// CustomsProcedureReference 指名监管程序（辖区、方向与程序的组合引用）。
type CustomsProcedureReference struct{ requiredValue }

func NewCustomsProcedureReference(value string) (CustomsProcedureReference, error) {
	required, err := newRequiredValue("customs procedure reference", value)
	return CustomsProcedureReference{required}, err
}

// DeclarationUnit 是一次申报、审查、查验、放行或撤销重报的对象集合：独立身份加可
// 追溯组成。组成在这里可变——它在逻辑提交版本形成时才被快照固定（CONTEXT 生命周期
// 「申报单元的组成在逻辑提交版本形成时固定」），已提交版本保持不变。
type DeclarationUnit struct {
	id        DeclarationUnitID
	procedure CustomsProcedureReference
	members   []DeclaredParcelReference
}

func FormDeclarationUnit(
	id DeclarationUnitID,
	procedure CustomsProcedureReference,
	members []DeclaredParcelReference,
) (DeclarationUnit, error) {
	if !id.valid() || !procedure.valid() || len(members) == 0 {
		return DeclarationUnit{}, ErrInvalidDeclarationUnit
	}
	seen := make(map[DeclaredParcelReference]bool, len(members))
	for _, member := range members {
		if !member.valid() || seen[member] {
			return DeclarationUnit{}, ErrInvalidDeclarationUnit
		}
		seen[member] = true
	}
	return DeclarationUnit{
		id:        id,
		procedure: procedure,
		members:   append([]DeclaredParcelReference(nil), members...),
	}, nil
}

func (unit DeclarationUnit) ID() DeclarationUnitID {
	return unit.id
}

func (unit DeclarationUnit) Procedure() CustomsProcedureReference {
	return unit.procedure
}

func (unit DeclarationUnit) Members() []DeclaredParcelReference {
	return append([]DeclaredParcelReference(nil), unit.members...)
}

// ReadinessBasisReference 指名就绪判断的依据（必要资料、角色资格、合报资格、凭证、
// 口岸/渠道与限制条件的核对结果）。
type ReadinessBasisReference struct{ requiredValue }

func NewReadinessBasisReference(value string) (ReadinessBasisReference, error) {
	required, err := newRequiredValue("readiness basis reference", value)
	return ReadinessBasisReference{required}, err
}

// ReadinessJudgment 是申报就绪判断：就绪必须绑定依据和适用时间；提交前适用条件变化
// 即`不再就绪`——原判断保留，但不得继续支持实际提交（CONTEXT 生命周期）。
type ReadinessJudgment struct {
	unit      DeclarationUnitID
	basis     ReadinessBasisReference
	judgedAt  time.Time
	revokedBy string
	revokedAt time.Time
}

func JudgeReady(
	unit DeclarationUnitID,
	basis ReadinessBasisReference,
	judgedAt time.Time,
) (ReadinessJudgment, error) {
	if !unit.valid() || !basis.valid() || judgedAt.IsZero() {
		return ReadinessJudgment{}, ErrInvalidReadiness
	}
	return ReadinessJudgment{unit: unit, basis: basis, judgedAt: judgedAt.UTC()}, nil
}

func (readiness ReadinessJudgment) Unit() DeclarationUnitID {
	return readiness.unit
}

func (readiness ReadinessJudgment) Basis() ReadinessBasisReference {
	return readiness.basis
}

func (readiness ReadinessJudgment) JudgedAt() time.Time {
	return readiness.judgedAt
}

// Effective 报告就绪是否仍然有效。
func (readiness ReadinessJudgment) Effective() bool {
	return readiness.revokedAt.IsZero()
}

// Revoke 记录`不再就绪`：资料、资格、规则、凭证、口岸、渠道或限制发生适用变化。
// 原判断（依据与时间）原样保留——这不是删除，是失效。
func (readiness ReadinessJudgment) Revoke(cause string, at time.Time) (ReadinessJudgment, error) {
	if !readiness.Effective() {
		return ReadinessJudgment{}, ErrReadinessAlreadyRevoked
	}
	if cause == "" || at.IsZero() || at.Before(readiness.judgedAt) {
		return ReadinessJudgment{}, ErrInvalidReadiness
	}
	revoked := readiness
	revoked.revokedBy = cause
	revoked.revokedAt = at.UTC()
	return revoked, nil
}

// SubmissionVersionID 是提交版本的标识。
type SubmissionVersionID struct{ requiredValue }

func NewSubmissionVersionID(value string) (SubmissionVersionID, error) {
	required, err := newRequiredValue("submission version ID", value)
	return SubmissionVersionID{required}, err
}

// DossierSnapshotReference 指名正式申报资料快照（字段级溯源在快照本体上）。
type DossierSnapshotReference struct{ requiredValue }

func NewDossierSnapshotReference(value string) (DossierSnapshotReference, error) {
	required, err := newRequiredValue("dossier snapshot reference", value)
	return DossierSnapshotReference{required}, err
}

// RoleSnapshotReference 指名关务参与方角色与资格快照。
type RoleSnapshotReference struct{ requiredValue }

func NewRoleSnapshotReference(value string) (RoleSnapshotReference, error) {
	required, err := newRequiredValue("role snapshot reference", value)
	return RoleSnapshotReference{required}, err
}

// SubmissionAuthorityReference 指名提交授权依据。授权与就绪分别形成和失效。
type SubmissionAuthorityReference struct{ requiredValue }

func NewSubmissionAuthorityReference(value string) (SubmissionAuthorityReference, error) {
	required, err := newRequiredValue("submission authority reference", value)
	return SubmissionAuthorityReference{required}, err
}

// CustomsSubmissionVersionSpec 是固定一个提交版本所需的全部输入。
type CustomsSubmissionVersionSpec struct {
	ID        SubmissionVersionID
	Unit      DeclarationUnit
	Dossier   DossierSnapshotReference
	Roles     RoleSnapshotReference
	Readiness ReadinessJudgment
	Authority SubmissionAuthorityReference
	FixedAt   time.Time
}

// CustomsSubmissionVersion 是逻辑申报目标首次实际对外发送前固定的不可覆盖快照
// （CONTEXT「提交版本」）。申报单元组成在此刻快照固定；就绪判断与授权必须都有效
// ——两者任一失效都固定不出版本；值类型无任何回写入口，后续变化不得回写已提交快照。
// 版本的存在不证明技术传输、监管接收、业务受理或放行成功——那些是尝试与外部结果
// 的事，这个类型上没有它们的字段。
type CustomsSubmissionVersion struct {
	id        SubmissionVersionID
	unit      DeclarationUnitID
	members   []DeclaredParcelReference
	dossier   DossierSnapshotReference
	roles     RoleSnapshotReference
	basis     ReadinessBasisReference
	authority SubmissionAuthorityReference
	fixedAt   time.Time
}

func FixSubmissionVersion(spec CustomsSubmissionVersionSpec) (CustomsSubmissionVersion, error) {
	if !spec.ID.valid() ||
		!spec.Unit.id.valid() ||
		!spec.Dossier.valid() ||
		!spec.Roles.valid() ||
		!spec.Authority.valid() ||
		spec.FixedAt.IsZero() {
		return CustomsSubmissionVersion{}, ErrInvalidSubmissionVersion
	}
	if !spec.Readiness.Effective() || spec.Readiness.unit != spec.Unit.id {
		// 不再就绪的判断不得继续支持实际提交；别的单元的就绪也支持不了这个单元。
		return CustomsSubmissionVersion{}, ErrInvalidSubmissionVersion
	}
	return CustomsSubmissionVersion{
		id:        spec.ID,
		unit:      spec.Unit.id,
		members:   spec.Unit.Members(),
		dossier:   spec.Dossier,
		roles:     spec.Roles,
		basis:     spec.Readiness.basis,
		authority: spec.Authority,
		fixedAt:   spec.FixedAt.UTC(),
	}, nil
}

func (version CustomsSubmissionVersion) ID() SubmissionVersionID {
	return version.id
}

func (version CustomsSubmissionVersion) Unit() DeclarationUnitID {
	return version.unit
}

// Members 是提交版本固定时快照的组成——单元后续变化不影响它。
func (version CustomsSubmissionVersion) Members() []DeclaredParcelReference {
	return append([]DeclaredParcelReference(nil), version.members...)
}

func (version CustomsSubmissionVersion) Dossier() DossierSnapshotReference {
	return version.dossier
}

func (version CustomsSubmissionVersion) Roles() RoleSnapshotReference {
	return version.roles
}

func (version CustomsSubmissionVersion) ReadinessBasis() ReadinessBasisReference {
	return version.basis
}

func (version CustomsSubmissionVersion) Authority() SubmissionAuthorityReference {
	return version.authority
}

// AttemptResult 是提交尝试的已知结果封闭三值。结果未知保持待确认——超时不得直接
// 解释为失败（CONTEXT「提交尝试」）。
type AttemptResult uint8

const (
	AttemptResultInvalid AttemptResult = iota
	AttemptAcknowledged
	AttemptFailed
	AttemptPendingConfirmation
)

func (result AttemptResult) valid() bool {
	return result >= AttemptAcknowledged && result <= AttemptPendingConfirmation
}

func (result AttemptResult) String() string {
	switch result {
	case AttemptAcknowledged:
		return "ACKNOWLEDGED"
	case AttemptFailed:
		return "FAILED"
	case AttemptPendingConfirmation:
		return "PENDING_CONFIRMATION"
	default:
		return ""
	}
}

// SafeResendReference 指名安全再次发送判断：合格来源与真实规则共同证明原尝试没有
// 形成可继续有效的外部申报身份，且重发不会造成重复申报。
type SafeResendReference struct{ requiredValue }

func NewSafeResendReference(value string) (SafeResendReference, error) {
	required, err := newRequiredValue("safe resend reference", value)
	return SafeResendReference{required}, err
}

// SubmissionAttempt 是针对明确提交版本实际发起的一次对外发送。首次尝试随版本形成；
// 同版本再次尝试必须携带安全再次发送判断——在那之前不得盲目重发（CONTEXT 硬句 170）。
type SubmissionAttempt struct {
	version    SubmissionVersionID
	sequence   int
	target     string
	result     AttemptResult
	safeResend SafeResendReference
	sentAt     time.Time
}

// InitialAttempt 形成版本的首次发送尝试。
func InitialAttempt(
	version CustomsSubmissionVersion,
	target string,
	result AttemptResult,
	sentAt time.Time,
) (SubmissionAttempt, error) {
	if !version.id.valid() || target == "" || !result.valid() || sentAt.IsZero() {
		return SubmissionAttempt{}, ErrInvalidSubmissionAttempt
	}
	return SubmissionAttempt{
		version:  version.id,
		sequence: 1,
		target:   target,
		result:   result,
		sentAt:   sentAt.UTC(),
	}, nil
}

// ControlledResend 依据安全再次发送判断对同一版本形成受控新尝试。安全判断必备——
// 没有它的重发与盲目重发分不开；新尝试不改前次尝试的任何结果。
func (attempt SubmissionAttempt) ControlledResend(
	safeResend SafeResendReference,
	result AttemptResult,
	sentAt time.Time,
) (SubmissionAttempt, error) {
	if !safeResend.valid() {
		return SubmissionAttempt{}, ErrUnsafeResend
	}
	if !result.valid() || sentAt.IsZero() || sentAt.Before(attempt.sentAt) {
		return SubmissionAttempt{}, ErrInvalidSubmissionAttempt
	}
	return SubmissionAttempt{
		version:    attempt.version,
		sequence:   attempt.sequence + 1,
		target:     attempt.target,
		result:     result,
		safeResend: safeResend,
		sentAt:     sentAt.UTC(),
	}, nil
}

func (attempt SubmissionAttempt) Version() SubmissionVersionID {
	return attempt.version
}

func (attempt SubmissionAttempt) Sequence() int {
	return attempt.sequence
}

func (attempt SubmissionAttempt) Result() AttemptResult {
	return attempt.result
}

// SafeResend 只在受控重发尝试上给出。
func (attempt SubmissionAttempt) SafeResend() (SafeResendReference, bool) {
	return attempt.safeResend, attempt.safeResend.valid()
}

func (attempt SubmissionAttempt) SentAt() time.Time {
	return attempt.sentAt
}
