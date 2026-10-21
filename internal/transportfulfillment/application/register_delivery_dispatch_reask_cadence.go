package application

import (
	"context"
	"math"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ReaskCadenceRegistrationOutcome 是派送重问间隔登记的结果代数（ADR-0230 决定五）。
type ReaskCadenceRegistrationOutcome uint8

const (
	ReaskCadenceRegistrationOutcomeInvalid ReaskCadenceRegistrationOutcome = iota
	ReaskCadenceRegistered
	ReaskCadenceExistingVersion
	ReaskCadenceContentConflict
	ReaskCadenceRegistrationNotAccepted
	ReaskCadenceRegistrationUndecided
)

func (outcome ReaskCadenceRegistrationOutcome) String() string {
	switch outcome {
	case ReaskCadenceRegistered:
		return "REASK_CADENCE_REGISTERED"
	case ReaskCadenceExistingVersion:
		return "EXISTING_VERSION"
	case ReaskCadenceContentConflict:
		return "CONTENT_CONFLICT"
	case ReaskCadenceRegistrationNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case ReaskCadenceRegistrationUndecided:
		return "REGISTRATION_UNDECIDED"
	default:
		return ""
	}
}

// RegisterReaskCadenceCommand 是一版重问间隔：租户取认证结果，版本由登记方指名，间隔按整分钟。
type RegisterReaskCadenceCommand struct {
	TenantID        domain.TenantID
	Version         string
	IntervalMinutes int
}

type RegisterReaskCadenceResult struct {
	outcome   ReaskCadenceRegistrationOutcome
	record    ports.DeliveryDispatchReaskCadenceRecord
	hasRecord bool
}

func (result RegisterReaskCadenceResult) Outcome() ReaskCadenceRegistrationOutcome {
	return result.outcome
}

// Record 在`已登记`、`已有版本`时给出那一版；`内容冲突`时给出撞上的在册那一版。
func (result RegisterReaskCadenceResult) Record() (ports.DeliveryDispatchReaskCadenceRecord, bool) {
	return result.record, result.hasRecord
}

type RegisterReaskCadenceDeps struct {
	Cadences ports.DeliveryDispatchReaskCadenceRegistry
	Clock    ports.Clock
}

// RegisterReaskCadenceHandler 是派送重问间隔的登记编排（登记册配置写，ADR-0151 判据）。没有意图交付：间隔是本上下文到期重问
// 自己读的租户取值，没有谁要在它变动时被通知；登记一版不回头重问任何一件。
type RegisterReaskCadenceHandler struct {
	deps RegisterReaskCadenceDeps
}

func NewRegisterReaskCadenceHandler(deps RegisterReaskCadenceDeps) *RegisterReaskCadenceHandler {
	return &RegisterReaskCadenceHandler{deps: deps}
}

// Register 登一版：过构造门 → 同版本在册就按正文分重放与冲突 → 首登；并发下另一方先落了同一版，读回它再分。
func (handler *RegisterReaskCadenceHandler) Register(
	ctx context.Context,
	command RegisterReaskCadenceCommand,
) (RegisterReaskCadenceResult, error) {
	version, err := domain.NewDeliveryDispatchReaskCadenceVersion(command.Version)
	if err != nil {
		return RegisterReaskCadenceResult{outcome: ReaskCadenceRegistrationNotAccepted}, nil
	}
	// 换成时长之前先挡住会溢出的分钟数：溢出之后的时长可能恰好是正的整分钟，构造门拦不住。
	if command.IntervalMinutes <= 0 || int64(command.IntervalMinutes) > math.MaxInt64/int64(time.Minute) {
		return RegisterReaskCadenceResult{outcome: ReaskCadenceRegistrationNotAccepted}, nil
	}
	cadence, err := domain.NewDeliveryDispatchReaskCadence(command.TenantID, version, time.Duration(command.IntervalMinutes)*time.Minute)
	if err != nil {
		return RegisterReaskCadenceResult{outcome: ReaskCadenceRegistrationNotAccepted}, nil
	}
	if answered, done, err := handler.answerRegistered(ctx, cadence); err != nil || done {
		return answered, err
	}
	record := ports.DeliveryDispatchReaskCadenceRecord{Cadence: cadence, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Cadences.Save(ctx, record)
	if err != nil {
		return RegisterReaskCadenceResult{outcome: ReaskCadenceRegistrationUndecided}, nil
	}
	if saved {
		return RegisterReaskCadenceResult{outcome: ReaskCadenceRegistered, record: record, hasRecord: true}, nil
	}
	answered, done, err := handler.answerRegistered(ctx, cadence)
	if err != nil || done {
		return answered, err
	}
	return RegisterReaskCadenceResult{outcome: ReaskCadenceRegistrationUndecided}, nil
}

// answerRegistered 同版本在册时交回重放或冲突；不在册答 done=false。读不动答未决。
func (handler *RegisterReaskCadenceHandler) answerRegistered(
	ctx context.Context,
	cadence domain.DeliveryDispatchReaskCadence,
) (RegisterReaskCadenceResult, bool, error) {
	existing, found, err := handler.deps.Cadences.FindByVersion(ctx, cadence.TenantID(), cadence.Version())
	if err != nil {
		return RegisterReaskCadenceResult{outcome: ReaskCadenceRegistrationUndecided}, true, nil
	}
	if !found {
		return RegisterReaskCadenceResult{}, false, nil
	}
	if existing.Cadence.SameAs(cadence) {
		return RegisterReaskCadenceResult{outcome: ReaskCadenceExistingVersion, record: existing, hasRecord: true}, true, nil
	}
	return RegisterReaskCadenceResult{outcome: ReaskCadenceContentConflict, record: existing, hasRecord: true}, true, nil
}
