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

// SegmentContinuationReference 非空说明收寄已登记、段那一半还欠着。它只在段登记册故障时
// 给出——领域拒绝（对象已在段内、段已关闭）是正当结果不是欠账。
func (result RegisterOffsitePickupResult) SegmentContinuationReference() string {
	return result.segment
}

// SegmentEntryRefusal 非空说明收寄已登记、段那一半被领域正当拒绝（今天只有`段已关闭`一格），
// 与 SegmentContinuationReference 不会同时非空——理由同交接那一侧。
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
// （UC-PS-003 揽收源链）。
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
			object: pickup.Object(),
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
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Task,
		command.Place,
		command.Control,
		command.ExecutedBy,
		command.OccurredAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
