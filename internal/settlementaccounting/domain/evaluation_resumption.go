package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidEvaluationResumption 说明停因、参数种类、续办依据或发起停办的某一格立不住。
var ErrInvalidEvaluationResumption = errors.New("settlement accounting: invalid evaluation resumption")

// EvaluationDirection 是一份评价请求的方向：BUY 指向供应商预期成本，SELL 指向预估客户费用。两册分表、两种请求标识，
// 停因与续办共用一套词表，方向决定一个停因在哪一册上能出现。
type EvaluationDirection uint8

const (
	EvaluationDirectionInvalid EvaluationDirection = iota
	BuyEvaluationDirection
	SellEvaluationDirection
)

func (direction EvaluationDirection) String() string {
	switch direction {
	case BuyEvaluationDirection:
		return "BUY"
	case SellEvaluationDirection:
		return "SELL"
	default:
		return ""
	}
}

// EvaluationDirectionFromName 把载荷或册上的方向词译回常量，词表外的拒。
func EvaluationDirectionFromName(name string) (EvaluationDirection, error) {
	for _, candidate := range []EvaluationDirection{BuyEvaluationDirection, SellEvaluationDirection} {
		if candidate.String() == name {
			return candidate, nil
		}
	}
	return EvaluationDirectionInvalid, fmt.Errorf("%w: evaluation direction %q", ErrInvalidEvaluationResumption, name)
}

// EvaluationStopStage 是停因停在链上的哪一段。三段的恢复方各不相同：发起之前缺的是结算这一侧的登记或包裹那一侧的
// 事实，请求还没有发出去；计价没形成评价时请求已在册、计价答不出；评价采用不了时评价已在计价那一侧、不可变。
type EvaluationStopStage uint8

const (
	EvaluationStopStageInvalid EvaluationStopStage = iota
	StopBeforeRequest
	StopInPricing
	StopAtAdoption
)

func (stage EvaluationStopStage) String() string {
	switch stage {
	case StopBeforeRequest:
		return "BEFORE_REQUEST"
	case StopInPricing:
		return "IN_PRICING"
	case StopAtAdoption:
		return "AT_ADOPTION"
	default:
		return ""
	}
}

// EvaluationStopCause 是一份评价请求（或还没发出的那一拍）停下的原因，封闭集。它记在册上，是续办按什么来的依据：
// 一次参数登记只续办它解得开的那几种停因（ResumedBy），其余的留给下一次登记或运营续办口。
//
// 停因都是如实的答复，不是缺省值的替身：`未配置`一族说的是租户取值缺位，补登之前续办不会发生，本上下文也不替租户
// 填任何一格。
type EvaluationStopCause uint8

const (
	EvaluationStopCauseInvalid EvaluationStopCause = iota
	// 发起之前·SELL：包裹到结算账户那条链停在哪一格。
	StopSettlementPolicyNotAdopted
	StopSettlementAccountNotRegistered
	StopSellBillingMomentNotAdopted
	StopParcelCommercialResolutionAbsent
	// 发起之前·BUY：BUY 触发册没登这个原因，或登了却没登费用项目。
	StopOccurrenceReasonNotRegistered
	StopTriggerFeeItemNotRegistered
	// 计价没形成评价：计价对请求答不出一份评价。
	StopPriceCardNotConfigured
	StopPriceCardApplicabilityConflict
	StopPricingInputUnavailable
	// 评价采用不了：评价已形成，结算这一侧形成不了费用或成本。待判断按缺的是什么再分几格，缺的东西不同，解得开它的
	// 登记就不同：目录读不出（分区、偏远档位）、序列读不出（参考序列、汇率）、价表上没有这一段、包裹事实不全；分不清
	// 或不止一样缺的，落笼统的那一格。
	StopEvaluationPendingJudgment
	StopEvaluationUnratable
	StopEvaluationConflict
	StopEvaluationFailed
	StopAmountRoundingUndeclared
	StopConversionStepMissing
	StopFeeItemMappingIncomplete
	StopEvaluationCatalogueUnresolved
	StopEvaluationSeriesUnresolved
	StopEvaluationRateNotFound
	StopEvaluationInputIncomplete
	// StopPendingJudgmentCauseUnrecorded 只出自迁移：停因一列落地之前就已记成`待判断`的 SELL 请求，当时没记是哪一种
	// 待判断。迁移不猜它是评价待判断、取整未声明还是映射不全，所以三类登记都解得开它。
	StopPendingJudgmentCauseUnrecorded
)

