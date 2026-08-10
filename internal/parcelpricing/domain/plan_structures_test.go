package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「版本内容摘要」—「对定价方案、价表、重量策略、附加费规则、判定条件、
// 互斥组、费用依赖、计价参考序列引用和固定费用规则按稳定结构生成的内容指纹」：摘要是重放
// 用来判断版本引用是否仍指向同一套可执行规则的东西。声明了附加费规则的方案，不得与其余
// 完全相同、却一条都没声明的方案共享摘要，否则一个已发布版本可以凭空多出一条可计收规则
// 而摘要纹丝不动。
func TestPricingPlanContentDigestCoversDeclaredSurchargeRules(t *testing.T) {
	bare := planWithStructures(t, domain.PricingPlanStructures{})
	declared := planWithStructures(t, structuresWithSurcharge(t, "48"))
	if bare.ContentDigest() == declared.ContentDigest() {
		t.Fatal("declared surcharge rules were omitted from the plan content digest")
	}
}

// 阈值长在判定条件里，所以两个只差「规则在哪触发」的方案同样不得摘要相同。
func TestPricingPlanContentDigestCoversConditionThresholds(t *testing.T) {
	lower := planWithStructures(t, structuresWithSurcharge(t, "48"))
	higher := planWithStructures(t, structuresWithSurcharge(t, "60"))
	if lower.ContentDigest() == higher.ContentDigest() {
		t.Fatal("condition threshold was omitted from the plan content digest")
	}
}

// Covers: CONTEXT「互斥组」—「组内先按声明优先级取最高级，同级再按金额取最高者」（推导
// 见计价规则模型最终设计的形态决定五）：组归属与优先级都会改变这张卡最终计收哪一条规则。
// 摘要若忽略它们，一个已发布版本就能把规则在组之间搬动而不报内容冲突。
func TestPricingPlanContentDigestCoversExclusivityGroupAndPriority(t *testing.T) {
	ungrouped := planWithStructures(t, structuresWithSurcharge(t, "48"))
	grouped := planWithStructures(t, structuresWithGroupedSurcharge(t, "48", "AHS", 1))
	if ungrouped.ContentDigest() == grouped.ContentDigest() {
		t.Fatal("exclusivity group was omitted from the plan content digest")
	}
	reprioritised := planWithStructures(t, structuresWithGroupedSurcharge(t, "48", "AHS", 2))
	if grouped.ContentDigest() == reprioritised.ContentDigest() {
		t.Fatal("exclusivity priority was omitted from the plan content digest")
	}
}

// 优先级只有相对同组其他成员才有意义，所以不属于任何组的规则不得携带优先级。
func TestSurchargeRuleRejectsPriorityWithoutAnExclusivityGroup(t *testing.T) {
	rule := surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10")
	if _, err := rule.InExclusivityGroup("", 1); !errors.Is(err, domain.ErrInvalidSurchargeRule) {
		t.Fatalf("ungrouped priority error = %v", err)
	}
	if _, err := rule.InExclusivityGroup("AHS", 0); !errors.Is(err, domain.ErrInvalidSurchargeRule) {
		t.Fatalf("grouped rule without priority error = %v", err)
	}
}

// Covers: CONTEXT「它抬高的是用于基础价查表与后续费用基数的同一个计价重量，不是某一条
// 费用行的私有基数」— 条件最低计价重量声明在附加费条款里，抬高的却是方案级计价重量，
// 因而基础运费查表与附加费一起变。它必须在摘要之内。
func TestPricingPlanContentDigestCoversConditionalMinimumWeight(t *testing.T) {
	plain := planWithStructures(t, structuresWithSurcharge(t, "48"))
	raised := planWithStructures(t, structuresWithMinimumWeight(t, "48", "40"))
	if plain.ContentDigest() == raised.ContentDigest() {
		t.Fatal("conditional minimum weight was omitted from the plan content digest")
	}
	higher := planWithStructures(t, structuresWithMinimumWeight(t, "48", "90"))
	if raised.ContentDigest() == higher.ContentDigest() {
		t.Fatal("conditional minimum weight value was omitted from the plan content digest")
	}
}

