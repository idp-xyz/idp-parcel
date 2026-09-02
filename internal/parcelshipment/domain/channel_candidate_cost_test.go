package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func unpriceableCost(t *testing.T, candidate string, grade domain.ChannelCostUnavailability) domain.ChannelCandidateCost {
	t.Helper()

	cost, err := domain.UnpriceableChannelCandidate(mustValue(t, domain.NewChannelCandidateID, candidate), grade)
	if err != nil {
		t.Fatalf("造不可计价候选 %s：%v", candidate, err)
	}
	return cost
}

func pricedCost(t *testing.T, candidate, amount string) domain.ChannelCandidateCost {
	t.Helper()

	return pricedCostIn(t, candidate, amount, "SYN")
}

func pricedCostIn(t *testing.T, candidate, amount, currency string) domain.ChannelCandidateCost {
	t.Helper()

	cost, err := domain.PricedChannelCandidate(
		mustValue(t, domain.NewChannelCandidateID, candidate),
		mustValue(t, domain.NewChannelCostAmount, amount),
		mustValue(t, domain.NewChannelCostCurrency, currency),
	)
	if err != nil {
		t.Fatalf("造已定价候选 %s：%v", candidate, err)
	}
	return cost
}

// Covers: 票 13 的硬约束「候选评价为待判断或不可计价时该候选出局，不得以零金额顶替」
// （PAR-NET-16）。取**唯一候选算不出**这一格，因为只有它把「出局」与「比输了」分得开：
// 折成零分时这个候选会以成本 0 胜出，而它其实一分钱的价都没算出来。
func TestAChannelCandidateWithNoEstablishedCostIsEjectedRatherThanTreatedAsFree(t *testing.T) {
	t.Parallel()

	unpriceable, err := domain.UnpriceableChannelCandidate(
		mustValue(t, domain.NewChannelCandidateID, "cand-uniuni-1"),
		domain.ChannelCostPendingEvidence,
	)
	if err != nil {
		t.Fatalf("造不可计价候选：%v", err)
	}

	if _, established := unpriceable.Amount(); established {
		t.Fatal("算不出成本的候选交出了一个成本取值——零金额顶替就是从这里开始的")
	}

	if _, err := domain.SelectChannelCandidateByCost([]domain.ChannelCandidateCost{unpriceable}); !errors.Is(err, domain.ErrNoQualifiedChannelCandidate) {
		t.Fatalf("择优 err = %v，want %v", err, domain.ErrNoQualifiedChannelCandidate)
	}
}

// Covers: 择优取成本最小者，出局候选不参选。把出局候选摆在最前、把贵的摆在便宜的之前都
// 是有意的——「折成零分」与「取遇到的第一个已确立取值」这两种写法各会在其中一处选错，而
// 两处选错都是选了一家不该选的供应商。
func TestChannelSelectionTakesTheCheapestAndNeverAnEjectedCandidate(t *testing.T) {
	t.Parallel()

	winner, err := domain.SelectChannelCandidateByCost([]domain.ChannelCandidateCost{
		unpriceableCost(t, "cand-pending", domain.ChannelCostPendingEvidence),
		pricedCost(t, "cand-expensive", "18.00"),
		pricedCost(t, "cand-cheap", "12.00"),
	})
	if err != nil {
		t.Fatalf("择优：%v", err)
	}
	if winner.String() != "cand-cheap" {
		t.Fatalf("选出 %q，want cand-cheap", winner.String())
	}
}

// Covers: 金额按十进制数值比大小，不按字符串序。金额照 parcel-pricing 交来的样子原样保全
// （ADR-0048 对申报数字的同一立场），于是同一批里出现不同小数位是常态，而字符串序在这里
// 恰好会反过来：字典序上 "10.00" 排在 "9.50" 之前，照字符串挑最小会选中更贵的那家。
//
// 整数位数不同（一位对两位）是这一格的要害：位数相同时字符串序碰巧与数值序一致，只有
// 位数不同才把两者分开。
func TestChannelCostsCompareByDecimalValueNotByStringOrder(t *testing.T) {
	t.Parallel()

	winner, err := domain.SelectChannelCandidateByCost([]domain.ChannelCandidateCost{
		pricedCost(t, "cand-ten", "10.00"),
		pricedCost(t, "cand-nine-fifty", "9.50"),
	})
	if err != nil {
		t.Fatalf("择优：%v", err)
	}
	if winner.String() != "cand-nine-fifty" {
		t.Fatalf("选出 %q，want cand-nine-fifty——9.50 比 10.00 便宜，字典序把它排到了后面", winner.String())
	}
}

