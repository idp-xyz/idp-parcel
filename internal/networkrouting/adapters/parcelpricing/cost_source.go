package parcelpricing

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件把候选成本取数侧接到 parcel-pricing 的批量评价口（票 routing-first-cut/10，
// ADR-0148 决定四）。位置由 ADR-0025 定死：只有消费侧的 adapters/<provider>/ 可以导入
// 提供方上下文——「逐段各算各的计费重与价」在提供方那里保证，这里只组装与合成。
//
// 两条纪律写死在实现里：任一段待判断 / 不可计价 / 没挂依据，该候选即缺成本依据，金额不
// 以零或他段顶替；合成按不舍入精度相加、合计后按比较币种最小币单位取整一次——逐段先
// 取整会随段数累积舍入误差，改变并列判断。

var (
	// errUntranslatableEvaluation 说提供方交回了本桥认不出的形状（含引用译不出、评价
	// 缺比较金额、方向配错）：逐格分派不留兜底，认不出就上抛（ADR-0031）。
	errUntranslatableEvaluation = errors.New("network routing: untranslatable pricing evaluation")
	// errNotACostEvaluation 说方案的方向与用途答的不是「运营企业为这一段花多少」。
	errNotACostEvaluation = errors.New("network routing: pricing plan is not a leg cost basis")
)

// LineBasisRead 读一条线路版本的逐段成本依据（network-routing 目录册的窄面）。
type LineBasisRead interface {
	LoadLineCostBases(
		ctx context.Context,
		tenant nrdomain.TenantID,
		lineCode string,
		version int32,
	) ([]nrports.LineSegmentCostBasis, error)
}

// InternalPlanResolver 把内部政策引用解成绑定方案引用（adapters/partycommercial 的同名桥）。
type InternalPlanResolver interface {
	PlanReferenceForInternalPolicy(
		ctx context.Context,
		tenant nrdomain.TenantID,
		policyReference string,
	) (string, bool, error)
}

// PricingInputSource 把一次路由判断折成计价输入快照（实例半边：包裹事实从哪取是消费方的
// 租户取值路径）。第二个返回值为假即「未配置」：缺席如实交出来，不代拟一份。
type PricingInputSource interface {
	PricingInputFor(
		ctx context.Context,
		key nrdomain.InitialRouteJudgmentKey,
	) (ppdomain.PricingInputSnapshot, bool, error)
}

// PlanVersionLoader 读回引用指名的那一版方案（parcel-pricing 的 PriceCardVersionLoader 窄面）。
// 租户按提供方自己的词汇收（不同上下文的 TenantID 是各自的值类型）。
type PlanVersionLoader interface {
	FindByReference(
		ctx context.Context,
		tenant ppdomain.TenantID,
		reference ppdomain.VersionReference,
	) (ppdomain.PricingPlanVersion, bool, error)
}

// RouteCandidateCostDeps 收拢取数侧的四个依赖口。
type RouteCandidateCostDeps struct {
	Bases    LineBasisRead
	Internal InternalPlanResolver
	Input    PricingInputSource
	Plans    PlanVersionLoader
}

// RouteCandidateCostAdapter 把一个判断的逐候选成本单维事实与逐段出处取齐。
type RouteCandidateCostAdapter struct {
	deps RouteCandidateCostDeps
}

func NewRouteCandidateCostAdapter(deps RouteCandidateCostDeps) *RouteCandidateCostAdapter {
	return &RouteCandidateCostAdapter{deps: deps}
}

var _ nrports.RouteCandidateCostSource = (*RouteCandidateCostAdapter)(nil)

// legEvaluation 是一次判断里一个计划段的取数侧结果：引用已解成可评价目标或已判缺依据，
// 评价之后携带它的作答。
type legEvaluation struct {
	ordinal       int
	policyRef     string
	hasPolicy     bool
	target        ppdomain.PlanEvaluationTarget
	hasTarget     bool
	evaluation    ppdomain.PricingEvaluation
	hasEvaluation bool
}

