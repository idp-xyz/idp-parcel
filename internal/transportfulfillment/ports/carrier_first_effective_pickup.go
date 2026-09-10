package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 实际承运商首次有效收寄的登记册、身份签发与出向交接（label-channel/31，ADR-0135）。
//
// 另开一个文件而不是并进 ports.go：收寄不是外部承运轨迹事实上的一格，也不是场外揽收改名——它是第三种控制事实，
// 端口挂在自己的登记册上，与 `OffsitePickupRegistry` / `EffectiveDeliveryStore` 并列。

// CarrierFirstEffectivePickupKey 是链上一版的幂等键。版本在键里：首登、替代、失效、待确认各签新版本，原版本
// 不被改写；parcel-shipment 按信封所指的这三维取回（ADR-0135 决定七）。
type CarrierFirstEffectivePickupKey struct {
	TenantID domain.TenantID
	Fact     domain.CarrierFirstEffectivePickupReference
	Version  domain.CarrierFirstEffectivePickupVersion
}

// CarrierFirstEffectivePickupRecord 是一版越过提交边界留下的东西。RecordedAt 是落库时刻，与事实上的业务发生
// 时间、判断形成时间都不合并——三者三种归属（ADR-0135 决定三）。
type CarrierFirstEffectivePickupRecord struct {
	Key        CarrierFirstEffectivePickupKey
	Pickup     domain.CarrierFirstEffectivePickup
	RecordedAt time.Time
}

// CarrierPickupSaveOutcome 是追加一版的结果。撞键是业务答案不是错误（ADR-0031）：同一版本重放、同一对象并发
// 首登第二条链、或另一方先把同一前版回指掉，都答`已登记`，编排读回再答。
type CarrierPickupSaveOutcome uint8

const (
	CarrierPickupSaveOutcomeInvalid CarrierPickupSaveOutcome = iota
	CarrierPickupSaved
	CarrierPickupAlreadyRegistered
)

func (outcome CarrierPickupSaveOutcome) String() string {
	switch outcome {
	case CarrierPickupSaved:
		return "SAVED"
	case CarrierPickupAlreadyRegistered:
		return "ALREADY_REGISTERED"
	default:
		return ""
	}
}

// CarrierFirstEffectivePickupRegistry 按键找回并追加收寄版本。**只插不改**：没有一个口能改写既有版本。
//
// FindByKey 交回指名的那一代，不问它是不是当前版（`EffectiveDeliveryStore.FindByKeyAndVersion` 的形）——
// 消费方每份信封代表一代，按当前版读会让更正之前入队的那一份也读成更正后那一代（lc/24 教训）。
//
// FindCurrentByObject 交回该对象那条链的链尾（未被任何版本回指的那一版）；一个对象至多一条链（CONTEXT Rules），
// 所以不需要再按事实身份问。ListByObject 交回整条链按判断形成时间升序，供查阅面与更正定位。
//
// Save 追加一版：首登（不回指）撞「一对象一链」答已登记；替代 / 失效 / 待确认（回指前版）撞「一版只被回指一次」
// 也答已登记——两格的续办都是读回链尾再来。写口按框架合同要求环境事务：版本登记与意图入队同笔落地。
type CarrierFirstEffectivePickupRegistry interface {
	FindByKey(ctx context.Context, key CarrierFirstEffectivePickupKey) (CarrierFirstEffectivePickupRecord, bool, error)
	FindCurrentByObject(
		ctx context.Context,
		tenant domain.TenantID,
		object domain.CarriedObjectReference,
	) (CarrierFirstEffectivePickupRecord, bool, error)
	ListByObject(
		ctx context.Context,
		tenant domain.TenantID,
		object domain.CarriedObjectReference,
	) ([]CarrierFirstEffectivePickupRecord, error)
	Save(ctx context.Context, record CarrierFirstEffectivePickupRecord) (CarrierPickupSaveOutcome, error)
}

// CarrierPickupIdentityFactory 签发本上下文自己的收寄事实身份与版本（ADR-0135 决定二：事实身份与载运对象
// 分开保存，同 ADR-0102 决定四）。
type CarrierPickupIdentityFactory interface {
	NextCarrierFirstEffectivePickupReference(ctx context.Context) (domain.CarrierFirstEffectivePickupReference, error)
	NextCarrierFirstEffectivePickupVersion(ctx context.Context) (domain.CarrierFirstEffectivePickupVersion, error)
}

// CarrierFirstEffectivePickupHandoffIntent 把一版**已形成、替代或失效**的收寄交给 parcel-shipment 面单渠道服务
// 的终局判断消费（lc/25）。待确认版本不构成收寄、不提供，没有可交的东西，不产生意图。
type CarrierFirstEffectivePickupHandoffIntent struct {
	Record CarrierFirstEffectivePickupRecord
}

// CarrierFirstEffectivePickupHandoff 由 OutboxCarrierFirstEffectivePickupHandoff 实现：意图与版本登记同一事务
// 入队；对待确认版本响亮拒绝（ADR-0135 决定七）。
type CarrierFirstEffectivePickupHandoff interface {
	HandOffCarrierFirstEffectivePickup(ctx context.Context, intent CarrierFirstEffectivePickupHandoffIntent) error
}