// Covers: 票 01 裁决四「成本并列且无其它已配置准则可分时，交冲突、不自行收尾」。候选按
// 标识倒序摆放是为了钉住那个具体的错法——退回标识升序会选出 cand-a 并且一路绿，而它等于
// 让字母序替运营企业挑供应商，审计问「为什么选它」时答不出任何业务理由。
//
// 两个金额小数位不同而数值相同，同时钉住另一件事：并列判的是数值，"12" 与 "12.00" 是同
// 一个价，不是两个价——把它们读成有高下，就等于让小数位数决定选谁。
func TestTiedChannelCostsAreHandedOverAsAConflictRatherThanBrokenByIdentity(t *testing.T) {
	t.Parallel()

	_, err := domain.SelectChannelCandidateByCost([]domain.ChannelCandidateCost{
		pricedCost(t, "cand-b", "12"),
		pricedCost(t, "cand-a", "12.00"),
	})
	if !errors.Is(err, domain.ErrChannelCandidateCostTied) {
		t.Fatalf("择优 err = %v，want %v", err, domain.ErrChannelCandidateCostTied)
	}
}

// Covers: ChannelCostCurrency 的存在理由——「跨币种比大小是个数值上完全合法、业务上毫无
// 意义的动作」。币种进了类型，但择优此前从不看它，于是那句理由没有任何东西在兑现。
//
// 摆位是有意的：USD 那个候选**数值最小**，所以今天的比较器会选它，而最小币单位的购买力
// 两边并不相同，数值最小根本不蕴含更便宜。机制手里也没有汇率可以把两者放到同一把尺上
// ——汇率属实例半边，本仓一个租户都没有。既然折不到同一把尺，就只能拒绝，不能挑一个：
// 挑出来的那个背后是一笔真实的供应商采购。
func TestChannelCostsInDifferentCurrenciesAreRefusedRatherThanComparedNumerically(t *testing.T) {
	t.Parallel()

	_, err := domain.SelectChannelCandidateByCost([]domain.ChannelCandidateCost{
		pricedCostIn(t, "cand-usd", "10.00", "USD"),
		pricedCostIn(t, "cand-jpy", "12", "JPY"),
	})
	if !errors.Is(err, domain.ErrChannelCostCurrencyMismatch) {
		t.Fatalf("择优 err = %v，want %v", err, domain.ErrChannelCostCurrencyMismatch)
	}
}

// Covers: 出局候选必须指名是哪一格，且四格互不顶替（parcel-pricing CONTEXT「依据不足形成
// 待判断，明确排除形成不可计价，互斥候选形成冲突，请求不合法或计算失败形成未形成；四者
// 不得互相替代」）。票 14 的落选留痕要答「这个候选当初为什么出局」，压成一格就答不出。
func TestEachUnavailabilityGradeKeepsItsOwnIdentity(t *testing.T) {
	t.Parallel()

	grades := []domain.ChannelCostUnavailability{
		domain.ChannelCostPendingEvidence,
		domain.ChannelCostRatecardExclusion,
		domain.ChannelCostConflict,
		domain.ChannelCostNotFormed,
	}

	seen := make(map[string]domain.ChannelCostUnavailability, len(grades))
	for _, grade := range grades {
		label := grade.String()
		if label == "" {
			t.Fatalf("出局格 %d 没有名字", grade)
		}
		if previous, duplicated := seen[label]; duplicated {
			t.Fatalf("出局格 %d 与 %d 同名 %q——两格顶替就是从共用一个名字开始的", grade, previous, label)
		}
		seen[label] = grade

		cost := unpriceableCost(t, "cand-"+label, grade)
		got, unavailable := cost.Unavailability()
		if !unavailable || got != grade {
			t.Fatalf("出局格交回 (%d, %v)，want (%d, true)", got, unavailable, grade)
		}
	}

	if _, unavailable := pricedCost(t, "cand-priced", "12.00").Unavailability(); unavailable {
		t.Fatal("已定价的候选报出了出局格")
	}
}