var allEvaluationStopCauses = []EvaluationStopCause{
	StopSettlementPolicyNotAdopted,
	StopSettlementAccountNotRegistered,
	StopSellBillingMomentNotAdopted,
	StopParcelCommercialResolutionAbsent,
	StopOccurrenceReasonNotRegistered,
	StopTriggerFeeItemNotRegistered,
	StopPriceCardNotConfigured,
	StopPriceCardApplicabilityConflict,
	StopPricingInputUnavailable,
	StopEvaluationPendingJudgment,
	StopEvaluationUnratable,
	StopEvaluationConflict,
	StopEvaluationFailed,
	StopAmountRoundingUndeclared,
	StopConversionStepMissing,
	StopFeeItemMappingIncomplete,
	StopEvaluationCatalogueUnresolved,
	StopEvaluationSeriesUnresolved,
	StopEvaluationRateNotFound,
	StopEvaluationInputIncomplete,
	StopPendingJudgmentCauseUnrecorded,
}

func (cause EvaluationStopCause) String() string {
	switch cause {
	case StopSettlementPolicyNotAdopted:
		return "SETTLEMENT_POLICY_NOT_ADOPTED"
	case StopSettlementAccountNotRegistered:
		return "SETTLEMENT_ACCOUNT_NOT_REGISTERED"
	case StopSellBillingMomentNotAdopted:
		return "SELL_BILLING_MOMENT_NOT_ADOPTED"
	case StopParcelCommercialResolutionAbsent:
		return "PARCEL_HAS_NO_COMMERCIAL_RESOLUTION"
	case StopOccurrenceReasonNotRegistered:
		return "OCCURRENCE_REASON_NOT_REGISTERED"
	case StopTriggerFeeItemNotRegistered:
		return "FEE_ITEM_NOT_REGISTERED"
	case StopPriceCardNotConfigured:
		return "PRICE_CARD_NOT_CONFIGURED"
	case StopPriceCardApplicabilityConflict:
		return "PRICE_CARD_APPLICABILITY_CONFLICT"
	case StopPricingInputUnavailable:
		return "PRICING_INPUT_UNAVAILABLE"
	case StopEvaluationPendingJudgment:
		return "EVALUATION_PENDING_JUDGMENT"
	case StopEvaluationUnratable:
		return "EVALUATION_UNRATABLE"
	case StopEvaluationConflict:
		return "EVALUATION_CONFLICT"
	case StopEvaluationFailed:
		return "EVALUATION_FAILED"
	case StopAmountRoundingUndeclared:
		return "AMOUNT_ROUNDING_UNDECLARED"
	case StopConversionStepMissing:
		return "CONVERSION_STEP_MISSING"
	case StopFeeItemMappingIncomplete:
		return "FEE_ITEM_MAPPING_INCOMPLETE"
	case StopEvaluationCatalogueUnresolved:
		return "EVALUATION_CATALOGUE_UNRESOLVED"
	case StopEvaluationSeriesUnresolved:
		return "EVALUATION_SERIES_UNRESOLVED"
	case StopEvaluationRateNotFound:
		return "EVALUATION_RATE_NOT_FOUND"
	case StopEvaluationInputIncomplete:
		return "EVALUATION_INPUT_INCOMPLETE"
	case StopPendingJudgmentCauseUnrecorded:
		return "PENDING_JUDGMENT_CAUSE_UNRECORDED"
	default:
		return ""
	}
}

// EvaluationStopCauseFromName 把册上或载荷里的停因词译回常量。词表外的拒：册上出现本上下文不认识的停因只可能是
// 适配器或迁移的 bug，读成某一格会让续办按错的登记种类来。
func EvaluationStopCauseFromName(name string) (EvaluationStopCause, error) {
	for _, candidate := range allEvaluationStopCauses {
		if candidate.String() == name {
			return candidate, nil
		}
	}
	return EvaluationStopCauseInvalid, fmt.Errorf("%w: evaluation stop cause %q", ErrInvalidEvaluationResumption, name)
}

