package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrUnexpectedAdoptionSave 说明采用库交回了封闭集合以外的写入结果。
var ErrUnexpectedAdoptionSave = errors.New("parcel shipment: unexpected intake adoption save outcome")

// IntakeAdoptionOutcome 是来源采用请求的应用处理结果。`正式承诺已形成`与`来源不采用`
// 是领域走向，`资格判断未决`是处理结果不是包裹生命周期状态——用例明写三者不能混。
type IntakeAdoptionOutcome uint8

const (
	IntakeAdoptionOutcomeInvalid IntakeAdoptionOutcome = iota
	IntakeCommitmentFormed
	IntakeSourceNotAdopted
	IntakeEligibilityUndecided
	IntakeNotApplicable
	IntakeExistingResult
	IntakeSourceConflict
	IntakeRequestNotAccepted
)

func (outcome IntakeAdoptionOutcome) String() string {
	switch outcome {
	case IntakeCommitmentFormed:
		return "COMMITMENT_FORMED"
	case IntakeSourceNotAdopted:
		return "SOURCE_NOT_ADOPTED"
	case IntakeEligibilityUndecided:
		return "ELIGIBILITY_UNDECIDED"
	case IntakeNotApplicable:
		return "NOT_APPLICABLE"
	case IntakeExistingResult:
		return "EXISTING_RESULT"
	case IntakeSourceConflict:
		return "SOURCE_CONFLICT"
	case IntakeRequestNotAccepted:
		return "REQUEST_NOT_ACCEPTED"
	default:
		return ""
	}
}

// IntakeUndecidedReason 指名资格判断停在哪一步。封闭集合，按依赖阶段分类统计。
type IntakeUndecidedReason uint8

const (
	IntakeUndecidedReasonNone IntakeUndecidedReason = iota
	IntakeRequestUnavailable
	IntakeEligibilityUnavailable
	IntakeEligibilityUnconfigured
	IntakeEligibilityNotEstablishedYet
	IntakeAdoptionStoreUnavailable
	IntakeCommitmentIdentityUnavailable
	IntakeCancellationViewUnavailable
	IntakeCancellationOrderConflict
	// IntakeCorrectionPredecessorUnjudged：来源自报更正的那一版在采用账上还没有任何记录——
	// 先后两封乱序，或前版那封还没消费到。重投会改变结果，所以是未决不是拒（ADR-0117 决定五）。
	IntakeCorrectionPredecessorUnjudged
)

func (reason IntakeUndecidedReason) String() string {
	switch reason {
	case IntakeRequestUnavailable:
		return "REQUEST_UNAVAILABLE"
	case IntakeEligibilityUnavailable:
		return "ELIGIBILITY_UNAVAILABLE"
	case IntakeEligibilityUnconfigured:
		return "ELIGIBILITY_UNCONFIGURED"
	case IntakeEligibilityNotEstablishedYet:
		return "ELIGIBILITY_NOT_ESTABLISHED"
	case IntakeAdoptionStoreUnavailable:
		return "ADOPTION_STORE_UNAVAILABLE"
	case IntakeCommitmentIdentityUnavailable:
		return "COMMITMENT_IDENTITY_UNAVAILABLE"
	case IntakeCancellationViewUnavailable:
		return "CANCELLATION_VIEW_UNAVAILABLE"
	case IntakeCancellationOrderConflict:
		return "CANCELLATION_ORDER_CONFLICT"
	case IntakeCorrectionPredecessorUnjudged:
		return "CORRECTION_PREDECESSOR_UNJUDGED"
	default:
		return ""
	}
}

// AdoptNetworkIntakeCommand 携带来源原料与目标委托的双重指名（来源身份 + 委托编号，
// 与撤回同一模式）。
type AdoptNetworkIntakeCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Source            domain.IntakeSourceSpec
}

type AdoptNetworkIntakeResult struct {
	outcome      IntakeAdoptionOutcome
	record       ports.IntakeAdoptionRecord
	hasRecord    bool
	basis        domain.CheckReason
	reason       IntakeUndecidedReason
	continuation domain.OwnershipContinuationReference
	handoff      domain.OwnershipContinuationReference
}

