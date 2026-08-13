package domain

import (
	"errors"
	"time"
)

// 本文件是账单接收与已发布对账单从持久化行回到领域的重建门（ADR-0030 的纪律）：
// 行数据不直接变成领域对象，先过与形成期同源的校验——五值匹配的逐格完备性、对账单
// 的勾稽与作废留痕，读回处复验，坏行响亮暴露而不是变成一个看起来合法的结论。
//
// MatchBillLine 与 PublishStatement 不能兼职重建：它们的输入是主张与草稿聚合本体，
// 而行里只有平铺结果——重建门以结果自证一致，不逆推形成过程。

var (
	ErrInvalidRehydratedMatch     = errors.New("settlement accounting: rehydrated bill line match violates its invariants")
	ErrInvalidRehydratedStatement = errors.New("settlement accounting: rehydrated statement violates its invariants")
)

// RehydrateBillLineMatchSpec 是从行数据重建一格匹配所需的全部字段。Expected 与
// Basis 以零值表示缺席——与 BillLineMatch 的 Expected()/Basis() 访问器同一约定。
type RehydrateBillLineMatchSpec struct {
	Claim          BillClaimID
	Line           BillLineReference
	Classification MatchClassification
	Expected       SupplierCostVersionID
	ClaimedMinor   int64
	ExpectedMinor  int64
	Currency       CurrencyCode
	Basis          MatchBasisReference
	MatchedAt      time.Time
}

// RehydrateBillLineMatch 重走 MatchBillLine 的五值逐格矩阵：`已匹配`金额相等且无差异
// 依据、量差/价差金额不等且带依据、`无匹配发生项`预期缺席、`重复计费`预期与依据同在。
func RehydrateBillLineMatch(spec RehydrateBillLineMatchSpec) (BillLineMatch, error) {
	if !spec.Claim.valid() ||
		!spec.Line.valid() ||
		!spec.Classification.valid() ||
		!spec.Currency.valid() ||
		spec.ClaimedMinor <= 0 ||
		spec.MatchedAt.IsZero() {
		return BillLineMatch{}, ErrInvalidRehydratedMatch
	}
	expectedPresent := spec.Expected.valid()
	if !expectedPresent && spec.ExpectedMinor != 0 {
		// 没有预期成本却有预期金额：这行的数字不知道从哪来的。
		return BillLineMatch{}, ErrInvalidRehydratedMatch
	}

	switch spec.Classification {
	case LineMatched:
		if !expectedPresent || spec.ClaimedMinor != spec.ExpectedMinor || spec.Basis.valid() {
			return BillLineMatch{}, ErrInvalidRehydratedMatch
		}
	case QuantityVariance, PriceVariance:
		if !expectedPresent || spec.ClaimedMinor == spec.ExpectedMinor || !spec.Basis.valid() {
			return BillLineMatch{}, ErrInvalidRehydratedMatch
		}
	case NoMatchingOccurrence:
		if expectedPresent || !spec.Basis.valid() {
			return BillLineMatch{}, ErrInvalidRehydratedMatch
		}
	case DuplicateBilling:
		if !expectedPresent || !spec.Basis.valid() {
			return BillLineMatch{}, ErrInvalidRehydratedMatch
		}
	}

	return BillLineMatch{
		claim:          spec.Claim,
		line:           spec.Line,
		classification: spec.Classification,
		expected:       spec.Expected,
		claimedMinor:   spec.ClaimedMinor,
		expectedMinor:  spec.ExpectedMinor,
		currency:       spec.Currency,
		basis:          spec.Basis,
		matchedAt:      spec.MatchedAt.UTC(),
	}, nil
}

// RehydratePublishedStatementSpec 是从行数据重建一份已发布对账单所需的全部字段。
// VoidBasis 与 VoidedAt 以零值表示未作废——两者必须同在或同缺。
type RehydratePublishedStatementSpec struct {
	Number      StatementNumber
	Account     SettlementAccountID
	Period      BillingPeriodReference
	Currency    CurrencyCode
	Lines       []StatementLine
	Adjustments []StatementAdjustmentLine
	TotalMinor  int64
	PublishedAt time.Time
	VoidBasis   StatementVoidBasisReference
	VoidedAt    time.Time
}

// RehydratePublishedStatement 复验发布快照的三类不变量：
//
//   - 行完备：至少一行、费用不重复、金额恒正；调整行必须挂在单内费用上（别人的数
//     不进这个账户）；
//   - 勾稽（AT-SA-065 的读回半边）：总额必须等于行合计加借减贷——存进去时勾稽过的
//     数字读出来不平，说明这行被改写过；
//   - 作废留痕（AT-SA-069）：依据与时刻同在或同缺，作废不早于发布；行与总额仍在
//     ——留痕不是删内容。
func RehydratePublishedStatement(spec RehydratePublishedStatementSpec) (PublishedStatement, error) {
	if !spec.Number.valid() ||
		!spec.Account.valid() ||
		!spec.Period.valid() ||
		!spec.Currency.valid() ||
		spec.PublishedAt.IsZero() ||
		len(spec.Lines) == 0 {
		return PublishedStatement{}, ErrInvalidRehydratedStatement
	}

	charges := make(map[CustomerChargeID]struct{}, len(spec.Lines))
	runningTotal := int64(0)
	for _, line := range spec.Lines {
		if !line.Charge.valid() || line.AmountMinor <= 0 {
			return PublishedStatement{}, ErrInvalidRehydratedStatement
		}
		if _, duplicated := charges[line.Charge]; duplicated {
			return PublishedStatement{}, ErrInvalidRehydratedStatement
		}
		charges[line.Charge] = struct{}{}
		runningTotal += line.AmountMinor
	}
	for _, adjustment := range spec.Adjustments {
		if !adjustment.Adjustment.valid() ||
			!adjustment.Direction.valid() ||
			adjustment.AmountMinor <= 0 {
			return PublishedStatement{}, ErrInvalidRehydratedStatement
		}
		if _, covered := charges[adjustment.Charge]; !covered {
			return PublishedStatement{}, ErrInvalidRehydratedStatement
		}
		if adjustment.Direction == AdjustmentDebit {
			runningTotal += adjustment.AmountMinor
		} else {
			runningTotal -= adjustment.AmountMinor
		}
	}
	if runningTotal != spec.TotalMinor {
		return PublishedStatement{}, ErrStatementImbalance
	}

	voided := !spec.VoidedAt.IsZero()
	if voided != spec.VoidBasis.valid() {
		return PublishedStatement{}, ErrInvalidRehydratedStatement
	}
	if voided && spec.VoidedAt.Before(spec.PublishedAt) {
		return PublishedStatement{}, ErrInvalidRehydratedStatement
	}

	statement := PublishedStatement{
		number:      spec.Number,
		account:     spec.Account,
		period:      spec.Period,
		currency:    spec.Currency,
		lines:       append([]StatementLine(nil), spec.Lines...),
		adjustments: append([]StatementAdjustmentLine(nil), spec.Adjustments...),
		totalMinor:  spec.TotalMinor,
		publishedAt: spec.PublishedAt.UTC(),
	}
	if voided {
		statement.voidBasis = spec.VoidBasis
		statement.voidedAt = spec.VoidedAt.UTC()
	}
	return statement, nil
}
