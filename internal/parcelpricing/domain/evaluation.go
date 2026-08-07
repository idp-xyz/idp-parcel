package domain

import (
	"errors"
	"fmt"
	"strings"
)

type EvaluationStatus string

const (
	EvaluationCompleted EvaluationStatus = "COMPLETED"
	EvaluationPending   EvaluationStatus = "PENDING"
	EvaluationConflict  EvaluationStatus = "CONFLICT"
	EvaluationFailed    EvaluationStatus = "FAILED"
	// EvaluationUnratable is the card saying no rather than the evaluator
	// falling short. CONTEXT keeps the four outcomes from standing in for one
	// another: PENDING promises that supplying more facts can produce a price,
	// which is false here, and FAILED already means a technical or structural
	// inability, which a caller answers with a retry rather than with a
	// business decision.
	EvaluationUnratable EvaluationStatus = "UNRATABLE"
)

func (status EvaluationStatus) valid() bool {
	switch status {
	case EvaluationCompleted, EvaluationPending, EvaluationConflict, EvaluationFailed, EvaluationUnratable:
		return true
	default:
		return false
	}
}

type EvaluationIssue struct {
	code    string
	message string
}

func newEvaluationIssue(code, message string) EvaluationIssue {
	return EvaluationIssue{code: code, message: message}
}

func (issue EvaluationIssue) Code() string    { return issue.code }
func (issue EvaluationIssue) Message() string { return issue.message }

type ChargeLineKind string

const (
	ChargeLineBase      ChargeLineKind = "BASE"
	ChargeLineFixed     ChargeLineKind = "FIXED"
	ChargeLineSurcharge ChargeLineKind = "SURCHARGE"
)

type ChargeLine struct {
	id          string
	kind        ChargeLineKind
	chargeCode  ChargeCode
	scope       ChargeScope
	basis       ChargeBasis
	method      ChargeMethod
	description string
	effect      ChargeEffect
	amount      Money
	order       int
	sourceRef   string
}

func newChargeLine(id string, kind ChargeLineKind, code ChargeCode, scope ChargeScope, basis ChargeBasis, method ChargeMethod, description string, effect ChargeEffect, amount Money, order int, sourceRef string) (ChargeLine, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id || !code.valid() || !scope.valid() || !basis.valid() || !method.valid() || strings.TrimSpace(description) == "" || strings.TrimSpace(description) != description || !effect.valid() || !amount.valid() || order < 0 || strings.TrimSpace(sourceRef) == "" || strings.TrimSpace(sourceRef) != sourceRef {
		return ChargeLine{}, ErrInvalidChargeLine
	}
	switch kind {
	case ChargeLineBase:
		if scope != ChargeScopePackage || basis != ChargeBasisRateEntry || method != ChargeMethodTableLookup || effect != ChargeEffectAdd || order != 0 {
			return ChargeLine{}, ErrInvalidChargeLine
		}
	case ChargeLineFixed, ChargeLineSurcharge:
		if scope != ChargeScopePackage || basis != ChargeBasisFixedAmount || method != ChargeMethodFixedAmount || order < 1 {
			return ChargeLine{}, ErrInvalidChargeLine
		}
	default:
		return ChargeLine{}, ErrInvalidChargeLine
	}
	return ChargeLine{id: id, kind: kind, chargeCode: code, scope: scope, basis: basis, method: method, description: description, effect: effect, amount: amount, order: order, sourceRef: sourceRef}, nil
}

func newBaseChargeLine(id string, code ChargeCode, description string, amount Money, sourceRef string) (ChargeLine, error) {
	return newChargeLine(id, ChargeLineBase, code, ChargeScopePackage, ChargeBasisRateEntry, ChargeMethodTableLookup, description, ChargeEffectAdd, amount, 0, sourceRef)
}

func newFixedChargeLine(id string, code ChargeCode, description string, effect ChargeEffect, amount Money, order int, sourceRef string) (ChargeLine, error) {
	return newChargeLine(id, ChargeLineFixed, code, ChargeScopePackage, ChargeBasisFixedAmount, ChargeMethodFixedAmount, description, effect, amount, order, sourceRef)
}

