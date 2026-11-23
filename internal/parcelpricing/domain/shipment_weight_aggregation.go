package domain

import "fmt"

// ShipmentWeightAggregation 是票级卡声明的计价重量聚合方式（ADR-0364 决定三）。
// 零值表示未声明：规范化版本早于这一键的票级卡按合计后派生读，键不写。
type ShipmentWeightAggregation string

const (
	// ShipmentWeightTotalsThenDerive 是合计实重与合计体积重之后按重量策略派生，即今天的读法。
	ShipmentWeightTotalsThenDerive ShipmentWeightAggregation = "TOTALS_THEN_DERIVE"
	// ShipmentWeightPerPieceThenSum 是逐件按重量策略派生之后求和。
	ShipmentWeightPerPieceThenSum ShipmentWeightAggregation = "PER_PIECE_THEN_SUM"
)

func (method ShipmentWeightAggregation) String() string {
	switch method {
	case ShipmentWeightTotalsThenDerive:
		return string(ShipmentWeightTotalsThenDerive)
	case ShipmentWeightPerPieceThenSum:
		return string(ShipmentWeightPerPieceThenSum)
	default:
		return ""
	}
}

func (method ShipmentWeightAggregation) valid() bool {
	return method.String() != ""
}

// ShipmentWeightRoundingPoint 是逐件求和时的进位点。合计后派生不带这一格。两种不叠：要么逐件进位后求和，要么求和之后进位一次。
type ShipmentWeightRoundingPoint string

const (
	// ShipmentWeightRoundEachThenSum 是逐件进位后求和。
	ShipmentWeightRoundEachThenSum ShipmentWeightRoundingPoint = "ROUND_EACH_THEN_SUM"
	// ShipmentWeightSumThenRoundOnce 是求和之后进位一次。
	ShipmentWeightSumThenRoundOnce ShipmentWeightRoundingPoint = "SUM_THEN_ROUND_ONCE"
)

func (point ShipmentWeightRoundingPoint) String() string {
	switch point {
	case ShipmentWeightRoundEachThenSum:
		return string(ShipmentWeightRoundEachThenSum)
	case ShipmentWeightSumThenRoundOnce:
		return string(ShipmentWeightSumThenRoundOnce)
	default:
		return ""
	}
}

func (point ShipmentWeightRoundingPoint) valid() bool {
	return point.String() != ""
}

// ShipmentWeightAggregation 交回票级卡声明的聚合方式。未声明时第二个返回值为假——那时按合计后派生读，与引入这一键之前同一条路。
func (plan PricingPlanVersion) ShipmentWeightAggregation() (ShipmentWeightAggregation, bool) {
	if plan.shipmentWeightAggregation == "" {
		return "", false
	}
	return plan.shipmentWeightAggregation, true
}

// ShipmentWeightRoundingPoint 只在逐件求和时给出进位点。
func (plan PricingPlanVersion) ShipmentWeightRoundingPoint() (ShipmentWeightRoundingPoint, bool) {
	if plan.shipmentWeightRoundingPoint == "" {
		return "", false
	}
	return plan.shipmentWeightRoundingPoint, true
}

// WithShipmentWeightAggregation 交回声明了票级计价重量聚合方式的副本。只有逐委托的卡能声明；主单级与逐包裹不开这一选项。
// 逐件求和必须同时给出进位点，合计后派生不带进位点。声明了的卡写 PPC-8，规范化版本与摘要随之重算。
func (plan PricingPlanVersion) WithShipmentWeightAggregation(method ShipmentWeightAggregation, point ShipmentWeightRoundingPoint) (PricingPlanVersion, error) {
	if !plan.valid() {
		return PricingPlanVersion{}, ErrInvalidPricingPlan
	}
	if plan.aggregation != AggregationPerShipment {
		return PricingPlanVersion{}, ErrShipmentWeightAggregationOnlyOnShipment
	}
	if !method.valid() {
		return PricingPlanVersion{}, fmt.Errorf("%w: shipment weight aggregation must name one of the two readings", ErrInvalidPricingPlan)
	}
	switch method {
	case ShipmentWeightTotalsThenDerive:
		if point != "" {
			return PricingPlanVersion{}, fmt.Errorf("%w: totals-then-derive does not declare a rounding point", ErrInvalidPricingPlan)
		}
	case ShipmentWeightPerPieceThenSum:
		if !point.valid() {
			return PricingPlanVersion{}, fmt.Errorf("%w: per-piece summing must declare where rounding happens", ErrInvalidPricingPlan)
		}
	default:
		return PricingPlanVersion{}, ErrInvalidPricingPlan
	}
	plan.shipmentWeightAggregation = method
	plan.shipmentWeightRoundingPoint = point
	plan.canonicalization = plan.requiredCanonicalization()
	plan.contentDigest = calculatePricingPlanContentDigest(plan)
	return plan, nil
}

