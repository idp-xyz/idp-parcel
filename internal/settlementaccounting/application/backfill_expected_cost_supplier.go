// backfill_expected_cost_supplier.go 是 ADR-0358 决定三的补记编排：本记录之前形成的预期成本没有供应商一格，迁移不跨上下文
// 回填，由运营点名租户补记——按各版所依的协议问 party-commercial，只写仍为空的那一格。
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// BackfillExpectedCostSupplierOutcome 是一次补记的结果。办完与「办完但有几份补不上」同属已办完：补不上的那几份要人去看两侧
// 为什么对不上，不能为它们挡住其余的补记；只有读不动、写不进才是未决，整笔回滚等重跑（ADR-0029）。
type BackfillExpectedCostSupplierOutcome uint8

const (
	BackfillExpectedCostSupplierOutcomeInvalid BackfillExpectedCostSupplierOutcome = iota
	ExpectedCostSupplierBackfilled
	ExpectedCostSupplierBackfillNotAccepted
	ExpectedCostSupplierBackfillUndecided
)

func (outcome BackfillExpectedCostSupplierOutcome) String() string {
	switch outcome {
	case ExpectedCostSupplierBackfilled:
		return "EXPECTED_COST_SUPPLIER_BACKFILLED"
	case ExpectedCostSupplierBackfillNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case ExpectedCostSupplierBackfillUndecided:
		return "EXPECTED_COST_SUPPLIER_BACKFILL_UNDECIDED"
	default:
		return ""
	}
}

// ExpectedCostSupplierGap 说一份补不上的成本为什么补不上：PC 不认它所依的那一版协议，或答出了译不过来的值（指称歧义也在其中）。
// 两种都是两侧矛盾，重跑同一份永远同一个答案。
type ExpectedCostSupplierGap uint8

const (
	ExpectedCostSupplierGapInvalid ExpectedCostSupplierGap = iota
	ExpectedCostAgreementNotRegistered
	ExpectedCostAgreementSupplierUntranslatable
)

func (gap ExpectedCostSupplierGap) String() string {
	switch gap {
	case ExpectedCostAgreementNotRegistered:
		return "AGREEMENT_NOT_REGISTERED"
	case ExpectedCostAgreementSupplierUntranslatable:
		return "AGREEMENT_SUPPLIER_UNTRANSLATABLE"
	default:
		return ""
	}
}

// BackfilledExpectedCost 是这一次写下了供应商的一版。
type BackfilledExpectedCost struct {
	Version  domain.SupplierCostVersionID
	Supplier domain.SupplierPartyReference
}

// UnresolvedExpectedCost 是这一次补不上、仍读作未记的一版。
type UnresolvedExpectedCost struct {
	Version   domain.SupplierCostVersionID
	Agreement domain.SupplierAgreementReference
	Gap       ExpectedCostSupplierGap
}

type BackfillExpectedCostSupplierCommand struct {
	TenantID domain.TenantID
}

type BackfillExpectedCostSupplierResult struct {
	Outcome    BackfillExpectedCostSupplierOutcome
	Recorded   []BackfilledExpectedCost
	Unresolved []UnresolvedExpectedCost
	// Cause 只在未决时在场：依赖原样答出的错误。
	Cause error
}

type BackfillExpectedCostSupplierDeps struct {
	Costs     ports.ExpectedCostSupplierBackfill
	Suppliers ports.SupplierAgreementSupplierView
}

type BackfillExpectedCostSupplierHandler struct {
	deps BackfillExpectedCostSupplierDeps
}

func NewBackfillExpectedCostSupplierHandler(deps BackfillExpectedCostSupplierDeps) (*BackfillExpectedCostSupplierHandler, error) {
	if deps.Costs == nil || deps.Suppliers == nil {
		return nil, errors.New("settlement accounting: expected cost supplier backfill needs the cost register and the agreement supplier view")
	}
	return &BackfillExpectedCostSupplierHandler{deps: deps}, nil
}

// Handle 列出这个租户还没记供应商的各版 → 按协议逐一问 PC（同一版协议只问一次）→ 只写仍为空的那一格。
//
// 补记是准的：已发布协议版本的正文不可覆盖，此刻问到的就是形成时会问到的那一家。问不到不填默认值，留空照实答未记。
func (handler *BackfillExpectedCostSupplierHandler) Handle(
	ctx context.Context,
	command BackfillExpectedCostSupplierCommand,
) (BackfillExpectedCostSupplierResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return BackfillExpectedCostSupplierResult{Outcome: ExpectedCostSupplierBackfillNotAccepted}, nil
	}
	awaiting, err := handler.deps.Costs.ListExpectedCostsAwaitingSupplier(ctx, command.TenantID)
	if err != nil {
		return backfillUndecided(err), nil
	}
	type answer struct {
		supplier domain.SupplierPartyReference
		gap      ExpectedCostSupplierGap
	}
	answers := make(map[domain.SupplierAgreementReference]answer)
	result := BackfillExpectedCostSupplierResult{Outcome: ExpectedCostSupplierBackfilled}
	for _, cost := range awaiting {
		known, asked := answers[cost.Agreement]
		if !asked {
			supplier, found, err := handler.deps.Suppliers.LoadSupplierAgreementSupplier(ctx, command.TenantID, cost.Agreement)
			switch {
			case errors.Is(err, ports.ErrSupplierAgreementSupplierUntranslatable):
				known = answer{gap: ExpectedCostAgreementSupplierUntranslatable}
			case err != nil:
				return backfillUndecided(fmt.Errorf("agreement %s: %w", cost.Agreement, err)), nil
			case !found:
				known = answer{gap: ExpectedCostAgreementNotRegistered}
			default:
				known = answer{supplier: supplier}
			}
			answers[cost.Agreement] = known
		}
		if known.gap != ExpectedCostSupplierGapInvalid {
			result.Unresolved = append(result.Unresolved, UnresolvedExpectedCost{Version: cost.Version, Agreement: cost.Agreement, Gap: known.gap})
			continue
		}
		recorded, err := handler.deps.Costs.RecordExpectedCostSupplier(ctx, command.TenantID, cost.Version, known.supplier)
		if err != nil {
			return backfillUndecided(fmt.Errorf("version %s: %w", cost.Version, err)), nil
		}
		if recorded {
			// 没写下的是列出之后被别的补记先写了：那一格已不为空，不改，也不算这一次补的。
			result.Recorded = append(result.Recorded, BackfilledExpectedCost{Version: cost.Version, Supplier: known.supplier})
		}
	}
	return result, nil
}

func backfillUndecided(cause error) BackfillExpectedCostSupplierResult {
	return BackfillExpectedCostSupplierResult{Outcome: ExpectedCostSupplierBackfillUndecided, Cause: cause}
}
