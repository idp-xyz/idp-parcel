package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedTrackingFactSave 说明登记册交回了封闭集合以外的写入结果。
var ErrUnexpectedTrackingFactSave = errors.New("transport fulfillment: unexpected external tracking fact save outcome")

// TrackingAdoptionOutcome 是收编一条轨迹素材的结果代数。
//
// `已认领`与`已认领、有效时间待判断`分两格，因为续办不同：前者已提供给 visibility-exception，
// 后者留在本上下文手上，等该源的有效时间规则登记或所有者显式判断（ADR-0102 决定三）。
// `留痕`与`未受理`分两格：前者是素材如实记下的缺陷（源未给发生时间、凭证不认识），后者是调用方
// 交来的东西不成形。
type TrackingAdoptionOutcome uint8

const (
	TrackingAdoptionOutcomeInvalid TrackingAdoptionOutcome = iota
	TrackingFactAdopted
	TrackingFactAdoptedPendingJudgment
	TrackingMaterialDuplicate
	TrackingMaterialLeftUnadopted
	TrackingMaterialNotAccepted
	TrackingAdoptionUndecided
)

func (outcome TrackingAdoptionOutcome) String() string {
	switch outcome {
	case TrackingFactAdopted:
		return "TRACKING_FACT_ADOPTED"
	case TrackingFactAdoptedPendingJudgment:
		return "TRACKING_FACT_ADOPTED_PENDING_JUDGMENT"
	case TrackingMaterialDuplicate:
		return "DUPLICATE_DELIVERY"
	case TrackingMaterialLeftUnadopted:
		return "MATERIAL_LEFT_UNADOPTED"
	case TrackingMaterialNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case TrackingAdoptionUndecided:
		return "ADOPTION_UNDECIDED"
	default:
		return ""
	}
}

// TrackingAdoptionUndecidedReason 指名收编停在哪一步等谁。`凭证登记册未配置`单列：它不是
// 线路故障，是本上下文自己的实例参数缺席（CONTEXT「实例参数缺席时如实表达未配置」）。
type TrackingAdoptionUndecidedReason uint8

const (
	TrackingAdoptionUndecidedReasonNone TrackingAdoptionUndecidedReason = iota
	TrackingFactRegistryUnavailable
	TrackingIdentityUnavailable
	TrackingCredentialResolverUnavailable
	TrackingCredentialRegistryUnconfigured
	TrackingRulesUnavailable
	TrackingLedgerUnavailable
)

func (reason TrackingAdoptionUndecidedReason) String() string {
	switch reason {
	case TrackingFactRegistryUnavailable:
		return "TRACKING_FACT_REGISTRY_UNAVAILABLE"
	case TrackingIdentityUnavailable:
		return "TRACKING_IDENTITY_UNAVAILABLE"
	case TrackingCredentialResolverUnavailable:
		return "CREDENTIAL_RESOLVER_UNAVAILABLE"
	case TrackingCredentialRegistryUnconfigured:
		return "CREDENTIAL_REGISTRY_UNCONFIGURED"
	case TrackingRulesUnavailable:
		return "EFFECTIVE_TIME_RULES_UNAVAILABLE"
	case TrackingLedgerUnavailable:
		return "UNADOPTED_LEDGER_UNAVAILABLE"
	default:
		return ""
	}
}

type AdoptTrackingMaterialResult struct {
	outcome      TrackingAdoptionOutcome
	reason       TrackingAdoptionUndecidedReason
	unadopted    ports.UnadoptedReason
	record       ports.ExternalTrackingFactRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result AdoptTrackingMaterialResult) Outcome() TrackingAdoptionOutcome { return result.outcome }

// UndecidedReason 只在`未决`时非零。
func (result AdoptTrackingMaterialResult) UndecidedReason() TrackingAdoptionUndecidedReason {
	return result.reason
}

// UnadoptedReason 只在`留痕`时非零。
func (result AdoptTrackingMaterialResult) UnadoptedReason() ports.UnadoptedReason {
	return result.unadopted
}

// Record 在`已认领`、`已认领待判断`与`重复投递`时给出那一版记录。
func (result AdoptTrackingMaterialResult) Record() (ports.ExternalTrackingFactRecord, bool) {
	return result.record, result.hasRecord
}

func (result AdoptTrackingMaterialResult) ContinuationReference() string { return result.continuation }

// HandoffReference 非空说明事实已登记但意图还没交出去，重放会重发同一份。
func (result AdoptTrackingMaterialResult) HandoffReference() string { return result.handoff }

