package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidDeclarationUnit      = errors.New("customs compliance: invalid declaration unit")
	ErrInvalidReadiness            = errors.New("customs compliance: invalid readiness judgment")
	ErrReadinessAlreadyRevoked     = errors.New("customs compliance: the readiness is already revoked")
	ErrInvalidAuthorization        = errors.New("customs compliance: invalid submission authorization")
	ErrAuthorizationAlreadyRevoked = errors.New("customs compliance: the authorization is already revoked")
	ErrInvalidSubmissionVersion    = errors.New("customs compliance: invalid submission version")
	ErrInvalidSubmissionAttempt    = errors.New("customs compliance: invalid submission attempt")
	ErrUnsafeResend                = errors.New("customs compliance: no safe-resend judgment for this version")
)

// DeclarationUnitID 是申报单元的独立身份——包裹、客户委托、集运单元、总单、运输
// 舱单、监管舱单或班次都不能直接替代它（CONTEXT「申报单元必须具有独立身份和可追溯组成」）。
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
//
// 案件维随形成即定且不可变更（ADR-0073 决定二）：「一个案件可以关联多个申报单元」
// 是多个单元各自指向同一案件；单元换案件即建立替代单元，不改这一个。类型上没有
// 改案件的方法，持久化层也没有更新路径。
type DeclarationUnit struct {
	id          DeclarationUnitID
	customsCase CustomsCaseID
	procedure   CustomsProcedureReference
	members     []DeclaredParcelReference
}

func FormDeclarationUnit(
	id DeclarationUnitID,
	customsCase CustomsCaseID,
	procedure CustomsProcedureReference,
	members []DeclaredParcelReference,
) (DeclarationUnit, error) {
	if !id.valid() || !customsCase.valid() || !procedure.valid() || len(members) == 0 {
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
		id:          id,
		customsCase: customsCase,
		procedure:   procedure,
		members:     append([]DeclaredParcelReference(nil), members...),
	}, nil
}

func (unit DeclarationUnit) ID() DeclarationUnitID {
	return unit.id
}