// A surcharge line is a fixed amount like an unconditional rule, but it is kept
// a distinct kind because it was produced by a predicate that has to be
// replayable 鈥?its source is a rule that could equally have missed.
func newSurchargeChargeLine(id string, code ChargeCode, description string, effect ChargeEffect, amount Money, order int, sourceRef string) (ChargeLine, error) {
	return newChargeLine(id, ChargeLineSurcharge, code, ChargeScopePackage, ChargeBasisFixedAmount, ChargeMethodFixedAmount, description, effect, amount, order, sourceRef)
}

func (line ChargeLine) valid() bool {
	_, err := newChargeLine(line.id, line.kind, line.chargeCode, line.scope, line.basis, line.method, line.description, line.effect, line.amount, line.order, line.sourceRef)
	return err == nil
}

func (line ChargeLine) ID() string              { return line.id }
func (line ChargeLine) Kind() ChargeLineKind    { return line.kind }
func (line ChargeLine) Code() ChargeCode        { return line.chargeCode }
func (line ChargeLine) Scope() ChargeScope      { return line.scope }
func (line ChargeLine) Basis() ChargeBasis      { return line.basis }
func (line ChargeLine) Method() ChargeMethod    { return line.method }
func (line ChargeLine) Description() string     { return line.description }
func (line ChargeLine) Effect() ChargeEffect    { return line.effect }
func (line ChargeLine) Amount() Money           { return line.amount }
func (line ChargeLine) Order() int              { return line.order }
func (line ChargeLine) SourceReference() string { return line.sourceRef }

type EvaluationRequest struct {
	id                       EvaluationID
	plan                     PricingPlanVersion
	input                    PricingInputSnapshot
	evidence                 EvidenceKind
	replayOf                 *EvaluationID
	expectedManifest         *VersionManifest
	expectedContentDigest    string
	expectedCanonicalization string
}

func NewEvaluationRequest(
	id EvaluationID,
	plan PricingPlanVersion,
	input PricingInputSnapshot,
	evidence EvidenceKind,
) (EvaluationRequest, error) {
	if !id.valid() || !plan.valid() || !input.valid() || !evidence.valid() {
		return EvaluationRequest{}, ErrEvaluationRequestInvalid
	}
	return EvaluationRequest{id: id, plan: plan, input: copyInputSnapshot(input), evidence: evidence}, nil
}

func NewReplayEvaluationRequest(
	id EvaluationID,
	original PricingEvaluation,
	plan PricingPlanVersion,
	evidence EvidenceKind,
) (EvaluationRequest, error) {
	if !id.valid() || !original.valid() || !plan.valid() || !evidence.valid() {
		return EvaluationRequest{}, ErrEvaluationRequestInvalid
	}
	if id == original.id {
		return EvaluationRequest{}, ErrReplayEvaluationIDReuse
	}
	if original.evidence == EvidenceSynthetic && evidence != EvidenceSynthetic {
		return EvaluationRequest{}, ErrEvaluationRequestInvalid
	}
	replayOf := original.id
	manifest := original.manifest
	return EvaluationRequest{
		id:                       id,
		plan:                     plan,
		input:                    copyInputSnapshot(original.input),
		evidence:                 evidence,
		replayOf:                 &replayOf,
		expectedManifest:         &manifest,
		expectedContentDigest:    original.planContentDigest,
		expectedCanonicalization: original.planCanonicalization,
	}, nil
}

func (request EvaluationRequest) ID() EvaluationID         { return request.id }
func (request EvaluationRequest) Plan() PricingPlanVersion { return request.plan }
func (request EvaluationRequest) Input() PricingInputSnapshot {
	return copyInputSnapshot(request.input)
}
func (request EvaluationRequest) Evidence() EvidenceKind { return request.evidence }

func (request EvaluationRequest) ReplayOf() (EvaluationID, bool) {
	if request.replayOf == nil {
		return EvaluationID{}, false
	}
	return *request.replayOf, true
}

func (request EvaluationRequest) valid() bool {
	return request.id.valid() && request.plan.valid() && request.input.valid() && request.evidence.valid()
}

type PricingEvaluation struct {
	id                   EvaluationID
	replayOf             *EvaluationID
	status               EvaluationStatus
	evidence             EvidenceKind
	input                PricingInputSnapshot
	direction            PricingDirection
	purpose              PricingPurpose
	planReference        VersionReference
	planPeriod           EffectivePeriod
	tablePeriod          EffectivePeriod
	planContentDigest    string
	planCanonicalization string
	manifest             VersionManifest
	pricingWeight        *PricingWeightResult
	matchedRate          *RateSelection
	chargeLines          []ChargeLine
	total                *Money
	conversion           *ConversionStep
	issues               []EvaluationIssue
	explanation          []string
	semanticDigest       string
}

