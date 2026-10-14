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

	// CustomerDirectionNets 交回该作用域的 P、W、已确认应收。
	// P 只加总已映射到本作用域对账单上的确认到账与资金退回，取链头金额。
	// W 只加总尚未撤销、且指向本作用域对账单的核销分配。
	CustomerDirectionNets(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (postedNet, writeoffNet, confirmed int64, err error)
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