// Covers: CONTEXT「依赖必须显式给出基数构成与排除集，不由声明顺序隐含」— 卡上的燃油基数
// 是「除运费复核费以外的其他全部费用」，所以排除集才是决定金额的那一半。排除了不同费用
// 代码的两个方案是两条不同的规则，不得共享摘要。
func TestPricingPlanContentDigestCoversChargeDependencies(t *testing.T) {
	plain := planWithStructures(t, structuresWithSurcharge(t, "48"))
	dependent := planWithStructures(t, structuresWithDependency(t, "FREIGHT_AUDIT_FEE"))
	if plain.ContentDigest() == dependent.ContentDigest() {
		t.Fatal("charge dependencies were omitted from the plan content digest")
	}
	other := planWithStructures(t, structuresWithDependency(t, "RESIDENTIAL_DELIVERY"))
	if dependent.ContentDigest() == other.ContentDigest() {
		t.Fatal("dependency exclusion set was omitted from the plan content digest")
	}
}

// 要解析燃油费率或汇率的方案会绑定到一条已登记的计价参考序列；换掉所绑的序列，即便其余
// 规则一字不差，这个方案收的钱也变了。
func TestPricingPlanContentDigestCoversReferenceSeriesBindings(t *testing.T) {
	unbound := planWithStructures(t, structuresWithSurcharge(t, "48"))
	bound := planWithStructures(t, structuresWithReferenceSeries(t, "fuel-weekly", "v1"))
	if unbound.ContentDigest() == bound.ContentDigest() {
		t.Fatal("reference series bindings were omitted from the plan content digest")
	}
	rebound := planWithStructures(t, structuresWithReferenceSeries(t, "fuel-weekly", "v2"))
	if bound.ContentDigest() == rebound.ContentDigest() {
		t.Fatal("reference series version was omitted from the plan content digest")
	}
}

// Covers: CONTEXT「计价参考序列必须按计价基准时点解析并写入版本清单；重放使用原序列取值，
// 不读取当前值」— 重放要拿回它当初用的那批序列取值，而它是去版本清单里找的。清单没有带上
// 的绑定，就只能对着序列今天的值去解析。
func TestPricingPlanManifestCarriesBoundReferenceSeries(t *testing.T) {
	plan := planWithStructures(t, structuresWithReferenceSeries(t, "fuel-weekly", "v1"))
	for _, reference := range plan.Manifest().References() {
		if reference.Kind() == domain.ArtifactReferenceSeries && reference.ID() == "fuel-weekly" {
			return
		}
	}
	t.Fatal("bound reference series is missing from the plan version manifest")
}

func structuresWithReferenceSeries(t testing.TB, id, version string) domain.PricingPlanStructures {
	t.Helper()
	binding, err := domain.NewReferenceSeriesBinding(
		domain.ReferenceSeriesFuelRate,
		versionReference(t, domain.ArtifactReferenceSeries, id, version),
	)
	if err != nil {
		t.Fatalf("reference series binding: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{standaloneRule(t, surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10"))},
		nil,
		[]domain.ReferenceSeriesBinding{binding},
	)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

// 一条附加费是与其他费用并列还是与它们竞争，是承运商的规则，不是引擎的：UPS 把大件费与
// 额外处理费放进同一个互斥组，FedEx 两者都收。把「未声明组」读成「独立并列」，等于悄悄拿
// 一家承运商的规则套到另一家的业务上，所以沉默被拒绝，而不是给默认值。
func TestPlanStructuresRefuseASurchargeRuleThatNeverDeclaredItsExclusivityStance(t *testing.T) {
	undeclared := surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10")
	if _, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{undeclared}, nil, nil); !errors.Is(err, domain.ErrUndeclaredExclusivity) {
		t.Fatalf("undeclared exclusivity error = %v", err)
	}
}

// 「独立并列」本身就是一次声明——卡上说这笔费用可以与其他费用同时计收——所以它必须表达
// 得出来，且不能读起来与「从来没说过」一样。
func TestSurchargeRuleMayDeclareThatItStandsAlone(t *testing.T) {
	standalone, err := surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10").Standalone()
	if err != nil {
		t.Fatalf("standalone: %v", err)
	}
	if _, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{standalone}, nil, nil); err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	if group, grouped := standalone.ExclusivityGroup(); grouped {
		t.Fatalf("a standalone rule reported group %q", group)
	}
}

// Covers: CONTEXT「被依赖的费用行本身永不进入自己的基数」— 接受这种声明，等于把一个环
// 当作合法规则记下来。
func TestChargeDependencyRejectsSelfReference(t *testing.T) {
	fuel := mustValue(t, domain.NewChargeCode, "FUEL")
	if _, err := domain.NewListedChargeDependency("fuel", fuel, []domain.ChargeCode{fuel}, nil); !errors.Is(err, domain.ErrInvalidChargeDependency) {
		t.Fatalf("self-referencing dependency error = %v", err)
	}
}