// LoadCandidateCosts 逐候选逐段取评价并按决定四合成（nrports.RouteCandidateCostSource）。
func (adapter *RouteCandidateCostAdapter) LoadCandidateCosts(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
	evidence nrports.InitialRouteEvidence,
) (nrports.RouteCandidateCosts, error) {
	none := nrports.RouteCandidateCosts{}
	input, configured, err := adapter.deps.Input.PricingInputFor(ctx, key)
	if err != nil {
		return none, fmt.Errorf("form pricing input: %w", err)
	}
	if !configured {
		return none, nrports.ErrRouteCostSourceNotConfigured
	}
	snapshot := input
	if evidence.HasComparisonCurrency {
		currency, err := ppdomain.NewCurrency(evidence.ComparisonCurrency)
		if err != nil {
			return none, fmt.Errorf("%w: comparison currency %q: %v",
				errUntranslatableEvaluation, evidence.ComparisonCurrency, err)
		}
		snapshot, err = snapshot.WithComparisonCurrency(currency)
		if err != nil {
			return none, fmt.Errorf("declare comparison currency: %w", err)
		}
	}

	legs, err := adapter.resolveLegs(ctx, key, evidence)
	if err != nil {
		return none, err
	}

	// 「逐候选各算各的计费重与体积系数」由批量口保证：同一份输入对全部目标逐份评价。
	carded := make([]ppdomain.PlanEvaluationTarget, 0)
	cardedOf := make(map[int]int)
	for index := range legs {
		if !legs[index].hasTarget {
			continue
		}
		cardedOf[index] = len(carded)
		carded = append(carded, legs[index].target)
	}
	evaluations := ppdomain.EvaluatePricingAcrossPlans(snapshot, ppdomain.EvidenceSynthetic, carded)
	if len(evaluations) != len(carded) {
		// 批量口的契约是等长对位。它不成立时这一批评价接不回段，只能上抛。
		return none, fmt.Errorf("%w: %d evaluations for %d targets",
			errUntranslatableEvaluation, len(evaluations), len(carded))
	}
	for index, evaluation := range evaluations {
		legs[cardedOf[index]].evaluation = evaluation
		legs[cardedOf[index]].hasEvaluation = true
	}

	// 合成：逐候选把各段折成三态之一，并记逐段出处。
	out := nrports.RouteCandidateCosts{}
	offset := 0
	for _, path := range evidence.Paths {
		count := len(path.Legs)
		legSlice := legs[offset : offset+count]
		offset += count

		citations, amountMinor, currency, pending, err := adapter.compose(legSlice, evidence.HasComparisonCurrency)
		if err != nil {
			return none, err
		}
		out.Citations = append(out.Citations, citations...)
		switch {
		case pending:
			fact, err := nrdomain.NewPendingCandidateCost(path.Candidate)
			if err != nil {
				return none, err
			}
			out.Facts = append(out.Facts, fact)
		case amountMinor == nil:
			fact, err := nrdomain.NewUnpriceableCandidateCost(path.Candidate)
			if err != nil {
				return none, err
			}
			out.Facts = append(out.Facts, fact)
		default:
			fact, err := nrdomain.NewPricedCandidateCost(path.Candidate, *amountMinor, currency)
			if err != nil {
				return none, err
			}
			out.Facts = append(out.Facts, fact)
		}
	}
	return out, nil
}

