package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedCarrierPickupSave 说明收寄登记册交回了封闭集合以外的写入结果。
var ErrUnexpectedCarrierPickupSave = errors.New("transport fulfillment: unexpected carrier pickup registry save outcome")

// CarrierPickupJudgmentOutcome 是「显式判断实际承运商首次有效收寄」的结果代数（ADR-0135 决定四、六、八）。
//
// 各格按恢复动作分（ADR-0029）：`已形成` / `已替代` / `已失效`是落了新版本；`待确认`是判过了但不够（等身份登记或
// 人裁）；`不构成`是读法说证据不表达取得控制且没有可失效的东西——什么都不留；`非首次`是对象已有当前有效参与，
// 证据去实际承运商判断那条路；`依据不可用`是轨迹事实有效时间待判断，先去判它；`依据不是当前依据`是更正指名的
// 前代不是链尾依据，改指名再来；`已在册`是同一依据重放；`未受理`改输入；`未决`等依赖恢复重试同一份。
type CarrierPickupJudgmentOutcome uint8

const (
	CarrierPickupJudgmentOutcomeInvalid CarrierPickupJudgmentOutcome = iota
	CarrierPickupFormedOutcome
	CarrierPickupSupersededOutcome
	CarrierPickupVoidedOutcome
	CarrierPickupPendingOutcome
	CarrierPickupNotAPickup
	CarrierPickupNotFirst
	CarrierPickupBasisUnavailable
	CarrierPickupBasisNotCurrent
	CarrierPickupAlreadyRecorded
	CarrierPickupNotAccepted
	CarrierPickupUndecided
)

func (outcome CarrierPickupJudgmentOutcome) String() string {
	switch outcome {
	case CarrierPickupFormedOutcome:
		return "PICKUP_FORMED"
	case CarrierPickupSupersededOutcome:
		return "PICKUP_SUPERSEDED"
	case CarrierPickupVoidedOutcome:
		return "PICKUP_VOIDED"
	case CarrierPickupPendingOutcome:
		return "PICKUP_PENDING"
	case CarrierPickupNotAPickup:
		return "NOT_A_PICKUP"
	case CarrierPickupNotFirst:
		return "NOT_FIRST"
	case CarrierPickupBasisUnavailable:
		return "BASIS_UNAVAILABLE"
	case CarrierPickupBasisNotCurrent:
		return "BASIS_NOT_CURRENT"
	case CarrierPickupAlreadyRecorded:
		return "ALREADY_RECORDED"
	case CarrierPickupNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case CarrierPickupUndecided:
		return "PICKUP_UNDECIDED"
	default:
		return ""
	}
}

// CarrierPickupUndecidedReason 指名`未决`停在哪一步等谁。
type CarrierPickupUndecidedReason uint8

const (
	CarrierPickupUndecidedReasonNone CarrierPickupUndecidedReason = iota
	CarrierPickupRegistryUnavailable
	CarrierPickupIdentityDirectoryUnavailable
	CarrierPickupVersionUnavailable
	CarrierPickupEvidenceRegistryUnavailable
	CarrierPickupSegmentRegistryUnavailable
)