// Stage 答这一停因停在链上的哪一段。
func (cause EvaluationStopCause) Stage() EvaluationStopStage {
	switch cause {
	case StopSettlementPolicyNotAdopted, StopSettlementAccountNotRegistered, StopSellBillingMomentNotAdopted,
		StopParcelCommercialResolutionAbsent, StopOccurrenceReasonNotRegistered, StopTriggerFeeItemNotRegistered:
		return StopBeforeRequest
	case StopPriceCardNotConfigured, StopPriceCardApplicabilityConflict, StopPricingInputUnavailable:
		return StopInPricing
	case StopEvaluationPendingJudgment, StopEvaluationUnratable, StopEvaluationConflict, StopEvaluationFailed,
		StopAmountRoundingUndeclared, StopConversionStepMissing, StopFeeItemMappingIncomplete,
		StopEvaluationCatalogueUnresolved, StopEvaluationSeriesUnresolved, StopEvaluationRateNotFound,
		StopEvaluationInputIncomplete, StopPendingJudgmentCauseUnrecorded:
		return StopAtAdoption
	default:
		return EvaluationStopStageInvalid
	}
}

// PendingJudgmentOfEvaluation 答这一停因是不是评价`待判断`的一格（笼统的那一格或它的细分）。读口按问题项分格，分不出
// 的落笼统那一格；形成编排拿到不是这几格的值时也落笼统那一格——宁可多推一次续办，不可让它停在一格什么登记都解不开的
// 停因上。
func (cause EvaluationStopCause) PendingJudgmentOfEvaluation() bool {
	switch cause {
	case StopEvaluationPendingJudgment, StopEvaluationCatalogueUnresolved, StopEvaluationSeriesUnresolved,
		StopEvaluationRateNotFound, StopEvaluationInputIncomplete:
		return true
	default:
		return false
	}
}

// OccursIn 答这一停因能不能出现在这个方向的册上。发起之前那几格各属一路的链；换算步骤缺席只在 BUY 那一路是一格
// （SELL 那一路结算币种对不上请求快照是矛盾，上抛）；映射不全与迁移留下的待判断只在 SELL 那一路有。
func (cause EvaluationStopCause) OccursIn(direction EvaluationDirection) bool {
	switch cause {
	case StopSettlementPolicyNotAdopted, StopSettlementAccountNotRegistered, StopSellBillingMomentNotAdopted,
		StopParcelCommercialResolutionAbsent, StopFeeItemMappingIncomplete, StopPendingJudgmentCauseUnrecorded:
		return direction == SellEvaluationDirection
	case StopOccurrenceReasonNotRegistered, StopTriggerFeeItemNotRegistered, StopConversionStepMissing:
		return direction == BuyEvaluationDirection
	case StopPriceCardNotConfigured, StopPriceCardApplicabilityConflict, StopPricingInputUnavailable,
		StopEvaluationPendingJudgment, StopEvaluationUnratable, StopEvaluationConflict, StopEvaluationFailed,
		StopAmountRoundingUndeclared, StopEvaluationCatalogueUnresolved, StopEvaluationSeriesUnresolved,
		StopEvaluationRateNotFound, StopEvaluationInputIncomplete:
		return direction == BuyEvaluationDirection || direction == SellEvaluationDirection
	default:
		return false
	}
}

