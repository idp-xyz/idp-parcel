// Package ports 定义 transport-fulfillment 应用层与外界的边界。领域包不依赖 HTTP/pgx
// 的纪律与其余上下文一致。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

type Clock interface {
	Now() time.Time
}

// PickupAttemptKey 是一次尝试来源提交的幂等键：同一来源身份和内容返回已有处理结果
// （UC-TF-002「同一尝试来源身份和内容返回已有结果；同一身份不同内容形成冲突」）。
type PickupAttemptKey struct {
	TenantID domain.TenantID
	SourceID string
}

// PickupAttemptRecord 是一次尝试提交越过提交边界留下的东西：尝试本体、逐对象结果与
// 成功对象的场外揽收。失败与拒收对象只出现在 Results 里——Pickups 只收真正取得控制的
// 对象，「失败结果不制造实际履约段」在记录形状上就成立。
type PickupAttemptRecord struct {
	Key           PickupAttemptKey
	ContentDigest string
	Attempt       domain.FulfillmentAttempt
	Results       []domain.AttemptObjectResult
	Pickups       []domain.OffsitePickup
	RecordedAt    time.Time
}

type PickupSaveOutcome uint8

const (
	PickupSaveOutcomeInvalid PickupSaveOutcome = iota
	PickupSaved
	PickupAlreadyRecorded
)

// PickupAttemptStore 按幂等键找回并保存尝试提交（写入代数同 ADR-0031：并发落败读回
// 赢家，不覆盖）。
type PickupAttemptStore interface {
	FindByKey(ctx context.Context, key PickupAttemptKey) (PickupAttemptRecord, bool, error)
	Save(ctx context.Context, record PickupAttemptRecord) (PickupSaveOutcome, error)
}

// PickupIdentityFactory 签发揽收结果版本。逐成功对象签发：来源更正形成新版本不覆盖
// 本版，parcel-shipment 的采用判断按版本幂等。
type PickupIdentityFactory interface {
	NextPickupResultVersion(ctx context.Context) (domain.PickupResultVersion, error)
}

// OffsitePickupHandoffIntent 把已提交的对象级揽收交给 parcel-shipment 判断有效网络
// 收寄（UC-TF-002 步骤 6A）。意图由幂等键认领，重放重发同一份（ADR-0043 同款纪律）；
// 没有成功对象的记录没有可交的东西，不产生意图。
type OffsitePickupHandoffIntent struct {
	Record PickupAttemptRecord
}

// OffsitePickupHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017
// 的 Bento/Outbox 闸门。
type OffsitePickupHandoff interface {
	HandOffOffsitePickup(ctx context.Context, intent OffsitePickupHandoffIntent) error
}

// DeliveryAttemptView 按（尝试、对象）取回派送尝试与该对象的结果供交付生效引用。
// found=false 表示指名的尝试/对象结果不存在——指错是提交矛盾，不是等谁。
type DeliveryAttemptView interface {
	LoadDeliveryResult(
		ctx context.Context,
		tenant domain.TenantID,
		attempt domain.AttemptReference,
		object domain.CarriedObjectReference,
	) (domain.FulfillmentAttempt, domain.DeliveryAttemptResult, bool, error)
}

// EffectiveDeliveryKey 是交付生效的幂等键：同一（租户+对象+尝试）的生效交付只登
// 一次，重放返回原版本；POD 更正走更正入口换版本，不在首登处顶替。
type EffectiveDeliveryKey struct {
	TenantID domain.TenantID
	Object   domain.CarriedObjectReference
	Attempt  domain.AttemptReference
}

// EffectiveDeliveryRecord 是一次交付生效越过提交边界留下的东西。更正后记录携带新
// 版本，版本链在 EffectiveDelivery 本体上（Corrects 回指前版）。
type EffectiveDeliveryRecord struct {
	Key           EffectiveDeliveryKey
	ContentDigest string
	Delivery      domain.EffectiveDelivery
	RecordedAt    time.Time
}

type DeliverySaveOutcome uint8

const (
	DeliverySaveOutcomeInvalid DeliverySaveOutcome = iota
	DeliverySaved
	DeliveryAlreadyRegistered
)

// EffectiveDeliveryStore 按幂等键找回并保存交付生效（写入代数同 ADR-0031）。
// Supersede 只在已有登记上落更正版本：found=false 表示没有可更正的登记。
type EffectiveDeliveryStore interface {
	FindByKey(ctx context.Context, key EffectiveDeliveryKey) (EffectiveDeliveryRecord, bool, error)
	Save(ctx context.Context, record EffectiveDeliveryRecord) (DeliverySaveOutcome, error)
	Supersede(ctx context.Context, record EffectiveDeliveryRecord) (bool, error)
}

// DeliveryIdentityFactory 签发交付结果版本。首登与更正各签新版：更正版本回指前版，
// 原版本不删。
type DeliveryIdentityFactory interface {
	NextDeliveryResultVersion(ctx context.Context) (domain.DeliveryResultVersion, error)
}

// EffectiveDeliveryHandoffIntent 把交付生效交给 parcel-shipment 终局判断消费（既有
// DeliveryOutcomeAdapter 的上游）。意图由幂等键认领，重放重发同一份（ADR-0043）。
type EffectiveDeliveryHandoffIntent struct {
	Record EffectiveDeliveryRecord
}

// EffectiveDeliveryHandoff 今天没有实现，唯一实现是测试替身。
type EffectiveDeliveryHandoff interface {
	HandOffEffectiveDelivery(ctx context.Context, intent EffectiveDeliveryHandoffIntent) error
}
