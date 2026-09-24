package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 派送尝试登记册的存取口（票 product-strategy-boundary/19）。交付生效只读它（DeliveryAttemptView），
// 执行侧经这里登记——两个口分开，理由在 DeliveryAttempts 适配器的头注：一个入口同时造尝试和造交付，
// 就没有东西拦得住「先声称到过场再声称交付成功」。

// DeliveryAttemptKey 是派送尝试的幂等键。
//
// 与揽收侧按来源身份作键不同：派送尝试表以尝试身份为主键，尝试身份就是执行方报来的那一次到场。
// 每次到场是新尝试（UC-TF-006），改约或重派换新身份、回指旧尝试，旧尝试原样留着。
type DeliveryAttemptKey struct {
	TenantID domain.TenantID
	Attempt  domain.AttemptReference
}

// DeliveryAttemptRecord 是一次派送尝试越过提交边界留下的东西：尝试本体与逐对象结果。
// 结果逐对象成立，任务汇总只能由它们派生（UC-TF-006），记录上因此没有任何整次到场的成败格。
type DeliveryAttemptRecord struct {
	Key        DeliveryAttemptKey
	Attempt    domain.FulfillmentAttempt
	Results    []domain.DeliveryAttemptResult
	RecordedAt time.Time
}

type DeliveryAttemptSaveOutcome uint8

const (
	DeliveryAttemptSaveOutcomeInvalid DeliveryAttemptSaveOutcome = iota
	DeliveryAttemptSaved
	DeliveryAttemptAlreadyRecorded
)

// DeliveryAttemptStore 按幂等键找回并保存派送尝试（写入代数同 ADR-0031：撞键是业务答案不是错误，
// 并发落败读回赢家，不覆盖）。
//
// 只有首登与读回，没有 Update：一次到场一旦登记就不改，再到场是新尝试。
type DeliveryAttemptStore interface {
	FindByKey(ctx context.Context, key DeliveryAttemptKey) (DeliveryAttemptRecord, bool, error)
	Save(ctx context.Context, record DeliveryAttemptRecord) (DeliveryAttemptSaveOutcome, error)
}

// DispatchTaskReader 按键取回揽派任务，供登记派送尝试时核对任务已开、且覆盖所报对象。
//
// 单开一个只读口而不直接依赖 DispatchTaskRegistry，理由同 FailedAttemptSource：那个口带 Save，
// 而登记派送尝试只读任务，依赖它等于声明自己可能改写工作范围。DispatchTasks 适配器天然满足它。
type DispatchTaskReader interface {
	FindByKey(ctx context.Context, key DispatchTaskKey) (DispatchTaskRecord, bool, error)
}