// ResumedBy 答一次这一类参数登记能不能解开这一停因。判据是恢复动作：登了这一类参数，按同一个问题再问一次可能得到
// 不同的答案，才算解得开；登了也不会变的不列。
//
//   - 结算政策没采用：答案在委托接受时固定的商业闭包上，结算这一侧的任何登记都改不了它，运营续办重跑读到的也是同一份
//     闭包——今天没有续办路（ADR-0229 越权风险点「结算政策没采用今天没有续办路」）。
//   - 包裹没有商业解析、计价输入不可得、包裹事实不全：缺的是小包托运或运输履约那一侧的事实，那一侧补上之后由运营续办
//     口点名。
//   - 目录读不出：卡绑了目录却没映到，补一版目录解得开；卡根本没绑目录、请求也没带分区，只有绑了目录的新一版卡解得开。
//     序列读不出同理：绑了却缺一期读数是序列的事，卡没绑汇率序列是卡的事。所以两格都认卡，各自再认自己那一类；序列
//     不推缺分区的请求。
//   - 价表没有这一段：多半出自卡，换一版卡才可能变；卡绑了目录、目录把邮编映到了错区时补一版目录也解得开。
//   - 不可计价、评价冲突、评价失败、金额取整未声明：答案出自卡，换一版卡才可能变。
//   - 换算步骤缺席：卡没绑汇率序列，或序列在计价基准时点没有在用版本。
//   - 笼统的待判断：分不清缺哪一样，卡、序列、目录三类登记都可能解开。
func (cause EvaluationStopCause) ResumedBy(kind EvaluationParameterKind) bool {
	switch cause {
	case StopSettlementAccountNotRegistered:
		return kind == ParameterSettlementAccount
	case StopSellBillingMomentNotAdopted:
		return kind == ParameterSellBillingMomentAdoption
	case StopOccurrenceReasonNotRegistered, StopTriggerFeeItemNotRegistered:
		return kind == ParameterBuyEvaluationTrigger
	case StopPriceCardNotConfigured, StopPriceCardApplicabilityConflict, StopEvaluationUnratable,
		StopEvaluationConflict, StopEvaluationFailed, StopAmountRoundingUndeclared:
		return kind == ParameterPriceCard
	case StopEvaluationCatalogueUnresolved, StopEvaluationRateNotFound:
		return kind == ParameterPriceCard || kind == ParameterReferenceCatalogue
	case StopEvaluationSeriesUnresolved:
		return kind == ParameterPriceCard || kind == ParameterReferenceSeries
	case StopEvaluationPendingJudgment:
		return kind == ParameterPriceCard || kind == ParameterReferenceSeries || kind == ParameterReferenceCatalogue
	case StopConversionStepMissing:
		return kind == ParameterPriceCard || kind == ParameterReferenceSeries
	case StopFeeItemMappingIncomplete:
		return kind == ParameterFeeItemMapping
	case StopPendingJudgmentCauseUnrecorded:
		return kind == ParameterPriceCard || kind == ParameterReferenceSeries ||
			kind == ParameterReferenceCatalogue || kind == ParameterFeeItemMapping
	default:
		return false
	}
}

// EvaluationStopCausesResumedBy 列出一类参数登记解得开的全部停因。续办按它去册上取等着的那几份；没有一格的种类
// 交回空集，续办据此什么都不做。
func EvaluationStopCausesResumedBy(kind EvaluationParameterKind) []EvaluationStopCause {
	var causes []EvaluationStopCause
	for _, cause := range allEvaluationStopCauses {
		if cause.ResumedBy(kind) {
			causes = append(causes, cause)
		}
	}
	return causes
}

// AllEvaluationStopCauses 是运营续办口用的全集：运营人员看过停因、确认缺的东西已经补上之后，任何一格都可以续办。
func AllEvaluationStopCauses() []EvaluationStopCause {
	return append([]EvaluationStopCause(nil), allEvaluationStopCauses...)
}

// EvaluationParameterKind 是一次「参数已登记」的登记种类，封闭集。它说的是「这一类参数在这个租户里有新一版落库
// 了」，不是登了什么：正文留在登记它的那一册上。前四种由结算自己的登记口交出，后三种由计价的登记口交出。
type EvaluationParameterKind uint8

const (
	EvaluationParameterKindInvalid EvaluationParameterKind = iota
	ParameterSettlementAccount
	ParameterSellBillingMomentAdoption
	ParameterFeeItemMapping
	ParameterBuyEvaluationTrigger
	ParameterPriceCard
	ParameterReferenceSeries
	ParameterReferenceCatalogue
)

func (kind EvaluationParameterKind) String() string {
	switch kind {
	case ParameterSettlementAccount:
		return "SETTLEMENT_ACCOUNT"
	case ParameterSellBillingMomentAdoption:
		return "SELL_BILLING_MOMENT_ADOPTION"
	case ParameterFeeItemMapping:
		return "FEE_ITEM_MAPPING"
	case ParameterBuyEvaluationTrigger:
		return "BUY_EVALUATION_TRIGGER"
	case ParameterPriceCard:
		return "PRICE_CARD"
	case ParameterReferenceSeries:
		return "REFERENCE_SERIES"
	case ParameterReferenceCatalogue:
		return "REFERENCE_CATALOGUE"
	default:
		return ""
	}
}

