// Package pptest 是 parcel-pricing 的测试专属合成评价夹具（S 级证据）：把「一张单分区重量段价卡 + 一份已受理包裹
// 输入 → 真算 domain.EvaluatePricing」这段在 SA parcelpricing 适配器用例、PP postgres 登记册用例与 parcel-dispatch
// 组合根用例里各写过一遍的构造收成一处（票 sa-cc/17，出处 sa-cc/01 评审 Standards ②）。
//
// 它是普通包而不是 `_test.go`，与 internal/platform/pgtest 同一种形：跨包共用的测试助手只能这么写，代价是它把
// testing 拉进任何 import 它的二进制，所以只许 `_test.go` import 它，生产代码不得依赖。
//
// 夹具不替调用方选值。三处调用方各自钉着不同的合成串、金额、取整策略与版本引用形（SA 的用例还断言着其中的规则
// 版本串与分值），所以规格字段全部显式、零默认值——少给一项是构造失败，不是悄悄换题。共有的只是三处本来就写成
// 一样的那几件：重量单位千克、单一价表行、分区重量段家族、实重策略且无体积因子、无附加费规则、逐包裹聚合、
// 已受理包裹主体、无尺寸、证据级 EvidenceSynthetic。
//
// 夹具不手搓评价：评价是闭合计算记录，手搓等于绕过守恒不变量，成功与失败两种形态都要来自真实计算路径。
package pptest

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PlanSpec 描述一张合成价卡的全部可变部分。
type PlanSpec struct {
	// Reference / TableReference / WeightPolicyReference 是价卡、价表、计价重量策略三条版本引用，带不带指纹由
	// 调用方经 IdentityReference / FingerprintReference 决定。
	Reference             domain.VersionReference
	TableReference        domain.VersionReference
	WeightPolicyReference domain.VersionReference
	Scope                 string
	Direction             domain.PricingDirection
	Purpose               domain.PricingPurpose
	BaseChargeCode        string
	Currency              string
	// Period 是价卡与价表共用的有效期。
	Period Period
	// RateEntryID / RateZone / RateAmount 是那唯一一行价表：分区 RateZone、重量段 [MinimumKilograms, MaximumKilograms]、
	// 金额 RateAmount（以 Currency 计）。
	RateEntryID      string
	RateZone         string
	MinimumKilograms string
	MaximumKilograms string
	RateAmount       string
	// WeightRounding / WeightStepKilograms 是计价重量的取整策略。
	WeightRounding      domain.RoundingMode
	WeightStepKilograms string
	// AmountRounding 为 nil 即价卡不声明金额取整策略——SA 消费侧按 ADR-0107 会以 AMOUNT_PRECISION_UNDECLARED 拒。
	AmountRounding *AmountRoundingSpec
}

// Period 是价卡与价表的有效期端点。
type Period struct {
	StartsAt time.Time
	EndsAt   time.Time
}

// AmountRoundingSpec 描述价卡声明的金额取整策略；Increment 以价卡币种计。
type AmountRoundingSpec struct {
	Mode      domain.RoundingMode
	Increment string
	Points    []domain.AmountRoundingPoint
}

// InputSpec 描述一份已受理包裹的计价输入。
type InputSpec struct {
	Tenant     string
	Scope      string
	PackageID  string
	Zone       string
	Kilograms  string
	BusinessAt time.Time
}

// Value 是夹具的统一失败出口：把返回 (T, error) 的字符串构造器直接喂进去，构造失败即 Fatal。
func Value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("pptest：构造 %q：%v", raw, err)
	}
	return value
}

// IdentityReference 造一条不带指纹的版本引用。
func IdentityReference(t *testing.T, kind domain.ArtifactKind, id, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReferenceIdentity(kind, id, version)
	if err != nil {
		t.Fatalf("pptest：版本引用 %s/%s@%s：%v", kind, id, version, err)
	}
	return reference
}

// FingerprintReference 造一条带指纹的版本引用。
func FingerprintReference(t *testing.T, kind domain.ArtifactKind, id, version, fingerprint string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReferenceWithFingerprint(kind, id, version, fingerprint)
	if err != nil {
		t.Fatalf("pptest：版本引用 %s/%s@%s（%s）：%v", kind, id, version, fingerprint, err)
	}
	return reference
}