type AdoptTrackingMaterialDeps struct {
	Facts       ports.ExternalTrackingFactRegistry
	Identities  ports.ExternalTrackingIdentityFactory
	Credentials ports.ExternalCarrierCredentialResolver
	Rules       ports.EffectiveTimeRules
	Ledger      ports.UnadoptedTrackingMaterialLedger
	Downstream  ports.ExternalTrackingFactHandoff
	Clock       ports.Clock
}

// AdoptTrackingMaterialHandler 是外部轨迹的收编执行器：把 TrackingSource 交来的一条原始素材
// 认领为本上下文的外部承运轨迹事实，或如实留痕。
type AdoptTrackingMaterialHandler struct {
	deps AdoptTrackingMaterialDeps
}

func NewAdoptTrackingMaterialHandler(deps AdoptTrackingMaterialDeps) *AdoptTrackingMaterialHandler {
	return &AdoptTrackingMaterialHandler{deps: deps}
}

// Adopt 收编一条素材：受理形状 → 幂等按（源，源事件）分重复投递 → 源未给发生时间即留痕 →
// 凭证解析到载运对象（不认识即留痕）→ 源声明更正则解析到当前版 → 有效时间按该源规则判断，
// 无规则即待判断 → 登记 → 判断过的版本交 visibility-exception。
//
// 三个时间各归各位：OccurredAt 只能来自素材（源给），ReceivedAt 取素材上端口铸的那一个，
// EffectiveAt 只在这里作为一次判断出现——本方法从不把任何一个时间抄给另一个。
func (handler *AdoptTrackingMaterialHandler) Adopt(
	ctx context.Context,
	material ports.TrackingMaterial,
) (AdoptTrackingMaterialResult, error) {
	shape, accepted := trackingShapeFrom(material)
	if !accepted {
		return AdoptTrackingMaterialResult{outcome: TrackingMaterialNotAccepted}, nil
	}

	if shape.eventGiven {
		existing, found, err := handler.deps.Facts.FindBySourceEvent(ctx, shape.tenant, shape.source, shape.event)
		if err != nil {
			return trackingUndecided(TrackingFactRegistryUnavailable, material), nil
		}
		if found {
			return handler.duplicate(ctx, existing), nil
		}
	}

	if !material.OccurredAt.Given {
		return handler.leaveUnadopted(ctx, material, ports.UnadoptedOccurredAtNotGiven)
	}

	object, resolution, err := handler.deps.Credentials.ResolveCredential(ctx, shape.tenant, shape.credential)
	if err != nil {
		return trackingUndecided(TrackingCredentialResolverUnavailable, material), nil
	}
	switch resolution {
	case ports.CredentialResolved:
	case ports.CredentialUnknown:
		return handler.leaveUnadopted(ctx, material, ports.UnadoptedCredentialUnknown)
	case ports.CredentialRegistryUnconfigured:
		return trackingUndecided(TrackingCredentialRegistryUnconfigured, material), nil
	default:
		return trackingUndecided(TrackingCredentialResolverUnavailable, material), nil
	}

	factRef, supersedes, undecided := handler.resolveCorrection(ctx, shape)
	if undecided != nil {
		return *undecided, nil
	}
	if factRef.String() == "" {
		factRef, err = handler.deps.Identities.NextExternalTrackingFactReference(ctx)
		if err != nil {
			return trackingUndecided(TrackingIdentityUnavailable, material), nil
		}
	}
	version, err := handler.deps.Identities.NextExternalTrackingFactVersion(ctx)
	if err != nil {
		return trackingUndecided(TrackingIdentityUnavailable, material), nil
	}

	effective, undecided := handler.judgeByRule(ctx, shape, material)
	if undecided != nil {
		return *undecided, nil
	}

	fact, err := domain.AdoptExternalCarrierTracking(domain.ExternalTrackingFactSpec{
		TenantID:     shape.tenant,
		Fact:         factRef,
		Version:      version,
		Source:       shape.source,
		Credential:   shape.credential,
		Object:       object,
		SourceEvent:  shape.event,
		Status:       shape.status,
		OccurredAt:   material.OccurredAt.At,
		ReceivedAt:   material.ReceivedAt,
		Effective:    effective,
		CorrectionOf: shape.correctionOf,
		Supersedes:   supersedes,
	})
	if err != nil {
		return AdoptTrackingMaterialResult{outcome: TrackingMaterialNotAccepted}, nil
	}

	record := ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: shape.tenant, Fact: factRef, Version: version},
		Fact:       fact,
		RecordedAt: handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return trackingUndecided(TrackingFactRegistryUnavailable, material), nil
	}
	switch saved {
	case ports.ExternalTrackingFactSaved:
		return handler.adopted(ctx, record), nil
	case ports.ExternalTrackingFactAlreadyRegistered:
		// 身份是刚签的，撞键只可能撞在（源，源事件）这一锚上：另一方先认领了同一条。
		if shape.eventGiven {
			winner, found, err := handler.deps.Facts.FindBySourceEvent(ctx, shape.tenant, shape.source, shape.event)
			if err == nil && found {
				return handler.duplicate(ctx, winner), nil
			}
		}
		return trackingUndecided(TrackingFactRegistryUnavailable, material), nil
	default:
		return AdoptTrackingMaterialResult{}, fmt.Errorf("%w: %d", ErrUnexpectedTrackingFactSave, saved)
	}
}