// shipmentWeightHolds 是读回门上的那一道：未声明则两格都空；声明了就只在逐委托的卡上，进位点与聚合方式成对。
func (plan PricingPlanVersion) shipmentWeightHolds() bool {
	method, point := plan.shipmentWeightAggregation, plan.shipmentWeightRoundingPoint
	if method == "" && point == "" {
		return true
	}
	if plan.aggregation != AggregationPerShipment || !method.valid() {
		return false
	}
	switch method {
	case ShipmentWeightTotalsThenDerive:
		return point == ""
	case ShipmentWeightPerPieceThenSum:
		return point.valid()
	default:
		return false
	}
}

// explainShipmentWeightAggregation 写下这次评价用的票级聚合读法。未声明记「按旧版读法」，声明了的记下是哪一种、进位在哪。
// 主单级与逐包裹没有这一格，不写。解释不进语义摘要。
func (plan PricingPlanVersion) explainShipmentWeightAggregation() string {
	if plan.aggregation != AggregationPerShipment {
		return ""
	}
	if plan.shipmentWeightAggregation == "" {
		return "shipment weight aggregation undeclared, read as totals then derive"
	}
	if plan.shipmentWeightAggregation == ShipmentWeightPerPieceThenSum {
		return fmt.Sprintf("shipment weight aggregation %s; rounding point %s", plan.shipmentWeightAggregation, plan.shipmentWeightRoundingPoint)
	}
	return fmt.Sprintf("shipment weight aggregation %s", plan.shipmentWeightAggregation)
}

// calculatePricingWeightForPlan 按卡声明的票级聚合方式派生计价重量。未声明与合计后派生走原来那条合计路，逐字节不变；
// 逐件求和另走一件一件的路，缺尺寸时不退回合计。
func calculatePricingWeightForPlan(request EvaluationRequest, converter *unitConverter, floors ...Weight) (PricingWeightResult, error) {
	if request.plan.aggregation == AggregationPerShipment && request.plan.shipmentWeightAggregation == ShipmentWeightPerPieceThenSum {
		return calculatePerPiecePricingWeight(request.input, request.plan.weight, request.plan.shipmentWeightRoundingPoint, request.plan.rateTable.unit, converter, floors...)
	}
	// 合计后派生与未声明走导出的 CalculatePricingWeight：不换算的那一遍必须经过它，接线棘轮才认这个工厂仍在生产路径上。
	if converter == nil {
		return CalculatePricingWeight(request.input, request.plan.weight, request.plan.rateTable.unit, floors...)
	}
	return calculatePricingWeight(request.input, request.plan.weight, request.plan.rateTable.unit, converter, floors...)
}

// calculatePerPiecePricingWeight 逐件按重量策略派生再求和。体积重按卡的体积系数逐件算；卡要体积重而缺任一件的尺寸时答缺尺寸，
// 不读成员清单上的合计体积重。条件最低计价重量作用在和上：逐件进位时和已经进过位，不再对和进一次；求和之后进位时，抬高发生在这一次进位之前。
func calculatePerPiecePricingWeight(input PricingInputSnapshot, policy PricingWeightPolicy, point ShipmentWeightRoundingPoint, expectedUnit WeightUnit, converter *unitConverter, floors ...Weight) (PricingWeightResult, error) {
	if !input.valid() || !policy.valid() || !point.valid() {
		return PricingWeightResult{}, ErrPricingInputInvalid
	}
	if policy.rounding.unit() != expectedUnit {
		return PricingWeightResult{}, ErrWeightUnitMismatch
	}
	manifest, ok := input.Members()
	if !ok {
		return PricingWeightResult{}, ErrPricingInputInvalid
	}
	pieces, declared := manifest.Pieces()
	if !declared {
		return PricingWeightResult{}, ErrMissingPieceMeasurements
	}
	actuals := make([]Weight, 0, len(pieces))
	var volumetrics []Weight
	derived := make([]Weight, 0, len(pieces))
	boundaryScale := uint32(0)
	allowTruncation := false
	if converter != nil {
		boundaryScale = conversionBoundaryScale(input, policy, floors)
		allowTruncation = policy.rounding.allowsTruncation()
	}
	for _, piece := range pieces {
		actual := piece.ActualWeight()
		if actual.Unit() != expectedUnit {
			if converter == nil {
				return PricingWeightResult{}, ErrWeightUnitMismatch
			}
			converted, err := converter.convertWeight(ConvertedActualWeight, actual, expectedUnit, boundaryScale, allowTruncation)
			if err != nil {
				return PricingWeightResult{}, err
			}
			actual = converted
		}
		raw := actual
		switch policy.Method() {
		case PricingWeightActualOnly:
		case PricingWeightMax:
			sides, measured := piece.Dimensions()
			if !measured {
				return PricingWeightResult{}, ErrMissingDimensions
			}
			volumetric, err := pieceVolumetric(sides, policy, converter)
			if err != nil {
				return PricingWeightResult{}, err
			}
			if volumetric.Unit() != expectedUnit {
				return PricingWeightResult{}, ErrWeightUnitMismatch
			}
			volumetrics = append(volumetrics, volumetric)
			if volumetric.value.Cmp(actual.value) > 0 {
				raw = volumetric
			}
		default:
			return PricingWeightResult{}, ErrPricingInputInvalid
		}
		actuals = append(actuals, actual)
		derived = append(derived, raw)
	}
	actualSum, err := sumWeights(actuals)
	if err != nil {
		return PricingWeightResult{}, err
	}
	var volumetricSum *Weight
	if len(volumetrics) > 0 {
		summed, sumErr := sumWeights(volumetrics)
		if sumErr != nil {
			return PricingWeightResult{}, sumErr
		}
		volumetricSum = &summed
	}
	switch point {
	case ShipmentWeightRoundEachThenSum:
		roundedPieces := make([]Weight, 0, len(derived))
		for _, raw := range derived {
			rounded, _, roundErr := policy.rounding.Apply(raw)
			if roundErr != nil {
				return PricingWeightResult{}, roundErr
			}
			roundedPieces = append(roundedPieces, rounded)
		}
		summed, sumErr := sumWeights(roundedPieces)
		if sumErr != nil {
			return PricingWeightResult{}, sumErr
		}
		return finishPerPiecePricingWeight(policy, point, actualSum, volumetricSum, summed, true, floors...)
	case ShipmentWeightSumThenRoundOnce:
		summed, sumErr := sumWeights(derived)
		if sumErr != nil {
			return PricingWeightResult{}, sumErr
		}
		return finishPerPiecePricingWeight(policy, point, actualSum, volumetricSum, summed, false, floors...)
	default:
		return PricingWeightResult{}, ErrPricingInputInvalid
	}
}

