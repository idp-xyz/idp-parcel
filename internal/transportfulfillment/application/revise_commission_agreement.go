package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 委托协议换版（ADR-0231 决定三）。
//
// **它是一次显式的运营决定。** 委托提交时固定的那一版协议此后被截止或退役，或租户另发一版补上收费发生条件，委托不会自己跟上：快照是
// 采购的权威（ADR-0204 决定一），party-commercial 发了新版不替委托改指。持「委托协议换版」授予的操作者指名后继版本与自何时起，本用例
// 核过后继、落下委托的下一版、同一笔交一封门铃，派发进程据它对采用了这份委托的各段补形成（ADR-0231 决定五）。旧版留在委托上，起用之前
// 的事实仍按它判。

// ErrUnexpectedRevisionSave 说明换版册在取到换版锁之后仍答撞号或撞依据。锁守着「已有几版」与「这份依据换过没有」，撞上就是锁没守住。
var ErrUnexpectedRevisionSave = errors.New("transport fulfillment: commission agreement revision collided under its lock")

// CommissionAgreementRevisionOutcome 是换版的结果代数。前置条件不满足的各格续办两两不同（ADR-0029）：委托不在册（核引用）、后继换了协议
// 对象（另起委托）、起用早于提交（改起用时刻）、后继在起用时刻的四个协议否定格（去 PC 发布、改起用时刻、取得新协议、改指那一份
// 供应商的协议），折成一格操作者只能去猜。
type CommissionAgreementRevisionOutcome uint8

const (
	CommissionAgreementRevisionOutcomeInvalid CommissionAgreementRevisionOutcome = iota
	CommissionAgreementRevised
	CommissionAgreementRevisionExisting
	CommissionAgreementRevisionConflict
	CommissionAgreementRevisionCommissionNotFound
	CommissionAgreementRevisionObjectMismatch
	CommissionAgreementRevisionAppliesBeforeSubmission
	CommissionAgreementRevisionAgreementNotRegistered
	CommissionAgreementRevisionAgreementNotEffective
	CommissionAgreementRevisionAgreementTerminated
	CommissionAgreementRevisionProviderMismatch
	CommissionAgreementRevisionNotAccepted
	CommissionAgreementRevisionUndecided
)

func (outcome CommissionAgreementRevisionOutcome) String() string {
	switch outcome {
	case CommissionAgreementRevised:
		return "COMMISSION_AGREEMENT_REVISED"
	case CommissionAgreementRevisionExisting:
		return "EXISTING_REVISION"
	case CommissionAgreementRevisionConflict:
		return "REVISION_CONFLICT"
	case CommissionAgreementRevisionCommissionNotFound:
		return "COMMISSION_NOT_FOUND"
	case CommissionAgreementRevisionObjectMismatch:
		return "AGREEMENT_OBJECT_MISMATCH"
	case CommissionAgreementRevisionAppliesBeforeSubmission:
		return "APPLIES_BEFORE_SUBMISSION"
	case CommissionAgreementRevisionAgreementNotRegistered:
		return ports.SupplierAgreementNotRegistered.String()
	case CommissionAgreementRevisionAgreementNotEffective:
		return ports.SupplierAgreementNotEffective.String()
	case CommissionAgreementRevisionAgreementTerminated:
		return ports.SupplierAgreementTerminated.String()
	case CommissionAgreementRevisionProviderMismatch:
		return ports.SupplierAgreementProviderMismatch.String()
	case CommissionAgreementRevisionNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case CommissionAgreementRevisionUndecided:
		return "REVISION_UNDECIDED"
	default:
		return ""
	}
}

// CommissionAgreementRevisionUndecidedReason 指名换版停在哪一口。
type CommissionAgreementRevisionUndecidedReason uint8

const (
	CommissionAgreementRevisionUndecidedReasonNone CommissionAgreementRevisionUndecidedReason = iota
	CommissionAgreementRevisionStoreUnavailable
	CommissionAgreementRevisionCommissionStoreUnavailable
	CommissionAgreementRevisionAgreementUnavailable
	CommissionAgreementRevisionDoorbellUnavailable
)

func (reason CommissionAgreementRevisionUndecidedReason) String() string {
	switch reason {
	case CommissionAgreementRevisionStoreUnavailable:
		return "REVISION_STORE_UNAVAILABLE"
	case CommissionAgreementRevisionCommissionStoreUnavailable:
		return "COMMISSION_STORE_UNAVAILABLE"
	case CommissionAgreementRevisionAgreementUnavailable:
		return "SUPPLIER_AGREEMENT_UNAVAILABLE"
	case CommissionAgreementRevisionDoorbellUnavailable:
		return "DOORBELL_UNAVAILABLE"
	default:
		return ""
	}
}

