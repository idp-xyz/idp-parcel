package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 发生项失效驱动的预期成本纠错（ADR-0232 决定七；票 demo-chain-gaps/15）：运输履约交出一版失效版本的发生项已形成信封，结算记下「那一
// 版已失效」，再把依那一版形成的各条成本链头撤下——每条链长出一版失效纠错版本，原版与计算展开一字不动（CONTEXT「运输收费发生项更正、
// 失效或替代时，原预期成本和计算展开继续保留，本上下文追加新的预期成本版本表达计价纠错」）。不形成供应商账单贷项，也不碰账单主张与
// 审核应付（AT-SA-164）。

// ErrSupplierCostWithdrawalContradiction 说明派生的失效纠错版本标识上已有另一份内容。标识由被纠正那一版派生、一版至多被纠正一次，同一标识
// 只可能对应同一份内容；撞上另一份是库被改过。重投不自愈。
var ErrSupplierCostWithdrawalContradiction = errors.New("settlement accounting: the derived withdrawal version already carries other content")

// SupplierCostWithdrawalOutcome 是失效纠错的结果。三格定局都入账：撤下了几条、全是重放、没有依那一版形成的成本（评价还在路上的，记下的
// 失效让它之后不再形成）。只有依赖答不出才是未决。
type SupplierCostWithdrawalOutcome uint8

const (
	SupplierCostWithdrawalOutcomeInvalid SupplierCostWithdrawalOutcome = iota
	SupplierCostWithdrawn
	SupplierCostWithdrawalExisting
	SupplierCostWithdrawalNothingFormed
	SupplierCostWithdrawalNotAccepted
	SupplierCostWithdrawalUndecided
)

func (outcome SupplierCostWithdrawalOutcome) String() string {
	switch outcome {
	case SupplierCostWithdrawn:
		return "SUPPLIER_COST_WITHDRAWN"
	case SupplierCostWithdrawalExisting:
		return "EXISTING_WITHDRAWAL"
	case SupplierCostWithdrawalNothingFormed:
		return "NO_COST_FORMED"
	case SupplierCostWithdrawalNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case SupplierCostWithdrawalUndecided:
		return "SUPPLIER_COST_WITHDRAWAL_UNDECIDED"
	default:
		return ""
	}
}

// WithdrawSupplierExpectedCostCommand 是一版失效发生项在结算眼里的样子：失效版本自己的引用（它是这次纠错的依据，纠错版本上记的就是它），
// 与它让哪一版失效。
type WithdrawSupplierExpectedCostCommand struct {
	TenantID    domain.TenantID
	Occurrence  domain.TransportChargeOccurrence
	Invalidates domain.OccurrenceVersion
}

type SupplierCostWithdrawalResult struct {
	outcome   SupplierCostWithdrawalOutcome
	withdrawn []domain.SupplierExpectedCost
	cause     error
}

func (result SupplierCostWithdrawalResult) Outcome() SupplierCostWithdrawalOutcome {
	return result.outcome
}

// Withdrawn 在撤下与重放时交回这一次（或先前那一次）长出的各版失效纠错版本。
func (result SupplierCostWithdrawalResult) Withdrawn() []domain.SupplierExpectedCost {
	return append([]domain.SupplierExpectedCost(nil), result.withdrawn...)
}

// UndecidedCause 在未决时是依赖原样答出的错误。
func (result SupplierCostWithdrawalResult) UndecidedCause() error { return result.cause }

// WithdrawSupplierExpectedCostDeps 全部必备。
type WithdrawSupplierExpectedCostDeps struct {
	Registry      ports.ExpectedCostRegistry
	Costs         ports.ExpectedCostView
	Invalidations ports.SupplierCostOccurrenceInvalidations
	Clock         ports.Clock
}

type WithdrawSupplierExpectedCostHandler struct {
	deps WithdrawSupplierExpectedCostDeps
}

func NewWithdrawSupplierExpectedCostHandler(deps WithdrawSupplierExpectedCostDeps) (*WithdrawSupplierExpectedCostHandler, error) {
	if deps.Registry == nil || deps.Costs == nil || deps.Invalidations == nil || deps.Clock == nil {
		return nil, fmt.Errorf("%w: withdraw supplier expected cost", ErrNilDependency)
	}
	return &WithdrawSupplierExpectedCostHandler{deps: deps}, nil
}