func (reason CarrierPickupUndecidedReason) String() string {
	switch reason {
	case CarrierPickupRegistryUnavailable:
		return "PICKUP_REGISTRY_UNAVAILABLE"
	case CarrierPickupIdentityDirectoryUnavailable:
		return "IDENTITY_DIRECTORY_UNAVAILABLE"
	case CarrierPickupVersionUnavailable:
		return "PICKUP_VERSION_UNAVAILABLE"
	case CarrierPickupEvidenceRegistryUnavailable:
		return "EVIDENCE_REGISTRY_UNAVAILABLE"
	case CarrierPickupSegmentRegistryUnavailable:
		return "SEGMENT_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// JudgeCarrierFirstEffectivePickupCommand 携带一次显式判断的全部输入（ADR-0135 决定四）。
//
// 一条合格来源事实（种类、引用、**来源版本**）；判断方对「该证据表达已接收实物或取得运输控制」的读法
// （ExpressesControl）；证据指名的承运主体——引用（分支 + 引用）或名称素材恰给其一，形与理由同
// FormActualCarrierJudgmentCommand。业务发生时间：来源是外部承运轨迹事实时**不由命令给**，编排读回那一代取它
// 已判断的有效时间（决定三），命令里给了也不用；其余三种来源本上下文没有登记册可读，由判断方连同证据一并交来。
//
// CorrectsSourceVersion 指名这条证据更正了同一来源事实的哪一代：给了它，编排就按「替代 / 失效」而不是「首次」
// 处理——被更正的那一代必须恰是链尾的依据（决定六）。来源是轨迹事实且缺席时，从读回的那一代的回指取。
//
// Segment / PlannedSegment / SegmentServiceAction 与收寄、交接登记同形：Segment 缺席不进段也不算失败。
type JudgeCarrierFirstEffectivePickupCommand struct {
	TenantID              domain.TenantID
	Object                string
	Source                domain.CarrierEvidenceSource
	EvidenceReference     string
	EvidenceVersion       string
	CorrectsSourceVersion string
	ExpressesControl      bool
	OccurredAt            time.Time
	SubjectKind           domain.CarrierSubjectKind
	SubjectReference      string
	NameMaterial          string
	Segment               string
	PlannedSegment        string
	SegmentServiceAction  string
}

type JudgeCarrierFirstEffectivePickupResult struct {
	outcome        CarrierPickupJudgmentOutcome
	reason         CarrierPickupUndecidedReason
	continuation   string
	record         ports.CarrierFirstEffectivePickupRecord
	hasRecord      bool
	handoff        string
	segment        string
	segmentRefusal SegmentEntryRefusal
	judgment       string
}

func (result JudgeCarrierFirstEffectivePickupResult) Outcome() CarrierPickupJudgmentOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result JudgeCarrierFirstEffectivePickupResult) UndecidedReason() CarrierPickupUndecidedReason {
	return result.reason
}

// ContinuationReference 只在`未决`时非空。
func (result JudgeCarrierFirstEffectivePickupResult) ContinuationReference() string {
	return result.continuation
}

// Record 在落了新版本、答`待确认`或`已在册`时带回链尾那一版。
func (result JudgeCarrierFirstEffectivePickupResult) Record() (ports.CarrierFirstEffectivePickupRecord, bool) {
	return result.record, result.hasRecord
}

// HandoffReference 非空说明版本已登记但意图还没交出去，重放会重发同一份。
func (result JudgeCarrierFirstEffectivePickupResult) HandoffReference() string { return result.handoff }

// SegmentContinuationReference 非空说明收寄已登记、段那一半还欠着——只在段登记册故障时给出。
func (result JudgeCarrierFirstEffectivePickupResult) SegmentContinuationReference() string {
	return result.segment
}

// SegmentEntryRefusal 非空说明收寄已登记、段那一半被领域正当拒绝；与 SegmentContinuationReference 不会同时非空。
func (result JudgeCarrierFirstEffectivePickupResult) SegmentEntryRefusal() SegmentEntryRefusal {
	return result.segmentRefusal
}

// CarrierJudgmentContinuationReference 非空说明收寄已登记、段已立，但把收寄的依据交给实际承运商判断那一步没成。
func (result JudgeCarrierFirstEffectivePickupResult) CarrierJudgmentContinuationReference() string {
	return result.judgment
}

// CarrierJudgmentFormer 是把收寄的依据交给实际承运商判断的那一道口（ADR-0135 决定五：段首登后首版为已识别，
// 依据即收寄的依据）。它就是 FormActualCarrierJudgmentHandler 的形；接口而不是具体类型，让装配处能换、测试能替。
type CarrierJudgmentFormer interface {
	Form(ctx context.Context, command FormActualCarrierJudgmentCommand) (FormActualCarrierJudgmentResult, error)
}

type JudgeCarrierFirstEffectivePickupDeps struct {
	Pickups    ports.CarrierFirstEffectivePickupRegistry
	Identities ports.CarrierPickupIdentityFactory
	Downstream ports.CarrierFirstEffectivePickupHandoff
	// Evidence 读回外部承运轨迹事实那一代；来源是其它三种时不问它。缺席时凡以轨迹事实为依据的命令一律`未决`。
	Evidence ports.ExternalTrackingFactRegistry
	// Directory 可缺席；缺席时凡给了承运主体引用的命令一律`未决`——装配缺件不是「未登记」这个业务答案。
	Directory ports.CarrierIdentityDirectory
	// Segments 答「对象此刻有没有当前有效参与」（首次判据）并承担进段 / 重派生；缺席时首次判据按无参与办，且不进段。
	Segments  ports.ActualFulfillmentSegmentRegistry
	Judgments ports.ActualCarrierJudgmentRegistry
	// CarrierJudgments 可缺席：缺席时段首版停在待确认（无合格证据），由「形成实际承运商判断」用例另行补证据。
	CarrierJudgments CarrierJudgmentFormer
	Clock            ports.Clock
}

type JudgeCarrierFirstEffectivePickupHandler struct {
	deps JudgeCarrierFirstEffectivePickupDeps
}

func NewJudgeCarrierFirstEffectivePickupHandler(deps JudgeCarrierFirstEffectivePickupDeps) *JudgeCarrierFirstEffectivePickupHandler {
	return &JudgeCarrierFirstEffectivePickupHandler{deps: deps}
}

// pickupBasisInput 是一条证据经受理与读回之后的样子：依据本体、业务时间（已形成才用）、它更正了哪一代（若有）。
type pickupBasisInput struct {
	basis      domain.CarrierPickupBasis
	occurredAt time.Time
	corrects   string
}

// Judge 就一条合格证据形成实际承运商首次有效收寄的判断（CONTEXT 生命周期「实际承运商首次有效收寄」）：
// 受理 → 读回依据（轨迹事实要有效时间已判断）→ 读回链尾 → 按链尾结果与读法分路 → 签版本 → 登记 → 交意图 →
// 进段或重派生 → 把依据交给实际承运商判断。
//
// **首次由本上下文自己的控制链判**（ADR-0135 决定四「非首次」）：链上没有已形成版本、且对象没有当前有效参与
// 才是首次；对象已凭场外揽收或`已交接`进段的，新到的承运证据不构成首次有效收寄，只作实际承运商判断的依据。
func (handler *JudgeCarrierFirstEffectivePickupHandler) Judge(
	ctx context.Context,
	command JudgeCarrierFirstEffectivePickupCommand,
) (JudgeCarrierFirstEffectivePickupResult, error) {
	object, accepted := carrierPickupTargetFrom(command)
	if !accepted {
		return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupNotAccepted}, nil
	}
	input, outcome, reason := handler.resolveBasis(ctx, command, object)
	if outcome != CarrierPickupJudgmentOutcomeInvalid {
		return handler.answer(command, outcome, reason), nil
	}

	current, found, err := handler.deps.Pickups.FindCurrentByObject(ctx, command.TenantID, object)
	if err != nil {
		return handler.answer(command, CarrierPickupUndecided, CarrierPickupRegistryUnavailable), nil
	}
	if found && current.Pickup.BasedOn(input.basis.Reference(), input.basis.SourceVersion()) {
		if current.Pickup.Result() != domain.CarrierPickupPending {
			// 同一依据重放：链尾已经是按这一代判出来的版本。
			return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupAlreadyRecorded, record: current, hasRecord: true}, nil
		}
		// 链尾待确认、同一依据再来：身份此刻若已在册，就是「PC 登记之后凭同一份证据形成新版本」（CONTEXT 生命周期）；
		// 仍未在册则链尾不动——同一依据不长第二版待确认。
		return handler.reconsiderPending(ctx, command, current, input)
	}

	// 更正那条路先走：被更正的那一代必须恰是链尾的依据，否则「更正」没有对象。
	if input.corrects != "" {
		if !found || !current.Pickup.BasedOn(input.basis.Reference(), input.corrects) {
			return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupBasisNotCurrent}, nil
		}
		return handler.rederive(ctx, command, object, current, input)
	}
	if !command.ExpressesControl {
		// 读法说这条证据不表达取得控制，又不是对某一代的更正：不构成，什么都不留（ADR-0135 决定四）。
		return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupNotAPickup}, nil
	}
	if found && current.Pickup.Formed() {
		// 已形成之后另一来源到达：不是收寄的事，是段级实际承运商判断的来源冲突（决定六）。
		return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupNotFirst}, nil
	}
	if !found || !current.Pickup.Formed() {
		// 链上无版本、链尾失效或待确认：这一步都可能让链上出现已形成版本，首次判据在此问一次（lc/33）。
		if refused, answered := handler.answerIfNotFirst(ctx, command, object); answered {
			return refused, nil
		}
	}
	return handler.formOrHold(ctx, command, object, current, found, input)
}

