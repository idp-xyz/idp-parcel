package parcelpricing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件把候选成本取数侧接到 parcel-pricing「对给定价卡形成试算」的入口（票 routing-first-cut/10、/20，
// ADR-0148 决定四、ADR-0189 决定一）。位置由 ADR-0025 定死：只有消费侧的 adapters/<provider>/ 可以导入
// 提供方上下文——「逐段各算各的计费重与价」、按卡的目录绑定选分区给法、补齐序列与目录读数，都在提供方那里
// 保证，这里只组装与合成。
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
	// errLegEstimateUndecided 说 parcel-pricing 补齐读数那几口读不回，这一段评价形成与否未知：整次取数报错、编排落依赖
	// 不可用的未决，不把一次故障记成这一段待判断。
	errLegEstimateUndecided = errors.New("network routing: leg estimate undecided")
)

// routeCostEvidence 是选路比价评价的证据层级。评价只为排候选，用完即弃，不入册、不交下游；取 S 的理由与受理前试算
// （cmd/parcel-dispatch 的 preAcceptanceTrialEvidence）同一条，将来按租户实例从登记册取时一并改，不在这里单独翻成 P。
const routeCostEvidence = ppdomain.EvidenceSynthetic

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

// ParcelFactsSource 交出一次路由判断的包裹事实。预路由取客户声明快照（network-routing CONTEXT，ADR-0148 决定四
// 第 7 条），来源随事实引用标明。第二个返回值为假即这件包裹此刻没有可用的事实：各段待判断，不代拟一份重量。
type ParcelFactsSource interface {
	ParcelFactsFor(
		ctx context.Context,
		key nrdomain.InitialRouteJudgmentKey,
	) (ParcelFacts, bool, error)
}

// ParcelFacts 是包裹事实：实重、可缺的外廓，以及它们出自哪一份事实。本上下文只转交，计价重量由 parcel-pricing 在
// 评价里按每张卡自己的体积系数派生。
type ParcelFacts struct {
	Weight     ppdomain.Weight
	Dimensions *ppdomain.Dimensions
	Facts      []ppdomain.VersionedFactReference
}

// LegZoneSource 交出一个计划段登的段计价区域（ADR-0189 决定二）。区域按卡给（决定一）：未绑分区目录的卡取这一格，
// 绑了目录的卡由 parcel-pricing 用包裹邮编路线经目录读出、不读它。哪张卡用哪一格由 parcel-pricing 按卡的目录绑定定，
// 本上下文把登的区与邮编路线一并交过去，不按卡分支、不自定区域。第二个返回值为假即这一段没登区。
type LegZoneSource interface {
	ZoneFor(ctx context.Context, query LegZoneQuery) (LegZone, bool, error)
}

// LegZoneQuery 指名要折区域的那一段。段号从 1 起，与出处里的段号同一口径。
type LegZoneQuery struct {
	Key       nrdomain.InitialRouteJudgmentKey
	Candidate nrdomain.CandidateID
	Ordinal   int
	Leg       nrdomain.PlannedLeg
}

// LegZone 是一段登的段计价区域，原样作为调用方给的分区交给 parcel-pricing。邮编路线不在这里：它是整件包裹的，
// 不随段变，取数侧从随请求携带的投影取一次、各段同交（LoadCandidateCosts）。
type LegZone struct {
	Zone string
}

// PlanGivenEstimates 是 parcel-pricing「对给定价卡形成试算」的入口（ppapplication.FormPlanGivenEstimateEvaluationsHandler
// 的窄面）：按卡的目录绑定造输入、补齐序列与目录读数、纯函数评价。补齐读数与正式评价、受理前估价同一份实现（ADR-0152
// 决定三），本上下文不另写：另写一份，择优用的成本与事后的供应商预期成本会在下一次改复核门时分叉。
type PlanGivenEstimates interface {
	Handle(
		ctx context.Context,
		command ppapplication.FormPlanGivenEstimateEvaluationsCommand,
	) (ppapplication.FormPlanGivenEstimateEvaluationsResult, error)
}

