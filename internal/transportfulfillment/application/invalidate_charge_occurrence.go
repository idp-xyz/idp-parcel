package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 运输收费发生项失效（ADR-0232 决定四）：一项已登记的发生项所说的范围不成立，由持「运输收费发生项失效」授予的操作者显式说出来。
// 发生项册上长出失效版本、回指链头，原项不动；已形成信封随新版本同笔交出，结算据此追加预期成本的纠错（票 demo-chain-gaps/15）。
//
// 它是发生项修订的生产入口：此前 ReviseValidity 没有生产调用方，「已形成的先走发生项失效」（解除采用的前置）无路可走。

// ChargeOccurrenceInvalidationOutcome 是失效的应用结果。各格续办两两不同：不在册（核引用）、点名的不是链头（先看链头是什么、是不是
// 别人已经改过）、已失效（什么都不必做）、未受理（改请求）、未决（原样重交）。
type ChargeOccurrenceInvalidationOutcome uint8

const (
	ChargeOccurrenceInvalidationOutcomeInvalid ChargeOccurrenceInvalidationOutcome = iota
	ChargeOccurrenceInvalidated
	ChargeOccurrenceInvalidationExisting
	ChargeOccurrenceInvalidationNotFound
	ChargeOccurrenceInvalidationVersionNotCurrent
	ChargeOccurrenceInvalidationAlreadyInvalidated
	ChargeOccurrenceInvalidationNotAccepted
	ChargeOccurrenceInvalidationUndecided
)

func (outcome ChargeOccurrenceInvalidationOutcome) String() string {
	switch outcome {
	case ChargeOccurrenceInvalidated:
		return "OCCURRENCE_INVALIDATED"
	case ChargeOccurrenceInvalidationExisting:
		return "EXISTING_INVALIDATION"
	case ChargeOccurrenceInvalidationNotFound:
		return "OCCURRENCE_NOT_FOUND"
	case ChargeOccurrenceInvalidationVersionNotCurrent:
		return "VERSION_NOT_CURRENT"
	case ChargeOccurrenceInvalidationAlreadyInvalidated:
		return "OCCURRENCE_ALREADY_INVALIDATED"
	case ChargeOccurrenceInvalidationNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case ChargeOccurrenceInvalidationUndecided:
		return "INVALIDATION_UNDECIDED"
	default:
		return ""
	}
}

// ChargeOccurrenceInvalidationUndecidedReason 指名失效停在哪一口。
type ChargeOccurrenceInvalidationUndecidedReason uint8

const (
	ChargeOccurrenceInvalidationUndecidedReasonNone ChargeOccurrenceInvalidationUndecidedReason = iota
	ChargeOccurrenceInvalidationRegistryUnavailable
	ChargeOccurrenceInvalidationHandoffUnavailable
)

func (reason ChargeOccurrenceInvalidationUndecidedReason) String() string {
	switch reason {
	case ChargeOccurrenceInvalidationRegistryUnavailable:
		return "OCCURRENCE_REGISTRY_UNAVAILABLE"
	case ChargeOccurrenceInvalidationHandoffUnavailable:
		return "OCCURRENCE_HANDOFF_UNAVAILABLE"
	default:
		return ""
	}
}

// InvalidateChargeOccurrenceCommand 携带一次失效决定：哪一项、作决定的人看到的是哪一版、凭什么。点名版本是并发闸：作决定的人看的是
// 链头那一版，别人已在那之后改过时，这一次不该落在别人的结果上。依据是作决定的人交来的一条引用（证据或决定编号），原样记作修订
// 依据。租户与操作者取自认证结果。
type InvalidateChargeOccurrenceCommand struct {
	TenantID   domain.TenantID
	Operator   string
	Occurrence string
	Version    string
	Basis      string
}

type ChargeOccurrenceInvalidationResult struct {
	outcome      ChargeOccurrenceInvalidationOutcome
	reason       ChargeOccurrenceInvalidationUndecidedReason
	record       ports.ChargeOccurrenceRecord
	hasRecord    bool
	continuation string
}

func (result ChargeOccurrenceInvalidationResult) Outcome() ChargeOccurrenceInvalidationOutcome {
	return result.outcome
}

func (result ChargeOccurrenceInvalidationResult) UndecidedReason() ChargeOccurrenceInvalidationUndecidedReason {
	return result.reason
}