func (result AdoptNetworkIntakeResult) Outcome() IntakeAdoptionOutcome {
	return result.outcome
}

// Record 只在越过提交边界或找回已有结果时给出。
func (result AdoptNetworkIntakeResult) Record() (ports.IntakeAdoptionRecord, bool) {
	return result.record, result.hasRecord
}

// Basis 在`不采用`与`不适用`时携带依据；`未决`的资格缺口也从这里读。
func (result AdoptNetworkIntakeResult) Basis() domain.CheckReason {
	return result.basis
}

func (result AdoptNetworkIntakeResult) UndecidedReason() IntakeUndecidedReason {
	return result.reason
}

func (result AdoptNetworkIntakeResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// IntakeHandoffReference 非空说明结果已提交但意图还没交出去，重放会重发同一份。
func (result AdoptNetworkIntakeResult) IntakeHandoffReference() domain.OwnershipContinuationReference {
	return result.handoff
}

type AdoptNetworkIntakeDeps struct {
	Requests    ports.ShipmentRequestRepository
	Eligibility ports.IntakeEligibilityView
	Adoptions   ports.IntakeAdoptionStore
	Identities  ports.CommitmentIdentityFactory
	Downstream  ports.NetworkIntakeHandoff
	Clock       ports.Clock
	// Cancellations 是包裹级取消决定的读口（UC-PS-006）。nil 与「取消机制未接入」
	// 同义：边界核验整段不做——UC-PS-006 编排落地前的装配没有取消可查。
	Cancellations ports.ParcelCancellationView
}

type AdoptNetworkIntakeHandler struct {
	deps AdoptNetworkIntakeDeps
}

func NewAdoptNetworkIntakeHandler(deps AdoptNetworkIntakeDeps) *AdoptNetworkIntakeHandler {
	return &AdoptNetworkIntakeHandler{deps: deps}
}

// Handle 把一份物理来源推进到采用结论：受理（来源五件与委托指名）→ 幂等/冲突按内容
// 指纹分界 → 取消边界按业务时间裁决 → 资格（未配置即未决，不默认通过）→ 责任起点
// 唯一，或同来源更正接续链尾 → 采用提交与发布意图。物理事实全程只读——不采用与未决
// 都不碰来源。更正版本把前面几道门重走一遍，用的是更正后的内容（tf/08 裁决说的「再判
// 一次」）；被拒时链尾原样站着。
func (handler *AdoptNetworkIntakeHandler) Handle(
	ctx context.Context,
	command AdoptNetworkIntakeCommand,
) (AdoptNetworkIntakeResult, error) {
	source, err := domain.NewIntakeSource(command.Source)
	if err != nil {
		return AdoptNetworkIntakeResult{outcome: IntakeRequestNotAccepted}, nil
	}

	// 委托按来源身份加编号双重指名。查无、编号不符与跨租户指名同一个答案——可区分即可
	// 枚举别人的委托与承诺（AT-PS-052 的统一不可见）。
	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return handler.undecided(command, source, IntakeRequestUnavailable), nil
	}
	if !found || request.ShipmentRequestID() != command.ShipmentRequestID {
		return AdoptNetworkIntakeResult{outcome: IntakeRequestNotAccepted}, nil
	}

	key := ports.IntakeAdoptionKey{
		TenantID: command.Identity.TenantID(),
		Parcel:   source.Parcel(),
		Kind:     source.Kind(),
		Version:  source.Version(),
	}
	digest := intakeContentDigest(source)
	existing, found, err := handler.deps.Adoptions.FindByKey(ctx, key)
	if err != nil {
		return handler.undecided(command, source, IntakeAdoptionStoreUnavailable), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一采用身份携带不同来源内容：冲突保留原结果（AT-PS-042）。
			return AdoptNetworkIntakeResult{outcome: IntakeSourceConflict}, nil
		}
		return handler.existingResult(ctx, existing), nil
	}

	// 委托必须已经合法接受；版本换代后旧基线的来源同样挂不上。两者都是「来源真实但不属
	// 于当前责任起点」的不采用，不是未决——重试一万次委托也不会变成已接受的那一版。
	if refusal, refused := adoptionRefusal(request, command.SubmissionVersion, source); refused {
		return handler.refuse(ctx, command, key, digest, refusal)
	}

	// 取消边界按业务时间裁决（AT-PS-044/080/081）：取消决定早于收寄发生——包裹在收寄
	// 前已经合法取消，来源不采用、物理事实保留；收寄发生早于取消决定——权威事实的
	// 业务顺序与两份决定的成立顺序矛盾（取消形成时收寄已发生却没拦住），事实冲突
	// 保持未决，不按消息到达顺序选边。
	if handler.deps.Cancellations != nil {
		cancellation, cancelled, err := handler.deps.Cancellations.FindCancellation(
			ctx, key.TenantID, source.Parcel())
		if err != nil {
			return handler.undecided(command, source, IntakeCancellationViewUnavailable), nil
		}
		if cancelled {
			if cancellation.RequestedAt().Before(source.OccurredAt()) {
				reason, err := domain.NewCheckReason("CANCELLED_BEFORE_INTAKE/" + cancellation.ID().String())
				if err != nil {
					return AdoptNetworkIntakeResult{}, fmt.Errorf("refusal reason: %w", err)
				}
				return handler.refuse(ctx, command, key, digest, reason)
			}
			return handler.undecided(command, source, IntakeCancellationOrderConflict), nil
		}
	}

	eligibility, configured, err := handler.deps.Eligibility.JudgeIntakeEligibility(
		ctx, command.Identity, command.ShipmentRequestID, source)
	if err != nil {
		return handler.undecided(command, source, IntakeEligibilityUnavailable), nil
	}
	if !configured {
		// 资格目录未配置保持未决（AT-PS-047）：不默认承诺，物理控制事实也不丢——本编排
		// 根本不碰来源。
		return handler.undecided(command, source, IntakeEligibilityUnconfigured), nil
	}
	switch eligibility.Outcome {
	case ports.IntakeEligibilityEstablished:
	case ports.IntakeEligibilityNotEstablished:
		result := handler.undecided(command, source, IntakeEligibilityNotEstablishedYet)
		result.basis = eligibility.Basis
		return result, nil
	case ports.IntakeServiceNotApplicable:
		return AdoptNetworkIntakeResult{outcome: IntakeNotApplicable, basis: eligibility.Basis}, nil
	default:
		return AdoptNetworkIntakeResult{}, fmt.Errorf(
			"parcel shipment: unhandled intake eligibility outcome %d", eligibility.Outcome)
	}

	// 责任起点唯一（AT-PS-049）与同来源更正（AT-PS-050）在这里分格（ADR-0117）：链尾在，
	// 本来源要么是它的更正——同种类、自报更正的恰是链尾——要么就是想开第二个责任起点。
	started, found, err := handler.deps.Adoptions.FindResponsibilityStart(ctx, key.TenantID, source.Parcel())
	if err != nil {
		return handler.undecided(command, source, IntakeAdoptionStoreUnavailable), nil
	}
	if found {
		corrects, declared := source.Corrects()
		if !declared || started.Key.Kind != source.Kind() {
			reason, err := domain.NewCheckReason(
				"RESPONSIBILITY_ALREADY_STARTED/" + started.Key.Kind.String() + "/" + started.Key.Version.String())
			if err != nil {
				return AdoptNetworkIntakeResult{}, fmt.Errorf("refusal reason: %w", err)
			}
			return handler.refuse(ctx, command, key, digest, reason)
		}
		if started.Key.Version == corrects {
			return handler.supersede(ctx, command, key, digest, source, started)
		}
		// 更正的不是链尾：前版一条记录都没有，是先后两封乱序——等；有记录（已被取代，或本就
		// 是不采用行）是分叉——要人看，不替人接。
		predecessor := key
		predecessor.Version = corrects
		if _, judged, err := handler.deps.Adoptions.FindByKey(ctx, predecessor); err != nil {
			return handler.undecided(command, source, IntakeAdoptionStoreUnavailable), nil
		} else if !judged {
			return handler.undecided(command, source, IntakeCorrectionPredecessorUnjudged), nil
		}
		reason, err := domain.NewCheckReason(
			"CORRECTION_TARGET_NOT_CURRENT/" + started.Key.Kind.String() + "/" + started.Key.Version.String())
		if err != nil {
			return AdoptNetworkIntakeResult{}, fmt.Errorf("refusal reason: %w", err)
		}
		return handler.refuse(ctx, command, key, digest, reason)
	}

	intake, err := domain.AdoptNetworkIntake(source, command.SubmissionVersion)
	if err != nil {
		return AdoptNetworkIntakeResult{}, fmt.Errorf("adopt network intake: %w", err)
	}
	expected, err := expectedCommitmentReference(request)
	if err != nil {
		return AdoptNetworkIntakeResult{}, err
	}
	version, err := handler.deps.Identities.NextCommitmentVersionID(ctx)
	if err != nil {
		return handler.undecided(command, source, IntakeCommitmentIdentityUnavailable), nil
	}
	commitment, err := domain.FormFormalCommitment(version, intake, expected)
	if err != nil {
		return AdoptNetworkIntakeResult{}, fmt.Errorf("form formal commitment: %w", err)
	}

	record := ports.IntakeAdoptionRecord{
		Key:               key,
		CustomerAccountID: command.Identity.CustomerAccountID(),
		ShipmentRequestID: command.ShipmentRequestID,
		ContentDigest:     digest,
		Adopted:           true,
		Intake:            intake,
		Commitment:        commitment,
		AdoptedAt:         handler.deps.Clock.Now(),
	}
	return handler.commit(ctx, record)
}

