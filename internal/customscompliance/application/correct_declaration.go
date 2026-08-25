package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ErrFollowUpTargetInconsistent 说明按键取回的后续动作目标指着另一个单元或案件。
// FormFollowUpTarget 在写入侧把六件锚在一起，读回不符即仓储不变量已破——响亮报错，
// 不落任何业务格。
var ErrFollowUpTargetInconsistent = fmt.Errorf(
	"customs compliance: the follow-up target disagrees with the unit it claims to correct")

// CorrectDeclarationCommand 携带一次原案内更正/补充的全部输入。单元身份四件（单元、
// 案件、程序、组成）用于与在册单元核对——原案内更正保留申报单元身份（CONTEXT 硬句
// 169/172），身份任何一件不同都不是更正，是撞身份冲突。Trigger 与 Kind 指名已形成的
// 后续动作目标：目标按（触发依据+被更正版本+动作类型）立键，被更正版本由编排取当前
// 版补齐，不采信调用方自报。
type CorrectDeclarationCommand struct {
	TenantID  domain.TenantID
	UnitID    string
	CaseID    string
	Procedure string
	Members   []string
	// Dossier/Roles 是新的正式申报资料与角色快照引用（原案内新资料，CONTEXT 生命
	// 周期「后续申报动作与替代」第二箭头）。
	Dossier string
	Roles   string
	// Trigger 与 Kind 定位授权本次更正的后续动作目标；Kind 只收原案内两道
	// （补充/更正），撤销与重报各有自己的对象，不从这里走。
	Trigger       string
	Kind          domain.FollowUpActionKind
	Target        string
	InitialResult domain.AttemptResult
	SentAt        time.Time
}

type CorrectDeclarationDeps struct {
	Submissions ports.DeclarationSubmissionStore
	Units       ports.DeclarationUnitStore
	FollowUps   ports.FollowUpStore
	Readiness   ports.ReadinessView
	Authority   ports.SubmissionAuthorityView
	Versions    ports.DeclarationVersionFactory
	Downstream  ports.DeclarationSubmissionHandoff
	Clock       ports.Clock
}

type CorrectDeclarationHandler struct {
	deps CorrectDeclarationDeps
}

func NewCorrectDeclarationHandler(deps CorrectDeclarationDeps) *CorrectDeclarationHandler {
	return &CorrectDeclarationHandler{deps: deps}
}

