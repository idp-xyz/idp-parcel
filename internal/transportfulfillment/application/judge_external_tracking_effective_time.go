package application

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// EffectiveTimeJudgmentOutcome 是所有者就一条外部承运轨迹事实显式给出有效时间的结果代数。
type EffectiveTimeJudgmentOutcome uint8

const (
	EffectiveTimeJudgmentOutcomeInvalid EffectiveTimeJudgmentOutcome = iota
	EffectiveTimeJudged
	EffectiveTimeAlreadyJudgedAsGiven
	EffectiveTimeJudgmentNotAccepted
	EffectiveTimeJudgmentUndecided
)

func (outcome EffectiveTimeJudgmentOutcome) String() string {
	switch outcome {
	case EffectiveTimeJudged:
		return "EFFECTIVE_TIME_JUDGED"
	case EffectiveTimeAlreadyJudgedAsGiven:
		return "ALREADY_JUDGED_AS_GIVEN"
	case EffectiveTimeJudgmentNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case EffectiveTimeJudgmentUndecided:
		return "JUDGMENT_UNDECIDED"
	default:
		return ""
	}
}

// JudgeEffectiveTimeCommand 指名一条事实并给出所有者的判断。指名到事实而不是版本：判断落在
// 此刻的当前版之上，回指它。
type JudgeEffectiveTimeCommand struct {
	TenantID    domain.TenantID
	Fact        string
	EffectiveAt time.Time
}

type JudgeEffectiveTimeResult struct {
	outcome      EffectiveTimeJudgmentOutcome
	reason       TrackingAdoptionUndecidedReason
	record       ports.ExternalTrackingFactRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result JudgeEffectiveTimeResult) Outcome() EffectiveTimeJudgmentOutcome { return result.outcome }

// UndecidedReason 只在`未决`时非零；取值与收编共用一套。
func (result JudgeEffectiveTimeResult) UndecidedReason() TrackingAdoptionUndecidedReason {
	return result.reason
}

func (result JudgeEffectiveTimeResult) Record() (ports.ExternalTrackingFactRecord, bool) {
	return result.record, result.hasRecord
}

func (result JudgeEffectiveTimeResult) ContinuationReference() string { return result.continuation }

// HandoffReference 非空说明新版本已登记但意图还没交出去。
func (result JudgeEffectiveTimeResult) HandoffReference() string { return result.handoff }

type JudgeEffectiveTimeDeps struct {
	Facts      ports.ExternalTrackingFactRegistry
	Identities ports.ExternalTrackingIdentityFactory
	Downstream ports.ExternalTrackingFactHandoff
	Clock      ports.Clock
}

// JudgeEffectiveTimeHandler 是「所有者就这一条显式给出」那一路（ADR-0102 决定三的第一种来源）。
// 它不改动源给的任何内容，只为事实换一版、落判断、回指前版，再交 visibility-exception。
type JudgeEffectiveTimeHandler struct {
	deps JudgeEffectiveTimeDeps
}

func NewJudgeEffectiveTimeHandler(deps JudgeEffectiveTimeDeps) *JudgeEffectiveTimeHandler {
	return &JudgeEffectiveTimeHandler{deps: deps}
}

// Judge 为指名事实的当前版落一次显式判断。当前版已按同一时间显式判过即重放，交回原版本并
// 重发意图；其余情形换新版本。
func (handler *JudgeEffectiveTimeHandler) Judge(
	ctx context.Context,
	command JudgeEffectiveTimeCommand,
) (JudgeEffectiveTimeResult, error) {
	factRef, err := domain.NewExternalTrackingFactReference(command.Fact)
	if err != nil || command.TenantID.String() == "" {
		return JudgeEffectiveTimeResult{outcome: EffectiveTimeJudgmentNotAccepted}, nil
	}
	judgment, err := domain.JudgeEffectiveTimeExplicitly(command.EffectiveAt)
	if err != nil {
		return JudgeEffectiveTimeResult{outcome: EffectiveTimeJudgmentNotAccepted}, nil
	}

	current, found, err := handler.deps.Facts.FindCurrent(ctx, command.TenantID, factRef)
	if err != nil {
		return judgmentUndecided(TrackingFactRegistryUnavailable, command), nil
	}
	if !found {
		// 判断不出无中生有的事实。
		return JudgeEffectiveTimeResult{outcome: EffectiveTimeJudgmentNotAccepted}, nil
	}
	if at, judged := current.Fact.EffectiveAt(); judged &&
		current.Fact.Effective().Basis() == domain.EffectiveTimeJudgedExplicitly && at.Equal(command.EffectiveAt.UTC()) {
		return JudgeEffectiveTimeResult{
			outcome:   EffectiveTimeAlreadyJudgedAsGiven,
			record:    current,
			hasRecord: true,
			handoff:   handler.handOff(ctx, current),
		}, nil
	}

	version, err := handler.deps.Identities.NextExternalTrackingFactVersion(ctx)
	if err != nil {
		return judgmentUndecided(TrackingIdentityUnavailable, command), nil
	}
	judged, err := current.Fact.JudgeEffectiveTime(judgment, version)
	if err != nil {
		return JudgeEffectiveTimeResult{outcome: EffectiveTimeJudgmentNotAccepted}, nil
	}
	record := ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: command.TenantID, Fact: factRef, Version: version},
		Fact:       judged,
		RecordedAt: handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil || saved != ports.ExternalTrackingFactSaved {
		// 版本是刚签的，撞键说明登记册出了本上下文任何路径都造不出的状态。
		return judgmentUndecided(TrackingFactRegistryUnavailable, command), nil
	}
	return JudgeEffectiveTimeResult{
		outcome:   EffectiveTimeJudged,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record),
	}, nil
}

func (handler *JudgeEffectiveTimeHandler) handOff(ctx context.Context, record ports.ExternalTrackingFactRecord) string {
	err := handler.deps.Downstream.HandOffExternalTrackingFact(ctx, ports.ExternalTrackingFactHandoffIntent{Record: record})
	if err == nil {
		return ""
	}
	return trackingContinuation("EXTERNAL_TRACKING_FACT_HANDOFF",
		record.Key.TenantID.String(), record.Key.Fact.String(), record.Key.Version.String())
}

func judgmentUndecided(reason TrackingAdoptionUndecidedReason, command JudgeEffectiveTimeCommand) JudgeEffectiveTimeResult {
	return JudgeEffectiveTimeResult{
		outcome:      EffectiveTimeJudgmentUndecided,
		reason:       reason,
		continuation: trackingContinuation(reason.String(), command.TenantID.String(), command.Fact),
	}
}
