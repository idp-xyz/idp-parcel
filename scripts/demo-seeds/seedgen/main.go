// demo-seeds/seedgen 生成计价种子快照 JSON，**仅限隔离环境**（票 master-data-wiring/08）。
//
// 价卡与序列的登记输入是领域折装的快照（MarshalPriceCardRegistration /
// MarshalReferenceSeriesRegistration 的产物），携带规范化版本号与内容摘要自校——
// 手写 JSON 拼不出合法摘要，所以种子由本生成器经真领域构造函数产出后落盘入库。
// 产物提交在 scripts/demo-seeds/data/pricing/ 下；规范化版本升级（PPC/PRS 换号）时
// 重跑本生成器再提交新产物即可。
//
// 全部实例值是 SYN- 前缀的合成内容，证据层级 S：不影射任何真实企业，不写生产默认值，
// 不进参数登记册。跨上下文引用按所有权只引不解析：方向授权与汇率口径指向
// party-commercial 的合成发布对象（SYN-AUTH-PRICE-DIR-01 / SYN-PRICE-RULE-CN-SG）。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

const (
	tenantID = "SYN-TENANT-01"
	scopeID  = "SYN-SCOPE-01"
	// 演示动线的统一生效起点：全套种子讲同一段 2026 年的合成故事。
	registrant = "SYN-PRICING-OPS-01"
	approver   = "SYN-PRICING-GOVERNOR-01"
)

func main() {
	outDir := "scripts/demo-seeds/data/pricing"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fail("建输出目录：%v", err)
	}

	write(outDir, "price-card-cn-sg.json", sellCardSnapshot())
	write(outDir, "price-card-cn-sg-cost.json", buyCardSnapshot())
	write(outDir, "reference-series-fuel.json", fuelSeriesSnapshot())
	write(outDir, "reference-series-fx-cny-sgd.json", fxSeriesSnapshot())
}

// sellCardSnapshot 折装售价卡 SYN-PLAN-CN-SG-01 v1：华东/华南→新加坡的首重+续重
// 客户计费卡，MAX 计费重（体积系数 5000），一条偏远附加固定规则，并把燃油序列绑进
// 方案结构——「建产品→配价」那半个故事里价卡引用序列的那根线。
func sellCardSnapshot() []byte {
	cny := currency("CNY")
	kilogram := domain.WeightUnitKilogram

	firstHalfKilo := weight("0.5", kilogram)
	east, err := domain.NewFirstContinueRate(
		rateEntryID("SYN-RATE-CN-SG-Z1"), "Z1",
		firstHalfKilo, money("55", cny),
		firstHalfKilo, money("18", cny),
	)
	must("Z1 首续费率", err)
	south, err := domain.NewFirstContinueRate(
		rateEntryID("SYN-RATE-CN-SG-Z2"), "Z2",
		firstHalfKilo, money("48", cny),
		firstHalfKilo, money("15", cny),
	)
	must("Z2 首续费率", err)
	table, err := domain.NewFirstContinueRateTable(
		reference(domain.ArtifactRateTable, "SYN-TABLE-CN-SG-01", "v1"),
		cny, kilogram, period("2026-01-01"),
		[]domain.FirstContinueRate{east, south},
	)
	must("售价表", err)

	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, firstHalfKilo)
	must("计费重进位", err)
	factorRounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight("0.1", kilogram))
	must("体积重进位", err)
	factor, err := domain.NewVolumetricFactor(decimal("5000"), domain.LengthUnitCentimeter, factorRounding)
	must("体积系数", err)
	weightPolicy, err := domain.NewPricingWeightPolicy(
		reference(domain.ArtifactWeightPolicy, "SYN-WEIGHT-CN-SG-01", "v1"),
		domain.PricingWeightMax, rounding, &factor,
	)
	must("计费重策略", err)

	remote, err := domain.NewFixedChargeRule(
		"SYN-RULE-REMOTE-AREA", chargeCode("REMOTE_AREA_SURCHARGE"),
		"SYN 偏远区域附加（合成演示值）", domain.ChargeEffectAdd, money("5", cny), 1,
	)
	must("偏远附加规则", err)

	fuelBinding, err := domain.NewReferenceSeriesBinding(
		domain.ReferenceSeriesFuelRate,
		reference(domain.ArtifactReferenceSeries, "SYN-SERIES-FUEL-01", "v1"),
	)
	must("燃油序列绑定", err)
	structures, err := domain.NewPricingPlanStructures(nil, nil, []domain.ReferenceSeriesBinding{fuelBinding})
	must("方案结构", err)

	plan, err := domain.NewPricingPlanVersion(
		reference(domain.ArtifactPricingPlan, "SYN-PLAN-CN-SG-01", "v1"),
		pricingScope(), domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		chargeCode("BASE_FREIGHT"), period("2026-01-01"),
		table, weightPolicy, []domain.FixedChargeRule{remote}, structures,
	)
	must("售价方案", err)

	return marshalCard(plan, "SYN-CARD-CN-SG-01.xlsx", "SYN-AUTH-PRICE-DIR-01")
}

