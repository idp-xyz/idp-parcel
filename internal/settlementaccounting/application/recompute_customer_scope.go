package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// recomputeCustomerScope 是入账余额与已确认未结唯一的求法入口（ADR-0181 决定三「函数只有一个」）：映射、核销、核销撤销与
// 费用确认都从这里推进，交回这次推进用的截至时点。先锁作用域再取时刻、再按账读：锁前取的时刻可能早于占锁那一笔推进时的
// 时刻，「同键只准往前」就会把这一次吞成未推进；锁后的读才含先到那笔已提交的账。已确认未结求出为负是账不平，推进失败、
// 整笔回滚；锁下仍答未推进同样整笔回滚（ErrPostingNotAdvanced）。
func recomputeCustomerScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	balances ports.OperationalBalanceAdvance,
	books ports.CustomerReceiptBooks,
	clock ports.Clock,
) (time.Time, error) {
	if err := balances.LockOperationalBalanceScope(ctx, tenant, scope); err != nil {
		return time.Time{}, err
	}
	asOf := clock.Now()
	postedNet, writeoffNet, confirmed, err := books.CustomerDirectionNets(ctx, tenant, scope)
	if err != nil {
		return time.Time{}, err
	}
	posted, unsettled, err := domain.PostedAndUnsettled(postedNet, writeoffNet, confirmed)
	if err != nil {
		// 带上是哪个作用域：一笔映射、核销或撤销可能一次重算几个作用域，续办要人去核对的是账不平的那一个。
		return time.Time{}, fmt.Errorf("%w：作用域 法人 %s、账户 %s、币种 %s", err,
			scope.LegalEntity().String(), scope.Account().String(), scope.Currency().String())
	}
	advanced, err := balances.AdvanceOperationalBalance(ctx, tenant, scope, posted, unsettled, asOf)
	if err != nil {
		return time.Time{}, err
	}
	if advanced != ports.OperationalBalanceAdvanced {
		return time.Time{}, fmt.Errorf("%w: operational balance %s", ErrPostingNotAdvanced, advanced)
	}
	return asOf, nil
}