// ReviseCommissionAgreementCommand 携带一次换版声明。租户与决定人取自认证结果，不来自载荷；起用时刻是业务时间，由操作者给（ADR-0023
// 同一条纪律：事实的时间由提出它的一方给）。
type ReviseCommissionAgreementCommand struct {
	TenantID         domain.TenantID
	DecidedBy        string
	Commission       string
	SuccessorObject  string
	SuccessorVersion string
	AppliesFrom      time.Time
	Basis            string
}

type CommissionAgreementRevisionResult struct {
	outcome        CommissionAgreementRevisionOutcome
	reason         CommissionAgreementRevisionUndecidedReason
	revision       domain.CommissionAgreementRevision
	hasRevision    bool
	agreedSupplier string
	continuation   string
}

func (result CommissionAgreementRevisionResult) Outcome() CommissionAgreementRevisionOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result CommissionAgreementRevisionResult) UndecidedReason() CommissionAgreementRevisionUndecidedReason {
	return result.reason
}

// Revision 在换版成立、已有与冲突三格交回那一版：成立是这一次落的，后两格是同一份依据在册的那一版。
func (result CommissionAgreementRevisionResult) Revision() (domain.CommissionAgreementRevision, bool) {
	return result.revision, result.hasRevision
}

// AgreedSupplier 在服务方不符那一格交回后继那一版约定的供应商。
func (result CommissionAgreementRevisionResult) AgreedSupplier() string { return result.agreedSupplier }

func (result CommissionAgreementRevisionResult) ContinuationReference() string {
	return result.continuation
}

// ReviseCommissionAgreementDeps 全部必备。委托只读：换版写在自己的册上，不改委托那一行。
type ReviseCommissionAgreementDeps struct {
	Commissions ports.TransportCommissionView
	Revisions   ports.CommissionAgreementRevisionStore
	Agreements  ports.SupplierAgreementSource
	Doorbell    ports.CommissionAgreementRevisionHandoff
	Clock       ports.Clock
}

type ReviseCommissionAgreementHandler struct {
	deps ReviseCommissionAgreementDeps
}

func NewReviseCommissionAgreementHandler(deps ReviseCommissionAgreementDeps) *ReviseCommissionAgreementHandler {
	return &ReviseCommissionAgreementHandler{deps: deps}
}

