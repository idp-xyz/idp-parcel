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

// ErrUnexpectedFinalSave 说明终局库交回了封闭集合以外的写入结果。
var ErrUnexpectedFinalSave = errors.New("parcel shipment: unexpected final outcome save outcome")

// FormParcelFinalOutcomeResult 的应用处理结果，对应 UC-PS-004 结果语义契约。
type ParcelFinalAdoptionOutcome uint8

const (
	ParcelFinalAdoptionOutcomeInvalid ParcelFinalAdoptionOutcome = iota
	ParcelFinalFormed
	ParcelFinalRederived
	ParcelFinalUndecided
	FinalSourceNotAdopted
	ParcelFinalExistingResult
	FinalSourceConflict
	FinalRequestNotAccepted
)

func (outcome ParcelFinalAdoptionOutcome) String() string {
	switch outcome {
	case ParcelFinalFormed:
		return "FINAL_FORMED"
	case ParcelFinalRederived:
		return "FINAL_REDERIVED"
	case ParcelFinalUndecided:
		return "FINAL_UNDECIDED"
	case FinalSourceNotAdopted:
		return "SOURCE_NOT_ADOPTED"
	case ParcelFinalExistingResult:
		return "EXISTING_RESULT"
	case FinalSourceConflict:
		return "SOURCE_CONFLICT"
	case FinalRequestNotAccepted:
		return "REQUEST_NOT_ACCEPTED"
	default:
		return ""
	}
}

// FinalUndecidedReason 指名终局判断停在哪一步。
type FinalUndecidedReason uint8

const (
	FinalUndecidedReasonNone FinalUndecidedReason = iota
	FinalRequestUnavailable
	FinalRuleUnavailable
	FinalRuleUnconfigured
	FinalRuleNotSatisfiedYet
	FinalStoreUnavailable
	FinalIdentityUnavailable
)