// formOrHold 在「首次」成立之后按承运主体身份分路：在册 → 已形成；不在册 → 待确认（承运主体身份未登记）；
// 链尾已是待确认且新依据指向另一主体 → 待确认（来源冲突），全部依据保留。
func (handler *JudgeCarrierFirstEffectivePickupHandler) formOrHold(
	ctx context.Context,
	command JudgeCarrierFirstEffectivePickupCommand,
	object domain.CarriedObjectReference,
	current ports.CarrierFirstEffectivePickupRecord,
	found bool,
	input pickupBasisInput,
) (JudgeCarrierFirstEffectivePickupResult, error) {
	subject, material, outcome := handler.resolveSubject(ctx, command)
	if outcome != CarrierPickupJudgmentOutcomeInvalid {
		if outcome == CarrierPickupUndecided {
			return handler.answer(command, outcome, CarrierPickupIdentityDirectoryUnavailable), nil
		}
		return JudgeCarrierFirstEffectivePickupResult{outcome: outcome}, nil
	}
	now := handler.deps.Clock.Now()
	version, err := handler.deps.Identities.NextCarrierFirstEffectivePickupVersion(ctx)
	if err != nil {
		return handler.answer(command, CarrierPickupUndecided, CarrierPickupVersionUnavailable), nil
	}

	var next domain.CarrierFirstEffectivePickup
	switch {
	case subject.Kind() != domain.CarrierSubjectKindInvalid && found:
		next, err = current.Pickup.Supersede(domain.CarrierPickupSupersession{
			Version: version, Carrier: subject, OccurredAt: input.occurredAt, JudgedAt: now,
			Bases: []domain.CarrierPickupBasis{input.basis},
		})
	case subject.Kind() != domain.CarrierSubjectKindInvalid:
		fact, factErr := handler.deps.Identities.NextCarrierFirstEffectivePickupReference(ctx)
		if factErr != nil {
			return handler.answer(command, CarrierPickupUndecided, CarrierPickupVersionUnavailable), nil
		}
		next, err = domain.FormCarrierFirstEffectivePickup(domain.CarrierFirstEffectivePickupSpec{
			TenantID: command.TenantID, Object: object, Fact: fact, Version: version,
			Carrier: subject, OccurredAt: input.occurredAt, JudgedAt: now,
			Bases: []domain.CarrierPickupBasis{input.basis},
		})
	case found:
		// 链尾待确认或失效、新依据仍无在册身份：同一名称素材是同一主体的又一条证据，仍是身份未登记；链尾待确认
		// 而素材不同，是来源冲突——全部依据保留，由人裁（CONTEXT 生命周期）。失效链尾上长待确认不算冲突：失效
		// 之后链上没有一个「当前主张」可与之冲突。
		reason := domain.PickupCarrierIdentityNotRegistered
		bases := []domain.CarrierPickupBasis{input.basis}
		if current.Pickup.Result() == domain.CarrierPickupPending {
			bases = append(current.Pickup.Bases(), input.basis)
			if !strings.EqualFold(material, current.Pickup.Material()) {
				reason = domain.PickupSourceConflict
			}
		}
		next, err = current.Pickup.HoldPending(domain.CarrierPickupPendingSupersession{
			Version: version, Reason: reason, Material: material, JudgedAt: now, Bases: bases,
		})
	default:
		fact, factErr := handler.deps.Identities.NextCarrierFirstEffectivePickupReference(ctx)
		if factErr != nil {
			return handler.answer(command, CarrierPickupUndecided, CarrierPickupVersionUnavailable), nil
		}
		next, err = domain.HoldCarrierFirstEffectivePickupPending(domain.PendingCarrierFirstEffectivePickupSpec{
			TenantID: command.TenantID, Object: object, Fact: fact, Version: version,
			Reason: domain.PickupCarrierIdentityNotRegistered, Material: material, JudgedAt: now,
			Bases: []domain.CarrierPickupBasis{input.basis},
		})
	}
	if err != nil {
		return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupNotAccepted}, nil
	}
	return handler.persist(ctx, command, next, now, found)
}

