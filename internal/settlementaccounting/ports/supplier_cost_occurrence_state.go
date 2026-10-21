package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 一项发生项在结算这一侧的成本状态（ADR-0232 决定七、八）：它在各费用项目上的版本链头，与结算记下的它已失效的那几版。形成首版、
// 按替代版本的新评价纠错、按失效版本撤下，三件事都先读它、在同一把发生项锁下读。另立文件、不并进 ports.go。

// ErrSeveralCostChainsOnOneFeeItem 说明同一发生项、同一费用项目上有不止一条版本链。首版按（发生项、费用项目、规则版本）唯一，同一份
// 发生项版本只有一份请求，所以合格的册上一个费用项目只有一条链；多于一条是库被改过或另有一条路形成了首版，不挑一条去纠正。
var ErrSeveralCostChainsOnOneFeeItem = errors.New("settlement accounting: several supplier cost chains on one fee item")

// OccurrenceCostState 是读口交回的样子。Heads 是这一项发生项各条版本链的链头（没有别的版本回指它的那一版），Invalidated 是结算记下
// 的它已失效的那几版。
type OccurrenceCostState struct {
	Heads       []domain.SupplierExpectedCost
	Invalidated []domain.OccurrenceVersion
}

// HeadFor 交这个费用项目上那条链的链头；没有链答 found=false。
func (state OccurrenceCostState) HeadFor(feeItem domain.FeeItemReference) (domain.SupplierExpectedCost, bool, error) {
	var head domain.SupplierExpectedCost
	found := false
	for _, candidate := range state.Heads {
		if candidate.FeeItem() != feeItem {
			continue
		}
		if found {
			return domain.SupplierExpectedCost{}, false, ErrSeveralCostChainsOnOneFeeItem
		}
		head, found = candidate, true
	}
	return head, found, nil
}

// Invalidates 说这一版发生项是不是已失效。
func (state OccurrenceCostState) Invalidates(version domain.OccurrenceVersion) bool {
	for _, invalidated := range state.Invalidated {
		if invalidated == version {
			return true
		}
	}
	return false
}

// SupplierCostOccurrenceInvalidations 是结算记「发生项这一版已失效」的那一本：失效信封到了就记，之后到的这一版的评价据它不再形成成本。
// 同一（租户、发生项、失效的那一版）再记是重放，不报错。
type SupplierCostOccurrenceInvalidations interface {
	RecordOccurrenceInvalidation(
		ctx context.Context,
		tenant domain.TenantID,
		occurrence domain.ChargeOccurrenceID,
		invalidated domain.OccurrenceVersion,
		invalidating domain.OccurrenceVersion,
		recordedAt time.Time,
	) error
}
