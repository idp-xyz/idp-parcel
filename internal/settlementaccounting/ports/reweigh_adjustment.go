package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ChargeSupersessionStore 记下后一代预估替代了前一代仍未确认的费用。确认口据此不再确认被替代的那一笔。
type ChargeSupersessionStore interface {
	SaveSupersession(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerChargeID,
		supersededBy domain.CustomerChargeID,
		at time.Time,
	) error
	Superseded(ctx context.Context, tenant domain.TenantID, charge domain.CustomerChargeID) (bool, error)
}

// ChargeStageLock 锁住一笔客户费用行到本事务结束，让同一笔费用上的「确认」与「记替代」排成一列：后到的那一路在这把锁
// 上等先到的提交，锁放开之后另起一句读阶段与替代关系，读到的就是先到者已提交的样子。确认编排（经它的各条入口）与复磅
// 形成那一头判前一笔费用的阶段之前，都先取它。
//
// 锁的是费用行而不是包裹：手工确认入口手里只有费用标识，不经包裹；各入口与复磅共同碰到的只有这一行。不用条件写代替：
// 读已提交下条件写里的子查询看的是语句快照，行被改写后的重验只看这一行，看不见对方刚提交的替代行或确认。
//
// 取锁的先后是：包裹确认锁（ParcelConfirmationSerializer，有包裹可锁的入口才取）→ 费用行锁 → 控制作用域锁 → 运营余额
// 作用域锁。手工确认没有包裹锁，从费用行锁起取，与别的入口的相对次序不变。锁模式是 FOR NO KEY UPDATE 而不是 FOR UPDATE：
// 记调整、记替代、登确认依据对费用行的外键检查取的是 KEY SHARE，FOR UPDATE 会挡住它们；而记调整那一路是先取余额作用域
// 锁、再插带外键的调整行，确认这一路是先取费用行锁、再取余额作用域锁，用 FOR UPDATE 两路就互等成环。NO KEY UPDATE 仍与
// 自己、FOR UPDATE 互斥，截单出账锁（ChargeBillingLock）取的 FOR UPDATE 与它照样排队。
//
// 必须在事务里：锁到提交才放，离开事务就只是一次读（bentopg.ErrTransactionRequired）。费用行不存在不算错——随后的读
// 会答不存在。
type ChargeStageLock interface {
	LockChargeStage(ctx context.Context, tenant domain.TenantID, charge domain.CustomerChargeID) error
}

// ReweighAdjustmentAdoptionView 问这一结算账户有没有采用「实测差异自动调整」。没有就是没采用。
type ReweighAdjustmentAdoptionView interface {
	ReweighAdjustmentAdopted(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccountID,
	) (bool, error)
}

// ReweighAdjustmentAdoptionRegister 写下这一户的采用。同一户再登是重放，不改已经写下的依据。
type ReweighAdjustmentAdoptionRegister interface {
	SaveReweighAdjustmentAdoption(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccountID,
		basis string,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}