// reconsiderPending 在链尾待确认、同一依据再来时只问一件事：承运主体身份此刻在册了没有。在册 → 已形成版本回指
// 待确认前版（业务时间、依据照旧）；未在册 → 答`待确认`带回链尾，不追加。
func (handler *JudgeCarrierFirstEffectivePickupHandler) reconsiderPending(
	ctx context.Context,
	command JudgeCarrierFirstEffectivePickupCommand,
	current ports.CarrierFirstEffectivePickupRecord,
	input pickupBasisInput,
) (JudgeCarrierFirstEffectivePickupResult, error) {
	if !command.ExpressesControl {
		return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupPendingOutcome, record: current, hasRecord: true}, nil
	}
	// 待确认期间对象可能已凭别的控制事实进段；这一步一旦形成就是收寄，所以先问首次判据（lc/33）。
	if refused, answered := handler.answerIfNotFirst(ctx, command, current.Pickup.Object()); answered {
		return refused, nil
	}
	subject, _, outcome := handler.resolveSubject(ctx, command)
	if outcome != CarrierPickupJudgmentOutcomeInvalid {
		if outcome == CarrierPickupUndecided {
			return handler.answer(command, outcome, CarrierPickupIdentityDirectoryUnavailable), nil
		}
		return JudgeCarrierFirstEffectivePickupResult{outcome: outcome}, nil
	}
	if subject.Kind() == domain.CarrierSubjectKindInvalid {
		return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupPendingOutcome, record: current, hasRecord: true}, nil
	}
	now := handler.deps.Clock.Now()
	version, err := handler.deps.Identities.NextCarrierFirstEffectivePickupVersion(ctx)
	if err != nil {
		return handler.answer(command, CarrierPickupUndecided, CarrierPickupVersionUnavailable), nil
	}
	next, err := current.Pickup.Supersede(domain.CarrierPickupSupersession{
		Version: version, Carrier: subject, OccurredAt: input.occurredAt, JudgedAt: now,
		Bases: []domain.CarrierPickupBasis{input.basis},
	})
	if err != nil {
		return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupNotAccepted}, nil
	}
	return handler.persist(ctx, command, next, now, true)
}

