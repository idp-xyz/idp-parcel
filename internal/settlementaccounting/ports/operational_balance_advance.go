package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// OperationalBalanceAdvanceOutcome 是一次入账余额推进的落点。未推进不是错误：
// 截至时点不比登记上的更新，迟到的快照不盖新值。
type OperationalBalanceAdvanceOutcome uint8

const (
	OperationalBalanceAdvanceInvalid OperationalBalanceAdvanceOutcome = iota
	OperationalBalanceAdvanced
	OperationalBalanceNotAdvanced
)

func (outcome OperationalBalanceAdvanceOutcome) String() string {
	switch outcome {
	case OperationalBalanceAdvanced:
		return "ADVANCED"
	case OperationalBalanceNotAdvanced:
		return "NOT_ADVANCED"
	default:
		return ""
	}
}

// CustomerReceiptBooks 读客户收款映射要重算的那几本账。COD 代收本金不在这里：
// 代收分户账归 collection-remittance，本接口不读那一侧。
type CustomerReceiptBooks interface {
	// CustomerStatementScope 把一张客户对账单收成应收方向的结算作用域。
	// found 为 false：没有这张单，或它的账户不是应收、币种对不上。那不是客户收款这一支。
	CustomerStatementScope(
		ctx context.Context,
		tenant domain.TenantID,
		statement string,
	) (domain.SettlementScope, bool, error)

	// ReceivableAccountScope 把一个结算账户收成应收方向的作用域。
	// found 为 false：没有这个账户，或它不是应收。
	ReceivableAccountScope(
		ctx context.Context,
		tenant domain.TenantID,
		account string,
	) (domain.SettlementScope, bool, error)

	// CustomerDirectionNets 交回该作用域的 P、W、已确认应收。
	// P 加总已映射到本作用域对账单或应收账户上的确认到账与资金退回，取链头金额。
	// W 只加总尚未撤销、且指向本作用域对账单的核销分配。
	CustomerDirectionNets(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (postedNet, writeoffNet, confirmed int64, err error)

	// ConfirmedChargeAging 交回该作用域已确认客户应收的到期画像。
	// 没有已确认费用时 ConfirmedCount 为 0。缺到期日时 MissingDueDate 为真，此时不写信用状况。
	ConfirmedChargeAging(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		asOf time.Time,
	) (ChargeDuePicture, error)
}

// ChargeDuePicture 是账户级收款映射重算信用状况时读到的已确认费用。
type ChargeDuePicture struct {
	ConfirmedCount int64
	MissingDueDate bool
	Overdue        bool
}

// OperationalBalanceAdvance 在调用方已经打开的事务里推进运营余额登记。
// 新行的授信额度列写 0；之后的推进不覆盖该列。
type OperationalBalanceAdvance interface {
	AdvanceOperationalBalance(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		postedMinor, unsettledMinor int64,
		asOf time.Time,
	) (OperationalBalanceAdvanceOutcome, error)
}

// CreditStandingAdvanceOutcome 是一次信用状况推进的落点。未推进不是错误。
type CreditStandingAdvanceOutcome uint8

const (
	CreditStandingAdvanceInvalid CreditStandingAdvanceOutcome = iota
	CreditStandingAdvanced
	CreditStandingNotAdvanced
)

func (outcome CreditStandingAdvanceOutcome) String() string {
	switch outcome {
	case CreditStandingAdvanced:
		return "ADVANCED"
	case CreditStandingNotAdvanced:
		return "NOT_ADVANCED"
	default:
		return ""
	}
}

// CreditStandingAdvance 在调用方已经打开的事务里推进信用状况。
// 新行的登记额度列写 0；之后的推进只改逾期与截至时点，不覆盖该列。
type CreditStandingAdvance interface {
	AdvanceCreditStanding(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		overdue bool,
		asOf time.Time,
	) (CreditStandingAdvanceOutcome, error)
}