// pieceVolumetric 用卡的体积系数算一件的体积重。长度单位与系数不一致且没有换算器时答不一致；
// 有换算器时走已有的尺寸换算，精确体积一次算完，不先把三边截成十进制。缺尺寸由调用方答，不退回合计体积重。
func pieceVolumetric(sides Dimensions, policy PricingWeightPolicy, converter *unitConverter) (Weight, error) {
	factor, declared := policy.VolumetricFactor()
	if !declared {
		return Weight{}, ErrInvalidRoundingPolicy
	}
	if sides.unit != factor.lengthUnit {
		if converter == nil {
			return Weight{}, ErrLengthUnitMismatch
		}
		return converter.volumetricWeightFromDimensions(sides, factor)
	}
	volume, err := sides.Volume()
	if err != nil {
		return Weight{}, err
	}
	return factor.Apply(volume)
}

// finishPerPiecePricingWeight 把逐件得出的和收成方案级计价重量。alreadyRounded 为真时和已经逐件进过位，未抬高就不再进一次；
// 抬高了才对抬高后的和进一次，与合计后派生「先抬高、再进位」同一个位置。
func finishPerPiecePricingWeight(policy PricingWeightPolicy, point ShipmentWeightRoundingPoint, actual Weight, volumetric *Weight, derived Weight, alreadyRounded bool, floors ...Weight) (PricingWeightResult, error) {
	raw := derived
	raised := false
	for _, floor := range floors {
		if !floor.valid() || floor.unit != raw.unit {
			return PricingWeightResult{}, ErrWeightUnitMismatch
		}
		if floor.value.Cmp(raw.value) > 0 {
			raw, raised = floor, true
		}
	}
	var rounded Weight
	var segment WeightRoundingSegment
	var err error
	if alreadyRounded && !raised {
		rounded = derived
		segment, err = policy.rounding.segmentFor(derived)
	} else {
		rounded, segment, err = policy.rounding.Apply(raw)
	}
	if err != nil {
		return PricingWeightResult{}, err
	}
	raiseText := ""
	if raised {
		raiseText = fmt.Sprintf("; raised from %s %s to %s %s by a conditional minimum", derived.value.String(), derived.unit, raw.value.String(), raw.unit)
	}
	explanation := fmt.Sprintf("pricing weight uses %s; shipment aggregation %s; rounding point %s; raw=%s %s%s; rounding=%s increment=%s %s%s; rounded=%s %s",
		policy.method, ShipmentWeightPerPieceThenSum, point, derived.value.String(), derived.unit, raiseText,
		segment.mode, segment.increment.value.String(), segment.increment.unit, roundingSegmentScope(segment),
		rounded.value.String(), rounded.unit)
	return PricingWeightResult{
		method:       policy.method,
		actual:       actual,
		volumetric:   volumetric,
		raw:          raw,
		rounded:      rounded,
		roundingMode: segment.mode,
		increment:    segment.increment,
		explanation:  explanation,
	}, nil
}

func sumWeights(weights []Weight) (Weight, error) {
	if len(weights) == 0 {
		return Weight{}, ErrPricingInputInvalid
	}
	total := weights[0]
	for _, next := range weights[1:] {
		if next.unit != total.unit {
			return Weight{}, ErrWeightUnitMismatch
		}
		added, err := total.value.Add(next.value)
		if err != nil {
			return Weight{}, err
		}
		summed, err := NewWeight(added, total.unit)
		if err != nil {
			return Weight{}, err
		}
		total = summed
	}
	return total, nil
}