// Handle 在原案件内形成新的提交版本（CONTEXT 硬句 169、生命周期「原案内补充或更正
// 目标已形成 → 形成新的正式申报资料准备版本，并针对新的拟提交动作重新经过就绪、
// 授权、提交」）：受理（单元四件+新资料快照+目标指名）→ 当前版在册（无版无可更正）→
// 幂等（新内容与当前版同指纹即重放返原）→ 单元身份核对（更正保留身份，任何一件不同
// 即冲突）→ 后续动作目标按**当前版**键住（目标缺席、目标对旧版、并发换版三种情形同
// 一格：对当前版重新形成后续决定再来）→ 就绪与授权重新取得（不得复用首次申报的判断，
// 失效各按业务负向作答）→ 新版本固定 + 首次尝试 → SaveCorrection 翻旧插新 → 发布
// 意图（信封按版本认领，前身随记录携带供下游登记替代关系）。
func (handler *CorrectDeclarationHandler) Handle(
	ctx context.Context,
	command CorrectDeclarationCommand,
) (SubmitDeclarationResult, error) {
	unit, err := formUnit(SubmitDeclarationCommand{
		UnitID:    command.UnitID,
		CaseID:    command.CaseID,
		Procedure: command.Procedure,
		Members:   command.Members,
	})
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationNotAccepted}, nil
	}
	dossier, err := domain.NewDossierSnapshotReference(command.Dossier)
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationNotAccepted}, nil
	}
	roles, err := domain.NewRoleSnapshotReference(command.Roles)
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationNotAccepted}, nil
	}
	trigger, err := domain.NewFollowUpTriggerReference(command.Trigger)
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationNotAccepted}, nil
	}
	if command.Kind != domain.InCaseSupplement && command.Kind != domain.InCaseCorrection {
		// 撤销是自身提交的新监管动作、重报是新逻辑申报目标（CONTEXT 硬句 172 四道
		// 分立）——都不产生原案内新版本，收下等于替它们伪造一条捷径。
		return SubmitDeclarationResult{outcome: DeclarationNotAccepted}, nil
	}
	if command.TenantID.String() == "" || command.Target == "" || command.SentAt.IsZero() {
		return SubmitDeclarationResult{outcome: DeclarationNotAccepted}, nil
	}

	key := ports.DeclarationSubmissionKey{
		TenantID:  command.TenantID,
		Unit:      unit.ID(),
		Procedure: unit.Procedure(),
	}
	current, found, err := handler.deps.Submissions.FindByKey(ctx, key)
	if err != nil {
		return submissionStoreUndecided(command.UnitID), nil
	}
	if !found {
		// 没有在册提交就没有「原案内」可言——首次提交走 SubmitDeclarationHandler。
		return SubmitDeclarationResult{outcome: DeclarationPriorMissing}, nil
	}

	digest := declarationDigest(SubmitDeclarationCommand{
		Procedure: command.Procedure,
		Members:   command.Members,
		Dossier:   command.Dossier,
		Roles:     command.Roles,
	})
	if current.ContentDigest == digest {
		// 与当前版同内容：重放返原版本，不重复形成，也不消耗版本标识。
		return handler.replayExisting(ctx, command, unit, current)
	}

	stored, unitFound, err := handler.deps.Units.FindByID(ctx, command.TenantID, unit.ID())
	if err != nil || !unitFound {
		// 提交在册而单元本体缺行是坏状态：Save 把单元钉在提交之前，缺行不该可见。
		return unitStoreUndecided(command.UnitID), nil
	}
	if !sameUnitIdentity(stored, unit) {
		// 原案内更正保留申报单元身份（案件、程序、组成）；改身份走替代申报单元或
		// 替代案件（CONTEXT 硬句 174），不在这里吸收。
		return SubmitDeclarationResult{outcome: DeclarationUnitConflict}, nil
	}

	if result, stop, err := handler.requireTargetOnCurrent(ctx, command, trigger, unit, current); err != nil {
		return SubmitDeclarationResult{}, err
	} else if stop {
		return result, nil
	}

	readiness, configured, err := handler.deps.Readiness.LoadReadiness(ctx, command.TenantID, unit.ID())
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: ReadinessUnavailable,
			continuation: declarationContinuation("READINESS_UNAVAILABLE", command.UnitID)}, nil
	}
	if !configured {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: ReadinessUnconfigured,
			continuation: declarationContinuation("READINESS_UNCONFIGURED", command.UnitID)}, nil
	}
	if !readiness.Effective() {
		return SubmitDeclarationResult{outcome: DeclarationNotReady}, nil
	}

	authorization, granted, err := handler.deps.Authority.LoadSubmissionAuthority(ctx, command.TenantID, unit.ID())
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: AuthorityUnavailable,
			continuation: declarationContinuation("AUTHORITY_UNAVAILABLE", command.UnitID)}, nil
	}
	if !granted {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: AuthorityUnconfigured,
			continuation: declarationContinuation("AUTHORITY_UNCONFIGURED", command.UnitID)}, nil
	}
	if !authorization.Effective() {
		return SubmitDeclarationResult{outcome: DeclarationNotAuthorized}, nil
	}

	versionID, err := handler.deps.Versions.NextSubmissionVersion(ctx)
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: VersionIdentityUnavailable,
			continuation: declarationContinuation("VERSION_IDENTITY_UNAVAILABLE", command.UnitID)}, nil
	}
	version, err := domain.FixSubmissionVersion(domain.CustomsSubmissionVersionSpec{
		ID:            versionID,
		Unit:          unit,
		Dossier:       dossier,
		Roles:         roles,
		Readiness:     readiness,
		Authorization: authorization,
		FixedAt:       handler.deps.Clock.Now(),
	})
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationNotAccepted}, nil
	}
	attempt, err := domain.InitialAttempt(version, command.Target, command.InitialResult, command.SentAt)
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationNotAccepted}, nil
	}

	record := ports.DeclarationSubmissionRecord{
		Key:           key,
		ContentDigest: digest,
		Version:       version,
		Attempt:       attempt,
		RecordedAt:    handler.deps.Clock.Now(),
		CorrectedFrom: current.Version.ID(),
	}
	saved, err := handler.deps.Submissions.SaveCorrection(ctx, record)
	if err != nil {
		return submissionStoreUndecided(command.UnitID), nil
	}
	switch saved {
	case ports.DeclarationCorrectionSaved:
		result := SubmitDeclarationResult{outcome: DeclarationCorrected, record: record, hasRecord: true}
		result.handoff = handler.handOffCorrection(ctx, record, unit.Case())
		return result, nil
	case ports.DeclarationCorrectionCurrentMoved:
		// 当前版在读回与落库之间被换（并发更正先落，或同一份更正的重放输了竞态）。
		// 读回赢家：内容同即重放返原；不同则目标已不再键住当前版——按同一格作答，
		// 对当前版重新形成后续决定再来。
		winner, winnerFound, err := handler.deps.Submissions.FindByKey(ctx, key)
		if err != nil || !winnerFound {
			return submissionStoreUndecided(command.UnitID), nil
		}
		if winner.ContentDigest == digest {
			return handler.existingCorrection(ctx, winner, unit.Case()), nil
		}
		return SubmitDeclarationResult{outcome: DeclarationCorrectionUnbased}, nil
	default:
		return SubmitDeclarationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedSubmissionSave, saved)
	}
}