// Revise 声明过构造门 → 取这份委托的换版锁 → 读委托 → 同一份依据换过就分已有与冲突 → 领域判声明那一半 → 在起用时刻核后继 → 落下
// 一版 → 交门铃。
//
// 先取锁再读委托：换版按「已有几版 + 1」铸号、按依据查重放，两笔并发的换版各读各的就会撞号，同一份依据并发重放也会各判「没换过」。
// 后继核在起用时刻：起用之后的事实都按它判，它在那一刻不支持采购，那一刻起结束的参与就都形成不了。
// **本编排答`换版成立`以外的任一格时，装配整笔回滚**：其余各格都不该留下写入。
func (handler *ReviseCommissionAgreementHandler) Revise(
	ctx context.Context,
	command ReviseCommissionAgreementCommand,
) (CommissionAgreementRevisionResult, error) {
	commissionRef, err := domain.NewTransportCommissionReference(command.Commission)
	if err != nil || strings.TrimSpace(command.TenantID.String()) == "" {
		return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevisionNotAccepted}, nil
	}
	spec, err := revisionSpecFrom(command)
	if err != nil {
		return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevisionNotAccepted}, nil
	}
	key := ports.TransportCommissionKey{TenantID: command.TenantID, Commission: commissionRef}

	if err := handler.deps.Revisions.HoldRevisions(ctx, key); err != nil {
		return revisionUndecided(CommissionAgreementRevisionStoreUnavailable, command), nil
	}
	record, found, err := handler.deps.Commissions.FindByKey(ctx, key)
	if err != nil {
		return revisionUndecided(CommissionAgreementRevisionCommissionStoreUnavailable, command), nil
	}
	if !found {
		return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevisionCommissionNotFound}, nil
	}
	commission := record.Commission
	if existing, revised := commission.AgreementRevisionBy(spec.Basis); revised {
		outcome := CommissionAgreementRevisionConflict
		if existing.SameDecisionAs(spec) {
			outcome = CommissionAgreementRevisionExisting
		}
		return CommissionAgreementRevisionResult{outcome: outcome, revision: existing, hasRevision: true}, nil
	}

	draft, err := commission.DraftAgreementRevision(spec)
	switch {
	case errors.Is(err, domain.ErrRevisionAppliesBeforeSubmission):
		return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevisionAppliesBeforeSubmission}, nil
	case errors.Is(err, domain.ErrRevisionAgreementObjectMismatch):
		return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevisionObjectMismatch}, nil
	case err != nil:
		return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevisionNotAccepted}, nil
	}

	check, err := handler.deps.Agreements.CheckSupplierAgreement(ctx, command.TenantID, draft.Successor(), commission.Provider(), draft.AppliesFrom())
	if err != nil {
		return revisionUndecided(CommissionAgreementRevisionAgreementUnavailable, command), nil
	}
	switch check.Standing {
	case ports.SupplierAgreementRegistered:
	case ports.SupplierAgreementNotRegistered:
		return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevisionAgreementNotRegistered}, nil
	case ports.SupplierAgreementNotEffective:
		return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevisionAgreementNotEffective}, nil
	case ports.SupplierAgreementTerminated:
		return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevisionAgreementTerminated}, nil
	case ports.SupplierAgreementProviderMismatch:
		return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevisionProviderMismatch, agreedSupplier: check.Supplier.String()}, nil
	default:
		return CommissionAgreementRevisionResult{}, errors.New("revise commission agreement: supplier agreement check answered outside its closed set")
	}

	_, revision, err := draft.Revise(check.Snapshot, check.LegalEntity, handler.deps.Clock.Now())
	if err != nil {
		// 声明那一半已过草稿门，这里还立不起来只剩核对口交回了不成形的快照或法人，是适配器缺陷。
		return CommissionAgreementRevisionResult{}, fmt.Errorf("revise commission agreement: forming after the agreement check: %w", err)
	}
	saved := ports.CommissionAgreementRevisionRecord{Key: key, Revision: revision}
	outcome, err := handler.deps.Revisions.SaveRevision(ctx, saved)
	if err != nil {
		return revisionUndecided(CommissionAgreementRevisionStoreUnavailable, command), nil
	}
	if outcome != ports.CommissionAgreementRevisionSaved {
		return CommissionAgreementRevisionResult{}, fmt.Errorf("%w: commission %s revision %d (%d)", ErrUnexpectedRevisionSave, commissionRef, revision.Number(), outcome)
	}
	if err := handler.deps.Doorbell.HandOffCommissionAgreementRevision(ctx, ports.CommissionAgreementRevisedIntent{Record: saved}); err != nil {
		return revisionUndecided(CommissionAgreementRevisionDoorbellUnavailable, command), nil
	}
	return CommissionAgreementRevisionResult{outcome: CommissionAgreementRevised, revision: revision, hasRevision: true}, nil
}

func revisionSpecFrom(command ReviseCommissionAgreementCommand) (domain.CommissionAgreementRevisionSpec, error) {
	successor, err := domain.NewSupplierAgreementVersion(command.SuccessorObject, command.SuccessorVersion)
	if err != nil {
		return domain.CommissionAgreementRevisionSpec{}, err
	}
	basis, err := domain.NewCommissionAgreementRevisionBasisReference(command.Basis)
	if err != nil {
		return domain.CommissionAgreementRevisionSpec{}, err
	}
	decidedBy, err := domain.NewRegistrantReference(command.DecidedBy)
	if err != nil {
		return domain.CommissionAgreementRevisionSpec{}, err
	}
	if domain.AtRegistryPrecision(command.AppliesFrom).IsZero() {
		return domain.CommissionAgreementRevisionSpec{}, domain.ErrInvalidCommissionAgreementRevision
	}
	return domain.CommissionAgreementRevisionSpec{
		Successor: successor, AppliesFrom: command.AppliesFrom, Basis: basis, DecidedBy: decidedBy,
	}, nil
}

func revisionUndecided(reason CommissionAgreementRevisionUndecidedReason, command ReviseCommissionAgreementCommand) CommissionAgreementRevisionResult {
	return CommissionAgreementRevisionResult{
		outcome:      CommissionAgreementRevisionUndecided,
		reason:       reason,
		continuation: commissionContinuation("COMMISSION_AGREEMENT_REVISION", reason.String(), command.Commission+"/"+command.Basis),
	}
}