func (reason FinalUndecidedReason) String() string {
	switch reason {
	case FinalRequestUnavailable:
		return "REQUEST_UNAVAILABLE"
	case FinalRuleUnavailable:
		return "FINAL_RULE_UNAVAILABLE"
	case FinalRuleUnconfigured:
		return "FINAL_RULE_UNCONFIGURED"
	case FinalRuleNotSatisfiedYet:
		return "FINAL_RULE_NOT_SATISFIED"
	case FinalStoreUnavailable:
		return "FINAL_STORE_UNAVAILABLE"
	case FinalIdentityUnavailable:
		return "FINAL_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// FormParcelFinalCommand 携带一份责任结果的终局采用请求。多包裹委托逐包裹提交——
// 责任结果本就按对象逐一成立。
type FormParcelFinalCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	Outcome           domain.ResponsibilityOutcomeSpec
}

type FormParcelFinalResult struct {
	outcome      ParcelFinalAdoptionOutcome
	record       ports.FinalOutcomeRecord
	hasRecord    bool
	basis        domain.CheckReason
	reason       FinalUndecidedReason
	completion   domain.ShipmentCompletionSummary
	hasSummary   bool
	continuation domain.OwnershipContinuationReference
	handoff      domain.OwnershipContinuationReference
}

func (result FormParcelFinalResult) Outcome() ParcelFinalAdoptionOutcome {
	return result.outcome
}

func (result FormParcelFinalResult) Record() (ports.FinalOutcomeRecord, bool) {
	return result.record, result.hasRecord
}

// Basis 在`不采用`时携带依据；`未决`的规则缺口也从这里读。
func (result FormParcelFinalResult) Basis() domain.CheckReason {
	return result.basis
}

func (result FormParcelFinalResult) UndecidedReason() FinalUndecidedReason {
	return result.reason
}

// Completion 是随终局提交派生的委托完成摘要（UC-PS-004 步骤 7），只在越过提交边界
// 时给出。
func (result FormParcelFinalResult) Completion() (domain.ShipmentCompletionSummary, bool) {
	return result.completion, result.hasSummary
}

func (result FormParcelFinalResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// FinalHandoffReference 非空说明结果已提交但意图还没交出去，重放会重发同一份。
func (result FormParcelFinalResult) FinalHandoffReference() domain.OwnershipContinuationReference {
	return result.handoff
}

type FormParcelFinalDeps struct {
	Requests      ports.ShipmentRequestRepository
	Rules         ports.FinalRuleView
	Finals        ports.FinalOutcomeStore
	Cancellations ports.ParcelCancellationView
	Identities    ports.FinalIdentityFactory
	Downstream    ports.FinalOutcomeHandoff
	Clock         ports.Clock
}

type FormParcelFinalHandler struct {
	deps FormParcelFinalDeps
}

func NewFormParcelFinalHandler(deps FormParcelFinalDeps) *FormParcelFinalHandler {
	return &FormParcelFinalHandler{deps: deps}
}

// Handle 把一份责任结果推进到终局采用判断：受理（结果联合构造期拦）→ 委托与成员核验
// （统一不可见）→ 幂等/冲突 → 既有终局分派（同源新版本重派生、异源不采用）→ 规则
// （未配置即未决，不默认有效交付即终局）→ 提交与发布意图 → 委托完成派生。来源事实
// 全程只读。
func (handler *FormParcelFinalHandler) Handle(
	ctx context.Context,
	command FormParcelFinalCommand,
) (FormParcelFinalResult, error) {
	outcome, err := domain.NewResponsibilityOutcome(command.Outcome)
	if err != nil {
		return FormParcelFinalResult{outcome: FinalRequestNotAccepted}, nil
	}

	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return handler.undecided(command, outcome, FinalRequestUnavailable), nil
	}
	if !found || request.ShipmentRequestID() != command.ShipmentRequestID ||
		!memberOfCurrentVersion(request, outcome.Parcel()) {
		return FormParcelFinalResult{outcome: FinalRequestNotAccepted}, nil
	}

	key := ports.FinalAdoptionKey{
		TenantID: command.Identity.TenantID(),
		Parcel:   outcome.Parcel(),
		Kind:     outcome.Kind(),
		Version:  outcome.Version(),
	}
	digest := finalContentDigest(outcome)
	existing, found, err := handler.deps.Finals.FindByKey(ctx, key)
	if err != nil {
		return handler.undecided(command, outcome, FinalStoreUnavailable), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一终局采用身份携带不同结果内容：冲突保留原判断（AT-PS-062）。
			return FormParcelFinalResult{outcome: FinalSourceConflict}, nil
		}
		return handler.existingResult(ctx, command, request, existing), nil
	}

	// 既有终局分派：同源（同来源种类）的新版本走重派生（AT-PS-063——来源更正形成新的
	// 当前判断版本）；异源结果在已有终局上不采用（当前终局边界之外，AT-PS-064 的对偶：
	// 终局不回退，后续另一种责任结果各有独立生命周期）。
	current, hasFinal, err := handler.deps.Finals.FindCurrentFinal(ctx, key.TenantID, outcome.Parcel())
	if err != nil {
		return handler.undecided(command, outcome, FinalStoreUnavailable), nil
	}
	if hasFinal && current.Finalized {
		if current.Final.Source().Kind() == outcome.Kind() {
			return handler.rederive(ctx, command, request, current, outcome, key, digest)
		}
		reason, err := domain.NewCheckReason("FINAL_ALREADY_FORMED/" + current.Final.Version().String())
		if err != nil {
			return FormParcelFinalResult{}, fmt.Errorf("refusal reason: %w", err)
		}
		return handler.refuse(ctx, command, request, key, digest, reason)
	}

	judgment, configured, err := handler.deps.Rules.JudgeFinalOutcome(ctx, command.Identity, outcome)
	if err != nil {
		return handler.undecided(command, outcome, FinalRuleUnavailable), nil
	}
	if !configured {
		// 终局规则未配置保持未决：不默认「有效交付即所有产品终局」（红线）。
		return handler.undecided(command, outcome, FinalRuleUnconfigured), nil
	}
	if !judgment.Satisfied {
		result := handler.undecided(command, outcome, FinalRuleNotSatisfiedYet)
		result.basis = judgment.Basis
		return result, nil
	}

	version, err := handler.deps.Identities.NextFinalOutcomeVersionID(ctx)
	if err != nil {
		return handler.undecided(command, outcome, FinalIdentityUnavailable), nil
	}
	final, err := domain.FormParcelFinalOutcome(domain.ParcelFinalOutcomeSpec{
		Version:     version,
		Parcel:      outcome.Parcel(),
		Kind:        judgment.Kind,
		Source:      outcome,
		RuleVersion: judgment.RuleVersion,
	})
	if err != nil {
		return FormParcelFinalResult{}, fmt.Errorf("form parcel final outcome: %w", err)
	}

	record := ports.FinalOutcomeRecord{
		Key:           key,
		ContentDigest: digest,
		Finalized:     true,
		Final:         final,
		AdoptedAt:     handler.deps.Clock.Now(),
	}
	return handler.commit(ctx, command, request, record, ParcelFinalFormed)
}