// rederive 走更正那条路：读法仍是收寄 → 替代版本（承运主体、业务时间随新一代）；读法不再是收寄 → 失效版本。
// 被更正的依据挂在待确认链尾上时，替代出来的才是这条链第一个已形成版本，所以先问首次判据（lc/33）；链尾已形成
// 的替代不问——对象在控正是这条链自己的参与。
func (handler *JudgeCarrierFirstEffectivePickupHandler) rederive(
	ctx context.Context,
	command JudgeCarrierFirstEffectivePickupCommand,
	object domain.CarriedObjectReference,
	current ports.CarrierFirstEffectivePickupRecord,
	input pickupBasisInput,
) (JudgeCarrierFirstEffectivePickupResult, error) {
	if command.ExpressesControl && !current.Pickup.Formed() {
		if refused, answered := handler.answerIfNotFirst(ctx, command, object); answered {
			return refused, nil
		}
	}
	now := handler.deps.Clock.Now()
	version, err := handler.deps.Identities.NextCarrierFirstEffectivePickupVersion(ctx)
	if err != nil {
		return handler.answer(command, CarrierPickupUndecided, CarrierPickupVersionUnavailable), nil
	}
	var next domain.CarrierFirstEffectivePickup
	if !command.ExpressesControl {
		next, err = current.Pickup.Void(domain.CarrierPickupVoiding{
			Version: version, JudgedAt: now, Bases: []domain.CarrierPickupBasis{input.basis},
		})
		if errors.Is(err, domain.ErrCarrierPickupNotFormed) {
			return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupNotAPickup}, nil
		}
	} else {
		subject, _, outcome := handler.resolveSubject(ctx, command)
		if outcome != CarrierPickupJudgmentOutcomeInvalid {
			if outcome == CarrierPickupUndecided {
				return handler.answer(command, outcome, CarrierPickupIdentityDirectoryUnavailable), nil
			}
			return JudgeCarrierFirstEffectivePickupResult{outcome: outcome}, nil
		}
		if subject.Kind() == domain.CarrierSubjectKindInvalid {
			// 更正后的证据指名的承运主体不在册：替代版本立不起来（已形成必须带在册身份）。链尾不动，等登记。
			return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupPendingOutcome, record: current, hasRecord: true}, nil
		}
		next, err = current.Pickup.Supersede(domain.CarrierPickupSupersession{
			Version: version, Carrier: subject, OccurredAt: input.occurredAt, JudgedAt: now,
			Bases: []domain.CarrierPickupBasis{input.basis},
		})
	}
	if err != nil {
		return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupNotAccepted}, nil
	}
	return handler.persist(ctx, command, next, now, true)
}

