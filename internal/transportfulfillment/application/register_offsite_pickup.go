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

// ErrUnexpectedPickupRegistrySave 说明揽收登记库交回了封闭集合以外的写入结果。
var ErrUnexpectedPickupRegistrySave = errors.New("transport fulfillment: unexpected offsite pickup registry save outcome")

// PickupRegistrationOutcome 是揽收登记提交的应用处理结果。失败到访没有控制证据可供
// ——缺控制在受理处就是未受理（领域构造器把门，编排不绕），不存在「无控制的揽收」格。
type PickupRegistrationOutcome uint8

const (
	PickupRegistrationOutcomeInvalid PickupRegistrationOutcome = iota
	PickupRegistered
	PickupExistingVersion
	PickupRegistrationConflict
	PickupCorrected
	PickupRegistrationNotAccepted
	PickupRegistrationUndecided
)

func (outcome PickupRegistrationOutcome) String() string {
	switch outcome {
	case PickupRegistered:
		return "PICKUP_REGISTERED"
	case PickupExistingVersion:
		return "EXISTING_VERSION"
	case PickupRegistrationConflict:
		return "SOURCE_CONFLICT"
	case PickupCorrected:
		return "PICKUP_CORRECTED"
	case PickupRegistrationNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case PickupRegistrationUndecided:
		return "PICKUP_UNDECIDED"
	default:
		return ""
	}
}

// PickupRegistrationUndecidedReason 指名提交停在哪一步等谁。
type PickupRegistrationUndecidedReason uint8

const (
	PickupRegistrationUndecidedReasonNone PickupRegistrationUndecidedReason = iota
	PickupRegistryUnavailable
	PickupVersionUnavailable
)

func (reason PickupRegistrationUndecidedReason) String() string {
	switch reason {
	case PickupRegistryUnavailable:
		return "PICKUP_REGISTRY_UNAVAILABLE"
	case PickupVersionUnavailable:
		return "PICKUP_VERSION_UNAVAILABLE"
	default:
		return ""
	}
}

// RegisterOffsitePickupCommand 携带一次对象级揽收登记的全部输入。控制证据必备——
// 客户不在、货物未备好、包装不合格都没有控制依据，构造期就进不来（领域硬句）。
type RegisterOffsitePickupCommand struct {
	TenantID   domain.TenantID
	Object     string
	Task       string
	Attempt    string
	Place      string
	Control    string
	ExecutedBy string
	OccurredAt time.Time
	// Segment 指名这次收寄把对象送进哪个实际履约段，**缺席时不立段**；PlannedSegment 是该
	// 对象自己关联的计划履约段，可缺席。两者与交接登记那一侧同形，理由见
	// enterFulfillmentSegment 的自注。
	Segment        string
	PlannedSegment string
	// SegmentServiceAction 是登记方对该段服务动作的声明，可缺席；形与理由同 RegisterTransportHandoverCommand
	// 的同名字段（ADR-0114 决定一）。收寄也能声明，因为段由两种控制事实任一成立。
	SegmentServiceAction string
}

// CorrectOffsitePickupCommand 携带更正入口的全部输入：指名被更正的前版（租户+对象+尝试+前版版本号），
// 更正只带「证据说了什么」四格与更正时刻（票 tf-segment-lifecycle-closure/08 裁决）。没有 Segment——
// 段侧「来源更正 → 参与关系重派生」另立票，本编排不进段。新版本号不由调用方指名：它是本上下文签发的
// 号，同首登（交接那一侧的版本由裁决过程指名，是另一种身份）。
type CorrectOffsitePickupCommand struct {
	TenantID           domain.TenantID
	Object             string
	Attempt            string
	PredecessorVersion string
	Place              string
	Control            string
	ExecutedBy         string
	OccurredAt         time.Time
	CorrectedAt        time.Time
}

type RegisterOffsitePickupResult struct {
	outcome        PickupRegistrationOutcome
	reason         PickupRegistrationUndecidedReason
	record         ports.OffsitePickupRecord
	hasRecord      bool
	continuation   string
	handoff        string
	segment        string
	segmentRefusal SegmentEntryRefusal
}

func (result RegisterOffsitePickupResult) Outcome() PickupRegistrationOutcome {
	return result.outcome
}

// SegmentContinuationReference 非空说明收寄（或更正）已登记、段那一半还欠着。它只在段登记册故障时
// 给出——领域拒绝（对象已在段内、段已关闭、无可替代的参与）是正当结果不是欠账。
func (result RegisterOffsitePickupResult) SegmentContinuationReference() string {
	return result.segment
}