// supersede 以更正后的来源形成新的采用判断版本（AT-PS-050，ADR-0117 决定二、三）：新采用
// 回指链尾那一版，承诺在链尾的承诺上重述——新版本号、指回前版、原因点名被更正的来源版本、
// 生效时间随更正后的发生时刻。链尾那一行一字不动；新行落不下（并发第二个更正撞链线性索引）
// 时由 commit 照旧译成未决，重试方读到新链尾后收敛。
func (handler *AdoptNetworkIntakeHandler) supersede(
	ctx context.Context,
	command AdoptNetworkIntakeCommand,
	key ports.IntakeAdoptionKey,
	digest string,
	source domain.IntakeSource,
	current ports.IntakeAdoptionRecord,
) (AdoptNetworkIntakeResult, error) {
	intake, err := domain.AdoptNetworkIntake(source, command.SubmissionVersion)
	if err != nil {
		return AdoptNetworkIntakeResult{}, fmt.Errorf("adopt corrected network intake: %w", err)
	}
	version, err := handler.deps.Identities.NextCommitmentVersionID(ctx)
	if err != nil {
		return handler.undecided(command, source, IntakeCommitmentIdentityUnavailable), nil
	}
	reason, err := domain.NewCommitmentAdjustmentReason(
		"SOURCE_CORRECTED/" + source.Kind().String() + "/" + current.Key.Version.String())
	if err != nil {
		return AdoptNetworkIntakeResult{}, fmt.Errorf("adjustment reason: %w", err)
	}
	commitment, err := current.Commitment.RestateOnCorrectedIntake(version, intake, reason)
	if err != nil {
		// 门在领域：同包裹、同接受基线、同来源种类。走到这里还立不住，说明链尾与本来源挂在
		// 不同的接受基线上——那是拼坏的聚合，不是一种业务未决。
		return AdoptNetworkIntakeResult{}, fmt.Errorf("restate formal commitment on corrected intake: %w", err)
	}

	record := ports.IntakeAdoptionRecord{
		Key:               key,
		CustomerAccountID: command.Identity.CustomerAccountID(),
		ShipmentRequestID: command.ShipmentRequestID,
		ContentDigest:     digest,
		Adopted:           true,
		Intake:            intake,
		Commitment:        commitment,
		SupersedesVersion: current.Key.Version,
		AdoptedAt:         handler.deps.Clock.Now(),
	}
	return handler.commit(ctx, record)
}