// resolveLegs 逐候选逐段解引用：外包段引用即方案「id/version」；内部段经解析桥拿方案引用；
// 没挂依据、政策没登正文或方案不在册，段如实缺目标（待判断）。段号对段链数组下标。
func (adapter *RouteCandidateCostAdapter) resolveLegs(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
	evidence nrports.InitialRouteEvidence,
) ([]legEvaluation, error) {
	pricingTenant, err := ppdomain.NewTenantID(key.TenantID.String())
	if err != nil {
		return nil, fmt.Errorf("%w: tenant %q is not a parcel-pricing tenant: %v",
			errUntranslatableEvaluation, key.TenantID, err)
	}
	var legs []legEvaluation
	for _, path := range evidence.Paths {
		code, versionRaw, ok := strings.Cut(path.Candidate.String(), "/")
		if !ok || code == "" || versionRaw == "" {
			return nil, fmt.Errorf("%w: candidate %q carries no line reference",
				errUntranslatableEvaluation, path.Candidate)
		}
		version64, err := strconv.ParseInt(versionRaw, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("%w: candidate %q version: %v",
				errUntranslatableEvaluation, path.Candidate, err)
		}
		bases, err := adapter.deps.Bases.LoadLineCostBases(ctx, key.TenantID, code, int32(version64))
		if err != nil {
			return nil, fmt.Errorf("load line cost bases for %s: %w", code, err)
		}
		for ordinal := range path.Legs {
			entry := legEvaluation{ordinal: ordinal + 1}
			basis := basisAt(bases, ordinal)
			if basis == nil {
				// 段没挂依据：缺依据如实待判断，不以零或邻段顶替。
				legs = append(legs, entry)
				continue
			}
			planRef := basis.Reference
			if basis.Kind == nrports.InternalPolicyBasis {
				resolved, found, err := adapter.deps.Internal.PlanReferenceForInternalPolicy(ctx, key.TenantID, basis.Reference)
				if err != nil {
					return nil, err
				}
				if !found {
					// 政策版本在而正文没登：缺依据，待判断。
					legs = append(legs, entry)
					continue
				}
				planRef = resolved
				entry.policyRef = basis.Reference
				entry.hasPolicy = true
			}
			reference, err := planReference(planRef)
			if err != nil {
				return nil, err
			}
			plan, found, err := adapter.deps.Plans.FindByReference(ctx, pricingTenant, reference)
			if err != nil {
				return nil, fmt.Errorf("load plan %s: %w", planRef, err)
			}
			if !found {
				// 引用指名的方案不在册：补登即出价，归待判断。
				legs = append(legs, entry)
				continue
			}
			if err := checkPlanDirection(*basis, plan); err != nil {
				return nil, err
			}
			evaluationID, err := ppdomain.NewEvaluationID(
				fmt.Sprintf("route-cost-%s-leg-%d", path.Candidate, ordinal+1))
			if err != nil {
				return nil, fmt.Errorf("form evaluation id: %w", err)
			}
			target, err := ppdomain.NewPlanEvaluationTarget(evaluationID, plan)
			if err != nil {
				return nil, fmt.Errorf("form evaluation target: %w", err)
			}
			entry.target = target
			entry.hasTarget = true
			legs = append(legs, entry)
		}
	}
	return legs, nil
}

// compose 把一条候选的逐段作答折成出处列表、合成金额（nil 即没合出来）与「是否待判断」。
// 任一段待判断即整体待判断；没有待判断也没有金额即整体不可计价；全段已计价则不舍入
// 求和、按比较币种最小币单位取整一次。
func (adapter *RouteCandidateCostAdapter) compose(
	legs []legEvaluation,
	hasComparison bool,
) ([]nrdomain.PlannedLegCostCitation, *int64, string, bool, error) {
	var citations []nrdomain.PlannedLegCostCitation
	var sum ppdomain.Money
	sumStarted := false
	for _, leg := range legs {
		if !leg.hasEvaluation {
			return nil, nil, "", true, nil
		}
		switch leg.evaluation.Status() {
		case ppdomain.EvaluationCompleted:
			amount, ok := comparisonAmountOf(leg.evaluation)
			if !ok {
				return nil, nil, "", false, fmt.Errorf("%w: completed evaluation %s carries no total",
					errUntranslatableEvaluation, leg.evaluation.ID())
			}
			if sumStarted {
				if amount.Currency().String() != sum.Currency().String() {
					if hasComparison {
						// 声明了比较币种时提供方各段应齐币种——不一致是桥接不出，不是缺依据。
						return nil, nil, "", false, fmt.Errorf("%w: leg %d compared in %s while others in %s",
							errUntranslatableEvaluation, leg.ordinal, amount.Currency(), sum.Currency())
					}
					// 没登比较币种的退路：只在同币种下合成（ADR-0148 决定四.4），异币种即
					// 停下不比——不换算、不编汇率。
					return nil, nil, "", true, nil
				}
				var err error
				sum, err = sum.Add(amount)
				if err != nil {
					return nil, nil, "", false, fmt.Errorf("sum leg amounts: %w", err)
				}
			} else {
				sum, sumStarted = amount, true
			}
			citation, err := citationOf(leg, amount)
			if err != nil {
				return nil, nil, "", false, err
			}
			citations = append(citations, citation)
		case ppdomain.EvaluationPending:
			return nil, nil, "", true, nil
		default:
			// 不可计价 / 冲突 / 失败：改价卡或候选集合的事，不是等——整体不可计价。
			return nil, nil, "", false, nil
		}
	}
	minor, err := minorUnitsOnce(sum)
	if err != nil {
		return nil, nil, "", false, err
	}
	return citations, &minor, sum.Currency().String(), false, nil
}

// comparisonAmountOf 取该段的比较金额：声明了比较币种时用全精度比较值（不过卡的取整点）；
// 没声明时退回评价总价（结算侧已按卡的策略取整）——退路模式的精度松弛由此而来，真相
// 照实记在出处里。
func comparisonAmountOf(evaluation ppdomain.PricingEvaluation) (ppdomain.Money, bool) {
	if amount, ok := evaluation.ComparisonAmount(); ok {
		return amount, true
	}
	return evaluation.Total()
}