func EvaluatePricing(request EvaluationRequest) PricingEvaluation {
	evaluation := baseEvaluation(request)
	if !request.valid() {
		return evaluation.withOutcome(EvaluationFailed, newEvaluationIssue("INVALID_REQUEST", ErrEvaluationRequestInvalid.Error()))
	}
	// A digest only means anything against the canonicalization that produced
	// it. Replaying an evaluation recorded under a shape this build no longer
	// implements is a structural inability to compute, not a content conflict,
	// so it must not be reported as one. See ADR-0014.
	if request.expectedCanonicalization != "" && request.expectedCanonicalization != CurrentCanonicalizationVersion() {
		return evaluation.withOutcome(EvaluationFailed, newEvaluationIssue("CANONICALIZATION_VERSION_UNSUPPORTED", ErrCanonicalizationVersionUnsupported.Error()))
	}
	if request.expectedManifest != nil && !request.expectedManifest.Equal(request.plan.manifest) {
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("VERSION_MANIFEST_MISMATCH", ErrEvaluationVersionConflict.Error()))
	}
	if request.expectedContentDigest != "" && request.expectedContentDigest != request.plan.contentDigest {
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("PLAN_CONTENT_MISMATCH", ErrEvaluationContentConflict.Error()))
	}
	if request.input.scope != request.plan.scope {
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("PRICING_SCOPE_MISMATCH", ErrPricingScopeMismatch.Error()))
	}
	if !request.plan.period.Contains(request.input.businessAt) || !request.plan.rateTable.period.Contains(request.input.businessAt) {
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("VERSION_NOT_APPLICABLE", ErrPricingPeriodNotApplicable.Error()))
	}
	// Pricing a plan on the base table alone while it declares charges nobody
	// executes would under-bill it, and the result would carry no sign of what
	// was skipped. The gate names what it cannot do and narrows as capabilities
	// land, rather than staying all-or-nothing.
	if reason, blocked := request.plan.structures.unexecutable(); blocked {
		return evaluation.withOutcome(EvaluationFailed, newEvaluationIssue("PLAN_STRUCTURES_NOT_EXECUTABLE", fmt.Sprintf("%s: %s", ErrPlanStructuresNotExecutable.Error(), reason)))
	}

	// Features are derived before anything is priced. Two declarations need
	// them this early: a conditional minimum is decided by a predicate yet
	// raises the plan-level pricing weight the base band is read with, and an
	// exclusion clause settles the outcome outright.
	var features PackageFeatures
	haveFeatures := false
	if len(request.plan.structures.surchargeRules) > 0 || len(request.plan.structures.exclusions) > 0 {
		derived, featuresErr := request.input.Features()
		if featuresErr != nil {
			// A predicate over dimensions can be neither confirmed nor excluded
			// without them: missing evidence, not a broken request.
			return evaluation.withOutcome(EvaluationPending, newEvaluationIssue("PACKAGE_FEATURES_UNAVAILABLE", featuresErr.Error()))
		}
		features, haveFeatures = derived, true
	}

	// Refusal is settled before any shortfall can be reported. A missing series
	// reading is 待判断, which tells the caller that supplying it will produce a
	// price; on a parcel the card refuses that promise is false and the caller
	// would keep retrying something that can never have one.
	if haveFeatures {
		rule, excluded, exclusionErr := request.plan.structures.resolveExclusions(features)
		if exclusionErr != nil {
			return evaluation.withCalculationError(exclusionErr)
		}
		if excluded {
			evaluation.explanation = append(evaluation.explanation,
				fmt.Sprintf("exclusion %s refused the parcel: %s; %s", rule.id, rule.clause, rule.condition.describe()))
			return evaluation.withOutcome(EvaluationUnratable,
				newEvaluationIssue("EXCLUDED_BY_RATE_CARD", fmt.Sprintf("%s: %s", rule.id, rule.clause)))
		}
	}

	// A bound series is resolved from the snapshot before anything is priced: a
	// missing reading is evidence this evaluation was not handed, a reading of
	// another version is a disagreement about which rate the plan declared.
	series, seriesErr := request.input.resolveSeries(request.plan.structures.referenceSeries)
	if seriesErr != nil {
		switch {
		case errors.Is(seriesErr, ErrMissingReferenceSeriesValue):
			return evaluation.withOutcome(EvaluationPending, newEvaluationIssue("REFERENCE_SERIES_UNRESOLVED", seriesErr.Error()))
		case errors.Is(seriesErr, ErrReferenceSeriesVersionConflict):
			return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("REFERENCE_SERIES_VERSION_MISMATCH", seriesErr.Error()))
		default:
			return evaluation.withCalculationError(seriesErr)
		}
	}
	for _, binding := range request.plan.structures.referenceSeries {
		reading := series[binding.kind]
		evaluation.explanation = append(evaluation.explanation,
			fmt.Sprintf("reference series %s resolved to %s from %s", binding.kind, reading.value.String(), reading.reference.ID()))
	}

	var floors []Weight
	if haveFeatures {
		raises, raisesErr := request.plan.structures.resolveMinimums(features)
		if raisesErr != nil {
			return evaluation.withCalculationError(raisesErr)
		}
		if highest, found := highestMinimum(raises); found {
			floors = append(floors, highest.minimum)
			evaluation.explanation = append(evaluation.explanation,
				fmt.Sprintf("conditional minimum %s raised the pricing weight to %s %s", highest.id, highest.minimum.value.String(), highest.minimum.unit))
		}
	}

	pricingWeight, err := CalculatePricingWeight(request.input, request.plan.weight, request.plan.rateTable.unit, floors...)
	if err != nil {
		return evaluation.withCalculationError(err)
	}
	evaluation.pricingWeight = &pricingWeight
	evaluation.explanation = append(evaluation.explanation, pricingWeight.explanation)

	matchedRate, err := request.plan.rateTable.Lookup(request.input.zone, pricingWeight.rounded)
	if err != nil {
		return evaluation.withCalculationError(err)
	}
	evaluation.matchedRate = &matchedRate
	// The explanation comes from the table because only it knows whether the
	// amount was matched in a bracket or derived from a step or unit price.
	evaluation.explanation = append(evaluation.explanation, matchedRate.explanation)

	baseLine, err := newBaseChargeLine("base:"+matchedRate.id.String(), request.plan.baseChargeCode, "Base rate", matchedRate.amount, matchedRate.id.String())
	if err != nil {
		return evaluation.withCalculationError(err)
	}
	evaluation.chargeLines = append(evaluation.chargeLines, baseLine)
	runningAmount := matchedRate.amount.amount

	for _, rule := range request.plan.rules {
		line, lineErr := newFixedChargeLine("fixed:"+rule.id, rule.chargeCode, rule.description, rule.effect, rule.amount, rule.order, rule.id)
		if lineErr != nil {
			return evaluation.withCalculationError(lineErr)
		}
		evaluation.chargeLines = append(evaluation.chargeLines, line)
		switch rule.effect {
		case ChargeEffectAdd:
			runningAmount, err = runningAmount.Add(rule.amount.amount)
		case ChargeEffectDeduct:
			if runningAmount.Cmp(rule.amount.amount) < 0 {
				return evaluation.withCalculationError(ErrNegativeChargeTotal)
			}
			runningAmount, err = runningAmount.Sub(rule.amount.amount)
		}
		if err != nil {
			return evaluation.withCalculationError(fmt.Errorf("%w: %v", ErrEvaluationArithmetic, err))
		}
		evaluation.explanation = append(evaluation.explanation, fmt.Sprintf("fixed rule %s %s %s %s", rule.id, rule.effect, rule.amount.amount.String(), rule.amount.currency))
	}

	if haveFeatures {
		outcomes, resolveErr := request.plan.structures.resolveSurcharges(surchargeContext{
			features:      features,
			zone:          request.input.zone,
			pricingWeight: pricingWeight.rounded,
			series:        series,
		})
		if resolveErr != nil {
			return evaluation.withCalculationError(resolveErr)
		}
		// Continue from the highest order already used, not from the line
		// count: a plan may declare fixed rules at non-contiguous orders, and
		// charge line order has to stay strictly increasing.
		order := 0
		for _, line := range evaluation.chargeLines {
			if line.order > order {
				order = line.order
			}
		}
		order++

		// A percent charge is summed over other charge lines, so the lines it
		// reads have to carry amounts first. Everything valued on its own is
		// collected in this pass; the percent charges follow in dependency
		// order below.
		basis := newDependencyBasis(request.plan.structures, request.plan.rateTable.currency)
		for _, line := range evaluation.chargeLines {
			if recordErr := basis.record(line.chargeCode, line.effect, line.amount); recordErr != nil {
				return evaluation.withCalculationError(recordErr)
			}
		}

		settled := sortedSurchargeOutcomes(outcomes)
		deferred := make([]*surchargeOutcome, 0, len(settled))
		for index := range settled {
			outcome := &settled[index]
			if outcome.selected && outcome.deferred {
				deferred = append(deferred, outcome)
			}
		}
		if len(deferred) > 0 {
			ordered, orderErr := orderPercentOutcomes(deferred, basis)
			if orderErr != nil {
				return evaluation.withCalculationError(orderErr)
			}
			deferred = ordered
		}

		collect := func(outcome *surchargeOutcome) error {
			line, lineErr := newSurchargeChargeLine("surcharge:"+outcome.rule.id, outcome.rule.chargeCode, outcome.rule.description, outcome.rule.effect, outcome.amount, order, outcome.rule.id)
			if lineErr != nil {
				return lineErr
			}
			evaluation.chargeLines = append(evaluation.chargeLines, line)
			order++
			if recordErr := basis.record(outcome.rule.chargeCode, outcome.rule.effect, outcome.amount); recordErr != nil {
				return recordErr
			}
			switch outcome.rule.effect {
			case ChargeEffectAdd:
				var addErr error
				runningAmount, addErr = runningAmount.Add(outcome.amount.amount)
				return addErr
			case ChargeEffectDeduct:
				if runningAmount.Cmp(outcome.amount.amount) < 0 {
					return ErrNegativeChargeTotal
				}
				var subErr error
				runningAmount, subErr = runningAmount.Sub(outcome.amount.amount)
				return subErr
			}
			return nil
		}

		for index := range settled {
			outcome := &settled[index]
			if outcome.deferred {
				continue
			}
			evaluation.explanation = append(evaluation.explanation, outcome.explain())
			if !outcome.selected {
				continue
			}
			if collectErr := collect(outcome); collectErr != nil {
				return evaluation.withCalculationError(collectErr)
			}
		}

		for _, outcome := range deferred {
			amount, resolveErr := outcome.rule.calculation.resolve(surchargeContext{
				features:      features,
				zone:          request.input.zone,
				pricingWeight: pricingWeight.rounded,
				series:        series,
			}, &basis)
			if resolveErr != nil {
				return evaluation.withCalculationError(resolveErr)
			}
			outcome.amount = amount
			evaluation.explanation = append(evaluation.explanation,
				fmt.Sprintf("surcharge %s charged %s over basis %s%s for %s %s",
					outcome.rule.id, outcome.rule.calculation.method, strings.Join(outcome.rule.calculation.basisDependencyIDs(), "+"),
					outcome.rule.calculation.describeRate(series), amount.amount.String(), amount.currency))
			if collectErr := collect(outcome); collectErr != nil {
				return evaluation.withCalculationError(collectErr)
			}
		}
	}

	total, err := NewMoney(runningAmount, request.plan.rateTable.currency)
	if err != nil {
		return evaluation.withCalculationError(fmt.Errorf("%w: %v", ErrEvaluationArithmetic, err))
	}

	// The card prices in its own currency while the contract settles in another,
	// and CONTEXT puts the conversion inside the evaluation so the result stays
	// a recomputable final price rather than half a figure the settlement side
	// has to finish.
	if settlement, declared := request.input.SettlementCurrency(); declared && settlement != total.currency {
		reading, resolved := series[ReferenceSeriesExchangeRate]
		if !resolved {
			return evaluation.withOutcome(EvaluationPending, newEvaluationIssue("EXCHANGE_RATE_UNRESOLVED",
				fmt.Errorf("%w: settling in %s needs an exchange rate", ErrMissingReferenceSeriesValue, settlement).Error()))
		}
		step, conversionErr := convertAmount(total, reading, settlement)
		if conversionErr != nil {
			return evaluation.withCalculationError(conversionErr)
		}
		basis, _ := reading.QuoteBasis()
		evaluation.conversion = &step
		evaluation.explanation = append(evaluation.explanation,
			fmt.Sprintf("converted %s %s to %s %s at %s from %s quoted per %s",
				step.original.amount.String(), step.original.currency,
				step.converted.amount.String(), step.converted.currency,
				step.rate.String(), step.series.ID(), basis.ID()))
		total = step.converted
	}

	evaluation.total = &total
	evaluation.status = EvaluationCompleted
	evaluation.semanticDigest = evaluation.calculateSemanticDigest()
	return evaluation
}