// adoptionRefusal 判「来源真实但不属于当前责任起点」的三格：委托未接受（含已拒绝与
// 已撤回——撤回只终止未决委托，被撤回的委托从未接受过，其包裹没有可开始的网络责任）、
// 基线已换代、成员不在基线内。`AT-PS-044/045` 的包裹级取消对收寄的业务时间裁决属
// UC-PS-006 的取消对象，那个对象建模后在此接入，不拿委托撤回冒充。
func adoptionRefusal(
	request domain.ShipmentRequest,
	version domain.SubmissionVersionID,
	source domain.IntakeSource,
) (domain.CheckReason, bool) {
	if request.State() != domain.ShipmentRequestAccepted {
		reason, err := domain.NewCheckReason("REQUEST_NOT_ACCEPTED/" + request.State().String())
		if err != nil {
			return domain.CheckReason{}, false
		}
		return reason, true
	}
	if request.CurrentSubmissionVersion().VersionID() != version {
		reason, err := domain.NewCheckReason(
			"BASELINE_SUPERSEDED/" + request.CurrentSubmissionVersion().VersionID().String())
		if err != nil {
			return domain.CheckReason{}, false
		}
		return reason, true
	}
	memberOf := false
	for _, member := range request.CurrentSubmissionVersion().DeclaredParcelIDs() {
		if member == source.Parcel() {
			memberOf = true
			break
		}
	}
	if !memberOf {
		reason, err := domain.NewCheckReason("PARCEL_OUTSIDE_ACCEPTANCE_BASELINE")
		if err != nil {
			return domain.CheckReason{}, false
		}
		return reason, true
	}
	return domain.CheckReason{}, false
}

