package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ErrUnexpectedSubmissionSave 说明提交库交回了封闭集合以外的写入结果。
var ErrUnexpectedSubmissionSave = errors.New("customs compliance: unexpected declaration submission save outcome")

// DeclarationOutcome 是一次提交申报的应用处理结果。`不再就绪`是业务负向结果——答案
// 已知（恢复动作是重新取得就绪），既不是等依赖的`未决`也不是改请求的`未受理`
// （ADR-0029 按恢复动作分格）。
type DeclarationOutcome uint8

const (
	DeclarationOutcomeInvalid DeclarationOutcome = iota
	DeclarationSubmitted
	DeclarationExistingVersion
	DeclarationSourceConflict
	DeclarationNotReady
	DeclarationNotAuthorized
	DeclarationNotAccepted
	DeclarationUndecided
	DeclarationCaseUnknown
	DeclarationUnitConflict
	DeclarationCorrected
	DeclarationPriorMissing
	DeclarationCorrectionUnbased
)

func (outcome DeclarationOutcome) String() string {
	switch outcome {
	case DeclarationSubmitted:
		return "DECLARATION_SUBMITTED"
	case DeclarationExistingVersion:
		return "EXISTING_VERSION"
	case DeclarationSourceConflict:
		return "SOURCE_CONFLICT"
	case DeclarationNotReady:
		return "NOT_READY"
	case DeclarationNotAuthorized:
		return "NOT_AUTHORIZED"
	case DeclarationNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case DeclarationUndecided:
		return "DECLARATION_UNDECIDED"
	case DeclarationCaseUnknown:
		return "CASE_UNKNOWN"
	case DeclarationUnitConflict:
		return "UNIT_CONFLICT"
	case DeclarationCorrected:
		return "DECLARATION_CORRECTED"
	case DeclarationPriorMissing:
		return "PRIOR_SUBMISSION_NOT_FOUND"
	case DeclarationCorrectionUnbased:
		return "CORRECTION_TARGET_NOT_CURRENT"
	default:
		return ""
	}
}

// DeclarationUndecidedReason 指名提交停在哪一步等谁。就绪规则与提交授权各占一格——
// 它们是两条实例缝，未配置分别可见（CONTEXT 244 双轨）。
type DeclarationUndecidedReason uint8

const (
	DeclarationUndecidedReasonNone DeclarationUndecidedReason = iota
	SubmissionStoreUnavailable
	ReadinessUnavailable
	ReadinessUnconfigured
	AuthorityUnavailable
	AuthorityUnconfigured
	VersionIdentityUnavailable
	CaseAuthorityUnavailable
	UnitStoreUnavailable
	FollowUpStoreUnavailable
)