func Evaluate(request EvaluationRequest) PricingEvaluation {
	return EvaluatePricing(request)
}

func ReplayPricingEvaluation(
	newID EvaluationID,
	original PricingEvaluation,
	originalPlan PricingPlanVersion,
	evidence EvidenceKind,
) (PricingEvaluation, error) {
	request, err := NewReplayEvaluationRequest(newID, original, originalPlan, evidence)
	if err != nil {
		return PricingEvaluation{}, err
	}
	replayed := EvaluatePricing(request)
	if hasReplayVersionConflict(replayed) || hasUnsupportedCanonicalization(replayed) {
		return replayed, nil
	}
	if replayed.status != original.status || replayed.semanticDigest != original.semanticDigest {
		return replayed.withOutcome(EvaluationConflict, newEvaluationIssue("REPLAY_RESULT_MISMATCH", ErrReplayResultMismatch.Error())), nil
	}
	return replayed, nil
}

func (evaluation PricingEvaluation) ID() EvaluationID                { return evaluation.id }
func (evaluation PricingEvaluation) Status() EvaluationStatus        { return evaluation.status }
func (evaluation PricingEvaluation) Evidence() EvidenceKind          { return evaluation.evidence }
func (evaluation PricingEvaluation) Direction() PricingDirection     { return evaluation.direction }
func (evaluation PricingEvaluation) Purpose() PricingPurpose         { return evaluation.purpose }
func (evaluation PricingEvaluation) PlanReference() VersionReference { return evaluation.planReference }
func (evaluation PricingEvaluation) PlanEffectivePeriod() EffectivePeriod {
	return evaluation.planPeriod
}
func (evaluation PricingEvaluation) TableEffectivePeriod() EffectivePeriod {
	return evaluation.tablePeriod
}
func (evaluation PricingEvaluation) PlanContentDigest() string { return evaluation.planContentDigest }