// persist 登记一版、交意图（已形成 / 替代 / 失效）、再做段那一半：首登进段，替代 / 失效重派生参与。
func (handler *JudgeCarrierFirstEffectivePickupHandler) persist(
	ctx context.Context,
	command JudgeCarrierFirstEffectivePickupCommand,
	next domain.CarrierFirstEffectivePickup,
	now time.Time,
	chained bool,
) (JudgeCarrierFirstEffectivePickupResult, error) {
	record := ports.CarrierFirstEffectivePickupRecord{
		Key:        ports.CarrierFirstEffectivePickupKey{TenantID: next.TenantID(), Fact: next.Fact(), Version: next.Version()},
		Pickup:     next,
		RecordedAt: now,
	}
	saved, err := handler.deps.Pickups.Save(ctx, record)
	if err != nil {
		return handler.answer(command, CarrierPickupUndecided, CarrierPickupRegistryUnavailable), nil
	}
	switch saved {
	case ports.CarrierPickupSaved:
	case ports.CarrierPickupAlreadyRegistered:
		// 另一方先落了同一前版的下一版（或同对象的另一条首登链）：读回链尾再来。
		return handler.answer(command, CarrierPickupUndecided, CarrierPickupRegistryUnavailable), nil
	default:
		return JudgeCarrierFirstEffectivePickupResult{}, fmt.Errorf("%w: %d", ErrUnexpectedCarrierPickupSave, saved)
	}

	result := JudgeCarrierFirstEffectivePickupResult{record: record, hasRecord: true}
	// 前版是已形成的，本版才是「替代」；从待确认或失效长出的已形成版本是这条链第一次成为收寄，答`已形成`。
	priorFormed := chained && handler.priorEnteredSegment(ctx, next)
	switch next.Result() {
	case domain.CarrierPickupPending:
		result.outcome = CarrierPickupPendingOutcome
		return result, nil
	case domain.CarrierPickupVoided:
		result.outcome = CarrierPickupVoidedOutcome
	case domain.CarrierPickupFormed:
		result.outcome = CarrierPickupFormedOutcome
		if priorFormed {
			result.outcome = CarrierPickupSupersededOutcome
		}
	}
	result.handoff = handler.handOff(ctx, record)

	// 段那一半：首登（或从待确认 / 失效长出的已形成版本，其前版从未进过段）走进段；替代已形成前版与失效走重派生。
	var entry segmentEntry
	if priorFormed {
		entry = rederiveFulfillmentParticipation(ctx, handler.deps.Segments, handler.deps.Clock, next.TenantID(), next.Object(),
			func(segment domain.ActualFulfillmentSegment) (domain.ActualFulfillmentSegment, error) {
				return segment.RederiveParticipationWithCarrierPickup(next)
			})
	} else if next.Formed() {
		entry = handler.establishSegment(ctx, command, next)
		result.judgment = handler.formCarrierJudgment(ctx, command, next)
	}
	result.segment, result.segmentRefusal = entry.continuation, entry.refusal
	return result, nil
}

// priorEnteredSegment 答新版本回指的前版有没有进过段：只有已形成的前版才进过。前版是待确认或失效时，新的已形成
// 版本是这条链第一次进段，走进段而不是重派生。读不回前版按「没进过」办——进段那一道门会拒对象已在段内的情形。
func (handler *JudgeCarrierFirstEffectivePickupHandler) priorEnteredSegment(ctx context.Context, next domain.CarrierFirstEffectivePickup) bool {
	prior, has := next.Supersedes()
	if !has {
		return false
	}
	record, found, err := handler.deps.Pickups.FindByKey(ctx, ports.CarrierFirstEffectivePickupKey{
		TenantID: next.TenantID(), Fact: next.Fact(), Version: prior,
	})
	return err == nil && found && record.Pickup.Formed()
}

// establishSegment 让这次收寄的对象进入实际履约段（ADR-0135 决定五）——收寄那一侧的两道领域门在此，其余与
// 揽收、交接逐字相同，收在 enterFulfillmentSegment 里。
func (handler *JudgeCarrierFirstEffectivePickupHandler) establishSegment(
	ctx context.Context,
	command JudgeCarrierFirstEffectivePickupCommand,
	pickup domain.CarrierFirstEffectivePickup,
) segmentEntry {
	return enterFulfillmentSegment(
		ctx, handler.deps.Segments, handler.deps.Judgments, handler.deps.Clock,
		command.TenantID, command.Segment, command.PlannedSegment,
		segmentEntryDoors{
			object:        pickup.Object(),
			serviceAction: command.SegmentServiceAction,
			establish: func(segment domain.FulfillmentSegmentReference, planned domain.PlannedSegmentReference) (domain.ActualFulfillmentSegment, error) {
				return domain.EstablishSegmentWithCarrierPickup(segment, pickup, planned)
			},
			join: func(existing domain.ActualFulfillmentSegment, planned domain.PlannedSegmentReference) (domain.ActualFulfillmentSegment, error) {
				return existing.JoinWithCarrierPickup(pickup, planned)
			},
		},
	)
}

// formCarrierJudgment 把收寄的依据交给该段的实际承运商判断（ADR-0135 决定五：首版为已识别，依据即收寄的依据）。
// 段引用缺席或口缺席时不做；做了没成留续办引用——它是派生的一侧，不回滚收寄。
func (handler *JudgeCarrierFirstEffectivePickupHandler) formCarrierJudgment(
	ctx context.Context,
	command JudgeCarrierFirstEffectivePickupCommand,
	pickup domain.CarrierFirstEffectivePickup,
) string {
	if handler.deps.CarrierJudgments == nil || strings.TrimSpace(command.Segment) == "" {
		return ""
	}
	carrier, identified := pickup.Carrier()
	occurredAt, _ := pickup.OccurredAt()
	if !identified || len(pickup.Bases()) == 0 {
		return ""
	}
	basis := pickup.Bases()[0]
	result, err := handler.deps.CarrierJudgments.Form(ctx, FormActualCarrierJudgmentCommand{
		TenantID:          command.TenantID,
		Segment:           command.Segment,
		Source:            basis.Source(),
		EvidenceReference: basis.Reference().String(),
		OccurredAt:        occurredAt,
		SubjectKind:       carrier.Kind(),
		SubjectReference:  carrier.Reference(),
	})
	if err != nil {
		return owed("CARRIER_JUDGMENT_NOT_FORMED", command.TenantID, command.Segment, pickup.Object())
	}
	switch result.Outcome() {
	case CarrierJudgmentVersionFormed, CarrierEvidenceAlreadyConsidered:
		return ""
	default:
		return owed("CARRIER_JUDGMENT_NOT_FORMED", command.TenantID, command.Segment, pickup.Object())
	}
}