// trackingShape 是素材过了各构造门之后的样子。event 与 correctionOf 允许缺席。
type trackingShape struct {
	tenant       domain.TenantID
	source       domain.TrackingSourceReference
	credential   domain.ExternalCarrierCredentialReference
	status       domain.RawStatusReference
	event        domain.SourceEventReference
	eventGiven   bool
	correctionOf domain.SourceEventReference
}

func trackingShapeFrom(material ports.TrackingMaterial) (trackingShape, bool) {
	var shape trackingShape
	if strings.TrimSpace(material.Subject.Tenant.String()) == "" || material.ReceivedAt.IsZero() {
		return shape, false
	}
	var err error
	shape.tenant = material.Subject.Tenant
	if shape.source, err = domain.NewTrackingSourceReference(string(material.Source)); err != nil {
		return shape, false
	}
	if shape.credential, err = domain.NewExternalCarrierCredentialReference(material.Subject.CredentialReference); err != nil {
		return shape, false
	}
	if shape.status, err = domain.NewRawStatusReference(material.StatusReference); err != nil {
		return shape, false
	}
	if strings.TrimSpace(material.SourceEventID) != "" {
		if shape.event, err = domain.NewSourceEventReference(material.SourceEventID); err != nil {
			return shape, false
		}
		shape.eventGiven = true
	}
	if strings.TrimSpace(material.CorrectionOf) != "" {
		if shape.correctionOf, err = domain.NewSourceEventReference(material.CorrectionOf); err != nil {
			return shape, false
		}
	}
	return shape, true
}

// resolveCorrection 把源声明的「本条更正了 X」解析到本上下文的某条事实：X 认得出，新版本落在
// 那条事实上并回指它此刻的当前版；认不出，声明照样登记但不回指——本上下文不替源猜它更正了谁。
// 交回的事实引用为空即「另起一条事实」。
func (handler *AdoptTrackingMaterialHandler) resolveCorrection(
	ctx context.Context,
	shape trackingShape,
) (domain.ExternalTrackingFactReference, domain.ExternalTrackingFactVersion, *AdoptTrackingMaterialResult) {
	none := domain.ExternalTrackingFactReference{}
	noVersion := domain.ExternalTrackingFactVersion{}
	if shape.correctionOf.String() == "" {
		return none, noVersion, nil
	}
	corrected, found, err := handler.deps.Facts.FindBySourceEvent(ctx, shape.tenant, shape.source, shape.correctionOf)
	if err != nil {
		undecided := trackingUndecidedFor(TrackingFactRegistryUnavailable, shape)
		return none, noVersion, &undecided
	}
	if !found {
		return none, noVersion, nil
	}
	current, found, err := handler.deps.Facts.FindCurrent(ctx, shape.tenant, corrected.Key.Fact)
	if err != nil || !found {
		undecided := trackingUndecidedFor(TrackingFactRegistryUnavailable, shape)
		return none, noVersion, &undecided
	}
	return current.Key.Fact, current.Key.Version, nil
}

