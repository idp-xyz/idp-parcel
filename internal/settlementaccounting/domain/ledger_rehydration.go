package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidRehydratedLedger 是账本重建入口因快照数据本身而拒绝时给出的理由。与
// ErrInvalidFreezeRequest 分开：后者说「此刻要发生的这件事不合规则」，前者说「这份
// 已经发生过的东西不可能是本账本记下来的」——处置是去查库里那一行或写它的适配器
// （与 parcel-shipment 的重建哨兵同一条分格纪律）。
var ErrInvalidRehydratedLedger = errors.New("settlement accounting: invalid rehydrated ledger entry")

// Scope 交回冻结的结算作用域（持久化摊行要用；账本内部比对用的仍是未导出字段）。
func (freeze FundsFreeze) Scope() SettlementScope {
	return freeze.scope
}

// Association 交回冻结的原业务关联。
func (freeze FundsFreeze) Association() BusinessAssociationReference {
	return freeze.association
}

// Scope 交回暴露的结算作用域。
func (exposure CreditExposure) Scope() SettlementScope {
	return exposure.scope
}

// Association 交回暴露的原业务关联。
func (exposure CreditExposure) Association() BusinessAssociationReference {
	return exposure.association
}

// Entries 按内部编号序交回账本里的全部冻结。`业务限制`从不入账本，所以这里只有
// HELD/RELEASED 两态。
func (ledger *FreezeLedger) Entries() []FundsFreeze {
	entries := make([]FundsFreeze, 0, len(ledger.byFreeze))
	for _, freeze := range ledger.byFreeze {
		entries = append(entries, freeze)
	}
	sortByID(entries, func(freeze FundsFreeze) string { return freeze.freezeID.String() })
	return entries
}

// DigestFor 交回账本为一次控制请求记下的幂等指纹。持久化必须整份带走它：丢了指纹，
// 重建后的账本对「同身份异内容」的冲突判定就退化成全部放行。
func (ledger *FreezeLedger) DigestFor(requestID ControlRequestID) (string, bool) {
	digest, found := ledger.digests[requestID]
	return digest, found
}

// Entries 按内部编号序交回账本里的全部暴露。
func (ledger *CreditExposureLedger) Entries() []CreditExposure {
	entries := make([]CreditExposure, 0, len(ledger.byExposure))
	for _, exposure := range ledger.byExposure {
		entries = append(entries, exposure)
	}
	sortByID(entries, func(exposure CreditExposure) string { return exposure.exposureID.String() })
	return entries
}

// DigestFor 交回账本为一次控制请求记下的幂等指纹。
func (ledger *CreditExposureLedger) DigestFor(requestID ControlRequestID) (string, bool) {
	digest, found := ledger.digests[requestID]
	return digest, found
}