// resolveBasis 受理证据并读回依据。来源是外部承运轨迹事实：按（租户，事实，版本）读回那一代，对象要对得上、
// 有效时间要已判断（待判断 → 依据不可用），业务时间取它的有效时间，更正的前代取它的回指；其余来源由命令给业务时间。
func (handler *JudgeCarrierFirstEffectivePickupHandler) resolveBasis(
	ctx context.Context,
	command JudgeCarrierFirstEffectivePickupCommand,
	object domain.CarriedObjectReference,
) (pickupBasisInput, CarrierPickupJudgmentOutcome, CarrierPickupUndecidedReason) {
	basis, err := domain.NewCarrierPickupBasis(command.Source, command.EvidenceReference, command.EvidenceVersion)
	if err != nil {
		return pickupBasisInput{}, CarrierPickupNotAccepted, CarrierPickupUndecidedReasonNone
	}
	input := pickupBasisInput{basis: basis, corrects: strings.TrimSpace(command.CorrectsSourceVersion)}
	if command.Source != domain.TrustedChannelCallback {
		if command.ExpressesControl && command.OccurredAt.IsZero() {
			return pickupBasisInput{}, CarrierPickupNotAccepted, CarrierPickupUndecidedReasonNone
		}
		input.occurredAt = command.OccurredAt.UTC()
		return input, CarrierPickupJudgmentOutcomeInvalid, CarrierPickupUndecidedReasonNone
	}
	if handler.deps.Evidence == nil {
		return pickupBasisInput{}, CarrierPickupUndecided, CarrierPickupEvidenceRegistryUnavailable
	}
	factRef, err := domain.NewExternalTrackingFactReference(command.EvidenceReference)
	if err != nil {
		return pickupBasisInput{}, CarrierPickupNotAccepted, CarrierPickupUndecidedReasonNone
	}
	versionRef, err := domain.NewExternalTrackingFactVersion(command.EvidenceVersion)
	if err != nil {
		return pickupBasisInput{}, CarrierPickupNotAccepted, CarrierPickupUndecidedReasonNone
	}
	record, found, err := handler.deps.Evidence.FindByKey(ctx, ports.ExternalTrackingFactKey{
		TenantID: command.TenantID, Fact: factRef, Version: versionRef,
	})
	if err != nil {
		return pickupBasisInput{}, CarrierPickupUndecided, CarrierPickupEvidenceRegistryUnavailable
	}
	if !found || record.Fact.Object() != object {
		// 指名了一代不存在的事实，或它说的是别的对象：是提交矛盾不是等谁。
		return pickupBasisInput{}, CarrierPickupNotAccepted, CarrierPickupUndecidedReasonNone
	}
	effectiveAt, judged := record.Fact.EffectiveAt()
	if !judged {
		return pickupBasisInput{}, CarrierPickupBasisUnavailable, CarrierPickupUndecidedReasonNone
	}
	input.occurredAt = effectiveAt
	if input.corrects == "" {
		if prior, has := record.Fact.Supersedes(); has {
			input.corrects = prior.String()
		}
	}
	return input, CarrierPickupJudgmentOutcomeInvalid, CarrierPickupUndecidedReasonNone
}