// Record 在失效成立、重放、点名的不是链头与已失效四格交回链头：前两格是那一版失效版本，后两格是此刻的链头，作决定的人据它看出
// 别人做了什么。
func (result ChargeOccurrenceInvalidationResult) Record() (ports.ChargeOccurrenceRecord, bool) {
	return result.record, result.hasRecord
}

func (result ChargeOccurrenceInvalidationResult) ContinuationReference() string {
	return result.continuation
}

// InvalidateChargeOccurrenceDeps 全部必备：发生项册（读链、落失效版本）、范围锁、已形成信封的交出口与时钟。
type InvalidateChargeOccurrenceDeps struct {
	Occurrences ports.ChargeOccurrenceRegistry
	Scopes      ports.ChargeOccurrenceScopeHold
	Handoff     ports.ChargeOccurrenceHandoff
	Clock       ports.Clock
}

type InvalidateChargeOccurrenceHandler struct {
	deps InvalidateChargeOccurrenceDeps
}

func NewInvalidateChargeOccurrenceHandler(deps InvalidateChargeOccurrenceDeps) *InvalidateChargeOccurrenceHandler {
	return &InvalidateChargeOccurrenceHandler{deps: deps}
}

// Invalidate 受理 → 读链头 → 取它那个范围的锁 → 锁下再读链头 → 判点名版本与已失效 → 长出失效版本 → 落册并交信封。
//
// 先读一次链头只为知道锁哪个范围；锁下再读一次才作数：同一范围上并发的登记（失效之后凭同一件事实重新成立）或另一次失效握着
// 这把锁时，锁前读到的链头可能已经过时。**未决时装配必须整笔回滚**：落了失效版本而信封没交上，结算就永远不知道那笔成本要纠错。
func (handler *InvalidateChargeOccurrenceHandler) Invalidate(
	ctx context.Context,
	command InvalidateChargeOccurrenceCommand,
) (ChargeOccurrenceInvalidationResult, error) {
	occurrenceRef, err := domain.NewChargeOccurrenceReference(command.Occurrence)
	if err != nil || strings.TrimSpace(command.TenantID.String()) == "" {
		return ChargeOccurrenceInvalidationResult{outcome: ChargeOccurrenceInvalidationNotAccepted}, nil
	}
	named, err := domain.NewOccurrenceValidityVersion(command.Version)
	if err != nil {
		return ChargeOccurrenceInvalidationResult{outcome: ChargeOccurrenceInvalidationNotAccepted}, nil
	}
	basis, err := domain.NewOccurrenceBasisReference(command.Basis)
	if err != nil {
		return ChargeOccurrenceInvalidationResult{outcome: ChargeOccurrenceInvalidationNotAccepted}, nil
	}
	operator, err := domain.NewRegistrantReference(command.Operator)
	if err != nil {
		return ChargeOccurrenceInvalidationResult{outcome: ChargeOccurrenceInvalidationNotAccepted}, nil
	}
	continuation := chargeOccurrenceContinuation("CHARGE_OCCURRENCE_INVALIDATION", command.TenantID.String(), occurrenceRef.String())

	head, found, err := occurrenceChainHead(ctx, handler.deps.Occurrences, command.TenantID, occurrenceRef)
	if errors.Is(err, ErrChargeOccurrenceChainBroken) {
		return ChargeOccurrenceInvalidationResult{}, err
	}
	if err != nil {
		return invalidationUndecided(ChargeOccurrenceInvalidationRegistryUnavailable, continuation), nil
	}
	if !found {
		return ChargeOccurrenceInvalidationResult{outcome: ChargeOccurrenceInvalidationNotFound}, nil
	}
	occurrence := head.Occurrence
	if err := handler.deps.Scopes.HoldOccurrenceScope(ctx, command.TenantID, occurrence.Reason(), occurrence.Scope()); err != nil {
		return invalidationUndecided(ChargeOccurrenceInvalidationRegistryUnavailable, continuation), nil
	}
	head, found, err = occurrenceChainHead(ctx, handler.deps.Occurrences, command.TenantID, occurrenceRef)
	if errors.Is(err, ErrChargeOccurrenceChainBroken) {
		return ChargeOccurrenceInvalidationResult{}, err
	}
	if err != nil || !found {
		return invalidationUndecided(ChargeOccurrenceInvalidationRegistryUnavailable, continuation), nil
	}
	if answer, answered := handler.answerFromHead(ctx, head, named, basis, continuation); answered {
		return answer, nil
	}

	now := handler.deps.Clock.Now().UTC()
	invalidated, err := domain.InvalidateChargeOccurrence(head.Occurrence, basis, now, operator)
	if err != nil {
		// 请求能出错的各格都已过了构造门；链头是重建门放行的一版、时刻取自时钟，走到这里是链或时钟违约，重试不会变。
		return ChargeOccurrenceInvalidationResult{}, fmt.Errorf("invalidate charge occurrence %s: %w", occurrenceRef, err)
	}
	record := ports.ChargeOccurrenceRecord{
		Key:        ports.ChargeOccurrenceKey{TenantID: command.TenantID, Occurrence: occurrenceRef, Validity: invalidated.Validity()},
		Occurrence: invalidated,
		RecordedAt: now,
	}
	saved, err := handler.deps.Occurrences.Save(ctx, record)
	if err != nil {
		return invalidationUndecided(ChargeOccurrenceInvalidationRegistryUnavailable, continuation), nil
	}
	if saved == ports.ChargeOccurrenceAlreadyRegistered {
		// 握着范围锁还撞键，只剩没走这把锁的写入者；读回链头照常作答，不替它挑一边。
		head, found, err = occurrenceChainHead(ctx, handler.deps.Occurrences, command.TenantID, occurrenceRef)
		if err != nil || !found {
			return invalidationUndecided(ChargeOccurrenceInvalidationRegistryUnavailable, continuation), nil
		}
		if answer, answered := handler.answerFromHead(ctx, head, named, basis, continuation); answered {
			return answer, nil
		}
		return ChargeOccurrenceInvalidationResult{}, fmt.Errorf("invalidate charge occurrence %s: %w: saving %s answered already registered",
			occurrenceRef, ErrChargeOccurrenceChainBroken, invalidated.Validity())
	}
	if err := handler.deps.Handoff.HandOffChargeOccurrence(ctx, ports.ChargeOccurrenceIntent{Record: record}); err != nil {
		return invalidationUndecided(ChargeOccurrenceInvalidationHandoffUnavailable, continuation), nil
	}
	return ChargeOccurrenceInvalidationResult{outcome: ChargeOccurrenceInvalidated, record: record, hasRecord: true}, nil
}