// judgeByRule 问该源的有效时间规则。无规则即待判断——**不在这里、也不在任何地方拿发生时间
// 顶上**。
func (handler *AdoptTrackingMaterialHandler) judgeByRule(
	ctx context.Context,
	shape trackingShape,
	material ports.TrackingMaterial,
) (domain.EffectiveTimeJudgment, *AdoptTrackingMaterialResult) {
	ruling, err := handler.deps.Rules.JudgeEffectiveTime(ctx, ports.EffectiveTimeRuleInput{
		TenantID:   shape.tenant,
		Source:     shape.source,
		Status:     shape.status,
		OccurredAt: material.OccurredAt.At,
		ReceivedAt: material.ReceivedAt,
	})
	if err != nil {
		undecided := trackingUndecided(TrackingRulesUnavailable, material)
		return domain.EffectiveTimeJudgment{}, &undecided
	}
	switch ruling.Outcome {
	case ports.EffectiveTimeRuleApplied:
		judgment, err := domain.JudgeEffectiveTimeByRule(ruling.Rule, ruling.EffectiveAt)
		if err != nil {
			// 规则答了「已判」却给不出带版本的规则或时间：那是规则实现的缺陷，不是待判断。
			undecided := trackingUndecided(TrackingRulesUnavailable, material)
			return domain.EffectiveTimeJudgment{}, &undecided
		}
		return judgment, nil
	case ports.EffectiveTimeRuleAbsent:
		return domain.PendingEffectiveTime(), nil
	default:
		undecided := trackingUndecided(TrackingRulesUnavailable, material)
		return domain.EffectiveTimeJudgment{}, &undecided
	}
}

func (handler *AdoptTrackingMaterialHandler) leaveUnadopted(
	ctx context.Context,
	material ports.TrackingMaterial,
	reason ports.UnadoptedReason,
) (AdoptTrackingMaterialResult, error) {
	err := handler.deps.Ledger.RecordUnadopted(ctx, ports.UnadoptedTrackingMaterial{
		TenantID:      material.Subject.Tenant,
		Source:        material.Source,
		SourceEvent:   material.SourceEventID,
		Credential:    material.Subject.CredentialReference,
		Status:        material.StatusReference,
		ReceivedAt:    material.ReceivedAt,
		PayloadDigest: material.PayloadDigest,
		Reason:        reason,
		RecordedAt:    handler.deps.Clock.Now(),
	})
	if err != nil {
		return trackingUndecided(TrackingLedgerUnavailable, material), nil
	}
	return AdoptTrackingMaterialResult{outcome: TrackingMaterialLeftUnadopted, unadopted: reason}, nil
}

func (handler *AdoptTrackingMaterialHandler) adopted(
	ctx context.Context,
	record ports.ExternalTrackingFactRecord,
) AdoptTrackingMaterialResult {
	if !record.Fact.Effective().Judged() {
		return AdoptTrackingMaterialResult{outcome: TrackingFactAdoptedPendingJudgment, record: record, hasRecord: true}
	}
	return AdoptTrackingMaterialResult{
		outcome:   TrackingFactAdopted,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record),
	}
}

// duplicate 按已有版本作答；判断过的版本重发同一份意图，重放因此无害。
func (handler *AdoptTrackingMaterialHandler) duplicate(
	ctx context.Context,
	record ports.ExternalTrackingFactRecord,
) AdoptTrackingMaterialResult {
	result := AdoptTrackingMaterialResult{outcome: TrackingMaterialDuplicate, record: record, hasRecord: true}
	if record.Fact.Effective().Judged() {
		result.handoff = handler.handOff(ctx, record)
	}
	return result
}

// handOff 把判断过的版本交给 visibility-exception。投递失败不翻结果，留续办引用重放时重发。
func (handler *AdoptTrackingMaterialHandler) handOff(
	ctx context.Context,
	record ports.ExternalTrackingFactRecord,
) string {
	err := handler.deps.Downstream.HandOffExternalTrackingFact(ctx, ports.ExternalTrackingFactHandoffIntent{Record: record})
	if err == nil {
		return ""
	}
	return trackingContinuation("EXTERNAL_TRACKING_FACT_HANDOFF",
		record.Key.TenantID.String(), record.Key.Fact.String(), record.Key.Version.String())
}

func trackingUndecided(reason TrackingAdoptionUndecidedReason, material ports.TrackingMaterial) AdoptTrackingMaterialResult {
	return AdoptTrackingMaterialResult{
		outcome: TrackingAdoptionUndecided,
		reason:  reason,
		continuation: trackingContinuation(reason.String(), material.Subject.Tenant.String(),
			string(material.Source), material.SourceEventID, material.PayloadDigest),
	}
}

func trackingUndecidedFor(reason TrackingAdoptionUndecidedReason, shape trackingShape) AdoptTrackingMaterialResult {
	return AdoptTrackingMaterialResult{
		outcome: TrackingAdoptionUndecided,
		reason:  reason,
		continuation: trackingContinuation(reason.String(), shape.tenant.String(),
			shape.source.String(), shape.event.String(), shape.correctionOf.String()),
	}
}

func trackingContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}
