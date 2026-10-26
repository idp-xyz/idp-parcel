package domain

import (
	"errors"
	"sort"
)

// ErrInvalidClaimTargetStanding 说明读回的金额本体与已核销净额立不成：金额非正、已核销为负，或已核销超过金额本体。
// 那是账不平，不是业务答案——核销门从不放超额的分配落下。
var ErrInvalidClaimTargetStanding = errors.New("settlement accounting: invalid claim target standing")

// ClaimTargetKey 指名核销指向的一条金额本体：哪一格、哪一条。
type ClaimTargetKey struct {
	Kind   SettlementTargetKind
	Target SettlementTargetReference
}

// ClaimTargetDemand 是一次核销分给某一条金额本体的合计。
type ClaimTargetDemand struct {
	Key         ClaimTargetKey
	AmountMinor int64
}

// ClaimTargetDemands 把指向金额本体两格的分配按目标合计。按（格、引用）排序交回，核销编排按这个顺序逐条锁：
// 两笔并发核销各锁一半再等另一半就成了环，同一个顺序取锁才不会。金额本体两格只有借向（claimAllocationDirected），
// 合计就是本次要从那条金额上核掉的数。
func (application SettlementApplication) ClaimTargetDemands() []ClaimTargetDemand {
	totals := map[ClaimTargetKey]int64{}
	for _, allocation := range application.allocations {
		if !allocation.TargetKind.PointsAtClaimAmount() {
			continue
		}
		totals[ClaimTargetKey{Kind: allocation.TargetKind, Target: allocation.Target}] += allocation.AmountMinor
	}
	demands := make([]ClaimTargetDemand, 0, len(totals))
	for key, amount := range totals {
		demands = append(demands, ClaimTargetDemand{Key: key, AmountMinor: amount})
	}
	sort.Slice(demands, func(i, j int) bool {
		if demands[i].Key.Kind != demands[j].Key.Kind {
			return demands[i].Key.Kind < demands[j].Key.Kind
		}
		return demands[i].Key.Target.String() < demands[j].Key.Target.String()
	})
	return demands
}

// ClaimTargetStanding 是一条被指向的金额本体此刻的未核销面：金额本体，与指向它、尚未撤销的核销分配净额。
// 未核销按目标各自算，不跨目标、不跨同一索赔项或同一应追偿下的别的条目合并——每条有自己的金额与身份，
// 核销只改它的未结关系，不改它的金额（CONTEXT「客户赔付与外部追偿」）。
type ClaimTargetStanding struct {
	currency     CurrencyCode
	amountMinor  int64
	appliedMinor int64
}

func NewClaimTargetStanding(currency CurrencyCode, amountMinor, appliedMinor int64) (ClaimTargetStanding, error) {
	if !currency.valid() || amountMinor <= 0 || appliedMinor < 0 || appliedMinor > amountMinor {
		return ClaimTargetStanding{}, ErrInvalidClaimTargetStanding
	}
	return ClaimTargetStanding{currency: currency, amountMinor: amountMinor, appliedMinor: appliedMinor}, nil
}

func (standing ClaimTargetStanding) Currency() CurrencyCode {
	return standing.currency
}

func (standing ClaimTargetStanding) AmountMinor() int64 {
	return standing.amountMinor
}

func (standing ClaimTargetStanding) AppliedMinor() int64 {
	return standing.appliedMinor
}

func (standing ClaimTargetStanding) RemainingMinor() int64 {
	return standing.amountMinor - standing.appliedMinor
}

// Admit 判断本次分给这条金额的合计能不能落：付款币种与金额本体不同停在待判断，不取当前汇率（AT-SA-110）；
// 超出未核销即失衡——只核销合格范围，超额保持未分配（AT-SA-109），不吞进金额本体，也不截成恰好核完。
func (standing ClaimTargetStanding) Admit(factCurrency CurrencyCode, demandMinor int64) error {
	if factCurrency != standing.currency {
		return ErrCrossCurrencyApplication
	}
	if demandMinor > standing.RemainingMinor() {
		return ErrApplicationImbalance
	}
	return nil
}