func (reason DeclarationUndecidedReason) String() string {
	switch reason {
	case SubmissionStoreUnavailable:
		return "SUBMISSION_STORE_UNAVAILABLE"
	case ReadinessUnavailable:
		return "READINESS_UNAVAILABLE"
	case ReadinessUnconfigured:
		return "READINESS_UNCONFIGURED"
	case AuthorityUnavailable:
		return "AUTHORITY_UNAVAILABLE"
	case AuthorityUnconfigured:
		return "AUTHORITY_UNCONFIGURED"
	case VersionIdentityUnavailable:
		return "VERSION_IDENTITY_UNAVAILABLE"
	case CaseAuthorityUnavailable:
		return "CASE_LOOKUP_UNAVAILABLE"
	case UnitStoreUnavailable:
		return "UNIT_STORE_UNAVAILABLE"
	case FollowUpStoreUnavailable:
		return "FOLLOW_UP_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// SubmitDeclarationCommand 携带一次提交申报的全部输入：单元与组成、所属案件、资料/
// 角色快照引用、发送目标与首次尝试的已知结果。案件维必填（ADR-0073 决定五）：案件
// 先于申报存在，提交前按标识反查核存在——不核等于让调用方随手填一个字符串。
type SubmitDeclarationCommand struct {
	TenantID      domain.TenantID
	UnitID        string
	CaseID        string
	Procedure     string
	Members       []string
	Dossier       string
	Roles         string
	Target        string
	InitialResult domain.AttemptResult
	SentAt        time.Time
}

type SubmitDeclarationResult struct {
	outcome      DeclarationOutcome
	reason       DeclarationUndecidedReason
	record       ports.DeclarationSubmissionRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result SubmitDeclarationResult) Outcome() DeclarationOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result SubmitDeclarationResult) UndecidedReason() DeclarationUndecidedReason {
	return result.reason
}

func (result SubmitDeclarationResult) Record() (ports.DeclarationSubmissionRecord, bool) {
	return result.record, result.hasRecord
}

func (result SubmitDeclarationResult) ContinuationReference() string {
	return result.continuation
}

// SubmissionHandoffReference 非空说明版本已固定但意图还没交出去，重放会重发同一份。
func (result SubmitDeclarationResult) SubmissionHandoffReference() string {
	return result.handoff
}

type SubmitDeclarationDeps struct {
	Submissions ports.DeclarationSubmissionStore
	Cases       ports.CustomsCaseStore
	Units       ports.DeclarationUnitStore
	Readiness   ports.ReadinessView
	Authority   ports.SubmissionAuthorityView
	Versions    ports.DeclarationVersionFactory
	Downstream  ports.DeclarationSubmissionHandoff
	Clock       ports.Clock
}

type SubmitDeclarationHandler struct {
	deps SubmitDeclarationDeps
}

func NewSubmitDeclarationHandler(deps SubmitDeclarationDeps) *SubmitDeclarationHandler {
	return &SubmitDeclarationHandler{deps: deps}
}

// Handle 把一个申报单元推进到不可覆盖的提交版本：受理（单元+案件+资料/角色快照）→
// 幂等/冲突按内容指纹分界（重放返原版本不重形成，CONTEXT「首次实际对外发送前都必须形成不可覆盖的提交版本」；重放的案件一致性对单元
// 本体核）→ 案件反查核存在（悬空引用拒绝，ADR-0073 决定五）→ 就绪读口（未配置→
// 未决；不再就绪→业务负向）→ 提交授权（与就绪分开，双有效才成版，CONTEXT 244）→
// 单元本体落册（同键异身份→冲突，ADR-0073 决定一/二）→ FixSubmissionVersion+
// InitialAttempt → 原子提交 → 发布意图（载荷带案件维）。
func (handler *SubmitDeclarationHandler) Handle(
	ctx context.Context,
	command SubmitDeclarationCommand,
) (SubmitDeclarationResult, error) {
	unit, err := formUnit(command)
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
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		strings.TrimSpace(command.Target) == "" ||
		command.SentAt.IsZero() {
		return SubmitDeclarationResult{outcome: DeclarationNotAccepted}, nil
	}

	key := ports.DeclarationSubmissionKey{
		TenantID:  command.TenantID,
		Unit:      unit.ID(),
		Procedure: unit.Procedure(),
	}
	digest := declarationDigest(command)
	existing, found, err := handler.deps.Submissions.FindByKey(ctx, key)
	if err != nil {
		return submissionStoreUndecided(command.UnitID), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一逻辑申报目标携带不同组成或快照：已固定版本不可覆盖，不在这里顶替。
			// 保留单元身份的修订走原案内更正/补充（CorrectDeclarationHandler，要先有
			// 已形成的后续动作目标）；不保留身份的走撤销重报（新逻辑申报目标）。
			return SubmitDeclarationResult{outcome: DeclarationSourceConflict}, nil
		}
		// 重复提交：返回原版本，不重复形成（CONTEXT「首次实际对外发送前都必须形成不可覆盖的提交版本」）。内容指纹不含案件维（案件属
		// 单元身份不属提交内容），重放的案件一致性对单元本体核——同单元换案件不是
		// 重放，是撞上「案件维成立即定」（ADR-0073 决定二）。
		stored, unitFound, err := handler.deps.Units.FindByID(ctx, command.TenantID, unit.ID())
		if err != nil {
			return unitStoreUndecided(command.UnitID), nil
		}
		if !unitFound {
			// 提交在册而单元本体缺行是坏状态：Save 把单元钉在提交之前，缺行不该可见。
			return unitStoreUndecided(command.UnitID), nil
		}
		if !sameUnitIdentity(stored, unit) {
			return SubmitDeclarationResult{outcome: DeclarationUnitConflict}, nil
		}
		return handler.existingResult(ctx, existing, stored.Case()), nil
	}

	// 案件先于申报存在（ADR-0073 决定五）：按铸造标识反查核在册，悬空引用在入库前
	// 拒绝——建案后重来，不是重试能消化的未决。
	if _, caseFound, err := handler.deps.Cases.FindByID(ctx, command.TenantID, unit.Case()); err != nil {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: CaseAuthorityUnavailable,
			continuation: declarationContinuation("CASE_LOOKUP_UNAVAILABLE", command.UnitID)}, nil
	} else if !caseFound {
		return SubmitDeclarationResult{outcome: DeclarationCaseUnknown}, nil
	}

	readiness, configured, err := handler.deps.Readiness.LoadReadiness(ctx, command.TenantID, unit.ID())
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: ReadinessUnavailable,
			continuation: declarationContinuation("READINESS_UNAVAILABLE", command.UnitID)}, nil
	}
	if !configured {
		// 就绪规则/资格目录是实例半边：未配置停在未决，不默认就绪。
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: ReadinessUnconfigured,
			continuation: declarationContinuation("READINESS_UNCONFIGURED", command.UnitID)}, nil
	}
	if !readiness.Effective() {
		// 不再就绪：原判断保留但不得继续支持实际提交——业务负向，重新取得就绪再来。
		return SubmitDeclarationResult{outcome: DeclarationNotReady}, nil
	}

	authorization, granted, err := handler.deps.Authority.LoadSubmissionAuthority(ctx, command.TenantID, unit.ID())
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: AuthorityUnavailable,
			continuation: declarationContinuation("AUTHORITY_UNAVAILABLE", command.UnitID)}, nil
	}
	if !granted {
		// 授权与就绪分别形成和失效：就绪在场也顶替不了授权（CONTEXT 244）。
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: AuthorityUnconfigured,
			continuation: declarationContinuation("AUTHORITY_UNCONFIGURED", command.UnitID)}, nil
	}
	if !authorization.Effective() {
		// 授权已失效：原授予保留但不得继续支持实际提交——业务负向，重新取得授权再来
		//（与不再就绪平行的另一条轨，恢复动作提示不再错成「等实例参数」）。
		return SubmitDeclarationResult{outcome: DeclarationNotAuthorized}, nil
	}

	// 单元本体先于版本落册（ADR-0073 决定一/二）：同键已在册就读回比对——同一单元
	// 换案件、换程序或换组成都是身份冲突，绝不顶替；输给身份竞争的请求不再消耗版本
	// 标识。单元行落了而后续步骤失败只留下一个已形成的单元（CONTEXT：进行中可以形成
	// 一个或多个申报单元），重试自然续上。
	saved, err := handler.deps.Units.Save(ctx, command.TenantID, unit, handler.deps.Clock.Now())
	if err != nil {
		return unitStoreUndecided(command.UnitID), nil
	}
	if saved == ports.DeclarationUnitAlreadyRecorded {
		stored, unitFound, err := handler.deps.Units.FindByID(ctx, command.TenantID, unit.ID())
		if err != nil || !unitFound {
			return unitStoreUndecided(command.UnitID), nil
		}
		if !sameUnitIdentity(stored, unit) {
			return SubmitDeclarationResult{outcome: DeclarationUnitConflict}, nil
		}
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

	return handler.commit(ctx, ports.DeclarationSubmissionRecord{
		Key:           key,
		ContentDigest: digest,
		Version:       version,
		Attempt:       attempt,
		RecordedAt:    handler.deps.Clock.Now(),
	}, unit.Case())
}