// Clock 给计价时点。与初始路由证据视图同取本上下文的路由判断时点：目录按它选版，价卡与汇率也按它选。
type Clock interface {
	Now() time.Time
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

// RouteCandidateCostDeps 收拢取数侧的依赖口。序列与目录读数不在这里：要哪一格由卡的绑定定，parcel-pricing 的入口自己补。
type RouteCandidateCostDeps struct {
	Bases     LineBasisRead
	Internal  InternalPlanResolver
	Facts     ParcelFactsSource
	Zones     LegZoneSource
	Estimates PlanGivenEstimates
	Plans     PlanVersionLoader
	Clock     Clock
}

// RouteCandidateCostAdapter 把一个判断的逐候选成本单维事实与逐段出处取齐。
type RouteCandidateCostAdapter struct {
	deps RouteCandidateCostDeps
}

func NewRouteCandidateCostAdapter(deps RouteCandidateCostDeps) *RouteCandidateCostAdapter {
	return &RouteCandidateCostAdapter{deps: deps}
}

var _ nrports.RouteCandidateCostSource = (*RouteCandidateCostAdapter)(nil)

// legEvaluation 是一次判断里一个计划段的取数侧结果：引用已解成在册的方案或已判缺依据，
// 评价之后携带它的作答。
type legEvaluation struct {
	ordinal       int
	candidate     nrdomain.CandidateID
	leg           nrdomain.PlannedLeg
	plan          ppdomain.PricingPlanVersion
	hasPlan       bool
	policyRef     string
	hasPolicy     bool
	evaluation    ppdomain.PricingEvaluation
	hasEvaluation bool
}

// LoadCandidateCosts 逐候选逐段取评价并按决定四合成（nrports.RouteCandidateCostSource）。
func (adapter *RouteCandidateCostAdapter) LoadCandidateCosts(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
	evidence nrports.InitialRouteEvidence,
	carried nrports.RequestCarriedContent,
) (nrports.RouteCandidateCosts, error) {
	none := nrports.RouteCandidateCosts{}
	var comparison *ppdomain.Currency
	if evidence.HasComparisonCurrency {
		currency, err := ppdomain.NewCurrency(evidence.ComparisonCurrency)
		if err != nil {
			return none, fmt.Errorf("%w: comparison currency %q: %v",
				errUntranslatableEvaluation, evidence.ComparisonCurrency, err)
		}
		comparison = &currency
	}

	legs, err := adapter.resolveLegs(ctx, key, evidence)
	if err != nil {
		return none, err
	}
	facts, found, err := adapter.deps.Facts.ParcelFactsFor(ctx, key)
	if err != nil {
		return none, fmt.Errorf("read parcel facts: %w", err)
	}
	// 没有包裹事实时一段都不评价，全部留作待判断：缺的是输入，不是整条取数路径没接。
	if found {
		if err := adapter.evaluateLegs(ctx, key, facts, comparison, postalRouteOf(carried.Geo), legs); err != nil {
			return none, err
		}
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
		for _, citation := range citations {
			out.Citations = append(out.Citations, nrports.CandidateLegCostCitation{
				Candidate: path.Candidate, PlannedLegCostCitation: citation,
			})
		}
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
		code, version, ok := path.Candidate.LineVersion()
		if !ok {
			return nil, fmt.Errorf("%w: candidate %q carries no line reference",
				errUntranslatableEvaluation, path.Candidate)
		}
		bases, err := adapter.deps.Bases.LoadLineCostBases(ctx, key.TenantID, code, version)
		if err != nil {
			return nil, fmt.Errorf("load line cost bases for %s: %w", code, err)
		}
		for ordinal := range path.Legs {
			entry := legEvaluation{ordinal: ordinal + 1, candidate: path.Candidate, leg: path.Legs[ordinal]}
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
			entry.plan = plan
			entry.hasPlan = true
			legs = append(legs, entry)
		}
	}
	return legs, nil
}

// evaluateLegs 逐段把包裹事实、这一段登的区与包裹的邮编路线交 parcel-pricing 对这一段的卡形成试算。卡要的那一格没有
// （没绑目录的卡没登区，绑了目录的卡没有邮编路线），那一段不评价，合成时它是待判断。
//
// 一段问一次而不是一次判断一份输入：各段的区不同，计价范围也取各段那张卡自己的——一条线路的各段可以引不同范围的卡。
// 「逐段各算各的计费重与体积系数」由评价口保证。
func (adapter *RouteCandidateCostAdapter) evaluateLegs(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
	facts ParcelFacts,
	comparison *ppdomain.Currency,
	route *ppdomain.PostalRoute,
	legs []legEvaluation,
) error {
	at := adapter.deps.Clock.Now()
	tenant, err := ppdomain.NewTenantID(key.TenantID.String())
	if err != nil {
		return fmt.Errorf("%w: tenant %q is not a parcel-pricing tenant: %v", errUntranslatableEvaluation, key.TenantID, err)
	}
	// 选路比价是试算：评价只为排候选，不形成任何费用，下游不得把它变成钱。
	subject := estimateReference(key)
	for index := range legs {
		leg := &legs[index]
		if !leg.hasPlan {
			continue
		}
		zone, registered, err := adapter.deps.Zones.ZoneFor(ctx, LegZoneQuery{
			Key: key, Candidate: leg.candidate, Ordinal: leg.ordinal, Leg: leg.leg,
		})
		if err != nil {
			return fmt.Errorf("fold leg %d of %s into a zone: %w", leg.ordinal, leg.candidate, err)
		}
		declaration := ppapplication.PlanGivenEstimateDeclaration{
			Subject:    subject,
			BasisAt:    at,
			Weight:     facts.Weight,
			Dimensions: facts.Dimensions,
			Route:      route,
			Comparison: comparison,
			Facts:      facts.Facts,
		}
		if registered {
			declaration.Zone = zone.Zone
		}
		result, err := adapter.deps.Estimates.Handle(ctx, ppapplication.FormPlanGivenEstimateEvaluationsCommand{
			Tenant:       tenant,
			Plan:         leg.plan,
			Evidence:     routeCostEvidence,
			Declarations: []ppapplication.PlanGivenEstimateDeclaration{declaration},
		})
		if err != nil {
			return fmt.Errorf("estimate leg %d of %s: %w", leg.ordinal, leg.candidate, err)
		}
		evaluation, evaluated, err := legEstimateOf(result)
		if err != nil {
			return fmt.Errorf("estimate leg %d of %s: %w", leg.ordinal, leg.candidate, err)
		}
		leg.evaluation, leg.hasEvaluation = evaluation, evaluated
	}
	return nil
}

// legEstimateOf 把一段的试算答复逐格译回：已评价交回评价；输入不全即这张卡要的区或邮编路线没有，这一段待判断；读不回是
// 依赖故障；未受理与认不出的形状是本桥译坏了，不留兜底（ADR-0031）。
func legEstimateOf(result ppapplication.FormPlanGivenEstimateEvaluationsResult) (ppdomain.PricingEvaluation, bool, error) {
	switch result.Outcome {
	case ppapplication.PlanGivenEstimatesFormed:
	case ppapplication.PlanGivenEstimatesUndecided:
		return ppdomain.PricingEvaluation{}, false, errLegEstimateUndecided
	case ppapplication.PlanGivenEstimatesNotAccepted:
		return ppdomain.PricingEvaluation{}, false, fmt.Errorf("%w: pricing refused the leg estimate command", errUntranslatableEvaluation)
	default:
		return ppdomain.PricingEvaluation{}, false, fmt.Errorf("%w: estimate outcome %d", errUntranslatableEvaluation, result.Outcome)
	}
	if len(result.Estimates) != 1 {
		return ppdomain.PricingEvaluation{}, false, fmt.Errorf("%w: %d estimates for one declaration", errUntranslatableEvaluation, len(result.Estimates))
	}
	estimate := result.Estimates[0]
	switch estimate.Answer {
	case ppapplication.EstimateCandidateEvaluated:
		return estimate.Evaluation, true, nil
	case ppapplication.EstimateCandidateInputIncomplete:
		return ppdomain.PricingEvaluation{}, false, nil
	default:
		return ppdomain.PricingEvaluation{}, false, fmt.Errorf("%w: estimate answer %d", errUntranslatableEvaluation, estimate.Answer)
	}
}

// postalRouteOf 从随请求携带的地理投影取包裹的邮编路线：寄件邮编为始发，收件邮编为目的，拼法取 parcel-pricing 那一份
// （PostalRouteFromDeclared）。投影没带、收件邮编没报或不成形即没有路线：绑了目录的卡各段待判断，不从节点推邮编
// （ADR-0189 决定一、三）。
func postalRouteOf(geo nrdomain.GeoResolutionProjection) *ppdomain.PostalRoute {
	if !geo.Carried() {
		return nil
	}
	destination, declared := geo.Delivery().PostalCode()
	if !declared {
		return nil
	}
	origin, _ := geo.Sender().PostalCode()
	route, formed := ppdomain.PostalRouteFromDeclared(origin, destination)
	if !formed {
		return nil
	}
	return &route
}

// estimateReference 由判断键确定性派生试算对象的引用：同一判断重算得到同一试算对象，评价可比对可复算。
func estimateReference(key nrdomain.InitialRouteJudgmentKey) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		key.TenantID.String(),
		key.CustomerAccountID.String(),
		key.ShipmentRequestID.String(),
		key.DeclaredParcelID.String(),
		key.ServicePurpose.String(),
	}, "\x00")))
	return "route-cost-" + hex.EncodeToString(digest[:8])
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
