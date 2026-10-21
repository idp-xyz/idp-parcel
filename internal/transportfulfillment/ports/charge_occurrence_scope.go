package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 发生项范围上的两口（ADR-0232）：按（租户、原因、主要业务范围）串行化，与按范围列出各项的链头。另立文件、不并进 ports.go 与
// charge_occurrence.go：那里的接口有别处写的替身，往上加方法替身就编不过（tf-chain-plan「跨票通则」X4）。

// ChargeOccurrenceScopeHold 取一个范围上发生项改动的事务级串行锁，直到事务结束。它与 ChargeOccurrenceRegistry.FindCovering 取的是
// 同一把锁：登记在查覆盖之前握着它直到落册提交，失效与解除采用在读这个范围上的发生项之前握着它——三者在同一个范围上就排成先后，
// 谁也不会凭一个对方还没提交的样子下判断。不在事务里调用由实现拒。
type ChargeOccurrenceScopeHold interface {
	HoldOccurrenceScope(
		ctx context.Context,
		tenant domain.TenantID,
		reason domain.ChargeOccurrenceReason,
		scope domain.OccurrenceScopeReference,
	) error
}

// ChargeOccurrenceScopeView 按（租户、原因、主要业务范围）列出这个范围上仍有效的各项，每项交它的链头：没有后继的那一版、且不是
// 失效版。被替代的旧版由新版接替，失效的那一项说的是「那个范围不成立」，都不算这个范围上的外部成本来源（与覆盖一问同一个读法）。
// 解除采用据它判「这一段是否已按那份委托形成了发生项」。
type ChargeOccurrenceScopeView interface {
	ListCurrentOnScope(
		ctx context.Context,
		tenant domain.TenantID,
		reason domain.ChargeOccurrenceReason,
		scope domain.OccurrenceScopeReference,
	) ([]ChargeOccurrenceRecord, error)
}