// Plan 按规格立一张价卡：一行价表、实重策略、可选的金额取整策略。
func Plan(t *testing.T, spec PlanSpec) domain.PricingPlanVersion {
	t.Helper()
	currency := Value(t, domain.NewCurrency, spec.Currency)
	amount, err := domain.NewMoney(Value(t, domain.ParseDecimal, spec.RateAmount), currency)
	if err != nil {
		t.Fatalf("pptest：价表行金额 %q：%v", spec.RateAmount, err)
	}
	entry, err := domain.NewRateEntry(Value(t, domain.NewRateEntryID, spec.RateEntryID), spec.RateZone,
		kilograms(t, spec.MinimumKilograms), kilograms(t, spec.MaximumKilograms), amount)
	if err != nil {
		t.Fatalf("pptest：价表行：%v", err)
	}
	period, err := domain.NewEffectivePeriod(spec.Period.StartsAt, spec.Period.EndsAt)
	if err != nil {
		t.Fatalf("pptest：有效期：%v", err)
	}
	table, err := domain.NewRateTableVersion(spec.TableReference, domain.RateTableFamilyWeightZone, currency,
		domain.WeightUnitKilogram, period, []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("pptest：价表：%v", err)
	}
	weightRounding, err := domain.NewWeightRoundingPolicy(spec.WeightRounding, kilograms(t, spec.WeightStepKilograms))
	if err != nil {
		t.Fatalf("pptest：重量取整策略：%v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(spec.WeightPolicyReference, domain.PricingWeightActualOnly, weightRounding, nil)
	if err != nil {
		t.Fatalf("pptest：计价重量策略：%v", err)
	}
	structures := domain.PricingPlanStructures{}
	if spec.AmountRounding != nil {
		increment, err := domain.NewMoney(Value(t, domain.ParseDecimal, spec.AmountRounding.Increment), currency)
		if err != nil {
			t.Fatalf("pptest：进位单位 %q：%v", spec.AmountRounding.Increment, err)
		}
		policy, err := domain.NewAmountRoundingPolicy(spec.AmountRounding.Mode, increment, spec.AmountRounding.Points)
		if err != nil {
			t.Fatalf("pptest：金额取整策略：%v", err)
		}
		structures, err = structures.WithAmountRounding(policy)
		if err != nil {
			t.Fatalf("pptest：结构：%v", err)
		}
	}
	plan, err := domain.NewPricingPlanVersion(spec.Reference, Value(t, domain.NewPricingScopeID, spec.Scope),
		spec.Direction, spec.Purpose, Value(t, domain.NewChargeCode, spec.BaseChargeCode), period, table, weightPolicy, nil, structures)
	if err != nil {
		t.Fatalf("pptest：价卡：%v", err)
	}
	return plan
}

// Input 按规格立一份已受理包裹的计价输入：实重、无尺寸。
func Input(t *testing.T, spec InputSpec) domain.PricingInputSnapshot {
	t.Helper()
	subject, err := domain.NewAcceptedPackageSubject(Value(t, domain.NewPackageID, spec.PackageID))
	if err != nil {
		t.Fatalf("pptest：评价主体：%v", err)
	}
	input, err := domain.NewPricingInputSnapshot(Value(t, domain.NewTenantID, spec.Tenant), Value(t, domain.NewPricingScopeID, spec.Scope),
		subject, spec.Zone, kilograms(t, spec.Kilograms), nil, spec.BusinessAt)
	if err != nil {
		t.Fatalf("pptest：计价输入：%v", err)
	}
	return input
}

// Evaluate 以合成证据级真算一份评价。不核结果格：分区落空得失败形态、命中得 COMPLETED，两种都是调用方要的。
func Evaluate(t *testing.T, id string, plan domain.PricingPlanVersion, input domain.PricingInputSnapshot) domain.PricingEvaluation {
	t.Helper()
	request, err := domain.NewEvaluationRequest(Value(t, domain.NewEvaluationID, id), plan, input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("pptest：评价请求 %q：%v", id, err)
	}
	return domain.EvaluatePricing(request)
}

func kilograms(t *testing.T, raw string) domain.Weight {
	t.Helper()
	weight, err := domain.NewWeight(Value(t, domain.ParseDecimal, raw), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("pptest：重量 %q：%v", raw, err)
	}
	return weight
}