// Covers: CONTEXT「计算方法」—「首发取值闭合，只有定额、查表、按基数百分比，以及在其中
// 两者之间取较大值」：四套参数现在就都该进规范化形状。若只有定额有位置，日后声明卡上的
// max(1.782, 运费 × 12%) 会把每一个从没用过它的方案的摘要一起挪动。
func TestPricingPlanContentDigestCoversEveryCalculationMethod(t *testing.T) {
	digests := make(map[string]string, 4)
	for name, calculation := range map[string]domain.SurchargeCalculation{
		"fixed amount":     fixedAmountCalculation(t, "10"),
		"table lookup":     tableLookupCalculation(t),
		"percent of basis": percentOfBasisCalculation(t, "12"),
		"greater of":       greaterOfCalculation(t, "1.782", "12"),
	} {
		plan := planWithStructures(t, structuresWithCalculation(t, calculation))
		for existing, digest := range digests {
			if digest == plan.ContentDigest() {
				t.Fatalf("%q and %q share a content digest", name, existing)
			}
		}
		digests[name] = plan.ContentDigest()
	}
}

// 「取较大值」是在另外两种方法之间做一次选择，不是可以无穷嵌套的第三种东西。
func TestGreaterOfSurchargeRejectsANestedGreaterOfOperand(t *testing.T) {
	nested := greaterOfCalculation(t, "1.782", "12")
	if _, err := domain.NewGreaterOfSurcharge(nested, fixedAmountCalculation(t, "10")); !errors.Is(err, domain.ErrInvalidSurchargeRule) {
		t.Fatalf("nested greater-of error = %v", err)
	}
}

// Covers: CONTEXT「费用依赖必须显式声明基数构成与排除集，不得以声明顺序隐含依赖」—
// 百分比脱离它所依附的基数就没有意义，而基数是一条显式声明的依赖，不是碰巧排在它前面的
// 那些费用。
func TestPlanStructuresRejectPercentSurchargeWithoutItsDeclaredBasis(t *testing.T) {
	_, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{standaloneRule(t, ruleWithCalculation(t, percentOfBasisCalculation(t, "12")))},
		nil,
		nil,
	)
	if !errors.Is(err, domain.ErrInvalidChargeDependency) {
		t.Fatalf("unresolved basis error = %v", err)
	}
}

func fixedAmountCalculation(t testing.TB, amount string) domain.SurchargeCalculation {
	t.Helper()
	calculation, err := domain.NewFixedAmountSurcharge(money(t, amount, mustValue(t, domain.NewCurrency, "USD")))
	if err != nil {
		t.Fatalf("fixed amount calculation: %v", err)
	}
	return calculation
}

func tableLookupCalculation(t testing.TB) domain.SurchargeCalculation {
	t.Helper()
	return tableLookupCalculationIn(t, domain.WeightUnitKilogram)
}

func tableLookupCalculationIn(t testing.TB, unit domain.WeightUnit) domain.SurchargeCalculation {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-surcharge"),
		"Z2",
		weight(t, "0", unit),
		weight(t, "10", unit),
		money(t, "7", currency),
	)
	if err != nil {
		t.Fatalf("surcharge rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-surcharge", "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		unit,
		effectivePeriod(t),
		[]domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("surcharge rate table: %v", err)
	}
	calculation, err := domain.NewTableLookupSurcharge(table)
	if err != nil {
		t.Fatalf("table lookup calculation: %v", err)
	}
	return calculation
}

func percentOfBasisCalculation(t testing.TB, percentage string) domain.SurchargeCalculation {
	t.Helper()
	calculation, err := domain.NewPercentOfBasisSurcharge(decimal(t, percentage), "fuel")
	if err != nil {
		t.Fatalf("percent of basis calculation: %v", err)
	}
	return calculation
}

func greaterOfCalculation(t testing.TB, amount, percentage string) domain.SurchargeCalculation {
	t.Helper()
	calculation, err := domain.NewGreaterOfSurcharge(
		fixedAmountCalculation(t, amount),
		percentOfBasisCalculation(t, percentage),
	)
	if err != nil {
		t.Fatalf("greater of calculation: %v", err)
	}
	return calculation
}