// buyCardSnapshot 折装采购成本卡 SYN-PLAN-CN-SG-COST-01 v1：同一动线的供应商成本半边，
// 重量段区间表，实际重计费——目录页上与售价卡并排，方向轴（SELL/BUY）就看得见了。
func buyCardSnapshot() []byte {
	cny := currency("CNY")
	kilogram := domain.WeightUnitKilogram

	brackets := []struct {
		id     string
		lower  string
		upper  string
		amount string
	}{
		{"SYN-COST-Z1-A", "0", "1", "30"},
		{"SYN-COST-Z1-B", "1", "5", "85"},
		{"SYN-COST-Z1-C", "5", "30", "260"},
	}
	entries := make([]domain.RateEntry, 0, len(brackets))
	for _, bracket := range brackets {
		entry, err := domain.NewRateEntry(
			rateEntryID(bracket.id), "Z1",
			weight(bracket.lower, kilogram), weight(bracket.upper, kilogram),
			money(bracket.amount, cny),
		)
		must("成本区间 "+bracket.id, err)
		entries = append(entries, entry)
	}
	table, err := domain.NewRateTableVersion(
		reference(domain.ArtifactRateTable, "SYN-TABLE-CN-SG-COST-01", "v1"),
		domain.RateTableFamilyWeightZone, cny, kilogram, period("2026-01-01"), entries,
	)
	must("成本价表", err)

	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight("0.5", kilogram))
	must("成本计费重进位", err)
	weightPolicy, err := domain.NewPricingWeightPolicy(
		reference(domain.ArtifactWeightPolicy, "SYN-WEIGHT-CN-SG-COST-01", "v1"),
		domain.PricingWeightActualOnly, rounding, nil,
	)
	must("成本计费重策略", err)

	plan, err := domain.NewPricingPlanVersion(
		reference(domain.ArtifactPricingPlan, "SYN-PLAN-CN-SG-COST-01", "v1"),
		pricingScope(), domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost,
		chargeCode("BASE_FREIGHT"), period("2026-01-01"),
		table, weightPolicy, nil, domain.PricingPlanStructures{},
	)
	must("成本方案", err)

	return marshalCard(plan, "SYN-CARD-CN-SG-COST-01.xlsx", "SYN-AUTH-COST-DIR-01")
}

// fuelSeriesSnapshot 折装燃油系数序列 SYN-SERIES-FUEL-01 v1：三期取值、每期带合成
// 取值凭证（整版可复核）。燃油不需要口径声明——折扣系数写在卡上。
func fuelSeriesSnapshot() []byte {
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           tenant(),
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        reference(domain.ArtifactReferenceSeries, "SYN-SERIES-FUEL-01", "v1"),
		SourceIdentifier: "SYN-CARRIER-FUEL-BULLETIN",
		Registrant:       registrant,
		Periods: []domain.SeriesPeriodValue{
			seriesPeriod("2026-01-01", "2026-04-01", "0.12", "SYN-EVID-FUEL-2601"),
			seriesPeriod("2026-04-01", "2026-07-01", "0.135", "SYN-EVID-FUEL-2604"),
			seriesPeriod("2026-07-01", "", "0.11", "SYN-EVID-FUEL-2607"),
		},
	})
	must("燃油序列登记", err)
	raw, err := domain.MarshalReferenceSeriesRegistration(registration)
	must("燃油序列折装", err)
	return raw
}