// EvaluationParameterKindFromName 把载荷里的登记种类译回常量，词表外的拒。
func EvaluationParameterKindFromName(name string) (EvaluationParameterKind, error) {
	for _, candidate := range []EvaluationParameterKind{
		ParameterSettlementAccount, ParameterSellBillingMomentAdoption, ParameterFeeItemMapping,
		ParameterBuyEvaluationTrigger, ParameterPriceCard, ParameterReferenceSeries, ParameterReferenceCatalogue,
	} {
		if candidate.String() == name {
			return candidate, nil
		}
	}
	return EvaluationParameterKindInvalid, fmt.Errorf("%w: evaluation parameter kind %q", ErrInvalidEvaluationResumption, name)
}

// EvaluationParameterNarrowing 是一类登记的「参数已登记」信封必带的收窄格：续办只取这一次登记真解得开的那几件，
// 不按种类把整个租户里同停因的件推一遍。
//
//   - 计价的三类（价卡、序列、目录）带计价范围与方向：卡带自己的，序列与目录带绑了它的每一张卡的。一张 BUY 卡登了，
//     SELL 那一路问同一个范围的请求照样等着；一条序列喂了一期，没绑它的卡上停着的请求不是在等它。
//   - BUY 触发册带发生项原因：登的是这一个原因那一行，别的原因上停着的发生项照样缺。
//   - SELL 计费时点采用带户：采用是按户登的。
//   - 结算账户不带：「结算账户已登记」那一封与小包托运共用，载荷只有租户与种类，不带账户键。停在账户没登的那一刻账户键
//     其实已求出、记得下，缺的是信封上可比的那一格；每登一户（含应付户）都把全租户停在账户没登上的发起停办重跑一遍，
//     多推的那几拍照样停着，只换上这一次的依据与时刻。
//   - 费用项目映射不带：映射按计价费用代码登，停下的请求上记的是映射不全，没有记缺哪一个代码；多推的请求起了下一代，
//     照样停在同一格（ADR-0229 越权风险点「收窄到哪一层」）。
type EvaluationParameterNarrowing struct {
	PricingScope     bool
	Direction        bool
	OccurrenceReason bool
	Account          bool
}

// Narrowing 答这一类登记的信封必带哪几格。
func (kind EvaluationParameterKind) Narrowing() EvaluationParameterNarrowing {
	switch kind {
	case ParameterPriceCard, ParameterReferenceSeries, ParameterReferenceCatalogue:
		return EvaluationParameterNarrowing{PricingScope: true, Direction: true}
	case ParameterBuyEvaluationTrigger:
		return EvaluationParameterNarrowing{OccurrenceReason: true}
	case ParameterSellBillingMomentAdoption:
		return EvaluationParameterNarrowing{Account: true}
	default:
		return EvaluationParameterNarrowing{}
	}
}

// 续办依据的两种来源段。不是标识铸造前缀，照样以 Prefix 结尾进前缀门禁：依据串与各上下文铸造的标识在日志里混看。
const (
	parameterResumeBasisPrefix = "PARAMREG"
	operatorResumeBasisPrefix  = "OPSRESUME"
)

// ResumeBasis 是一次续办凭的什么：一次参数登记（种类加那封信的标识），或一次运营续办（运营人员给的续办引用）。它记在
// 续办出来的那一代请求上、也记在重跑之后仍停着的发起停办上，同一个依据对同一条代次链至多续办一次——同一封信重投、
// 同一条运营命令重跑，都认得出自己已经续办过。
type ResumeBasis struct{ requiredValue }

// NewParameterResumeBasis 由一次参数登记造依据。reference 是那封「参数已登记」信封的标识：同一次登记重放发的是同一封，
// 两次登记是两封。
func NewParameterResumeBasis(kind EvaluationParameterKind, reference string) (ResumeBasis, error) {
	if kind.String() == "" || strings.TrimSpace(reference) == "" {
		return ResumeBasis{}, fmt.Errorf("%w: parameter resume basis", ErrInvalidEvaluationResumption)
	}
	return ResumeBasis{requiredValue{value: parameterResumeBasisPrefix + "/" + kind.String() + "/" + reference}}, nil
}

