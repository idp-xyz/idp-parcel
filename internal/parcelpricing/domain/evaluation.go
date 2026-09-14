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
	// EvaluationUnratable 是卡说不，不是求值器没算出来。CONTEXT 不许四种结果互相顶替：
	// `待判断` 承诺补齐事实就能得出价格，而在这里这个承诺是假的；`未形成` 表示技术或
	// 结构上算不出来，调用方的应对是重试而不是形成业务决定。
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

// SeriesSubject 是问题项「涉及序列」的结构化主体（ADR-0105 Decision 一）：序列种类必有、序列标识可缺——
// 结算币种要汇率而方案没绑汇率序列时，只知道种类。它是 message 里已有信息的类型化重述，不是新的评价语义，
// 所以进快照不进规范化文档（Decision 二）。
type SeriesSubject struct {
	kind     ReferenceSeriesKind
	seriesID string
}

func (subject SeriesSubject) Kind() ReferenceSeriesKind { return subject.kind }

// SeriesID 只在主体指得出具体序列时给出。
func (subject SeriesSubject) SeriesID() (string, bool) {
	return subject.seriesID, subject.seriesID != ""
}

func (subject SeriesSubject) valid() bool {
	return subject.kind.valid() && (subject.seriesID == "" || trimmed(subject.seriesID))
}

type EvaluationIssue struct {
	code    string
	message string
	// series 只在 REFERENCE_SERIES_UNRESOLVED 与 EXCHANGE_RATE_UNRESOLVED 两种问题项上有（ADR-0105 Decision 一、六）；
	// 其余问题项的主体不是序列，这一格为空。
	series *SeriesSubject
}

func newEvaluationIssue(code, message string) EvaluationIssue {
	return EvaluationIssue{code: code, message: message}
}

// newSeriesEvaluationIssue 造一条带「涉及序列」主体的问题项；seriesID 可空。
func newSeriesEvaluationIssue(code, message string, kind ReferenceSeriesKind, seriesID string) EvaluationIssue {
	subject := SeriesSubject{kind: kind, seriesID: seriesID}
	return EvaluationIssue{code: code, message: message, series: &subject}
}

func (issue EvaluationIssue) Code() string    { return issue.code }
func (issue EvaluationIssue) Message() string { return issue.message }

// Series 交回问题项涉及的序列；没有主体的问题项第二个返回值为假。
func (issue EvaluationIssue) Series() (SeriesSubject, bool) {
	if issue.series == nil {
		return SeriesSubject{}, false
	}
	return *issue.series, true
}

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
		if basis != ChargeBasisRateEntry || method != ChargeMethodTableLookup || effect != ChargeEffectAdd || order != 0 {
			return ChargeLine{}, ErrInvalidChargeLine
		}
	case ChargeLineFixed, ChargeLineSurcharge:
		if basis != ChargeBasisFixedAmount || method != ChargeMethodFixedAmount || order < 1 {
			return ChargeLine{}, ErrInvalidChargeLine
		}
	default:
		return ChargeLine{}, ErrInvalidChargeLine
	}
	return ChargeLine{id: id, kind: kind, chargeCode: code, scope: scope, basis: basis, method: method, description: description, effect: effect, amount: amount, order: order, sourceRef: sourceRef}, nil
}

// 费用行的聚合单位（ADR-0111 Decision 一）：基础运费与「每主体一次」的规则标方案的主体单位，按件计收的
// 规则标 PACKAGE——金额是件数乘定额，解释里写着乘法。
func newBaseChargeLine(id string, code ChargeCode, scope ChargeScope, description string, amount Money, sourceRef string) (ChargeLine, error) {
	return newChargeLine(id, ChargeLineBase, code, scope, ChargeBasisRateEntry, ChargeMethodTableLookup, description, ChargeEffectAdd, amount, 0, sourceRef)
}

func newFixedChargeLine(id string, code ChargeCode, scope ChargeScope, description string, effect ChargeEffect, amount Money, order int, sourceRef string) (ChargeLine, error) {
	return newChargeLine(id, ChargeLineFixed, code, scope, ChargeBasisFixedAmount, ChargeMethodFixedAmount, description, effect, amount, order, sourceRef)
}

// 附加费费用行和无条件固定规则一样是一个定额，但它单列一种类型，因为它由一个必须
// 可重放的判定条件产生——它的来源是一条同样可能未命中的规则。
func newSurchargeChargeLine(id string, code ChargeCode, scope ChargeScope, description string, effect ChargeEffect, amount Money, order int, sourceRef string) (ChargeLine, error) {
	return newChargeLine(id, ChargeLineSurcharge, code, scope, ChargeBasisFixedAmount, ChargeMethodFixedAmount, description, effect, amount, order, sourceRef)
}

