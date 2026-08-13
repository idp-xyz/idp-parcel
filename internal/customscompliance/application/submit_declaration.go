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
	DeclarationNotAccepted
	DeclarationUndecided
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
	case DeclarationNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case DeclarationUndecided:
		return "DECLARATION_UNDECIDED"
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
	default:
		return ""
	}
}

// SubmitDeclarationCommand 携带一次提交申报的全部输入：单元与组成、资料/角色快照
// 引用、发送目标与首次尝试的已知结果。
type SubmitDeclarationCommand struct {
	TenantID      domain.TenantID
	UnitID        string
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

// Handle 把一个申报单元推进到不可覆盖的提交版本：受理（单元+资料/角色快照）→ 幂等/
// 冲突按内容指纹分界（重放返原版本不重形成，硬句 168）→ 就绪读口（未配置→未决；
// 不再就绪→业务负向）→ 提交授权（与就绪分开，双有效才成版，CONTEXT 244）→
// FixSubmissionVersion+InitialAttempt → 原子提交 → 发布意图。
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
			// 同一逻辑申报目标携带不同组成或快照：已固定版本不可覆盖，修订走撤销
			// 重报，不在这里顶替。
			return SubmitDeclarationResult{outcome: DeclarationSourceConflict}, nil
		}
		// 重复提交：返回原版本，不重复形成（硬句 168）。
		return handler.existingResult(ctx, existing), nil
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

	authority, granted, err := handler.deps.Authority.LoadSubmissionAuthority(ctx, command.TenantID, unit.ID())
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: AuthorityUnavailable,
			continuation: declarationContinuation("AUTHORITY_UNAVAILABLE", command.UnitID)}, nil
	}
	if !granted {
		// 授权与就绪分别形成和失效：就绪在场也顶替不了授权（CONTEXT 244）。
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: AuthorityUnconfigured,
			continuation: declarationContinuation("AUTHORITY_UNCONFIGURED", command.UnitID)}, nil
	}

	versionID, err := handler.deps.Versions.NextSubmissionVersion(ctx)
	if err != nil {
		return SubmitDeclarationResult{outcome: DeclarationUndecided, reason: VersionIdentityUnavailable,
			continuation: declarationContinuation("VERSION_IDENTITY_UNAVAILABLE", command.UnitID)}, nil
	}
	version, err := domain.FixSubmissionVersion(domain.CustomsSubmissionVersionSpec{
		ID:        versionID,
		Unit:      unit,
		Dossier:   dossier,
		Roles:     roles,
		Readiness: readiness,
		Authority: authority,
		FixedAt:   handler.deps.Clock.Now(),
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
	})
}

func formUnit(command SubmitDeclarationCommand) (domain.DeclarationUnit, error) {
	unitID, err := domain.NewDeclarationUnitID(command.UnitID)
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
	return domain.FormDeclarationUnit(unitID, procedure, members)
}

func submissionStoreUndecided(unitID string) SubmitDeclarationResult {
	return SubmitDeclarationResult{
		outcome:      DeclarationUndecided,
		reason:       SubmissionStoreUnavailable,
		continuation: declarationContinuation("SUBMISSION_STORE_UNAVAILABLE", unitID),
	}
}

// commit 提交记录并交发布意图；并发下另一方先提交时读回赢家。
func (handler *SubmitDeclarationHandler) commit(
	ctx context.Context,
	record ports.DeclarationSubmissionRecord,
) (SubmitDeclarationResult, error) {
	saved, err := handler.deps.Submissions.Save(ctx, record)
	if err != nil {
		return submissionStoreUndecided(record.Key.Unit.String()), nil
	}
	switch saved {
	case ports.DeclarationSubmissionSaved:
		result := SubmitDeclarationResult{outcome: DeclarationSubmitted, record: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.DeclarationSubmissionAlreadyRecorded:
		winner, found, err := handler.deps.Submissions.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return submissionStoreUndecided(record.Key.Unit.String()), nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return SubmitDeclarationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedSubmissionSave, saved)
	}
}

// existingResult 按已有记录作答并重发同一份意图。
func (handler *SubmitDeclarationHandler) existingResult(
	ctx context.Context,
	record ports.DeclarationSubmissionRecord,
) SubmitDeclarationResult {
	return SubmitDeclarationResult{
		outcome:   DeclarationExistingVersion,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record),
	}
}

// handOff 交发布意图。投递失败不翻结果，留续办引用重放时重发同一份。
func (handler *SubmitDeclarationHandler) handOff(
	ctx context.Context,
	record ports.DeclarationSubmissionRecord,
) string {
	if err := handler.deps.Downstream.HandOffDeclarationSubmission(ctx, ports.DeclarationSubmissionHandoffIntent{Record: record}); err == nil {
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