// Withdraw 在发生项锁下读成本状态 → 记下那一版已失效 → 逐条链头：依那一版形成的长出失效纠错版本，已由这一次撤下的算重放，依别的
// 版本形成的不动。
//
// 只撤依被失效那一版形成的链头：发生项版本链是一条线（V1、V2……），失效那一版回指的就是它要撤的那一版；依更晚版本形成的链头说的是
// 另一次判断，一封迟到的旧失效信不该把它撤掉。**本编排只该跑在调用方的事务里**：锁、失效记录与纠错版本同生同灭。
func (handler *WithdrawSupplierExpectedCostHandler) Withdraw(
	ctx context.Context,
	command WithdrawSupplierExpectedCostCommand,
) (SupplierCostWithdrawalResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" || command.Occurrence.ID().String() == "" ||
		command.Invalidates.String() == "" || command.Invalidates == command.Occurrence.Version() {
		return SupplierCostWithdrawalResult{outcome: SupplierCostWithdrawalNotAccepted}, nil
	}
	state, err := handler.deps.Registry.LoadOccurrenceCostState(ctx, command.TenantID, command.Occurrence.ID())
	if err != nil {
		return SupplierCostWithdrawalResult{outcome: SupplierCostWithdrawalUndecided, cause: err}, nil
	}
	now := handler.deps.Clock.Now()
	if err := handler.deps.Invalidations.RecordOccurrenceInvalidation(ctx, command.TenantID, command.Occurrence.ID(),
		command.Invalidates, command.Occurrence.Version(), now); err != nil {
		return SupplierCostWithdrawalResult{outcome: SupplierCostWithdrawalUndecided, cause: err}, nil
	}

	var withdrawn, replayed []domain.SupplierExpectedCost
	for _, head := range state.Heads {
		if head.Withdrawn() {
			if head.Occurrence().Version() == command.Occurrence.Version() {
				replayed = append(replayed, head)
			}
			continue
		}
		if head.Occurrence().Version() != command.Invalidates {
			continue
		}
		cost, err := head.Withdraw(command.Occurrence)
		if err != nil {
			return SupplierCostWithdrawalResult{}, fmt.Errorf("settlement accounting: withdrawing %s: %w", head.Version(), err)
		}
		saved, err := handler.deps.Registry.Save(ctx, command.TenantID, cost, now)
		if err != nil {
			return SupplierCostWithdrawalResult{outcome: SupplierCostWithdrawalUndecided, cause: err}, nil
		}
		switch saved {
		case ports.ExpectedCostSaved:
			withdrawn = append(withdrawn, cost)
		case ports.ExpectedCostAlreadyRecorded:
			existing, found, err := handler.deps.Costs.LoadExpectedCost(ctx, command.TenantID, cost.Version())
			if err != nil {
				return SupplierCostWithdrawalResult{outcome: SupplierCostWithdrawalUndecided, cause: err}, nil
			}
			if !found || !existing.Withdrawn() || expectedCostDigest(existing) != expectedCostDigest(cost) {
				return SupplierCostWithdrawalResult{}, fmt.Errorf("%w: %s", ErrSupplierCostWithdrawalContradiction, cost.Version())
			}
			replayed = append(replayed, existing)
		default:
			return SupplierCostWithdrawalResult{}, fmt.Errorf("%w: %d", ErrUnexpectedExpectedCostSave, saved)
		}
	}
	switch {
	case len(withdrawn) > 0:
		return SupplierCostWithdrawalResult{outcome: SupplierCostWithdrawn, withdrawn: append(withdrawn, replayed...)}, nil
	case len(replayed) > 0:
		return SupplierCostWithdrawalResult{outcome: SupplierCostWithdrawalExisting, withdrawn: replayed}, nil
	default:
		return SupplierCostWithdrawalResult{outcome: SupplierCostWithdrawalNothingFormed}, nil
	}
}