// citationOf 记一段已计价段落的出处：评价标识、按之计价的方案引用、（内部段的）政策引用，
// 以及全精度比较金额。
func citationOf(leg legEvaluation, amount ppdomain.Money) (nrdomain.PlannedLegCostCitation, error) {
	plan := leg.evaluation.PlanReference()
	return nrdomain.NewPlannedLegCostCitation(nrdomain.PlannedLegCostCitationSpec{
		Ordinal:            leg.ordinal,
		EvaluationID:       leg.evaluation.ID().String(),
		PlanReference:      plan.ID() + "/" + plan.Version(),
		PolicyReference:    leg.policyRef,
		HasPolicy:          leg.hasPolicy,
		ComparisonAmount:   amount.Amount().String(),
		ComparisonCurrency: amount.Currency().String(),
	})
}

// minorUnitsOnce 把精确合计按币种最小币单位取整一次（ADR-0148 决定四.3），折成最小币单位
// 整数。取整模式 HALF_UP：比较口径的取整模式没有别的权威声明，按行业惯例内置。
func minorUnitsOnce(sum ppdomain.Money) (int64, error) {
	scale := sum.Currency().MinorUnitScale()
	increment, err := ppdomain.ParseDecimal(minorUnitIncrements[scale])
	if err != nil {
		return 0, fmt.Errorf("minor unit increment: %w", err)
	}
	rounded, err := sum.Amount().RoundToIncrement(increment, ppdomain.RoundingHalfUp)
	if err != nil {
		return 0, fmt.Errorf("round comparison total: %w", err)
	}
	shift, err := ppdomain.ParseDecimal(minorUnitShifts[scale])
	if err != nil {
		return 0, fmt.Errorf("minor unit shift: %w", err)
	}
	scaled, err := rounded.Mul(shift)
	if err != nil {
		return 0, fmt.Errorf("scale comparison total: %w", err)
	}
	minor, err := strconv.ParseInt(scaled.String(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: comparison total %q does not parse as minor units",
			errUntranslatableEvaluation, scaled.String())
	}
	return minor, nil
}

// minorUnitIncrements 是 10^(-scale) 的小表；minorUnitShifts 是它的倒数（10^scale）。
// scale 由 Currency.MinorUnitScale 给，封闭在 0..3（ISO 4217 的三种最小币单位小数位）。
var (
	minorUnitIncrements = map[uint8]string{
		0: "1",
		1: "0.1",
		2: "0.01",
		3: "0.001",
	}
	minorUnitShifts = map[uint8]string{
		0: "1",
		1: "10",
		2: "100",
		3: "1000",
	}
)

// basisAt 找段号对应的依据行；缺行答 nil（没挂依据）。
func basisAt(bases []nrports.LineSegmentCostBasis, ordinal int) *nrports.LineSegmentCostBasis {
	for index := range bases {
		if bases[index].SegmentIndex == ordinal {
			return &bases[index]
		}
	}
	return nil
}

// planReference 把「id/version」引用串译回方案版本引用（身份三元，不猜指纹）。
func planReference(raw string) (ppdomain.VersionReference, error) {
	id, version, ok := strings.Cut(raw, "/")
	if !ok || id == "" || version == "" {
		return ppdomain.VersionReference{}, fmt.Errorf("%w: plan reference %q", errUntranslatableEvaluation, raw)
	}
	return ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactPricingPlan, id, version)
}

// checkPlanDirection 按依据种类核方案方向与用途：外包段要供应商采购成本价卡，内部段要
// 内部价格价卡。方向配错是目录引用的错，响亮拒译而不是译出一个金额。
func checkPlanDirection(basis nrports.LineSegmentCostBasis, plan ppdomain.PricingPlanVersion) error {
	switch basis.Kind {
	case nrports.SupplierBuyPlanBasis:
		if plan.Direction() == ppdomain.PricingDirectionBuy && plan.Purpose() == ppdomain.PricingPurposeSupplierCost {
			return nil
		}
	case nrports.InternalPolicyBasis:
		if plan.Direction() == ppdomain.PricingDirectionInternal && plan.Purpose() == ppdomain.PricingPurposeInternalPrice {
			return nil
		}
	}
	return fmt.Errorf("%w: kind %s direction %q purpose %q",
		errNotACostEvaluation, basis.Kind, plan.Direction(), plan.Purpose())
}