// PlanCanonicalizationVersion reports the shape the plan content digest this
// evaluation froze was produced under. Digests are only comparable within the
// same value. See ADR-0014.
func (evaluation PricingEvaluation) PlanCanonicalizationVersion() string {
	return evaluation.planCanonicalization
}
func (evaluation PricingEvaluation) Manifest() VersionManifest { return evaluation.manifest }
func (evaluation PricingEvaluation) Input() PricingInputSnapshot {
	return copyInputSnapshot(evaluation.input)
}
func (evaluation PricingEvaluation) SemanticDigest() string { return evaluation.semanticDigest }

func (evaluation PricingEvaluation) ReplayOf() (EvaluationID, bool) {
	if evaluation.replayOf == nil {
		return EvaluationID{}, false
	}
	return *evaluation.replayOf, true
}

func (evaluation PricingEvaluation) PricingWeight() (PricingWeightResult, bool) {
	if evaluation.pricingWeight == nil {
		return PricingWeightResult{}, false
	}
	return *evaluation.pricingWeight, true
}

func (evaluation PricingEvaluation) MatchedRate() (RateSelection, bool) {
	if evaluation.matchedRate == nil {
		return RateSelection{}, false
	}
	return *evaluation.matchedRate, true
}

// ConversionStep reports the currency conversion this evaluation performed, if
// any. An evaluation settling in the card's own currency performs none, and
// recording a rate of 1 would invite a reader to think one was applied.
func (evaluation PricingEvaluation) ConversionStep() (ConversionStep, bool) {
	if evaluation.conversion == nil {
		return ConversionStep{}, false
	}
	return *evaluation.conversion, true
}