// fxSeriesSnapshot 折装汇率序列 SYN-SERIES-FX-CNY-SGD v1。汇率必须声明取值口径：
// 口径由 party-commercial 的合成价格规则版本（SYN-PRICE-RULE-CN-SG v1）声明——
// 这根引用就是「序列↔商业策略」互引的那条线。
func fxSeriesSnapshot() []byte {
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           tenant(),
		Kind:             domain.ReferenceSeriesExchangeRate,
		Reference:        reference(domain.ArtifactReferenceSeries, "SYN-SERIES-FX-CNY-SGD", "v1"),
		SourceIdentifier: "SYN-TREASURY-FX-DESK",
		Registrant:       registrant,
		QuoteBasis:       reference(domain.ArtifactCommercialPolicy, "SYN-PRICE-RULE-CN-SG", "v1"),
		Periods: []domain.SeriesPeriodValue{
			seriesPeriod("2026-01-01", "2026-07-01", "5.20", "SYN-EVID-FX-2601"),
			seriesPeriod("2026-07-01", "", "5.35", "SYN-EVID-FX-2607"),
		},
	})
	must("汇率序列登记", err)
	raw, err := domain.MarshalReferenceSeriesRegistration(registration)
	must("汇率序列折装", err)
	return raw
}

func marshalCard(plan domain.PricingPlanVersion, sourceName, authorizationID string) []byte {
	// 合成源文件并不存在，SHA-256 取文件名字符串的摘要：确定性、可复算，且不冒充
	// 真实证据库里的任何东西。
	digest := sha256.Sum256([]byte(sourceName))
	source, err := domain.NewSourceFileIdentity(sourceName, hex.EncodeToString(digest[:]))
	must("源文件身份 "+sourceName, err)

	registration, err := domain.NewPriceCardRegistration(
		tenant(), plan, source,
		reference(domain.ArtifactCommercialAuthorization, authorizationID, "v1"),
		approver,
	)
	must("价卡登记 "+sourceName, err)
	raw, err := domain.MarshalPriceCardRegistration(registration)
	must("价卡折装 "+sourceName, err)
	return raw
}

// ---- 构造小件：错就地退出，生成器没有降级路径 ----

func tenant() domain.TenantID {
	value, err := domain.NewTenantID(tenantID)
	must("租户", err)
	return value
}

func pricingScope() domain.PricingScopeID {
	value, err := domain.NewPricingScopeID(scopeID)
	must("计价范围", err)
	return value
}

func reference(kind domain.ArtifactKind, id, version string) domain.VersionReference {
	value, err := domain.NewVersionReference(kind, id, version, "sha256:syn-"+id+"-"+version)
	must("版本引用 "+id, err)
	return value
}

func currency(code string) domain.Currency {
	value, err := domain.NewCurrency(code)
	must("币种 "+code, err)
	return value
}

func decimal(text string) domain.Decimal {
	value, err := domain.ParseDecimal(text)
	must("数值 "+text, err)
	return value
}

func weight(text string, unit domain.WeightUnit) domain.Weight {
	value, err := domain.NewWeight(decimal(text), unit)
	must("重量 "+text, err)
	return value
}

func money(text string, unit domain.Currency) domain.Money {
	value, err := domain.NewMoney(decimal(text), unit)
	must("金额 "+text, err)
	return value
}

func chargeCode(text string) domain.ChargeCode {
	value, err := domain.NewChargeCode(text)
	must("费目码 "+text, err)
	return value
}

func rateEntryID(text string) domain.RateEntryID {
	value, err := domain.NewRateEntryID(text)
	must("费率行 ID "+text, err)
	return value
}

// period 从起始日构造无上界生效区间；种子故事统一从 2026 年展开。
func period(startDate string) domain.EffectivePeriod {
	value, err := domain.NewUnboundedEffectivePeriod(day(startDate))
	must("生效区间 "+startDate, err)
	return value
}

// seriesPeriod 构造一期序列取值；endDate 为空即无上界（只在最后一期）。
func seriesPeriod(startDate, endDate, value, evidence string) domain.SeriesPeriodValue {
	endsAt := time.Time{}
	if endDate != "" {
		endsAt = day(endDate)
	}
	result, err := domain.NewSeriesPeriodValue(day(startDate), endsAt, decimal(value), evidence)
	must("序列期次 "+startDate, err)
	return result
}

func day(text string) time.Time {
	value, err := time.Parse("2006-01-02", text)
	must("日期 "+text, err)
	return value.UTC()
}

func write(dir, name string, raw []byte) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		fail("写 %s：%v", path, err)
	}
	fmt.Printf("已生成 %s\n", path)
}

func must(what string, err error) {
	if err != nil {
		fail("%s：%v", what, err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