// rederive 依据同源新版本形成新的当前判断版本：原终局历史保留在原版本上。
func (handler *FormParcelFinalHandler) rederive(
	ctx context.Context,
	command FormParcelFinalCommand,
	request domain.ShipmentRequest,
	current ports.FinalOutcomeRecord,
	outcome domain.ResponsibilityOutcome,
	key ports.FinalAdoptionKey,
	digest string,
) (FormParcelFinalResult, error) {
	version, err := handler.deps.Identities.NextFinalOutcomeVersionID(ctx)
	if err != nil {
		return handler.undecided(command, outcome, FinalIdentityUnavailable), nil
	}
	reason, err := domain.NewRederivationReason(
		"SOURCE_REVISED/" + outcome.Version().String())
	if err != nil {
		return FormParcelFinalResult{}, fmt.Errorf("rederivation reason: %w", err)
	}
	rederived, err := current.Final.Rederive(version, outcome, reason)
	if err != nil {
		return FormParcelFinalResult{}, fmt.Errorf("rederive final outcome: %w", err)
	}
	record := ports.FinalOutcomeRecord{
		Key:           key,
		ContentDigest: digest,
		Finalized:     true,
		Final:         rederived,
		AdoptedAt:     handler.deps.Clock.Now(),
	}
	return handler.commit(ctx, command, request, record, ParcelFinalRederived)
}

// refuse 提交一份不采用记录：来源保留、依据可查，重复到达按已有结果作答。
func (handler *FormParcelFinalHandler) refuse(
	ctx context.Context,
	command FormParcelFinalCommand,
	request domain.ShipmentRequest,
	key ports.FinalAdoptionKey,
	digest string,
	reason domain.CheckReason,
) (FormParcelFinalResult, error) {
	record := ports.FinalOutcomeRecord{
		Key:           key,
		ContentDigest: digest,
		RefusalBasis:  reason,
		AdoptedAt:     handler.deps.Clock.Now(),
	}
	return handler.commit(ctx, command, request, record, FinalSourceNotAdopted)
}

// commit 提交终局判断、交发布意图并派生委托完成摘要。
func (handler *FormParcelFinalHandler) commit(
	ctx context.Context,
	command FormParcelFinalCommand,
	request domain.ShipmentRequest,
	record ports.FinalOutcomeRecord,
	formed ParcelFinalAdoptionOutcome,
) (FormParcelFinalResult, error) {
	saved, err := handler.deps.Finals.Save(ctx, record)
	if err != nil {
		return FormParcelFinalResult{
			outcome:      ParcelFinalUndecided,
			reason:       FinalStoreUnavailable,
			continuation: finalContinuation(record.Key, FinalStoreUnavailable),
		}, nil
	}
	switch saved {
	case ports.FinalOutcomeSaved:
		result := FormParcelFinalResult{outcome: formed, record: record, hasRecord: true}
		if !record.Finalized {
			result.basis = record.RefusalBasis
		}
		result.handoff = handler.handOff(ctx, record)
		handler.deriveCompletion(ctx, command, request, &result)
		return result, nil
	case ports.FinalOutcomeAlreadyRecorded:
		winner, found, err := handler.deps.Finals.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return FormParcelFinalResult{
				outcome:      ParcelFinalUndecided,
				reason:       FinalStoreUnavailable,
				continuation: finalContinuation(record.Key, FinalStoreUnavailable),
			}, nil
		}
		return handler.existingResult(ctx, command, request, winner), nil
	default:
		return FormParcelFinalResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFinalSave, saved)
	}
}