// resolveSubject 按命令给的引用或素材定出证据指名的承运主体，判据同 FormActualCarrierJudgmentHandler.resolveSubject：
// 引用查得到 → 身份；查不到 → 素材保留；读口读不通或未装 → `未决`。第三个返回值非零表示这一步已有答案。
func (handler *JudgeCarrierFirstEffectivePickupHandler) resolveSubject(
	ctx context.Context,
	command JudgeCarrierFirstEffectivePickupCommand,
) (domain.CarrierSubject, string, CarrierPickupJudgmentOutcome) {
	material := strings.TrimSpace(command.NameMaterial)
	if strings.TrimSpace(command.SubjectReference) == "" {
		if material == "" {
			return domain.CarrierSubject{}, "", CarrierPickupNotAccepted
		}
		return domain.CarrierSubject{}, material, CarrierPickupJudgmentOutcomeInvalid
	}
	claimed, err := domain.NewCarrierSubject(command.SubjectKind, command.SubjectReference)
	if err != nil {
		return domain.CarrierSubject{}, "", CarrierPickupNotAccepted
	}
	if handler.deps.Directory == nil {
		return domain.CarrierSubject{}, "", CarrierPickupUndecided
	}
	registered, err := handler.deps.Directory.IdentityRegistered(ctx, command.TenantID, claimed)
	if err != nil {
		return domain.CarrierSubject{}, "", CarrierPickupUndecided
	}
	if registered {
		return claimed, "", CarrierPickupJudgmentOutcomeInvalid
	}
	if material == "" {
		material = claimed.Reference()
	}
	return domain.CarrierSubject{}, material, CarrierPickupJudgmentOutcomeInvalid
}

// answerIfNotFirst 在「这一步会让链上出现已形成版本」之前问首次判据：对象已有当前有效参与 → `非首次`，不形成版本
// （ADR-0135 决定四），证据留给实际承运商判断那条路；段登记册读不通 → `未决`。第二个返回值为 false 即首次仍成立，
// 调用方接着走。凡链尾不是已形成的入口——首登、失效后再登、待确认后同一依据再来或另一依据到达、更正待确认的依据
// ——都经这里；链尾已形成的替代不问，对象在控正是这条链自己的参与。
func (handler *JudgeCarrierFirstEffectivePickupHandler) answerIfNotFirst(
	ctx context.Context,
	command JudgeCarrierFirstEffectivePickupCommand,
	object domain.CarriedObjectReference,
) (JudgeCarrierFirstEffectivePickupResult, bool) {
	active, err := handler.objectUnderControl(ctx, command.TenantID, object)
	if err != nil {
		return handler.answer(command, CarrierPickupUndecided, CarrierPickupSegmentRegistryUnavailable), true
	}
	if active {
		return JudgeCarrierFirstEffectivePickupResult{outcome: CarrierPickupNotFirst}, true
	}
	return JudgeCarrierFirstEffectivePickupResult{}, false
}

// objectUnderControl 答对象此刻有没有当前有效履约参与（首次判据）。段登记册缺席按无参与办——派生一侧缺席不让
// 收寄停摆，判据同 enterFulfillmentSegment。
func (handler *JudgeCarrierFirstEffectivePickupHandler) objectUnderControl(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) (bool, error) {
	if handler.deps.Segments == nil {
		return false, nil
	}
	keys, err := handler.deps.Segments.FindActiveSegments(ctx, tenant, object)
	if err != nil {
		return false, err
	}
	return len(keys) > 0, nil
}

func (handler *JudgeCarrierFirstEffectivePickupHandler) handOff(ctx context.Context, record ports.CarrierFirstEffectivePickupRecord) string {
	if handler.deps.Downstream == nil {
		return carrierPickupContinuation("PICKUP_HANDOFF_UNCONFIGURED", record.Key)
	}
	if err := handler.deps.Downstream.HandOffCarrierFirstEffectivePickup(ctx, ports.CarrierFirstEffectivePickupHandoffIntent{Record: record}); err != nil {
		return carrierPickupContinuation("PICKUP_HANDOFF_PENDING", record.Key)
	}
	return ""
}

func (handler *JudgeCarrierFirstEffectivePickupHandler) answer(
	command JudgeCarrierFirstEffectivePickupCommand,
	outcome CarrierPickupJudgmentOutcome,
	reason CarrierPickupUndecidedReason,
) JudgeCarrierFirstEffectivePickupResult {
	result := JudgeCarrierFirstEffectivePickupResult{outcome: outcome}
	if outcome == CarrierPickupUndecided {
		result.reason = reason
		digest := sha256.Sum256([]byte(strings.Join([]string{
			reason.String(), command.TenantID.String(), command.Object, command.EvidenceReference, command.EvidenceVersion,
		}, "\x00")))
		result.continuation = "CONT-" + hex.EncodeToString(digest[:8])
	}
	return result
}

func carrierPickupTargetFrom(command JudgeCarrierFirstEffectivePickupCommand) (domain.CarriedObjectReference, bool) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return domain.CarriedObjectReference{}, false
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return domain.CarriedObjectReference{}, false
	}
	if _, err := domain.ParseCarrierEvidenceSource(command.Source.String()); err != nil {
		return domain.CarriedObjectReference{}, false
	}
	return object, true
}

func carrierPickupContinuation(cause string, key ports.CarrierFirstEffectivePickupKey) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		cause, key.TenantID.String(), key.Fact.String(), key.Version.String(),
	}, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}
