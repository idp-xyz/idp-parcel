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