// NewOperatorResumeBasis 由一次运营续办造依据。reference 由运营人员给出（工单号或决定标识），同一个引用重跑是重放。
func NewOperatorResumeBasis(reference string) (ResumeBasis, error) {
	if strings.TrimSpace(reference) == "" {
		return ResumeBasis{}, fmt.Errorf("%w: operator resume basis", ErrInvalidEvaluationResumption)
	}
	return ResumeBasis{requiredValue{value: operatorResumeBasisPrefix + "/" + reference}}, nil
}

// ResumeBasisFromName 重建册上的依据串。只认两种来源段，别的拒。
func ResumeBasisFromName(value string) (ResumeBasis, error) {
	if strings.HasPrefix(value, parameterResumeBasisPrefix+"/") || strings.HasPrefix(value, operatorResumeBasisPrefix+"/") {
		return ResumeBasis{requiredValue{value: value}}, nil
	}
	return ResumeBasis{}, fmt.Errorf("%w: resume basis %q", ErrInvalidEvaluationResumption, value)
}

// Present 答依据在不在场：首代请求由原始那一拍发起时没有依据。
func (basis ResumeBasis) Present() bool { return basis.valid() }

// firstGeneration 是代次的起点。代次从一起数：零值规格当首代读，旧调用点不必为它改。
const firstGeneration = 1

// normalizeLineage 核一份请求的代次三格并交回规整后的代次。首代没有前一代；续办出来的各代必须指名前一代并带依据——
// 没有依据的第二代说不清凭什么续办，同一封信重投时也认不出自己续办过。首代可以带依据：发起停办经续办重跑之后
// 发出的第一份请求就是。
func normalizeLineage(generation int, hasPrevious bool, basis ResumeBasis) (int, error) {
	if generation == 0 {
		generation = firstGeneration
	}
	switch {
	case generation < firstGeneration:
		return 0, fmt.Errorf("%w: generation %d", ErrInvalidEvaluationResumption, generation)
	case generation == firstGeneration && hasPrevious:
		return 0, fmt.Errorf("%w: the first generation has no previous request", ErrInvalidEvaluationResumption)
	case generation > firstGeneration && (!hasPrevious || !basis.Present()):
		return 0, fmt.Errorf("%w: generation %d needs its previous request and a resume basis", ErrInvalidEvaluationResumption, generation)
	}
	return generation, nil
}

// SellEvaluationInitiationStallSpec 是一拍包裹计费时点停在发起之前的全部输入：那一拍本身（包裹、时点、来源事实、计价
// 基准时点，续办时原样重跑），停因，停下的时刻，以及若是续办重跑之后又停下，凭的那个依据。Account 只在停在计费时点
// 没采用时在场：链已走到那一户，补登采用时只续办那一户的。
type SellEvaluationInitiationStallSpec struct {
	Parcel         ParcelReference
	Moment         ParcelBillingMoment
	Source         SourceFactReference
	PricingBasisAt time.Time
	Cause          EvaluationStopCause
	StoppedAt      time.Time
	Basis          ResumeBasis
	Account        SettlementAccountID
}

// SellEvaluationInitiationStall 是 SELL 那一路的发起停办：包裹计费时点到了，链停在结算账户之前或采用册上，一份请求都
// 没发出去。它按自然键（包裹、时点）记，不是请求：请求要的账户、币种与计价范围此刻都答不出来。续办时按记下的那一拍
// 原样重跑发起编排；这件包裹在这一时点有了请求，它就不再等着。
type SellEvaluationInitiationStall struct {
	naturalKey     SellEvaluationRequestNaturalKey
	source         SourceFactReference
	pricingBasisAt time.Time
	cause          EvaluationStopCause
	stoppedAt      time.Time
	basis          ResumeBasis
	account        SettlementAccountID
}