// chargeScopeFor 定一条规则的费用行标哪种聚合单位；按件的行还交回乘数（成员数）。
func chargeScopeFor(plan PricingPlanVersion, input PricingInputSnapshot, unit ChargeUnit) (ChargeScope, int) {
	if unit == ChargeUnitPerPiece {
		return ChargeScopePackage, input.memberCount()
	}
	return plan.aggregation.subjectScope(), 1
}

// multiplyByPieces 把一条按件计收的定额乘成员数（ADR-0111 Decision 二「按件的行按件数乘定额」）。
func multiplyByPieces(amount Money, pieces int) (Money, error) {
	if pieces == 1 {
		return amount, nil
	}
	product, err := amount.amount.Mul(NewDecimalFromInt64(int64(pieces)))
	if err != nil {
		return Money{}, err
	}
	return NewMoney(product, amount.currency)
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
	seriesNotes              []string
	// requestReference 是这份评价为哪一份 SA 评价请求而形成（票 sa-cc/11 裁决 2），可缺席：PP 内部或测试路径
	// 形成的评价没有它。它不是计算输入，与 replayOf 同一族——随评价入册、供读口按它取，不进任何摘要。
	requestReference *EvaluationRequestReference
}

// WithRequestReference 带上回指。回指随评价入册、读口可按它取；它**不是计算输入**：不进 semanticDigest 也不进
// planContentDigest——重放按原输入重算，带不带回指都判成同一结果；两份只差回指的评价语义摘要逐字相同。
func (request EvaluationRequest) WithRequestReference(reference EvaluationRequestReference) (EvaluationRequest, error) {
	if !request.valid() || !reference.valid() {
		return EvaluationRequest{}, ErrEvaluationRequestInvalid
	}
	copyOfRequest := request
	declared := reference
	copyOfRequest.requestReference = &declared
	return copyOfRequest, nil
}

// RequestReference 交回回指；PP 内部形成的评价第二个返回值为假。
func (request EvaluationRequest) RequestReference() (EvaluationRequestReference, bool) {
	if request.requestReference == nil {
		return EvaluationRequestReference{}, false
	}
	return *request.requestReference, true
}

// WithInput 换掉输入快照，其余原样保留：标识、方案、证据层级、重放期望、回指、解析说明。编排补齐序列取值 /
// 目录读数时要重立请求，此前按四参重新 NewEvaluationRequest——凡不在四参里的东西都会在那一步丢掉，回指一格
// 就是第一件会丢的。换输入不改这份请求「是谁、为谁」，所以这里是就地换一格而不是重新构造。
func (request EvaluationRequest) WithInput(input PricingInputSnapshot) (EvaluationRequest, error) {
	if !request.valid() || !input.valid() {
		return EvaluationRequest{}, ErrEvaluationRequestInvalid
	}
	copyOfRequest := request
	copyOfRequest.input = copyInputSnapshot(input)
	copyOfRequest.seriesNotes = append([]string(nil), request.seriesNotes...)
	return copyOfRequest, nil
}

// WithSeriesResolutionNotes 带上编排层在解析在用序列版本时留下的说明（无已登记版本 /
// 有版本未复核 / 在用版本无覆盖该时点的期次 / 种类不合）。它们只进解释，不进输入快照
// 也不进语义摘要：那是形成评价那一刻登记册的状态，不是评价的语义——重放按原输入重算，
// 不该因登记册后来变了而判成结果不一致。
func (request EvaluationRequest) WithSeriesResolutionNotes(notes ...string) EvaluationRequest {
	copyOfRequest := request
	copyOfRequest.seriesNotes = append([]string(nil), request.seriesNotes...)
	for _, note := range notes {
		if trimmed(note) {
			copyOfRequest.seriesNotes = append(copyOfRequest.seriesNotes, note)
		}
	}
	return copyOfRequest
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
	id       EvaluationID
	replayOf *EvaluationID
	// requestReference 见 EvaluationRequest 上的同名字段：为哪一份 SA 评价请求形成，可缺席，不进摘要。
	requestReference     *EvaluationRequestReference
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
	// amountRounding 是本次评价按卡上策略做过的每一次取整（ADR-0107）：逐行的先于换算后的，
	// 换算后的先于合计的。合计不再等于费用行之和时，差在这里可复算。
	amountRounding []AmountRoundingStep
	issues         []EvaluationIssue
	explanation    []string
	semanticDigest string
}

