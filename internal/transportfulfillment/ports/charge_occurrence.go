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
