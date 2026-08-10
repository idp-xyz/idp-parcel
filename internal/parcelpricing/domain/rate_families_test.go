package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PA-PP-01（产品需求假设）：商务快递代理按首重定价，超出部分按续重步长计收。步长整段
// 计收，所以在 0.5 kg 首重加 0.5 kg 步长下，0.6 kg 收一整步，不是收 0.2 步。
func TestFirstContinueChargesTheFirstWeightThenWholeSteps(t *testing.T) {
	table := firstContinueTable(t)
	for _, testCase := range []struct {
		weight string
		want   string
	}{
		{"0.4", "30"},
		{"0.5", "30"},
		{"0.6", "38"},
		{"1", "38"},
		{"1.1", "46"},
	} {
		selection, err := table.Lookup("Z1", weight(t, testCase.weight, domain.WeightUnitKilogram))
		if err != nil {
			t.Fatalf("lookup %s kg: %v", testCase.weight, err)
		}
		if got := selection.Amount().Amount().String(); got != testCase.want {
			t.Fatalf("%s kg priced at %s, want %s", testCase.weight, got, testCase.want)
		}
		if selection.Family() != domain.RateTableFamilyFirstContinue {
			t.Fatalf("family = %s", selection.Family())
		}
	}
}

// PA-PP-01（产品需求假设）：经济线路按计价重量乘单价报一个价，完全没有档位，因而没有
// 区间可匹配——金额是派生出来的。
func TestUnitPriceMultipliesTheChargeableWeight(t *testing.T) {
	table := unitPriceTable(t)
	selection, err := table.Lookup("Z1", weight(t, "2.5", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got := selection.Amount().Amount().String(); got != "112.5" {
		t.Fatalf("2.5 kg at 45/kg priced at %s, want 112.5", got)
	}
	if selection.Family() != domain.RateTableFamilyUnitPrice {
		t.Fatalf("family = %s", selection.Family())
	}
}

// Covers: CONTEXT「缺少必需版本、区间空档、边界重叠、单位/币种不一致或依赖未决时，结果
// 保持待判断或冲突，不使用隐式默认价」— 每个族都按分区查表；表没有定价的分区若回退到别的
// 分区费率，就是拿一个隐式默认价冒充结果。
func TestNewFamiliesKeepZonesIsolated(t *testing.T) {
	for name, table := range map[string]domain.RateTableVersion{
		"first-continue": firstContinueTable(t),
		"unit-price":     unitPriceTable(t),
	} {
		if _, err := table.Lookup("Z9", weight(t, "1", domain.WeightUnitKilogram)); !errors.Is(err, domain.ErrNoMatchingRate) {
			t.Fatalf("%s: unknown zone error = %v, want ErrNoMatchingRate", name, err)
		}
	}
}

// Covers: CONTEXT「价表区间、分区、重量策略、附加费、折扣、最低/最高收费、燃油、组合方式、
// 精度和取整顺序必须可解释、可复算」— 解释是争议复核要读的东西。派生金额没有档位可引，
// 必须讲清自己是怎么派生出来的，不能套用重量分区那套措辞。
func TestDerivedFamiliesExplainHowTheAmountWasReached(t *testing.T) {
	for name, table := range map[string]domain.RateTableVersion{
		"first-continue": firstContinueTable(t),
		"unit-price":     unitPriceTable(t),
	} {
		selection, err := table.Lookup("Z1", weight(t, "1.1", domain.WeightUnitKilogram))
		if err != nil {
			t.Fatalf("%s: lookup: %v", name, err)
		}
		if selection.Explanation() == "" {
			t.Fatalf("%s: derived amount carried no explanation", name)
		}
	}
}

// 一张表只声明一个族，也只带该族的费率。建一张没有费率的表，等于把「这个分区查不到价」
// 推迟到评价期才暴露；构造期就该拒绝。
func TestNewFamiliesRejectEmptyRateSets(t *testing.T) {
	reference := versionReference(t, domain.ArtifactRateTable, "table-empty", "v1")
	currency := mustValue(t, domain.NewCurrency, "USD")
	if _, err := domain.NewFirstContinueRateTable(reference, currency, domain.WeightUnitKilogram, effectivePeriod(t), nil); !errors.Is(err, domain.ErrInvalidRateTable) {
		t.Fatalf("empty first-continue table error = %v", err)
	}
	if _, err := domain.NewUnitPriceRateTable(reference, currency, domain.WeightUnitKilogram, effectivePeriod(t), nil); !errors.Is(err, domain.ErrInvalidRateTable) {
		t.Fatalf("empty unit-price table error = %v", err)
	}
}

func firstContinueTable(t *testing.T) domain.RateTableVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	rate, err := domain.NewFirstContinueRate(
		mustValue(t, domain.NewRateEntryID, "fc-z1"),
		"Z1",
		weight(t, "0.5", domain.WeightUnitKilogram),
		money(t, "30", currency),
		weight(t, "0.5", domain.WeightUnitKilogram),
		money(t, "8", currency),
	)
	if err != nil {
		t.Fatalf("first-continue rate: %v", err)
	}
	table, err := domain.NewFirstContinueRateTable(
		versionReference(t, domain.ArtifactRateTable, "table-fc", "v1"),
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.FirstContinueRate{rate},
	)
	if err != nil {
		t.Fatalf("first-continue table: %v", err)
	}
	return table
}

func unitPriceTable(t *testing.T) domain.RateTableVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	rate, err := domain.NewUnitPriceRate(
		mustValue(t, domain.NewRateEntryID, "up-z1"),
		"Z1",
		money(t, "45", currency),
	)
	if err != nil {
		t.Fatalf("unit-price rate: %v", err)
	}
	table, err := domain.NewUnitPriceRateTable(
		versionReference(t, domain.ArtifactRateTable, "table-up", "v1"),
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.UnitPriceRate{rate},
	)
	if err != nil {
		t.Fatalf("unit-price table: %v", err)
	}
	return table
}