func sortByID[T any](entries []T, id func(T) string) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && id(entries[j]) < id(entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// RehydrateFundsFreezeSpec 是一条冻结在库里的样子。Digest 是账本当初记下的幂等指纹，
// 原样带回不重算——重算等于让适配器替账本决定「什么算同一份内容」。
type RehydrateFundsFreezeSpec struct {
	FreezeID    string
	RequestID   ControlRequestID
	Scope       SettlementScope
	AmountMinor int64
	Association BusinessAssociationReference
	Status      FreezeStatus
	FrozenAt    time.Time
	ReleasedAt  time.Time
	Digest      string
}

// RehydrateFreezeLedger 从库里读到的产物重建冻结账本。
//
// 它只校验，不重算（与 RehydrateShipmentRequest 同一条 ADR-0028 纪律）：`业务限制`
// 从不入账本所以 RESTRICTED 拒；HELD 不带释放时间、RELEASED 必带且不早于冻结；编号
// 必须是本账本签发的形状（FRZ-数字），否则续编号无从恢复，下一笔冻结会与历史重号。
func RehydrateFreezeLedger(specs []RehydrateFundsFreezeSpec) (*FreezeLedger, error) {
	ledger := NewFreezeLedger()
	for _, spec := range specs {
		sequence, err := ledgerSequence(spec.FreezeID, "FRZ-")
		if err != nil {
			return nil, err
		}
		if err := validRehydratedEntry(
			spec.RequestID, spec.Scope, spec.AmountMinor, spec.Association,
			spec.FrozenAt, spec.Digest,
		); err != nil {
			return nil, err
		}
		switch spec.Status {
		case FreezeHeld:
			if !spec.ReleasedAt.IsZero() {
				return nil, rehydratedLedgerRefusal("HELD 的冻结带着释放时间")
			}
		case FreezeReleased:
			if spec.ReleasedAt.IsZero() || spec.ReleasedAt.Before(spec.FrozenAt) {
				return nil, rehydratedLedgerRefusal("RELEASED 的冻结缺释放时间或早于冻结")
			}
		default:
			return nil, rehydratedLedgerRefusal(fmt.Sprintf("冻结状态不入账本：%d", uint8(spec.Status)))
		}

		freezeID := FreezeID{requiredValue{value: spec.FreezeID}}
		if _, duplicated := ledger.byFreeze[freezeID]; duplicated {
			return nil, rehydratedLedgerRefusal("冻结编号重号")
		}
		if _, duplicated := ledger.byRequest[spec.RequestID]; duplicated {
			return nil, rehydratedLedgerRefusal("同一控制请求挂着两条冻结")
		}
		ledger.byFreeze[freezeID] = FundsFreeze{
			freezeID:    freezeID,
			requestID:   spec.RequestID,
			scope:       spec.Scope,
			amountMinor: spec.AmountMinor,
			association: spec.Association,
			status:      spec.Status,
			frozenAt:    spec.FrozenAt.UTC(),
			releasedAt:  spec.ReleasedAt.UTC(),
		}
		ledger.byRequest[spec.RequestID] = freezeID
		ledger.digests[spec.RequestID] = spec.Digest
		if sequence > ledger.nextID {
			ledger.nextID = sequence
		}
	}
	return ledger, nil
}

// RehydrateCreditExposureSpec 是一条暴露在库里的样子。Policy 是形成时据以判额度的信用政策版本，
// 零值只可能是政策引用列落地之前入库的存量行——重建门不替它补一个出处，也不因它为空而拒：
// 那一行当初确实是在没有政策出处的状况下判的，读回如实。
type RehydrateCreditExposureSpec struct {
	ExposureID  string
	RequestID   ControlRequestID
	Scope       SettlementScope
	AmountMinor int64
	Association BusinessAssociationReference
	Status      ExposureStatus
	ExposedAt   time.Time
	ReleasedAt  time.Time
	Digest      string
	Policy      CreditPolicyReference
}

// RehydrateCreditExposureLedger 从库里读到的产物重建信用暴露账本。代数与冻结账本
// 一致——但它是另一本账，两个重建入口刻意不合并（ADR-0047 两轨在重建面同样成立）。
func RehydrateCreditExposureLedger(specs []RehydrateCreditExposureSpec) (*CreditExposureLedger, error) {
	ledger := NewCreditExposureLedger()
	for _, spec := range specs {
		sequence, err := ledgerSequence(spec.ExposureID, "EXP-")
		if err != nil {
			return nil, err
		}
		if err := validRehydratedEntry(
			spec.RequestID, spec.Scope, spec.AmountMinor, spec.Association,
			spec.ExposedAt, spec.Digest,
		); err != nil {
			return nil, err
		}
		switch spec.Status {
		case ExposureRecorded:
			if !spec.ReleasedAt.IsZero() {
				return nil, rehydratedLedgerRefusal("RECORDED 的暴露带着释放时间")
			}
		case ExposureReleased:
			if spec.ReleasedAt.IsZero() || spec.ReleasedAt.Before(spec.ExposedAt) {
				return nil, rehydratedLedgerRefusal("RELEASED 的暴露缺释放时间或早于暴露")
			}
		default:
			return nil, rehydratedLedgerRefusal(fmt.Sprintf("暴露状态不入账本：%d", uint8(spec.Status)))
		}

		exposureID := ExposureID{requiredValue{value: spec.ExposureID}}
		if _, duplicated := ledger.byExposure[exposureID]; duplicated {
			return nil, rehydratedLedgerRefusal("暴露编号重号")
		}
		if _, duplicated := ledger.byRequest[spec.RequestID]; duplicated {
			return nil, rehydratedLedgerRefusal("同一控制请求挂着两条暴露")
		}
		ledger.byExposure[exposureID] = CreditExposure{
			exposureID:  exposureID,
			requestID:   spec.RequestID,
			scope:       spec.Scope,
			amountMinor: spec.AmountMinor,
			association: spec.Association,
			status:      spec.Status,
			exposedAt:   spec.ExposedAt.UTC(),
			releasedAt:  spec.ReleasedAt.UTC(),
			policy:      spec.Policy,
		}
		ledger.byRequest[spec.RequestID] = exposureID
		ledger.digests[spec.RequestID] = spec.Digest
		if sequence > ledger.nextID {
			ledger.nextID = sequence
		}
	}
	return ledger, nil
}

func validRehydratedEntry(
	requestID ControlRequestID,
	scope SettlementScope,
	amountMinor int64,
	association BusinessAssociationReference,
	occurredAt time.Time,
	digest string,
) error {
	if !requestID.valid() || !scope.valid() || !association.valid() {
		return rehydratedLedgerRefusal("控制请求身份、作用域或业务关联缺失")
	}
	if amountMinor <= 0 {
		return rehydratedLedgerRefusal("金额不是正数——零金额控制从不入账本")
	}
	if occurredAt.IsZero() {
		return rehydratedLedgerRefusal("发生时间缺失")
	}
	if strings.TrimSpace(digest) == "" {
		return rehydratedLedgerRefusal("幂等指纹缺失——丢了它，同身份异内容的冲突判定退化成全部放行")
	}
	return nil
}

// ledgerSequence 解析本账本签发的内部编号，恢复续编号。编号不是本账本的形状即拒：
// 续不上号，下一笔会与历史重号。
func ledgerSequence(id string, prefix string) (int, error) {
	raw, found := strings.CutPrefix(id, prefix)
	if !found {
		return 0, rehydratedLedgerRefusal(fmt.Sprintf("内部编号不是本账本签发的形状：%q", id))
	}
	sequence, err := strconv.Atoi(raw)
	if err != nil || sequence <= 0 {
		return 0, rehydratedLedgerRefusal(fmt.Sprintf("内部编号不是本账本签发的形状：%q", id))
	}
	return sequence, nil
}

func rehydratedLedgerRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedLedger, reason)
}