func ruleWithCalculation(t testing.TB, calculation domain.SurchargeCalculation) domain.SurchargeRule {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, "48", domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	rule, err := domain.NewSurchargeRule(
		"ahs-dimension",
		mustValue(t, domain.NewChargeCode, "AHS_DIMENSION"),
		"ahs-dimension",
		domain.ChargeEffectAdd,
		leafTrigger(t, condition),
		calculation,
	)
	if err != nil {
		t.Fatalf("surcharge rule: %v", err)
	}
	return rule
}

// structuresWithCalculation 总是声明燃油依赖，好让百分比类方法有它指名的基数；依赖本身
// 在整轮摘要比对中保持不变。
func structuresWithCalculation(t testing.TB, calculation domain.SurchargeCalculation) domain.PricingPlanStructures {
	t.Helper()
	dependency, err := domain.NewAllChargesDependency(
		"fuel",
		mustValue(t, domain.NewChargeCode, "FUEL"),
		[]domain.ChargeCode{mustValue(t, domain.NewChargeCode, "FREIGHT_AUDIT_FEE")},
	)
	if err != nil {
		t.Fatalf("charge dependency: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{standaloneRule(t, ruleWithCalculation(t, calculation))},
		[]domain.ChargeDependency{dependency},
		nil,
	)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

// 这份夹具原来压的是「不可执行」闸门；如今每种已声明结构都有执行器，它压的是取代该闸门
// 的东西。绑定了序列、而快照从未提供该取值的方案仍然不得完成——那个费率是本次评价没拿到
// 的证据，只按基础费率收钱会在读起来像一次已完成评价的同时静默少收。
//
// 尺寸是给足的，好让缺失的读数是唯一那处不足。夹具同时声明了附加费，若快照连尺寸也没有，
// 就会一次缺两样，报哪一样将取决于评价器碰巧按什么顺序检查。
func TestEvaluationDoesNotCompleteWhenABoundSeriesWasNotSupplied(t *testing.T) {
	plan := planWithStructures(t, structuresWithReferenceSeries(t, "fuel-weekly", "v1"))
	evaluation := evaluateWithSides(t, plan, "eval-declared-structures", "50")
	if evaluation.Status() == domain.EvaluationCompleted {
		t.Fatal("evaluation completed while a bound reference series had no reading")
	}
	if _, formed := evaluation.Total(); formed {
		t.Fatal("evaluation produced a total from the base rate alone")
	}
	issues := evaluation.Issues()
	if len(issues) != 1 || issues[0].Code() != "REFERENCE_SERIES_UNRESOLVED" {
		t.Fatalf("issues = %#v", issues)
	}
}

// 分档的附加费按计价重量去读，而计价重量以基础价表分档所用的单位表述。因此按另一种单位
// 分档的附加费价表永远查不到：卡建得起来，却在每一次评价上冲突。同一张表的币种已经在这里
// 被拒，单位就并排一起拒，不留给评价器。
func TestPlanRefusesASurchargeTableBandedInAnotherWeightUnit(t *testing.T) {
	structures := declaredSurcharges(t, standaloneRule(t, ruleWithCalculation(t, tableLookupCalculationIn(t, domain.WeightUnitPound))))
	if _, err := newPlanWithStructures(t, structures); !errors.Is(err, domain.ErrWeightUnitMismatch) {
		t.Fatalf("plan formation error = %v, want ErrWeightUnitMismatch", err)
	}
}

// 条件最低计价重量抬高的是计价重量，所以按另一种单位表述的下限在构造期就被拒，而不是等
// 评价时才去比。留给评价器的话，它只会在触发该条款的包裹上失败，于是同一张卡对一部分包裹
// 算得出价、对另一部分报冲突。
func TestPlanRefusesAConditionalMinimumStatedInAnotherWeightUnit(t *testing.T) {
	structures := structuresWithMinimumWeightIn(t, "48", "40", domain.WeightUnitPound)
	if _, err := newPlanWithStructures(t, structures); !errors.Is(err, domain.ErrWeightUnitMismatch) {
		t.Fatalf("plan formation error = %v, want ErrWeightUnitMismatch", err)
	}
}

// 「取较大值」两边各表述一个金额，只查其中一个就会漏掉另一个。下游没有任何一处兜得住它：
// 费用合计以裸小数累加、最后才盖上方案币种，于是操作数里的一个外币金额会被当作方案本币
// 计入，而那次评价读起来仍是已完成。构造期是唯一能拒绝它的地方。
func TestPlanRefusesAGreaterOfWhoseOperandsDisagreeOnCurrency(t *testing.T) {
	floor, err := domain.NewFixedAmountSurcharge(money(t, "12", mustValue(t, domain.NewCurrency, "USD")))
	if err != nil {
		t.Fatalf("floor operand: %v", err)
	}
	foreign, err := domain.NewFixedAmountSurcharge(money(t, "999", mustValue(t, domain.NewCurrency, "EUR")))
	if err != nil {
		t.Fatalf("foreign operand: %v", err)
	}
	calculation, err := domain.NewGreaterOfSurcharge(floor, foreign)
	if err != nil {
		t.Fatalf("greater-of calculation: %v", err)
	}
	structures := declaredSurcharges(t, standaloneRule(t, ruleWithCalculation(t, calculation)))
	if _, err := newPlanWithStructures(t, structures); !errors.Is(err, domain.ErrCurrencyMismatch) {
		t.Fatalf("plan formation error = %v, want ErrCurrencyMismatch", err)
	}
}

func structuresWithDependency(t testing.TB, excluded string) domain.PricingPlanStructures {
	t.Helper()
	dependency, err := domain.NewAllChargesDependency(
		"fuel",
		mustValue(t, domain.NewChargeCode, "FUEL"),
		[]domain.ChargeCode{mustValue(t, domain.NewChargeCode, excluded)},
	)
	if err != nil {
		t.Fatalf("charge dependency: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{standaloneRule(t, surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10"))}, []domain.ChargeDependency{dependency},
		nil,
	)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func structuresWithGroupedSurcharge(t testing.TB, threshold, group string, priority int) domain.PricingPlanStructures {
	t.Helper()
	rule, err := surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", threshold, "10").InExclusivityGroup(group, priority)
	if err != nil {
		t.Fatalf("exclusivity group: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, nil, nil)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func structuresWithMinimumWeight(t testing.TB, threshold, minimum string) domain.PricingPlanStructures {
	t.Helper()
	return structuresWithMinimumWeightIn(t, threshold, minimum, domain.WeightUnitKilogram)
}

func structuresWithMinimumWeightIn(t testing.TB, threshold, minimum string, unit domain.WeightUnit) domain.PricingPlanStructures {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, threshold, domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	conditionalMinimum, err := domain.NewConditionalMinimumWeight(
		"oversize-minimum",
		leafTrigger(t, condition),
		weight(t, minimum, unit),
	)
	if err != nil {
		t.Fatalf("conditional minimum weight: %v", err)
	}
	rule, err := standaloneRule(t, surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", threshold, "10")).
		WithConditionalMinimumWeight(conditionalMinimum)
	if err != nil {
		t.Fatalf("attach conditional minimum: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, nil, nil)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func structuresWithSurcharge(t testing.TB, threshold string) domain.PricingPlanStructures {
	t.Helper()
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{standaloneRule(t, surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", threshold, "10"))},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func standaloneRule(t testing.TB, rule domain.SurchargeRule) domain.SurchargeRule {
	t.Helper()
	declared, err := rule.Standalone()
	if err != nil {
		t.Fatalf("standalone: %v", err)
	}
	return declared
}

func surchargeRule(t testing.TB, id, code, threshold, amount string) domain.SurchargeRule {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, threshold, domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	currency := mustValue(t, domain.NewCurrency, "USD")
	calculation, err := domain.NewFixedAmountSurcharge(money(t, amount, currency))
	if err != nil {
		t.Fatalf("calculation: %v", err)
	}
	rule, err := domain.NewSurchargeRule(
		id,
		mustValue(t, domain.NewChargeCode, code),
		id,
		domain.ChargeEffectAdd,
		leafTrigger(t, condition),
		calculation,
	)
	if err != nil {
		t.Fatalf("surcharge rule %s: %v", id, err)
	}
	return rule
}

// planWithStructures 让每次调用声明的版本引用完全一致，这样摘要的差异只可能来自被测的
// 那组结构。
func planWithStructures(t testing.TB, structures domain.PricingPlanStructures) domain.PricingPlanVersion {
	t.Helper()
	plan, err := newPlanWithStructures(t, structures)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}

func newPlanWithStructures(t testing.TB, structures domain.PricingPlanStructures) (domain.PricingPlanVersion, error) {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-structures"),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-structures", "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingNone, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("rounding policy: %v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-structures", "v1"),
		domain.PricingWeightActualOnly,
		rounding,
		nil,
	)
	if err != nil {
		t.Fatalf("pricing weight policy: %v", err)
	}
	return domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-structures", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell,
		domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t),
		table,
		weightPolicy,
		nil,
		structures,
	)
}