func (evaluation PricingEvaluation) ChargeLines() []ChargeLine {
	return append([]ChargeLine(nil), evaluation.chargeLines...)
}

func (evaluation PricingEvaluation) Total() (Money, bool) {
	if evaluation.status != EvaluationCompleted || evaluation.total == nil {
		return Money{}, false
	}
	return *evaluation.total, true
}

func (evaluation PricingEvaluation) Issues() []EvaluationIssue {
	return append([]EvaluationIssue(nil), evaluation.issues...)
}

func (evaluation PricingEvaluation) Explanation() []string {
	return append([]string(nil), evaluation.explanation...)
}

func (evaluation PricingEvaluation) valid() bool {
	if !evaluation.id.valid() || !evaluation.status.valid() || !evaluation.evidence.valid() || !evaluation.input.valid() || !evaluation.direction.valid() || !evaluation.purpose.valid() || !evaluation.planReference.valid() || !evaluation.planPeriod.valid() || !evaluation.tablePeriod.valid() || evaluation.planContentDigest == "" || evaluation.planCanonicalization == "" || !evaluation.manifest.valid() {
		return false
	}
	if evaluation.replayOf != nil && (!evaluation.replayOf.valid() || *evaluation.replayOf == evaluation.id) {
		return false
	}
	if !manifestContains(evaluation.manifest, evaluation.planReference) {
		return false
	}
	for _, line := range evaluation.chargeLines {
		if !line.valid() {
			return false
		}
	}
	if evaluation.pricingWeight != nil && !evaluation.pricingWeight.valid() {
		return false
	}
	if evaluation.matchedRate != nil && !evaluation.matchedRate.valid() {
		return false
	}
	if evaluation.status == EvaluationCompleted {
		if evaluation.total == nil || evaluation.pricingWeight == nil || evaluation.matchedRate == nil || len(evaluation.chargeLines) == 0 {
			return false
		}
		if !evaluation.validCompletedCharges() {
			return false
		}
	} else if evaluation.total != nil || len(evaluation.issues) == 0 {
		return false
	}
	if evaluation.semanticDigest == "" {
		return false
	}
	// A digest recorded under another canonicalization cannot be recomputed by
	// this build, so it is not self-checkable here; the replay path refuses it
	// explicitly instead of silently treating it as corrupt. See ADR-0014.
	if evaluation.planCanonicalization != CurrentCanonicalizationVersion() {
		return true
	}
	return evaluation.semanticDigest == evaluation.calculateSemanticDigest()
}