// refuse 提交一份不采用记录：物理事实保留、原因随记录可查，重复到达按已有结果作答。
func (handler *AdoptNetworkIntakeHandler) refuse(
	ctx context.Context,
	command AdoptNetworkIntakeCommand,
	key ports.IntakeAdoptionKey,
	digest string,
	reason domain.CheckReason,
) (AdoptNetworkIntakeResult, error) {
	record := ports.IntakeAdoptionRecord{
		Key:               key,
		CustomerAccountID: command.Identity.CustomerAccountID(),
		ShipmentRequestID: command.ShipmentRequestID,
		ContentDigest:     digest,
		RefusalBasis:      reason,
		AdoptedAt:         handler.deps.Clock.Now(),
	}
	return handler.commit(ctx, record)
}

// commit 提交采用记录并交发布意图；并发下另一方先提交时读回赢家。
func (handler *AdoptNetworkIntakeHandler) commit(
	ctx context.Context,
	record ports.IntakeAdoptionRecord,
) (AdoptNetworkIntakeResult, error) {
	saved, err := handler.deps.Adoptions.Save(ctx, record)
	if err != nil {
		return AdoptNetworkIntakeResult{
			outcome:      IntakeEligibilityUndecided,
			reason:       IntakeAdoptionStoreUnavailable,
			continuation: intakeContinuation(record.Key, IntakeAdoptionStoreUnavailable),
		}, nil
	}
	switch saved {
	case ports.IntakeAdoptionSaved:
		result := resultForAdoption(record)
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.IntakeAdoptionAlreadyRecorded:
		winner, found, err := handler.deps.Adoptions.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return AdoptNetworkIntakeResult{
				outcome:      IntakeEligibilityUndecided,
				reason:       IntakeAdoptionStoreUnavailable,
				continuation: intakeContinuation(record.Key, IntakeAdoptionStoreUnavailable),
			}, nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return AdoptNetworkIntakeResult{}, fmt.Errorf("%w: %d", ErrUnexpectedAdoptionSave, saved)
	}
}

// existingResult 按已有记录作答并重发同一份意图（AT-PS-041/051）。
func (handler *AdoptNetworkIntakeHandler) existingResult(
	ctx context.Context,
	record ports.IntakeAdoptionRecord,
) AdoptNetworkIntakeResult {
	result := resultForAdoption(record)
	result.outcome = IntakeExistingResult
	result.handoff = handler.handOff(ctx, record)
	return result
}

