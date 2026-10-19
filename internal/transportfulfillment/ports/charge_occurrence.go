package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 运输收费发生项登记册的存取口（tf-unwired-seven/04，ADR-0098）。另开一份文件不并进
// `ports.go`，同 `fulfillment_segment.go` 的理由。

// ChargeOccurrenceKey 是发生项某一有效性版本的幂等键。
//
// 版本进键而不是另存一列：更正换版本、原版本留在册上（CONTEXT「保留原发生项……不删除原
// 成本」），两代因此在主键层面就共存，不需要任何「哪个是当前」的标记——那种标记会与
// corrects_version 形成两个都能回答同一问题的口径。
type ChargeOccurrenceKey struct {
	TenantID   domain.TenantID
	Occurrence domain.ChargeOccurrenceReference
	Validity   domain.OccurrenceValidityVersion
}

// ChargeOccurrenceRecord 是一个发生项版本连同其对象范围越过提交边界留下的东西。
type ChargeOccurrenceRecord struct {
	Key        ChargeOccurrenceKey
	Occurrence domain.TransportChargeOccurrence
	RecordedAt time.Time
}

type ChargeOccurrenceSaveOutcome uint8

const (
	ChargeOccurrenceSaveOutcomeInvalid ChargeOccurrenceSaveOutcome = iota
	ChargeOccurrenceSaved
	ChargeOccurrenceAlreadyRegistered
)

// FailedAttemptSource 按尝试来源键取回一次揽收尝试及其逐对象结果，供失败尝试费登记取用。
//
// 单开一个只读口而不直接依赖 `PickupAttemptStore`：那个口带 `Save`，而本用例只读——依赖它
// 等于声明自己可能写揽收尝试，而按 ADR-0098 本用例恰恰**不碰**揽收那一侧的任何东西。契约
// 窄一格，说的话就准一格。`PickupAttempts` 适配器天然满足它，不需要第二个实现。
type FailedAttemptSource interface {
	FindByKey(ctx context.Context, key PickupAttemptKey) (PickupAttemptRecord, bool, error)
}

// ChargeOccurrenceRegistry 按幂等键找回并保存发生项版本（写入代数同 ADR-0031）。
//
// **只有首登与读回，没有 Update**，理由同 ADR-0097：发生项一旦形成就不改，演进走的是
// 新有效性版本新行。这里连"窄写口"都不需要——修订本身就是一次新的首登。
type ChargeOccurrenceRegistry interface {
	FindByKey(ctx context.Context, key ChargeOccurrenceKey) (ChargeOccurrenceRecord, bool, error)
	Save(ctx context.Context, record ChargeOccurrenceRecord) (ChargeOccurrenceSaveOutcome, error)
}

// ChargeOccurrenceIntent 把一个已登记的发生项版本交给消费方（ADR-0207 决定五）。意图由发生项键认领：重放重发同一份，
// 由入队那一步按信封 ID 吞掉（ADR-0043）。
type ChargeOccurrenceIntent struct {
	Record ChargeOccurrenceRecord
}

// ChargeOccurrenceHandoff 由 OutboxChargeOccurrenceHandoff 实现：意图与发生项同一事务入队。写口按框架合同无事务即拒，
// 事务边界归装配点；两者分两笔，就会有一份发生项落了册而结算永远等不到它。
type ChargeOccurrenceHandoff interface {
	HandOffChargeOccurrence(ctx context.Context, intent ChargeOccurrenceIntent) error
}

// FulfillmentSegmentView 按键取回一个实际履约段连同全部参与，供实际履约发生项登记核成员的参与是否已结束。
//
// 单开只读口而不依赖 ActualFulfillmentSegmentRegistry，理由同 FailedAttemptSource：那个口带进段、离场、关段各写口，
// 登记发生项只读段——依赖它等于声明自己可能改写履约事实，而 ADR-0098 决定一恰恰要发生项不碰物理编排。
// FulfillmentSegments 适配器天然满足它。
type FulfillmentSegmentView interface {
	FindByKey(ctx context.Context, key FulfillmentSegmentKey) (FulfillmentSegmentRecord, bool, error)
}

// ActualCarrierJudgmentView 按段取回实际承运商判断的全部版本，供登记判「这一段是不是自营」（ADR-0207 决定二）。
// 只读，理由同上；ActualCarrierJudgments 适配器天然满足它。
type ActualCarrierJudgmentView interface {
	FindByKey(ctx context.Context, key ActualCarrierJudgmentKey) (ActualCarrierJudgmentRecord, bool, error)
}