func (evaluation PricingEvaluation) validCompletedCharges() bool {
	if evaluation.total == nil || len(evaluation.chargeLines) == 0 {
		return false
	}
	// Charge lines stay in the card's own currency; only the total is converted.
	// Checking them against the settlement currency would reject every converted
	// evaluation, and converting each line instead would round the same rate
	// many times over.
	currency := evaluation.total.currency
	if evaluation.conversion != nil {
		currency = evaluation.conversion.original.currency
	}
	running := NewDecimalFromInt64(0)
	seenIDs := make(map[string]struct{}, len(evaluation.chargeLines))
	seenCodes := make(map[string]struct{}, len(evaluation.chargeLines))
	for index, line := range evaluation.chargeLines {
		if line.amount.currency != currency {
			return false
		}
		if _, exists := seenIDs[line.id]; exists {
			return false
		}
		seenIDs[line.id] = struct{}{}
		if _, exists := seenCodes[line.chargeCode.String()]; exists {
			return false
		}
		seenCodes[line.chargeCode.String()] = struct{}{}
		if index == 0 {
			if line.kind != ChargeLineBase || line.scope != ChargeScopePackage || line.basis != ChargeBasisRateEntry || line.method != ChargeMethodTableLookup || line.effect != ChargeEffectAdd || line.order != 0 {
				return false
			}
		} else if (line.kind != ChargeLineFixed && line.kind != ChargeLineSurcharge) || line.scope != ChargeScopePackage || line.basis != ChargeBasisFixedAmount || line.method != ChargeMethodFixedAmount || line.order < 1 {
			return false
		}
		if index > 0 {
			previous := evaluation.chargeLines[index-1]
			if line.order <= previous.order {
				return false
			}
		}
		var err error
		switch line.effect {
		case ChargeEffectAdd:
			running, err = running.Add(line.amount.amount)
		case ChargeEffectDeduct:
			if running.Cmp(line.amount.amount) < 0 {
				return false
			}
			running, err = running.Sub(line.amount.amount)
		default:
			return false
		}
		if err != nil {
			return false
		}
	}
	if evaluation.conversion != nil {
		return running.Equal(evaluation.conversion.original.amount)
	}
	return running.Equal(evaluation.total.amount)
}