// requireTargetOnCurrent 核对已形成的后续动作目标按当前版键住。stop 为真时 result
// 是要交回的答案；目标读回但指着别的单元/案件时交回错误——写入侧 FormFollowUpTarget
// 锚死六件，读回不符是仓储不变量已破，不是业务格。
func (handler *CorrectDeclarationHandler) requireTargetOnCurrent(
	ctx context.Context,
	command CorrectDeclarationCommand,
	trigger domain.FollowUpTriggerReference,
	unit domain.DeclarationUnit,
	current ports.DeclarationSubmissionRecord,
) (result SubmitDeclarationResult, stop bool, err error) {
	targetKey := ports.FollowUpTargetKey{
		TenantID: command.TenantID,
		Trigger:  trigger,
		Version:  current.Version.ID(),
		Kind:     command.Kind,
	}
	target, found, findErr := handler.deps.FollowUps.FindTarget(ctx, targetKey)
	if findErr != nil {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: FollowUpStoreUnavailable,
			continuation: declarationContinuation("FOLLOW_UP_STORE_UNAVAILABLE", command.UnitID)}, true, nil
	}
	if !found {
		// 目标从未形成、目标形成于旧版、当前版刚被并发换掉——三种情形恢复动作同一：
		// 对当前版形成（新的）后续申报处理决定，再来更正。不合并进 NOT_ACCEPTED：
		// 那格的恢复动作是改请求，这格是先走另一个用例。
		return SubmitDeclarationResult{outcome: DeclarationCorrectionUnbased}, true, nil
	}
	if target.Unit() != unit.ID() || target.CaseRef() != unit.Case() {
		return SubmitDeclarationResult{}, true, fmt.Errorf("%w: unit %q target unit %q",
			ErrFollowUpTargetInconsistent, unit.ID(), target.Unit())
	}
	return SubmitDeclarationResult{}, false, nil
}

// replayExisting 是更正的重放半边：内容与当前版一致时返回原版本并重发同一份意图，
// 单元身份仍要核（同内容不同身份是坏请求，不是重放）。
func (handler *CorrectDeclarationHandler) replayExisting(
	ctx context.Context,
	command CorrectDeclarationCommand,
	unit domain.DeclarationUnit,
	current ports.DeclarationSubmissionRecord,
) (SubmitDeclarationResult, error) {
	stored, unitFound, err := handler.deps.Units.FindByID(ctx, command.TenantID, unit.ID())
	if err != nil || !unitFound {
		return unitStoreUndecided(command.UnitID), nil
	}
	if !sameUnitIdentity(stored, unit) {
		return SubmitDeclarationResult{outcome: DeclarationUnitConflict}, nil
	}
	return handler.existingCorrection(ctx, current, stored.Case()), nil
}

func (handler *CorrectDeclarationHandler) existingCorrection(
	ctx context.Context,
	record ports.DeclarationSubmissionRecord,
	customsCase domain.CustomsCaseID,
) SubmitDeclarationResult {
	return SubmitDeclarationResult{
		outcome:   DeclarationExistingVersion,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOffCorrection(ctx, record, customsCase),
	}
}

// handOffCorrection 交发布意图。投递失败不翻结果，留续办引用重放时重发同一份——
// 与首版提交同一条纪律。
func (handler *CorrectDeclarationHandler) handOffCorrection(
	ctx context.Context,
	record ports.DeclarationSubmissionRecord,
	customsCase domain.CustomsCaseID,
) string {
	intent := ports.DeclarationSubmissionHandoffIntent{Record: record, Case: customsCase}
	if err := handler.deps.Downstream.HandOffDeclarationSubmission(ctx, intent); err == nil {
		return ""
	}
	return declarationContinuation("DECLARATION_SUBMISSION_HANDOFF",
		record.Key.TenantID.String(), record.Key.Unit.String(), record.Version.ID().String())
}
