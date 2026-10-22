package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ChargeAttributionRegister 写下一条费用项目的归属日形态。读口是 ChargeAttributionView。
type ChargeAttributionRegister interface {
	SaveChargeAttribution(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.ChargeAttributionRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// ChargeAttributionView 按费用项目取归属日形态。found=false 表示没有这一行：不形成归属日。
type ChargeAttributionView interface {
	LoadChargeAttribution(
		ctx context.Context,
		tenant domain.TenantID,
		feeItem domain.FeeItemReference,
	) (domain.ChargeAttributionRegistration, bool, error)
}

// StatementArrangementRegister 给一个结算账户的对账安排追加一版。与最新一版同内容答重放，否则落下一版；
// 账户不在账户册上答 domain.ErrCatalogueTargetMissing。不答冲突：换时区或截单时刻正是追加新一版的用途
// （ADR-0269 决定一）。
type StatementArrangementRegister interface {
	SaveStatementArrangement(
		ctx context.Context,
		tenant domain.TenantID,
		arrangement domain.StatementArrangement,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// StatementArrangementView 取一个结算账户对账安排的最新一版。found=false 表示这一户没登：不形成归属日，
// 不拿服务器时区顶上。
type StatementArrangementView interface {
	LoadStatementArrangement(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccountID,
	) (domain.StatementArrangement, bool, error)
}

// ChargeAttributionDateRecorder 把截单时当场判出的归属日落到已确认费用上。只在这件费用还没有归属日时写；
// 已有的不改，交回册上那一天（ADR-0269 决定三：判出即落库，此后不变）。
type ChargeAttributionDateRecorder interface {
	RecordAttributionDate(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerChargeID,
		date domain.AttributionDate,
	) (domain.AttributionDate, error)
}

// ChargeBillingLock 锁住一件费用的出账判断到本事务结束，并答它是否已在一张未作废的已发布对账单上。
// 锁与对账单的写在同一事务里：两笔截单并发列了同一件费用时，后到者等先到者提交后再读，读到的是已出账，
// 不会两张单各收一次（UC-SA-003「同一费用被两个账户或周期同时纳入」）。作废单上的费用不算已出账：
// 整单作废后替代单要重新收它们。
type ChargeBillingLock interface {
	LockChargeBilling(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerChargeID,
	) (domain.StatementNumber, bool, error)
}