// SegmentEntryRefusal 非空说明收寄（或更正）已登记、段那一半被领域正当拒绝——首登进段答`段已关闭`，
// 更正重派生答`无可替代的参与`或`更正后的起点晚于继承的终点`；与 SegmentContinuationReference 不会同时非空
// ——理由同交接那一侧。
func (result RegisterOffsitePickupResult) SegmentEntryRefusal() SegmentEntryRefusal {
	return result.segmentRefusal
}

// UndecidedReason 只在`未决`时非零。
func (result RegisterOffsitePickupResult) UndecidedReason() PickupRegistrationUndecidedReason {
	return result.reason
}

func (result RegisterOffsitePickupResult) Record() (ports.OffsitePickupRecord, bool) {
	return result.record, result.hasRecord
}

func (result RegisterOffsitePickupResult) ContinuationReference() string {
	return result.continuation
}

// PickupHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result RegisterOffsitePickupResult) PickupHandoffReference() string {
	return result.handoff
}

type RegisterOffsitePickupDeps struct {
	Pickups ports.OffsitePickupRegistry
	// Segments 可缺席：没有段登记册时收寄照登不误。派生一侧缺席不该让来源保全停摆。
	Segments ports.ActualFulfillmentSegmentRegistry
	// Judgments 让段首登后同笔铸实际承运商判断的首版（票 tf-segment-lifecycle-closure/02）。可缺席，
	// 判据同 Segments；在场而写不进是段那一半的欠账——理由在 enterFulfillmentSegment 的自注。
	Judgments  ports.ActualCarrierJudgmentRegistry
	Versions   ports.PickupIdentityFactory
	Downstream ports.OffsitePickupRegistrationHandoff
	Clock      ports.Clock
}

type RegisterOffsitePickupHandler struct {
	deps RegisterOffsitePickupDeps
}

func NewRegisterOffsitePickupHandler(deps RegisterOffsitePickupDeps) *RegisterOffsitePickupHandler {
	return &RegisterOffsitePickupHandler{deps: deps}
}