func formUnit(command SubmitDeclarationCommand) (domain.DeclarationUnit, error) {
	unitID, err := domain.NewDeclarationUnitID(command.UnitID)
	if err != nil {
		return domain.DeclarationUnit{}, err
	}
	customsCase, err := domain.NewCustomsCaseID(command.CaseID)
	if err != nil {
		return domain.DeclarationUnit{}, err
	}
	procedure, err := domain.NewCustomsProcedureReference(command.Procedure)
	if err != nil {
		return domain.DeclarationUnit{}, err
	}
	members := make([]domain.DeclaredParcelReference, 0, len(command.Members))
	for _, raw := range command.Members {
		member, err := domain.NewDeclaredParcelReference(raw)
		if err != nil {
			return domain.DeclarationUnit{}, err
		}
		members = append(members, member)
	}
	return domain.FormDeclarationUnit(unitID, customsCase, procedure, members)
}

// sameUnitIdentity 比较单元身份内容：案件、程序与组成集合。组成按排序比——形成顺序
// 不构成不同的组成（与提交内容指纹同一口径）；形成时刻是记录事实不是身份内容，不比。
func sameUnitIdentity(stored, formed domain.DeclarationUnit) bool {
	if stored.Case() != formed.Case() || stored.Procedure() != formed.Procedure() {
		return false
	}
	storedMembers := memberStrings(stored)
	formedMembers := memberStrings(formed)
	if len(storedMembers) != len(formedMembers) {
		return false
	}
	for index := range storedMembers {
		if storedMembers[index] != formedMembers[index] {
			return false
		}
	}
	return true
}