// Case 是本单元所属的关务案件（多对一，成立即定）。
func (unit DeclarationUnit) Case() CustomsCaseID {
	return unit.customsCase
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

// Revocation 只在已失效的判断上给出原因与时间。没有这个出口，登记册适配器写得进
// 原判断却写不进失效那两列，`不再就绪`在库里就只能靠删行或改写原依据表达——两者
// 都与「撤销不是删除」相悖（同 ExternalResult 三件出口的理由）。
func (readiness ReadinessJudgment) Revocation() (string, time.Time, bool) {
	return readiness.revokedBy, readiness.revokedAt, !readiness.Effective()
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

// SubmissionAuthorization 是提交授权判断——与 ReadinessJudgment 同形的另一条轨
// （CONTEXT 244：「提交授权与就绪判断分别形成和失效」）。授权失效不是删除：原依据
// 与授予时间保留，但不得继续支持实际提交；没有这半边，失效授权在读口上只能被误读成
// 「未配置」或「仍有效」。
type SubmissionAuthorization struct {
	unit      DeclarationUnitID
	authority SubmissionAuthorityReference
	grantedAt time.Time
	revokedBy string
	revokedAt time.Time
}

func GrantSubmissionAuthority(
	unit DeclarationUnitID,
	authority SubmissionAuthorityReference,
	grantedAt time.Time,
) (SubmissionAuthorization, error) {
	if !unit.valid() || !authority.valid() || grantedAt.IsZero() {
		return SubmissionAuthorization{}, ErrInvalidAuthorization
	}
	return SubmissionAuthorization{unit: unit, authority: authority, grantedAt: grantedAt.UTC()}, nil
}

func (authorization SubmissionAuthorization) Unit() DeclarationUnitID {
	return authorization.unit
}

func (authorization SubmissionAuthorization) Authority() SubmissionAuthorityReference {
	return authorization.authority
}

func (authorization SubmissionAuthorization) GrantedAt() time.Time {
	return authorization.grantedAt
}

// Effective 报告授权是否仍然有效。
func (authorization SubmissionAuthorization) Effective() bool {
	return authorization.revokedAt.IsZero()
}

// Revocation 只在已失效的授权上给出原因与时间。与就绪同一条理由：没有出口就写不出
// `授权已失效`那一格。
func (authorization SubmissionAuthorization) Revocation() (string, time.Time, bool) {
	return authorization.revokedBy, authorization.revokedAt, !authorization.Effective()
}

// Revoke 记录授权失效：委托关系、资质或授权范围发生适用变化。原授予（依据与时间）
// 原样保留——这不是删除，是失效。
func (authorization SubmissionAuthorization) Revoke(cause string, at time.Time) (SubmissionAuthorization, error) {
	if !authorization.Effective() {
		return SubmissionAuthorization{}, ErrAuthorizationAlreadyRevoked
	}
	if cause == "" || at.IsZero() || at.Before(authorization.grantedAt) {
		return SubmissionAuthorization{}, ErrInvalidAuthorization
	}
	revoked := authorization
	revoked.revokedBy = cause
	revoked.revokedAt = at.UTC()
	return revoked, nil
}

// CustomsSubmissionVersionSpec 是固定一个提交版本所需的全部输入。就绪与授权都以
// 判断对象进入——两条轨各自的有效性在成版处同权重把门。
type CustomsSubmissionVersionSpec struct {
	ID            SubmissionVersionID
	Unit          DeclarationUnit
	Dossier       DossierSnapshotReference
	Roles         RoleSnapshotReference
	Readiness     ReadinessJudgment
	Authorization SubmissionAuthorization
	FixedAt       time.Time
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
		!spec.Authorization.authority.valid() ||
		spec.FixedAt.IsZero() {
		return CustomsSubmissionVersion{}, ErrInvalidSubmissionVersion
	}
	if !spec.Readiness.Effective() || spec.Readiness.unit != spec.Unit.id {
		// 不再就绪的判断不得继续支持实际提交；别的单元的就绪也支持不了这个单元。
		return CustomsSubmissionVersion{}, ErrInvalidSubmissionVersion
	}
	if !spec.Authorization.Effective() || spec.Authorization.unit != spec.Unit.id {
		// 授权与就绪同权重：失效授权固定不出版本，别的单元的授权也支持不了这个单元
		// （CONTEXT 244 双有效才成版）。
		return CustomsSubmissionVersion{}, ErrInvalidSubmissionVersion
	}
	return CustomsSubmissionVersion{
		id:        spec.ID,
		unit:      spec.Unit.id,
		members:   spec.Unit.Members(),
		dossier:   spec.Dossier,
		roles:     spec.Roles,
		basis:     spec.Readiness.basis,
		authority: spec.Authorization.authority,
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

func (version CustomsSubmissionVersion) FixedAt() time.Time {
	return version.fixedAt
}

// CustomsSubmissionVersionSnapshot 是持久化层重建提交版本所需的全量状态。就绪判断
// 与授权在成版那一刻已把过门，快照只带核对结果的引用——重建不重演 FixSubmissionVersion
// （重演需要判断对象在手，而库里只有版本固定时留下的依据引用）。
type CustomsSubmissionVersionSnapshot struct {
	ID        SubmissionVersionID
	Unit      DeclarationUnitID
	Members   []DeclaredParcelReference
	Dossier   DossierSnapshotReference
	Roles     RoleSnapshotReference
	Basis     ReadinessBasisReference
	Authority SubmissionAuthorityReference
	FixedAt   time.Time
}

// Snapshot 折出提交版本的全量状态供持久化。
func (version CustomsSubmissionVersion) Snapshot() CustomsSubmissionVersionSnapshot {
	return CustomsSubmissionVersionSnapshot{
		ID:        version.id,
		Unit:      version.unit,
		Members:   version.Members(),
		Dossier:   version.dossier,
		Roles:     version.roles,
		Basis:     version.basis,
		Authority: version.authority,
		FixedAt:   version.fixedAt,
	}
}

// RehydrateSubmissionVersion 从快照重建提交版本。读回的东西同样要过一遍不变量——
// 组成快照非空不重复、八件引用齐全在这里重验，一次坏写入不得变成一个看起来合法的
// 提交版本。
func RehydrateSubmissionVersion(snapshot CustomsSubmissionVersionSnapshot) (CustomsSubmissionVersion, error) {
	if !snapshot.ID.valid() ||
		!snapshot.Unit.valid() ||
		!snapshot.Dossier.valid() ||
		!snapshot.Roles.valid() ||
		!snapshot.Basis.valid() ||
		!snapshot.Authority.valid() ||
		snapshot.FixedAt.IsZero() ||
		len(snapshot.Members) == 0 {
		return CustomsSubmissionVersion{}, ErrInvalidSubmissionVersion
	}
	seen := make(map[DeclaredParcelReference]bool, len(snapshot.Members))
	for _, member := range snapshot.Members {
		if !member.valid() || seen[member] {
			return CustomsSubmissionVersion{}, ErrInvalidSubmissionVersion
		}
		seen[member] = true
	}
	return CustomsSubmissionVersion{
		id:        snapshot.ID,
		unit:      snapshot.Unit,
		members:   append([]DeclaredParcelReference(nil), snapshot.Members...),
		dossier:   snapshot.Dossier,
		roles:     snapshot.Roles,
		basis:     snapshot.Basis,
		authority: snapshot.Authority,
		fixedAt:   snapshot.FixedAt.UTC(),
	}, nil
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
// 同版本再次尝试必须携带安全再次发送判断——在那之前不得盲目重发（CONTEXT「在此之前不得盲目重发」）。
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

func (attempt SubmissionAttempt) Target() string {
	return attempt.target
}

func (attempt SubmissionAttempt) SentAt() time.Time {
	return attempt.sentAt
}

// SubmissionAttemptSnapshot 是持久化层重建发送尝试所需的全量状态。
type SubmissionAttemptSnapshot struct {
	Version    SubmissionVersionID
	Sequence   int
	Target     string
	Result     AttemptResult
	SafeResend SafeResendReference
	SentAt     time.Time
}

// Snapshot 折出发送尝试的全量状态供持久化。
func (attempt SubmissionAttempt) Snapshot() SubmissionAttemptSnapshot {
	return SubmissionAttemptSnapshot{
		Version:    attempt.version,
		Sequence:   attempt.sequence,
		Target:     attempt.target,
		Result:     attempt.result,
		SafeResend: attempt.safeResend,
		SentAt:     attempt.sentAt,
	}
}

// RehydrateSubmissionAttempt 从快照重建发送尝试并重验形状：首次尝试没有安全再次
// 发送判断、受控重发必须携带（CONTEXT「在此之前不得盲目重发」的构造期与读回期是同一道门）。
func RehydrateSubmissionAttempt(snapshot SubmissionAttemptSnapshot) (SubmissionAttempt, error) {
	if !snapshot.Version.valid() ||
		snapshot.Sequence < 1 ||
		snapshot.Target == "" ||
		!snapshot.Result.valid() ||
		snapshot.SentAt.IsZero() {
		return SubmissionAttempt{}, ErrInvalidSubmissionAttempt
	}
	if (snapshot.Sequence == 1) == snapshot.SafeResend.valid() {
		return SubmissionAttempt{}, ErrInvalidSubmissionAttempt
	}
	return SubmissionAttempt{
		version:    snapshot.Version,
		sequence:   snapshot.Sequence,
		target:     snapshot.Target,
		result:     snapshot.Result,
		safeResend: snapshot.SafeResend,
		sentAt:     snapshot.SentAt.UTC(),
	}, nil
}