// answerFromHead 按链头答不必再写的各格。链头恰是点名那一版的失效、依据也同，是这一次的重放：重交同一份意图（按信封 ID 认领，
// 先前若有一次没交成，这一次补上）。链头不是点名那一版——别人已在点名那一版之后改过，或点名的本就是更早的一版——答点名的不是
// 链头、回显链头。点名的就是链头而它已是失效版本，答已失效。
func (handler *InvalidateChargeOccurrenceHandler) answerFromHead(
	ctx context.Context,
	head ports.ChargeOccurrenceRecord,
	named domain.OccurrenceValidityVersion,
	basis domain.OccurrenceBasisReference,
	continuation string,
) (ChargeOccurrenceInvalidationResult, bool) {
	occurrence := head.Occurrence
	if head.Key.Validity == named {
		if occurrence.Invalidated() {
			return ChargeOccurrenceInvalidationResult{outcome: ChargeOccurrenceInvalidationAlreadyInvalidated, record: head, hasRecord: true}, true
		}
		return ChargeOccurrenceInvalidationResult{}, false
	}
	corrects, revised := occurrence.Corrects()
	_, revisionBasis, _, _ := occurrence.Revision()
	if occurrence.Invalidated() && revised && corrects == named && revisionBasis == basis {
		if err := handler.deps.Handoff.HandOffChargeOccurrence(ctx, ports.ChargeOccurrenceIntent{Record: head}); err != nil {
			return invalidationUndecided(ChargeOccurrenceInvalidationHandoffUnavailable, continuation), true
		}
		return ChargeOccurrenceInvalidationResult{outcome: ChargeOccurrenceInvalidationExisting, record: head, hasRecord: true}, true
	}
	return ChargeOccurrenceInvalidationResult{outcome: ChargeOccurrenceInvalidationVersionNotCurrent, record: head, hasRecord: true}, true
}

func invalidationUndecided(reason ChargeOccurrenceInvalidationUndecidedReason, continuation string) ChargeOccurrenceInvalidationResult {
	return ChargeOccurrenceInvalidationResult{outcome: ChargeOccurrenceInvalidationUndecided, reason: reason, continuation: continuation}
}