func baseEvaluation(request EvaluationRequest) PricingEvaluation {
	var replayOf *EvaluationID
	if request.replayOf != nil {
		copy := *request.replayOf
		replayOf = &copy
	}
	return PricingEvaluation{
		id:                   request.id,
		replayOf:             replayOf,
		evidence:             request.evidence,
		input:                copyInputSnapshot(request.input),
		direction:            request.plan.direction,
		purpose:              request.plan.purpose,
		planReference:        request.plan.reference,
		planPeriod:           request.plan.period,
		tablePeriod:          request.plan.rateTable.period,
		planContentDigest:    request.plan.contentDigest,
		planCanonicalization: request.plan.canonicalization,
		manifest:             request.plan.manifest,
	}
}

func (evaluation PricingEvaluation) withCalculationError(err error) PricingEvaluation {
	switch {
	case errors.Is(err, ErrMissingDimensions):
		return evaluation.withOutcome(EvaluationPending, newEvaluationIssue("DIMENSIONS_REQUIRED", err.Error()))
	case errors.Is(err, ErrNoMatchingRate):
		return evaluation.withOutcome(EvaluationPending, newEvaluationIssue("RATE_NOT_FOUND", err.Error()))
	case errors.Is(err, ErrWeightUnitMismatch):
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("WEIGHT_UNIT_MISMATCH", err.Error()))
	case errors.Is(err, ErrLengthUnitMismatch):
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("LENGTH_UNIT_MISMATCH", err.Error()))
	case errors.Is(err, ErrRateTableConflict), errors.Is(err, ErrRateIntervalOverlap):
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("RATE_TABLE_CONFLICT", err.Error()))
	case errors.Is(err, ErrNegativeChargeTotal):
		return evaluation.withOutcome(EvaluationFailed, newEvaluationIssue("NEGATIVE_TOTAL", err.Error()))
	default:
		return evaluation.withOutcome(EvaluationFailed, newEvaluationIssue("CALCULATION_FAILED", err.Error()))
	}
}

func (evaluation PricingEvaluation) withOutcome(status EvaluationStatus, issue EvaluationIssue) PricingEvaluation {
	evaluation.status = status
	evaluation.total = nil
	evaluation.issues = append(evaluation.issues, issue)
	evaluation.semanticDigest = evaluation.calculateSemanticDigest()
	return evaluation
}

func (evaluation PricingEvaluation) calculateSemanticDigest() string {
	return hashPricingEvaluation(evaluation)
}

func copyInputSnapshot(input PricingInputSnapshot) PricingInputSnapshot {
	copy := input
	if input.dimensions != nil {
		sides := *input.dimensions
		copy.dimensions = &sides
	}
	copy.factReferences = append([]VersionedFactReference(nil), input.factReferences...)
	copy.seriesValues = append([]ReferenceSeriesValue(nil), input.seriesValues...)
	return copy
}

func rateMaximumText(entry RateEntry) string {
	if !entry.hasMaximum {
		return "+INF"
	}
	return entry.maximum.value.String()
}

func manifestContains(manifest VersionManifest, expected VersionReference) bool {
	for _, reference := range manifest.references {
		if reference == expected {
			return true
		}
	}
	return false
}

// hasUnsupportedCanonicalization keeps a replay that could not be canonicalized
// under its recorded shape out of the result comparison below, so it is never
// reclassified as a mismatch. See ADR-0014.
func hasUnsupportedCanonicalization(evaluation PricingEvaluation) bool {
	for _, issue := range evaluation.issues {
		if issue.code == "CANONICALIZATION_VERSION_UNSUPPORTED" {
			return true
		}
	}
	return false
}

func hasReplayVersionConflict(evaluation PricingEvaluation) bool {
	for _, issue := range evaluation.issues {
		switch issue.code {
		case "VERSION_MANIFEST_MISMATCH", "PLAN_CONTENT_MISMATCH":
			return true
		}
	}
	return false
}