func memberStrings(unit domain.DeclarationUnit) []string {
	members := make([]string, 0, len(unit.Members()))
	for _, member := range unit.Members() {
		members = append(members, member.String())
	}
	sort.Strings(members)
	return members
}

func submissionStoreUndecided(unitID string) SubmitDeclarationResult {
	return SubmitDeclarationResult{
		outcome:      DeclarationUndecided,
		reason:       SubmissionStoreUnavailable,
		continuation: declarationContinuation("SUBMISSION_STORE_UNAVAILABLE", unitID),
	}
}

func unitStoreUndecided(unitID string) SubmitDeclarationResult {
	return SubmitDeclarationResult{
		outcome:      DeclarationUndecided,
		reason:       UnitStoreUnavailable,
		continuation: declarationContinuation("UNIT_STORE_UNAVAILABLE", unitID),
	}
}

// commit 提交记录并交发布意图；并发下另一方先提交时读回赢家。赢家与本请求同键即
// 同单元，单元的案件维已在前一步核过一致，意图照用它。
func (handler *SubmitDeclarationHandler) commit(
	ctx context.Context,
	record ports.DeclarationSubmissionRecord,
	customsCase domain.CustomsCaseID,
) (SubmitDeclarationResult, error) {
	saved, err := handler.deps.Submissions.Save(ctx, record)
	if err != nil {
		return submissionStoreUndecided(record.Key.Unit.String()), nil
	}
	switch saved {
	case ports.DeclarationSubmissionSaved:
		result := SubmitDeclarationResult{outcome: DeclarationSubmitted, record: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record, customsCase)
		return result, nil
	case ports.DeclarationSubmissionAlreadyRecorded:
		winner, found, err := handler.deps.Submissions.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return submissionStoreUndecided(record.Key.Unit.String()), nil
		}
		return handler.existingResult(ctx, winner, customsCase), nil
	default:
		return SubmitDeclarationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedSubmissionSave, saved)
	}
}

// existingResult 按已有记录作答并重发同一份意图。
func (handler *SubmitDeclarationHandler) existingResult(
	ctx context.Context,
	record ports.DeclarationSubmissionRecord,
	customsCase domain.CustomsCaseID,
) SubmitDeclarationResult {
	return SubmitDeclarationResult{
		outcome:   DeclarationExistingVersion,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record, customsCase),
	}
}

// handOff 交发布意图。投递失败不翻结果，留续办引用重放时重发同一份。
func (handler *SubmitDeclarationHandler) handOff(
	ctx context.Context,
	record ports.DeclarationSubmissionRecord,
	customsCase domain.CustomsCaseID,
) string {
	intent := ports.DeclarationSubmissionHandoffIntent{Record: record, Case: customsCase}
	if err := handler.deps.Downstream.HandOffDeclarationSubmission(ctx, intent); err == nil {
		return ""
	}
	return declarationContinuation("DECLARATION_SUBMISSION_HANDOFF", record.Key.TenantID.String(), record.Key.Unit.String())
}

func declarationContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// declarationDigest 是同一逻辑申报目标的内容比对锚：组成、资料快照与角色快照任一
// 不同即是另一份内容。成员先排序——提交顺序不构成不同的内容。
func declarationDigest(command SubmitDeclarationCommand) string {
	members := append([]string(nil), command.Members...)
	sort.Strings(members)
	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		command.Procedure,
		command.Dossier,
		command.Roles,
	}, members...), "\x00")))
	return hex.EncodeToString(digest[:])
}