// existingResult 按已有记录作答并重发同一份意图（AT-PS-061/065）。
func (handler *FormParcelFinalHandler) existingResult(
	ctx context.Context,
	command FormParcelFinalCommand,
	request domain.ShipmentRequest,
	record ports.FinalOutcomeRecord,
) FormParcelFinalResult {
	result := FormParcelFinalResult{
		outcome:   ParcelFinalExistingResult,
		record:    record,
		hasRecord: true,
	}
	if !record.Finalized {
		result.basis = record.RefusalBasis
	}
	result.handoff = handler.handOff(ctx, record)
	handler.deriveCompletion(ctx, command, request, &result)
	return result
}

// deriveCompletion 依据当前有效成员集派生委托完成摘要（UC-PS-004 步骤 7）。成员的
// 终局来自两条路：本用例的履约终局与 UC-PS-006 的取消终局，同格进汇总。派生读不回
// 时摘要缺席——摘要是派生便利不是判断本体，缺席不翻已提交的终局。
func (handler *FormParcelFinalHandler) deriveCompletion(
	ctx context.Context,
	command FormParcelFinalCommand,
	request domain.ShipmentRequest,
	result *FormParcelFinalResult,
) {
	members := request.CurrentSubmissionVersion().DeclaredParcelIDs()
	states := make([]domain.MemberFinalState, 0, len(members))
	for _, member := range members {
		state := domain.MemberFinalState{Parcel: member}
		if record, found, err := handler.deps.Finals.FindCurrentFinal(
			ctx, command.Identity.TenantID(), member); err == nil && found && record.Finalized {
			state.Finalized = true
		} else if err != nil {
			return
		}
		if !state.Finalized && handler.deps.Cancellations != nil {
			if _, cancelled, err := handler.deps.Cancellations.FindCancellation(
				ctx, command.Identity.TenantID(), member); err == nil && cancelled {
				state.Finalized = true
				state.Cancelled = true
			} else if err != nil {
				return
			}
		}
		states = append(states, state)
	}
	summary, err := domain.DeriveShipmentCompletion(states)
	if err != nil {
		return
	}
	result.completion = summary
	result.hasSummary = true
}

// handOff 交发布意图。终局与不采用都交——下游对两者都有消费（追踪要知道不采用的
// 迟到来源）；失败不翻结果，留续办引用重发同一份（AT-PS-065）。
func (handler *FormParcelFinalHandler) handOff(
	ctx context.Context,
	record ports.FinalOutcomeRecord,
) domain.OwnershipContinuationReference {
	if err := handler.deps.Downstream.HandOffFinalOutcome(
		ctx, ports.FinalOutcomeHandoffIntent{Record: record}); err == nil {
		return domain.OwnershipContinuationReference{}
	}
	return derivedContinuation(
		"PARCEL_FINAL_HANDOFF",
		record.Key.TenantID.String(),
		record.Key.Parcel.String(),
		record.Key.Kind.String(),
		record.Key.Version.String(),
	)
}

func (handler *FormParcelFinalHandler) undecided(
	command FormParcelFinalCommand,
	outcome domain.ResponsibilityOutcome,
	reason FinalUndecidedReason,
) FormParcelFinalResult {
	key := ports.FinalAdoptionKey{
		TenantID: command.Identity.TenantID(),
		Parcel:   outcome.Parcel(),
		Kind:     outcome.Kind(),
		Version:  outcome.Version(),
	}
	return FormParcelFinalResult{
		outcome:      ParcelFinalUndecided,
		reason:       reason,
		continuation: finalContinuation(key, reason),
	}
}

func finalContinuation(
	key ports.FinalAdoptionKey,
	reason FinalUndecidedReason,
) domain.OwnershipContinuationReference {
	return derivedContinuation(
		reason.String(),
		key.TenantID.String(),
		key.Parcel.String(),
		key.Kind.String(),
		key.Version.String(),
	)
}

// finalContentDigest 是同一终局采用身份的内容比对锚。
func finalContentDigest(outcome domain.ResponsibilityOutcome) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		outcome.Decision().String(),
		outcome.Execution().String(),
		outcome.OccurredAt().UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