func resultForAdoption(record ports.IntakeAdoptionRecord) AdoptNetworkIntakeResult {
	if record.Adopted {
		return AdoptNetworkIntakeResult{
			outcome:   IntakeCommitmentFormed,
			record:    record,
			hasRecord: true,
		}
	}
	return AdoptNetworkIntakeResult{
		outcome:   IntakeSourceNotAdopted,
		record:    record,
		hasRecord: true,
		basis:     record.RefusalBasis,
	}
}

// handOff 交发布意图。失败不翻结果——结果已越过提交边界，留续办引用重发同一份
// （AT-PS-051 的纪律）。
func (handler *AdoptNetworkIntakeHandler) handOff(
	ctx context.Context,
	record ports.IntakeAdoptionRecord,
) domain.OwnershipContinuationReference {
	if err := handler.deps.Downstream.HandOffNetworkIntake(ctx, ports.NetworkIntakeHandoffIntent{Record: record}); err == nil {
		return domain.OwnershipContinuationReference{}
	}
	return derivedContinuation(
		"NETWORK_INTAKE_HANDOFF",
		record.Key.TenantID.String(),
		record.Key.Parcel.String(),
		record.Key.Kind.String(),
		record.Key.Version.String(),
	)
}

func (handler *AdoptNetworkIntakeHandler) undecided(
	command AdoptNetworkIntakeCommand,
	source domain.IntakeSource,
	reason IntakeUndecidedReason,
) AdoptNetworkIntakeResult {
	key := ports.IntakeAdoptionKey{
		TenantID: command.Identity.TenantID(),
		Parcel:   source.Parcel(),
		Kind:     source.Kind(),
		Version:  source.Version(),
	}
	return AdoptNetworkIntakeResult{
		outcome:      IntakeEligibilityUndecided,
		reason:       reason,
		continuation: intakeContinuation(key, reason),
	}
}

func intakeContinuation(key ports.IntakeAdoptionKey, reason IntakeUndecidedReason) domain.OwnershipContinuationReference {
	return derivedContinuation(
		reason.String(),
		key.TenantID.String(),
		key.Parcel.String(),
		key.Kind.String(),
		key.Version.String(),
	)
}

// derivedContinuation 与 judgmentContinuation 同一条派生纪律：同一范围同一原因恒同引用。
func derivedContinuation(parts ...string) domain.OwnershipContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	continuation, err := domain.NewOwnershipContinuationReference("CONT-" + hex.EncodeToString(digest[:8]))
	if err != nil {
		return domain.OwnershipContinuationReference{}
	}
	return continuation
}

// expectedCommitmentReference 从已接受委托派生预计承诺引用：解析标识加形成时刻，两样
// 都来自接受时固定的依据。
func expectedCommitmentReference(request domain.ShipmentRequest) (domain.ExpectedCommitmentReference, error) {
	expected, formed := request.ExpectedCommitment()
	if !formed {
		// 已接受委托必有预计承诺；缺席说明聚合被拼坏了，不是一种业务未决。
		return domain.ExpectedCommitmentReference{}, errors.New(
			"parcel shipment: an accepted request carries no expected commitment")
	}
	return domain.NewExpectedCommitmentReference(
		expected.Basis().ResolutionID().String() + "@" + expected.FormedAt().UTC().Format(time.RFC3339))
}

// intakeContentDigest 是同一采用身份的内容比对锚：对象、地点、控制与业务时间任一不同
// 即是另一份内容；更正版本还带上它自报更正的那一版——同一版本号两次到达却各说更正
// 不同的前版，也是两份内容。首登不带这一段，首登的摘要因此与它以前的写法逐字相同。
func intakeContentDigest(source domain.IntakeSource) string {
	parts := []string{
		source.Object().String(),
		source.Place().String(),
		source.Control().String(),
		source.OccurredAt().UTC().Format(time.RFC3339Nano),
	}
	if corrects, declared := source.Corrects(); declared {
		parts = append(parts, "corrects:"+corrects.String())
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}