// amountPrecisionUndeclaredIssue 是 ADR-0107 Decision 四那条结构化问题项的编码：卡没声明金额取整
// 策略，评价照精确十进制完成，消费方据此知道这个金额没被任何规则取过整、不得自己补一次。
const amountPrecisionUndeclaredIssue = "AMOUNT_PRECISION_UNDECLARED"

func EvaluatePricing(request EvaluationRequest) PricingEvaluation {
	evaluation := baseEvaluation(request)
	if !request.valid() {
		return evaluation.withOutcome(EvaluationFailed, newEvaluationIssue("INVALID_REQUEST", ErrEvaluationRequestInvalid.Error()))
	}
	// 内容摘要只在产生它的那个规范化版本内有意义。重放一次按本构建已不再实现的形状
	// 记录的评价，是结构上算不出来，不是版本内容冲突，因此不得报成冲突。见 ADR-0014。
	if request.expectedCanonicalization != "" && request.expectedCanonicalization != CurrentCanonicalizationVersion() {
		return evaluation.withOutcome(EvaluationFailed, newEvaluationIssue("CANONICALIZATION_VERSION_UNSUPPORTED", ErrCanonicalizationVersionUnsupported.Error()))
	}
	// 比的是评价自己的清单（方案清单 + 本次采用的序列版本），不是方案清单：重放带着原输入
	// 快照进来，快照里的序列版本引用就是当初冻结的那一版，换了一版就在这里露出来。
	if request.expectedManifest != nil && !request.expectedManifest.Equal(evaluation.manifest) {
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("VERSION_MANIFEST_MISMATCH", ErrEvaluationVersionConflict.Error()))
	}
	if request.expectedContentDigest != "" && request.expectedContentDigest != request.plan.contentDigest {
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("PLAN_CONTENT_MISMATCH", ErrEvaluationContentConflict.Error()))
	}
	if request.input.scope != request.plan.scope {
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("PRICING_SCOPE_MISMATCH", ErrPricingScopeMismatch.Error()))
	}
	// 聚合方式与主体种类必须一致（ADR-0111）：逐委托的卡拿到一个包裹，「每票」的金额会在每个包裹上各出现一次；
	// 逐包裹的卡拿到一个委托，合计重量会被当成一件包裹查表。两边都是分歧，不是缺口。
	if !request.plan.aggregation.admits(request.input.subject.kind) {
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("AGGREGATION_SUBJECT_MISMATCH",
			fmt.Sprintf("plan aggregates %s but the evaluation subject is %s", request.plan.aggregation, request.input.subject.kind)))
	}
	if !request.plan.period.Contains(request.input.businessAt) || !request.plan.rateTable.period.Contains(request.input.businessAt) {
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("VERSION_NOT_APPLICABLE", ErrPricingPeriodNotApplicable.Error()))
	}
	// 方案声明了没人执行的费用，却只按基础价表计价，会少收，而结果上看不出漏了什么。
	// 这道门明说自己做不到哪一项，并随能力落地逐步收窄，而不是一直全有或全无。
	if reason, blocked := request.plan.structures.unexecutable(); blocked {
		return evaluation.withOutcome(EvaluationFailed, newEvaluationIssue("PLAN_STRUCTURES_NOT_EXECUTABLE", fmt.Sprintf("%s: %s", ErrPlanStructuresNotExecutable.Error(), reason)))
	}

	// 分区与偏远档位在一切判定之前先定下来（ADR-0109 Decision 四）：拒收条款与附加费条件可能读分区，
	// 基础运费查表更离不开它。绑了目录的卡只认目录读数，没绑的卡读调用方给的分区；两边都没有即待判断。
	categories, categoriesErr := request.plan.structures.resolveCatalogues(request.input)
	if categoriesErr != nil {
		return evaluation.withCalculationError(categoriesErr)
	}
	evaluation.explanation = append(evaluation.explanation, categories.explanations...)
	zone := string(categories.zone)

	// 特征在计价之前先派生。有两种声明这么早就需要它：条件最低计价重量由判定条件决定，
	// 却会抬高读取基础档位所用的方案级计价重量；而拒收条款直接就定了结果。
	var features PackageFeatures
	haveFeatures := false
	if len(request.plan.structures.surchargeRules) > 0 || len(request.plan.structures.exclusions) > 0 {
		derived, featuresErr := request.input.Features()
		if featuresErr != nil {
			// 缺了尺寸，基于尺寸的判定条件既不能确认也不能排除：这是证据不足，
			// 不是请求不合法。
			return evaluation.withOutcome(EvaluationPending, newEvaluationIssue("PACKAGE_FEATURES_UNAVAILABLE", featuresErr.Error()))
		}
		// 类别量随解析结果填进特征：分区一定有；偏远档位只在卡绑了档位目录时有，没绑的卡该格缺席，
		// 地址类型条件读到缺席照旧报特征不可用。
		features, haveFeatures = derived.WithCategories(categories.zone, categories.tier), true
	}

	// 先定拒收，再谈任何缺口。缺一期序列取值是`待判断`，它等于告诉调用方补上就能得出
	// 价格；而对一件卡明确拒收的包裹，这个承诺是假的，调用方会一直重试一个永远不会有
	// 价格的东西。
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

	// 已绑定的序列在计价之前先从快照解析：缺取值是这次评价没拿到的证据，
	// 而拿到另一个版本的取值，是对「方案声明的是哪个费率」有分歧。
	series, amounts, seriesErr := request.input.resolveSeries(request.plan.structures.referenceSeries)
	if seriesErr != nil {
		switch {
		case errors.Is(seriesErr, ErrMissingReferenceSeriesValue):
			return evaluation.withOutcome(EvaluationPending, unresolvedSeriesIssue(seriesErr))
		case errors.Is(seriesErr, ErrReferenceSeriesVersionConflict):
			return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("REFERENCE_SERIES_VERSION_MISMATCH", seriesErr.Error()))
		default:
			return evaluation.withCalculationError(seriesErr)
		}
	}
	for _, binding := range request.plan.structures.referenceSeries {
		if binding.kind.carriesAmount() {
			reading := amounts[binding.seriesID]
			if reading.absent {
				evaluation.explanation = append(evaluation.explanation,
					fmt.Sprintf("reference series %s (%s) in-force version %s has no period at the pricing basis time", binding.seriesID, binding.kind, reading.reference.Version()))
				continue
			}
			// 金额币种须与方案币种一致（ADR-0110 Decision 二）：币种在读数解出时才可知，不一致是分歧不是缺口。
			if *reading.currency != request.plan.rateTable.currency {
				return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("REFERENCE_SERIES_CURRENCY_MISMATCH",
					fmt.Sprintf("%s: series %s publishes %s amounts but the plan prices in %s", ErrCurrencyMismatch.Error(), binding.seriesID, reading.currency, request.plan.rateTable.currency)))
			}
			evaluation.explanation = append(evaluation.explanation,
				fmt.Sprintf("reference series %s (%s) resolved to %s %s from %s@%s", binding.seriesID, binding.kind, reading.value.String(), reading.currency, reading.reference.ID(), reading.reference.Version()))
			continue
		}
		reading := series[binding.kind]
		evaluation.explanation = append(evaluation.explanation,
			fmt.Sprintf("reference series %s resolved to %s from %s@%s", binding.kind, reading.value.String(), reading.reference.ID(), reading.reference.Version()))
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

	matchedRate, err := request.plan.rateTable.Lookup(zone, pricingWeight.rounded)
	if err != nil {
		return evaluation.withCalculationError(err)
	}
	evaluation.matchedRate = &matchedRate
	// 解释由价表给出，因为只有它知道这个金额是在某个档位里匹配到的，
	// 还是由续重步长或单价推导出来的。
	evaluation.explanation = append(evaluation.explanation, matchedRate.explanation)

	// 金额取整按卡上的声明做（ADR-0107）：逐行在费用行成形处、换算后与合计在各自那一步；没声明
	// 就一次也不取整，评价末尾如实记问题项。roundLine 是逐行那一点的唯一入口——基础运费、固定规则、
	// 附加费三处费用行都经它，逐行取整的留痕与费用行上的金额才始终一致。
	amountRounding := request.plan.structures.amountRounding
	roundLine := func(lineID string, amount Money) (Money, error) {
		rounded, step, roundErr := applyAmountRounding(amountRounding, AmountRoundingPerLine, lineID, amount)
		if roundErr != nil {
			return Money{}, roundErr
		}
		if step != nil {
			evaluation.amountRounding = append(evaluation.amountRounding, *step)
			evaluation.explanation = append(evaluation.explanation, step.explain())
		}
		return rounded, nil
	}

	baseAmount, err := roundLine("base:"+matchedRate.id.String(), matchedRate.amount)
	if err != nil {
		return evaluation.withCalculationError(err)
	}
	baseLine, err := newBaseChargeLine("base:"+matchedRate.id.String(), request.plan.baseChargeCode, request.plan.aggregation.subjectScope(), "Base rate", baseAmount, matchedRate.id.String())
	if err != nil {
		return evaluation.withCalculationError(err)
	}
	evaluation.chargeLines = append(evaluation.chargeLines, baseLine)
	runningAmount := baseAmount.amount

	for _, rule := range request.plan.rules {
		scope, pieces := chargeScopeFor(request.plan, request.input, rule.unit)
		ruleAmount, multiplyErr := multiplyByPieces(rule.amount, pieces)
		if multiplyErr != nil {
			return evaluation.withCalculationError(fmt.Errorf("%w: %v", ErrEvaluationArithmetic, multiplyErr))
		}
		if pieces != 1 {
			evaluation.explanation = append(evaluation.explanation, fmt.Sprintf("fixed rule %s charged per piece: %s %s × %d pieces", rule.id, rule.amount.amount.String(), rule.amount.currency, pieces))
		}
		lineAmount, roundErr := roundLine("fixed:"+rule.id, ruleAmount)
		if roundErr != nil {
			return evaluation.withCalculationError(roundErr)
		}
		line, lineErr := newFixedChargeLine("fixed:"+rule.id, rule.chargeCode, scope, rule.description, rule.effect, lineAmount, rule.order, rule.id)
		if lineErr != nil {
			return evaluation.withCalculationError(lineErr)
		}
		evaluation.chargeLines = append(evaluation.chargeLines, line)
		switch rule.effect {
		case ChargeEffectAdd:
			runningAmount, err = runningAmount.Add(lineAmount.amount)
		case ChargeEffectDeduct:
			if runningAmount.Cmp(lineAmount.amount) < 0 {
				return evaluation.withCalculationError(ErrNegativeChargeTotal)
			}
			runningAmount, err = runningAmount.Sub(lineAmount.amount)
		}
		if err != nil {
			return evaluation.withCalculationError(fmt.Errorf("%w: %v", ErrEvaluationArithmetic, err))
		}
		evaluation.explanation = append(evaluation.explanation, fmt.Sprintf("fixed rule %s %s %s %s", rule.id, rule.effect, lineAmount.amount.String(), lineAmount.currency))
	}

	if haveFeatures {
		outcomes, resolveErr := request.plan.structures.resolveSurcharges(surchargeContext{
			features:      features,
			zone:          zone,
			pricingWeight: pricingWeight.rounded,
			series:        series,
			amounts:       amounts,
		})
		if resolveErr != nil {
			return evaluation.withCalculationError(resolveErr)
		}
		// 从已用过的最大序号往下接，而不是从费用行条数接：方案可以在不连续的序号上
		// 声明固定规则，而费用行序号必须保持严格递增。
		order := 0
		for _, line := range evaluation.chargeLines {
			if line.order > order {
				order = line.order
			}
		}
		order++

		// 百分比费用要在其他费用行上求和，所以它读取的那些行必须先有金额。这一遍先
		// 收齐所有能自行定值的费用；百分比费用留到下面按依赖顺序处理。
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
			scope, pieces := chargeScopeFor(request.plan, request.input, outcome.rule.unit)
			ruleAmount, multiplyErr := multiplyByPieces(outcome.amount, pieces)
			if multiplyErr != nil {
				return fmt.Errorf("%w: %v", ErrEvaluationArithmetic, multiplyErr)
			}
			if pieces != 1 {
				evaluation.explanation = append(evaluation.explanation, fmt.Sprintf("surcharge %s charged per piece: %s %s × %d pieces", outcome.rule.id, outcome.amount.amount.String(), outcome.amount.currency, pieces))
			}
			// 逐行取整在费用行成形之前、在百分比依据登记之前：读这一行的百分比费用要读到的是卡上
			// 那个金额，不是取整前的中间值。
			lineAmount, roundErr := roundLine("surcharge:"+outcome.rule.id, ruleAmount)
			if roundErr != nil {
				return roundErr
			}
			outcome.amount = lineAmount
			line, lineErr := newSurchargeChargeLine("surcharge:"+outcome.rule.id, outcome.rule.chargeCode, scope, outcome.rule.description, outcome.rule.effect, outcome.amount, order, outcome.rule.id)
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
				zone:          zone,
				pricingWeight: pricingWeight.rounded,
				series:        series,
				amounts:       amounts,
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

	// 卡按自己的币种计价，合同却按另一个币种结算；CONTEXT 把换算放进评价内部，
	// 使结果保持为一个可复算的最终价格，而不是一个还要结算侧补完的半成品数字。
	if settlement, declared := request.input.SettlementCurrency(); declared && settlement != total.currency {
		reading, resolved := series[ReferenceSeriesExchangeRate]
		if !resolved {
			// 主体只有种类（ADR-0105 Decision 一）：结算币种要汇率，而方案未必声明过汇率绑定，没有标识可指。
			return evaluation.withOutcome(EvaluationPending, newSeriesEvaluationIssue("EXCHANGE_RATE_UNRESOLVED",
				fmt.Errorf("%w: settling in %s needs an exchange rate", ErrMissingReferenceSeriesValue, settlement).Error(),
				ReferenceSeriesExchangeRate, ""))
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
		// 换算后那一点只在发生了换算时才有对象；卡币种结算的评价这一点空过。
		converted, roundStep, roundErr := applyAmountRounding(amountRounding, AmountRoundingAfterConversion, "", total)
		if roundErr != nil {
			return evaluation.withCalculationError(roundErr)
		}
		if roundStep != nil {
			evaluation.amountRounding = append(evaluation.amountRounding, *roundStep)
			evaluation.explanation = append(evaluation.explanation, roundStep.explain())
		}
		total = converted
	}

	// 合计是策略必声明的那一点（ADR-0107 Decision 二）：声明了策略的卡，交出去的合计 scale 就是进位
	// 单位的 scale，消费方直接采用。
	roundedTotal, totalStep, err := applyAmountRounding(amountRounding, AmountRoundingTotal, "", total)
	if err != nil {
		return evaluation.withCalculationError(err)
	}
	if totalStep != nil {
		evaluation.amountRounding = append(evaluation.amountRounding, *totalStep)
		evaluation.explanation = append(evaluation.explanation, totalStep.explain())
	}
	total = roundedTotal
	if amountRounding == nil {
		// 未声明即不取整，且如实记问题项、不给默认（ADR-0107 Decision 四）：评价照样完成，但读
		// Total() 的人要看见这个金额没被任何规则取过整。
		evaluation.issues = append(evaluation.issues, newEvaluationIssue(amountPrecisionUndeclaredIssue,
			"the price card declares no amount rounding policy; the total is exact decimal and has not been rounded by any rule"))
		evaluation.explanation = append(evaluation.explanation, "amount rounding: not declared by the price card; total left unrounded")
	}

	evaluation.total = &total
	evaluation.status = EvaluationCompleted
	evaluation.semanticDigest = evaluation.calculateSemanticDigest()
	return evaluation
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

// PlanCanonicalizationVersion 报出本次评价冻结的方案内容摘要是在哪一套规范化形状下
// 产生的。摘要只在同一规范化版本内可比。见 ADR-0014。
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

// RequestReference 交回这份评价为哪一份 SA 评价请求而形成；PP 内部或测试路径形成的评价第二个返回值为假。
// 采用评价的消费者按它回查 SA 登记册取合格来源引用（票 sa-cc/08 判据 4、sa-cc/01「形成」路）。
func (evaluation PricingEvaluation) RequestReference() (EvaluationRequestReference, bool) {
	if evaluation.requestReference == nil {
		return EvaluationRequestReference{}, false
	}
	return *evaluation.requestReference, true
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

// ConversionStep 报出本次评价执行的换算步骤（如果有）。按卡本币结算的评价不执行换算，
// 而记一个汇率 1 会让读的人以为发生过一次换算。
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
	if evaluation.requestReference != nil && !evaluation.requestReference.valid() {
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
	for _, issue := range evaluation.issues {
		if issue.series != nil && !issue.series.valid() {
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
	// 在另一个规范化版本下记录的摘要，本构建无法重新算出，因此在这里无法自校；
	// 重放路径会显式拒绝它，而不是静默当成数据损坏。见 ADR-0014。
	if evaluation.planCanonicalization != CurrentCanonicalizationVersion() {
		return true
	}
	return evaluation.semanticDigest == evaluation.calculateSemanticDigest()
}

func (evaluation PricingEvaluation) validCompletedCharges() bool {
	if evaluation.total == nil || len(evaluation.chargeLines) == 0 {
		return false
	}
	// 费用行保持卡本币，只有合计被换算。拿结算币种去校验费用行会把每一次发生了换算的
	// 评价都判为不合法；改成逐行换算又会把同一个汇率反复取整多次。
	currency := evaluation.total.currency
	if evaluation.conversion != nil {
		currency = evaluation.conversion.original.currency
	}
	running := NewDecimalFromInt64(0)
	seenIDs := make(map[string]struct{}, len(evaluation.chargeLines))
	seenCodes := make(map[string]struct{}, len(evaluation.chargeLines))
	// 费用行的聚合单位只能是主体那一级或 PACKAGE（按件计收的行）；逐包裹主体上两者是同一格（ADR-0111 Decision 一）。
	subjectScope := ChargeScopePackage
	switch evaluation.input.subject.kind {
	case SubjectShipment:
		subjectScope = ChargeScopeShipment
	case SubjectMasterDocument:
		subjectScope = ChargeScopeMasterDocument
	}
	for index, line := range evaluation.chargeLines {
		if line.amount.currency != currency {
			return false
		}
		if line.scope != subjectScope && line.scope != ChargeScopePackage {
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
			if line.kind != ChargeLineBase || line.scope != subjectScope || line.basis != ChargeBasisRateEntry || line.method != ChargeMethodTableLookup || line.effect != ChargeEffectAdd || line.order != 0 {
				return false
			}
		} else if (line.kind != ChargeLineFixed && line.kind != ChargeLineSurcharge) || line.basis != ChargeBasisFixedAmount || line.method != ChargeMethodFixedAmount || line.order < 1 {
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
	// 费用行之和到合计之间只允许两种东西：一次换算，与卡上声明的取整（换算后、合计两点，逐行那一点
	// 已经在费用行金额里）。逐步重走留痕：每一步的 before 必须等于此刻的值，after 成为下一刻的值，
	// 最后落在合计上——多一步、少一步、顺序不对都不合法。
	value := Money{amount: running, currency: currency}
	steps := evaluation.amountRounding
	for len(steps) > 0 && steps[0].point == AmountRoundingPerLine {
		steps = steps[1:]
	}
	if evaluation.conversion != nil {
		if !running.Equal(evaluation.conversion.original.amount) {
			return false
		}
		value = evaluation.conversion.converted
		if len(steps) > 0 && steps[0].point == AmountRoundingAfterConversion {
			if !steps[0].valid() || !steps[0].before.Equal(value) {
				return false
			}
			value = steps[0].after
			steps = steps[1:]
		}
	}
	if len(steps) > 0 && steps[0].point == AmountRoundingTotal {
		if !steps[0].valid() || !steps[0].before.Equal(value) {
			return false
		}
		value = steps[0].after
		steps = steps[1:]
	}
	if len(steps) != 0 {
		return false
	}
	return value.Equal(*evaluation.total)
}

// AmountRounding 交回本次评价做过的每一次取整，按评价里的先后。空切片即卡没声明策略（那时
// Issues 里有 AMOUNT_PRECISION_UNDECLARED）或声明了但一次也没落到可取整的金额上。
func (evaluation PricingEvaluation) AmountRounding() []AmountRoundingStep {
	return append([]AmountRoundingStep(nil), evaluation.amountRounding...)
}

func baseEvaluation(request EvaluationRequest) PricingEvaluation {
	var replayOf *EvaluationID
	if request.replayOf != nil {
		copy := *request.replayOf
		replayOf = &copy
	}
	// 回指与 replayOf 同一种搬法：随请求进评价，只是元数据，不参与下面任何一步计算。
	var requestReference *EvaluationRequestReference
	if request.requestReference != nil {
		copy := *request.requestReference
		requestReference = &copy
	}
	return PricingEvaluation{
		id:                   request.id,
		replayOf:             replayOf,
		requestReference:     requestReference,
		evidence:             request.evidence,
		input:                copyInputSnapshot(request.input),
		direction:            request.plan.direction,
		purpose:              request.plan.purpose,
		planReference:        request.plan.reference,
		planPeriod:           request.plan.period,
		tablePeriod:          request.plan.rateTable.period,
		planContentDigest:    request.plan.contentDigest,
		planCanonicalization: request.plan.canonicalization,
		manifest:             composeEvaluationManifest(request.plan, request.input),
		explanation:          append([]string(nil), request.seriesNotes...),
	}
}

// composeEvaluationManifest 把本次采用的序列版本并进方案清单，得到评价自己的清单
// （ADR-0099 决定四）。方案清单不含序列版本——方案绑的是序列标识；哪一版是这次评价的
// 结论，所以由评价冻结。序列取值缺席时清单就是方案清单，评价随后落待判断。
func composeEvaluationManifest(plan PricingPlanVersion, input PricingInputSnapshot) VersionManifest {
	adopted := input.boundSeriesReferences(plan.structures.referenceSeries)
	// 目录版本同理（ADR-0109 Decision 三）：卡绑的是目录标识，查的是哪一版由评价冻结。
	adopted = append(adopted, input.boundCatalogueReferences(plan.structures.referenceCatalogues)...)
	if len(adopted) == 0 {
		return plan.manifest
	}
	manifest, err := NewVersionManifest(append(plan.manifest.References(), adopted...))
	if err != nil {
		// 序列引用与方案清单撞键只会是「同一（种类、标识、版本）出现两次」，而方案清单
		// 里没有序列种类；这里守的是不让一次坏输入把评价做成无清单。
		return plan.manifest
	}
	return manifest
}

func (evaluation PricingEvaluation) withCalculationError(err error) PricingEvaluation {
	switch {
	case errors.Is(err, ErrMissingDimensions):
		return evaluation.withOutcome(EvaluationPending, newEvaluationIssue("DIMENSIONS_REQUIRED", err.Error()))
	case errors.Is(err, ErrNoMatchingRate):
		return evaluation.withOutcome(EvaluationPending, newEvaluationIssue("RATE_NOT_FOUND", err.Error()))
	// 分区 / 档位两格分开（ADR-0109 Decision 四）：前者影响基础运费查表，后者只影响一类附加费，续办不同。
	case errors.Is(err, ErrZoneUnresolved):
		return evaluation.withOutcome(EvaluationPending, newEvaluationIssue("ZONE_UNRESOLVED", err.Error()))
	case errors.Is(err, ErrRemoteTierUnresolved):
		return evaluation.withOutcome(EvaluationPending, newEvaluationIssue("REMOTE_TIER_UNRESOLVED", err.Error()))
	case errors.Is(err, ErrReferenceCatalogueMismatch):
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("REFERENCE_CATALOGUE_MISMATCH", err.Error()))
	case errors.Is(err, ErrWeightUnitMismatch):
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("WEIGHT_UNIT_MISMATCH", err.Error()))
	case errors.Is(err, ErrLengthUnitMismatch):
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("LENGTH_UNIT_MISMATCH", err.Error()))
	case errors.Is(err, ErrRateTableConflict), errors.Is(err, ErrRateIntervalOverlap):
		return evaluation.withOutcome(EvaluationConflict, newEvaluationIssue("RATE_TABLE_CONFLICT", err.Error()))
	case errors.Is(err, ErrNegativeChargeTotal):
		return evaluation.withOutcome(EvaluationFailed, newEvaluationIssue("NEGATIVE_TOTAL", err.Error()))
	// 绑了金额序列的规则在窗外声明「待判断」（ADR-0110 Decision 三）：与缺一期取值同一格原因码。
	case errors.Is(err, ErrMissingReferenceSeriesValue):
		return evaluation.withOutcome(EvaluationPending, unresolvedSeriesIssue(err))
	default:
		return evaluation.withOutcome(EvaluationFailed, newEvaluationIssue("CALCULATION_FAILED", err.Error()))
	}
}

// unresolvedSeriesIssue 把「缺一期序列取值」译成带主体的问题项（ADR-0105 Decision 一）：错误里带着绑定时填
// 种类与标识；错误里没有绑定（结构上不该发生）时只留文字，主体为空——不从文字反解析。code 与 message 与
// 此前逐字相同，主体不进规范化文档。
func unresolvedSeriesIssue(err error) EvaluationIssue {
	var missing *missingSeriesReadingError
	if errors.As(err, &missing) {
		return newSeriesEvaluationIssue("REFERENCE_SERIES_UNRESOLVED", err.Error(), missing.binding.kind, missing.binding.seriesID)
	}
	return newEvaluationIssue("REFERENCE_SERIES_UNRESOLVED", err.Error())
}

func (evaluation PricingEvaluation) withOutcome(status EvaluationStatus, issue EvaluationIssue) PricingEvaluation {
	evaluation.status = status
	evaluation.total = nil
	// 「金额精度未声明」修饰的是交出去的那个合计；合计被收回（重放判冲突、后续步骤失败）时它没有
	// 对象了，留着会让一份没有金额的结果看起来像在说金额的事。
	kept := make([]EvaluationIssue, 0, len(evaluation.issues)+1)
	for _, existing := range evaluation.issues {
		if existing.code != amountPrecisionUndeclaredIssue {
			kept = append(kept, existing)
		}
	}
	evaluation.issues = append(kept, issue)
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
	if input.postal != nil {
		route := *input.postal
		copy.postal = &route
	}
	if input.members != nil {
		manifest := *input.members
		manifest.members = append([]PackageID(nil), input.members.members...)
		if input.members.totalVolumetric != nil {
			volumetric := *input.members.totalVolumetric
			manifest.totalVolumetric = &volumetric
		}
		copy.members = &manifest
	}
	copy.factReferences = append([]VersionedFactReference(nil), input.factReferences...)
	copy.seriesValues = append([]ReferenceSeriesValue(nil), input.seriesValues...)
	copy.catalogueReadings = append([]ResolvedCatalogueValue(nil), input.catalogueReadings...)
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
		if reference.SameIdentity(expected) {
			return true
		}
	}
	return false
}

// hasUnsupportedCanonicalization 把「无法按记录形状重新规范化」的重放挡在下面的结果
// 比对之外，使它永远不会被改判成结果不一致。见 ADR-0014。
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