// Register 首登一次对象级揽收：受理（控制证据等七件由领域构造器把门）→ 幂等按
// （租户+对象+尝试）分重放/冲突 → FormOffsitePickup → 原子提交 → 意图交 PS 采认
// （UC-PS-003 揽收源链）。重放/冲突对的是这次揽收的**当前版**：首登被更正之后，原内容的重放
// 对不上当前版，答冲突——首登不顶替，来源更正走 Correct。
func (handler *RegisterOffsitePickupHandler) Register(
	ctx context.Context,
	command RegisterOffsitePickupCommand,
) (RegisterOffsitePickupResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return RegisterOffsitePickupResult{outcome: PickupRegistrationNotAccepted}, nil
	}
	objectRef, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return RegisterOffsitePickupResult{outcome: PickupRegistrationNotAccepted}, nil
	}
	attemptRef, err := domain.NewAttemptReference(command.Attempt)
	if err != nil {
		return RegisterOffsitePickupResult{outcome: PickupRegistrationNotAccepted}, nil
	}

	key := ports.OffsitePickupKey{TenantID: command.TenantID, Object: objectRef, Attempt: attemptRef}
	digest := pickupRegistrationDigest(command)
	existing, found, err := handler.deps.Pickups.FindByKey(ctx, key)
	if err != nil {
		return pickupRegistryUndecided(command.Object), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一（对象+尝试）携带不同控制/地点/时间：首登不顶替，来源更正走新版本。
			return RegisterOffsitePickupResult{outcome: PickupRegistrationConflict}, nil
		}
		return handler.existingResult(ctx, existing), nil
	}

	spec, badInput := pickupSpecFrom(command)
	if badInput {
		// 缺控制证据的到访立不成揽收——失败到访没有可登的东西（领域硬句编排不绕）。
		return RegisterOffsitePickupResult{outcome: PickupRegistrationNotAccepted}, nil
	}
	version, err := handler.deps.Versions.NextPickupResultVersion(ctx)
	if err != nil {
		return RegisterOffsitePickupResult{outcome: PickupRegistrationUndecided, reason: PickupVersionUnavailable,
			continuation: pickupRegistrationContinuation("PICKUP_VERSION_UNAVAILABLE", command.Object)}, nil
	}
	spec.Version = version

	pickup, err := domain.FormOffsitePickup(spec)
	if err != nil {
		return RegisterOffsitePickupResult{outcome: PickupRegistrationNotAccepted}, nil
	}

	record := ports.OffsitePickupRecord{
		Key:           key,
		ContentDigest: digest,
		Pickup:        pickup,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Pickups.Save(ctx, record)
	if err != nil {
		return pickupRegistryUndecided(command.Object), nil
	}
	switch saved {
	case ports.OffsitePickupSaved:
		result := RegisterOffsitePickupResult{outcome: PickupRegistered, record: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record)
		entry := handler.establishSegment(ctx, command, pickup)
		result.segment, result.segmentRefusal = entry.continuation, entry.refusal
		return result, nil
	case ports.OffsitePickupAlreadyRegistered:
		winner, found, err := handler.deps.Pickups.FindByKey(ctx, key)
		if err != nil || !found {
			return pickupRegistryUndecided(command.Object), nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return RegisterOffsitePickupResult{}, fmt.Errorf("%w: %d", ErrUnexpectedPickupRegistrySave, saved)
	}
}

// establishSegment 让这次收寄的对象进入实际履约段（CONTEXT 生命周期①②）。
//
// 本编排只提供收寄那一侧的两道领域门；其余判断与交接那一侧逐字相同，收在
// enterFulfillmentSegment 里。**失败尝试走不到这里**——没有控制证据就形成不了 OffsitePickup，
// CONTEXT「客户不在、货物未备好、包装不合格或其他失败结果不制造实际履约段」由那道门守着。
func (handler *RegisterOffsitePickupHandler) establishSegment(
	ctx context.Context,
	command RegisterOffsitePickupCommand,
	pickup domain.OffsitePickup,
) segmentEntry {
	return enterFulfillmentSegment(
		ctx, handler.deps.Segments, handler.deps.Judgments, handler.deps.Clock,
		command.TenantID, command.Segment, command.PlannedSegment,
		segmentEntryDoors{
			object:        pickup.Object(),
			serviceAction: command.SegmentServiceAction,
			establish: func(
				segment domain.FulfillmentSegmentReference,
				planned domain.PlannedSegmentReference,
			) (domain.ActualFulfillmentSegment, error) {
				return domain.EstablishSegmentWithPickup(segment, pickup, planned)
			},
			join: func(
				existing domain.ActualFulfillmentSegment,
				planned domain.PlannedSegmentReference,
			) (domain.ActualFulfillmentSegment, error) {
				return existing.JoinWithPickup(pickup, planned)
			},
		},
	)
}

// Correct 对已登记的揽收落更正版本（票 tf-segment-lifecycle-closure/08 裁决 A）：读回当前版 → 指名的
// 前版必须就是当前版 → 更正时刻不早于其登记时刻 → 签发新版本 → 领域 Correct（四格完备性同首登、
// 回指前版）→ 以新版本落新行 → 意图重新交 parcel-shipment 采认 → 同事务在段上重派生该对象的参与
// （ADR-0112 决定二：替代版本回指前版、起点随更正后的发生时刻；段由登记册按对象反查，命令不带段号；
// 这一半失败不翻更正，答法与首登进段那一半同一格）。
//
// 前版核对为什么钉在「当前版」而不是「链上任一版」：登记册按键只答一个当前版（下游 parcel-shipment
// 也按键读），链因此必须线性——一版最多被更正一次。指名一个已被更正过的版本时，同内容是这份更正
// 的重放（答已有版本、重发同一份意图），异内容是冲突（v1 已被 v2 更正为别的内容，要改请对当前版
// 提更正），两格与首登那一侧的重放/冲突同一套判据：内容比对锚。
func (handler *RegisterOffsitePickupHandler) Correct(
	ctx context.Context,
	command CorrectOffsitePickupCommand,
) (RegisterOffsitePickupResult, error) {
	key, predecessor, badKey := pickupCorrectionKey(command)
	if badKey {
		return RegisterOffsitePickupResult{outcome: PickupRegistrationNotAccepted}, nil
	}
	current, found, err := handler.deps.Pickups.FindByKey(ctx, key)
	if err != nil {
		return pickupRegistryUndecided(command.Object), nil
	}
	if !found {
		// 没有可更正的登记：更正不出无中生有的揽收。
		return RegisterOffsitePickupResult{outcome: PickupRegistrationNotAccepted}, nil
	}
	digest := pickupRegistrationContentDigest(current.Pickup.Task().String(), command.Place, command.Control, command.ExecutedBy, command.OccurredAt)
	if current.Pickup.Version() != predecessor {
		if current.ContentDigest == digest {
			return handler.existingResult(ctx, current), nil
		}
		return RegisterOffsitePickupResult{outcome: PickupRegistrationConflict}, nil
	}
	if command.CorrectedAt.IsZero() || command.CorrectedAt.Before(current.RecordedAt) {
		// 更正时刻不得早于被更正版本的登记时刻（裁决）。下界取登记时刻不取发生时刻——发生时刻本身
		// 是可更正的四格之一，被更正的那一版可能恰恰把它记晚了。
		return RegisterOffsitePickupResult{outcome: PickupRegistrationNotAccepted}, nil
	}
	correction, badInput := pickupCorrectionFrom(command)
	if badInput {
		return RegisterOffsitePickupResult{outcome: PickupRegistrationNotAccepted}, nil
	}
	version, err := handler.deps.Versions.NextPickupResultVersion(ctx)
	if err != nil {
		return RegisterOffsitePickupResult{outcome: PickupRegistrationUndecided, reason: PickupVersionUnavailable,
			continuation: pickupRegistrationContinuation("PICKUP_VERSION_UNAVAILABLE", command.Object)}, nil
	}
	correction.Version = version

	corrected, err := current.Pickup.Correct(correction)
	if err != nil {
		return RegisterOffsitePickupResult{outcome: PickupRegistrationNotAccepted}, nil
	}

	record := ports.OffsitePickupRecord{
		Key:           key,
		ContentDigest: digest,
		Pickup:        corrected,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Pickups.Save(ctx, record)
	if err != nil {
		return pickupRegistryUndecided(command.Object), nil
	}
	switch saved {
	case ports.OffsitePickupSaved:
		result := RegisterOffsitePickupResult{outcome: PickupCorrected, record: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record)
		entry := handler.rederiveParticipation(ctx, command.TenantID, corrected)
		result.segment, result.segmentRefusal = entry.continuation, entry.refusal
		return result, nil
	case ports.OffsitePickupAlreadyRegistered:
		// 另一方先把同一前版更正掉了（一版最多被更正一次）：读回赢家作答。
		winner, found, err := handler.deps.Pickups.FindByKey(ctx, key)
		if err != nil || !found {
			return pickupRegistryUndecided(command.Object), nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return RegisterOffsitePickupResult{}, fmt.Errorf("%w: %d", ErrUnexpectedPickupRegistrySave, saved)
	}
}

// rederiveParticipation 让更正后的揽收在段上替代该对象凭前版入场的参与（ADR-0112 决定一至三）。
//
// 本编排只提供收寄那一侧的领域门；段在哪、失败算不算欠账、领域拒绝哪几格答出去，与交接那一侧逐字相同，
// 收在 rederiveFulfillmentParticipation 里——与两条来源共用 enterFulfillmentSegment 同形。
func (handler *RegisterOffsitePickupHandler) rederiveParticipation(
	ctx context.Context,
	tenant domain.TenantID,
	corrected domain.OffsitePickup,
) segmentEntry {
	return rederiveFulfillmentParticipation(
		ctx, handler.deps.Segments, handler.deps.Clock, tenant, corrected.Object(),
		func(segment domain.ActualFulfillmentSegment) (domain.ActualFulfillmentSegment, error) {
			return segment.RederiveParticipationWithPickup(corrected)
		},
	)
}

func pickupCorrectionKey(command CorrectOffsitePickupCommand) (ports.OffsitePickupKey, domain.PickupResultVersion, bool) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return ports.OffsitePickupKey{}, domain.PickupResultVersion{}, true
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return ports.OffsitePickupKey{}, domain.PickupResultVersion{}, true
	}
	attempt, err := domain.NewAttemptReference(command.Attempt)
	if err != nil {
		return ports.OffsitePickupKey{}, domain.PickupResultVersion{}, true
	}
	predecessor, err := domain.NewPickupResultVersion(command.PredecessorVersion)
	if err != nil {
		return ports.OffsitePickupKey{}, domain.PickupResultVersion{}, true
	}
	return ports.OffsitePickupKey{TenantID: command.TenantID, Object: object, Attempt: attempt}, predecessor, false
}

// pickupCorrectionFrom 只译四格与更正时刻；版本由编排签发后填入。四格在这里就要齐——签发在它之后，
// 一份形成不了的更正不该烧掉一个版本号。
func pickupCorrectionFrom(command CorrectOffsitePickupCommand) (domain.PickupCorrection, bool) {
	if command.OccurredAt.IsZero() {
		return domain.PickupCorrection{}, true
	}
	correction := domain.PickupCorrection{
		OccurredAt:  command.OccurredAt,
		CorrectedAt: command.CorrectedAt,
	}
	var err error
	if correction.Place, err = domain.NewPickupPlaceReference(command.Place); err != nil {
		return domain.PickupCorrection{}, true
	}
	if correction.Control, err = domain.NewTransportControlReference(command.Control); err != nil {
		return domain.PickupCorrection{}, true
	}
	if correction.ExecutedBy, err = domain.NewExecutingPartyReference(command.ExecutedBy); err != nil {
		return domain.PickupCorrection{}, true
	}
	return correction, false
}

func pickupSpecFrom(command RegisterOffsitePickupCommand) (domain.OffsitePickupSpec, bool) {
	spec := domain.OffsitePickupSpec{
		TenantID:   command.TenantID,
		OccurredAt: command.OccurredAt,
	}
	var err error
	if spec.Object, err = domain.NewCarriedObjectReference(command.Object); err != nil {
		return domain.OffsitePickupSpec{}, true
	}
	if spec.Task, err = domain.NewPickupTaskReference(command.Task); err != nil {
		return domain.OffsitePickupSpec{}, true
	}
	if spec.Attempt, err = domain.NewAttemptReference(command.Attempt); err != nil {
		return domain.OffsitePickupSpec{}, true
	}
	if spec.Place, err = domain.NewPickupPlaceReference(command.Place); err != nil {
		return domain.OffsitePickupSpec{}, true
	}
	if spec.Control, err = domain.NewTransportControlReference(command.Control); err != nil {
		return domain.OffsitePickupSpec{}, true
	}
	if spec.ExecutedBy, err = domain.NewExecutingPartyReference(command.ExecutedBy); err != nil {
		return domain.OffsitePickupSpec{}, true
	}
	return spec, false
}

func pickupRegistryUndecided(object string) RegisterOffsitePickupResult {
	return RegisterOffsitePickupResult{
		outcome:      PickupRegistrationUndecided,
		reason:       PickupRegistryUnavailable,
		continuation: pickupRegistrationContinuation("PICKUP_REGISTRY_UNAVAILABLE", object),
	}
}

// existingResult 按已有登记作答并重发同一份意图。
func (handler *RegisterOffsitePickupHandler) existingResult(
	ctx context.Context,
	record ports.OffsitePickupRecord,
) RegisterOffsitePickupResult {
	return RegisterOffsitePickupResult{
		outcome:   PickupExistingVersion,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record),
	}
}