func NewSellEvaluationInitiationStall(spec SellEvaluationInitiationStallSpec) (SellEvaluationInitiationStall, error) {
	if !spec.Parcel.valid() || !spec.Moment.valid() || !spec.Source.valid() || spec.PricingBasisAt.IsZero() ||
		spec.StoppedAt.IsZero() || spec.Cause.Stage() != StopBeforeRequest || !spec.Cause.OccursIn(SellEvaluationDirection) ||
		(spec.Cause == StopSellBillingMomentNotAdopted) != spec.Account.valid() {
		return SellEvaluationInitiationStall{}, fmt.Errorf("%w: sell evaluation initiation stall", ErrInvalidEvaluationResumption)
	}
	return SellEvaluationInitiationStall{
		naturalKey:     SellEvaluationRequestNaturalKey{Parcel: spec.Parcel, Moment: spec.Moment},
		source:         spec.Source,
		pricingBasisAt: spec.PricingBasisAt.UTC(),
		cause:          spec.Cause,
		stoppedAt:      spec.StoppedAt.UTC(),
		basis:          spec.Basis,
		account:        spec.Account,
	}, nil
}

func (stall SellEvaluationInitiationStall) NaturalKey() SellEvaluationRequestNaturalKey {
	return stall.naturalKey
}
func (stall SellEvaluationInitiationStall) Source() SourceFactReference { return stall.source }
func (stall SellEvaluationInitiationStall) PricingBasisAt() time.Time   { return stall.pricingBasisAt }
func (stall SellEvaluationInitiationStall) Cause() EvaluationStopCause  { return stall.cause }
func (stall SellEvaluationInitiationStall) StoppedAt() time.Time        { return stall.stoppedAt }
func (stall SellEvaluationInitiationStall) Basis() ResumeBasis          { return stall.basis }

// Account 交回停在计费时点没采用时那一户；别的停因第二个返回值为假。
func (stall SellEvaluationInitiationStall) Account() (SettlementAccountID, bool) {
	return stall.account, stall.account.valid()
}

// BuyEvaluationInitiationStallSpec 是一份已形成的发生项停在 BUY 发起之前的全部输入：发生项（标识、原因、有效性版本、
// 业务时间）与它固定的供应商协议，续办时原样重跑发起编排。修订版不经本路发起，所以不在这里。
type BuyEvaluationInitiationStallSpec struct {
	Occurrence TransportChargeOccurrence
	Agreement  SupplierAgreementReference
	Cause      EvaluationStopCause
	StoppedAt  time.Time
	Basis      ResumeBasis
}

// BuyEvaluationInitiationStall 是 BUY 那一路的发起停办：发生项形成了，触发册答`未配置`，一份请求都没发出去。按发生项
// 的（标识、有效性版本）记；这一版发生项有了请求，它就不再等着。
type BuyEvaluationInitiationStall struct {
	occurrence TransportChargeOccurrence
	agreement  SupplierAgreementReference
	cause      EvaluationStopCause
	stoppedAt  time.Time
	basis      ResumeBasis
}

func NewBuyEvaluationInitiationStall(spec BuyEvaluationInitiationStallSpec) (BuyEvaluationInitiationStall, error) {
	if !spec.Occurrence.ID().valid() || !spec.Occurrence.Reason().valid() || !spec.Occurrence.Version().valid() ||
		spec.Occurrence.OccurredAt().IsZero() || !spec.Agreement.valid() || spec.StoppedAt.IsZero() ||
		spec.Cause.Stage() != StopBeforeRequest || !spec.Cause.OccursIn(BuyEvaluationDirection) {
		return BuyEvaluationInitiationStall{}, fmt.Errorf("%w: buy evaluation initiation stall", ErrInvalidEvaluationResumption)
	}
	return BuyEvaluationInitiationStall{
		occurrence: spec.Occurrence,
		agreement:  spec.Agreement,
		cause:      spec.Cause,
		stoppedAt:  spec.StoppedAt.UTC(),
		basis:      spec.Basis,
	}, nil
}

func (stall BuyEvaluationInitiationStall) Occurrence() TransportChargeOccurrence {
	return stall.occurrence
}
func (stall BuyEvaluationInitiationStall) Agreement() SupplierAgreementReference {
	return stall.agreement
}
func (stall BuyEvaluationInitiationStall) Cause() EvaluationStopCause { return stall.cause }
func (stall BuyEvaluationInitiationStall) StoppedAt() time.Time       { return stall.stoppedAt }
func (stall BuyEvaluationInitiationStall) Basis() ResumeBasis         { return stall.basis }