// handOff 把对象级揽收交给 parcel-shipment 采认。投递失败不翻结果，留续办引用重放
// 时重发同一份。
func (handler *RegisterOffsitePickupHandler) handOff(
	ctx context.Context,
	record ports.OffsitePickupRecord,
) string {
	if err := handler.deps.Downstream.HandOffOffsitePickupRegistration(ctx, ports.OffsitePickupRegistrationIntent{Record: record}); err == nil {
		return ""
	}
	return pickupRegistrationContinuation("OFFSITE_PICKUP_REGISTRATION_HANDOFF", record.Key.TenantID.String(), record.Key.Object.String())
}

func pickupRegistrationContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// pickupRegistrationDigest 是同一（对象+尝试）首登的内容比对锚：任务、地点、控制、
// 执行方与业务时间任一不同即是另一份内容。
func pickupRegistrationDigest(command RegisterOffsitePickupCommand) string {
	return pickupRegistrationContentDigest(command.Task, command.Place, command.Control, command.ExecutedBy, command.OccurredAt)
}

// pickupRegistrationContentDigest 让首登与更正版本的记录带同一种内容比对锚——它描述的是这一版**说了什么**，
// 而不是这一版怎么来的。于是重放与冲突在两个入口上是同一套判据：首登重放对上当前版是已有版本、
// 对不上是冲突；更正重放同理。
func pickupRegistrationContentDigest(task, place, control, executedBy string, occurredAt time.Time) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		task,
		place,
		control,
		executedBy,
		occurredAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
