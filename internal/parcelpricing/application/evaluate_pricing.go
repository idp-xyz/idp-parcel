// Package application 编排 parcelpricing 的评价用例。计算全部在领域（EvaluatePricing
// 是纯函数），这里只做受理、幂等、存续与交付的协调。
package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ErrUnexpectedEvaluationSave 说明评价库交回了封闭集合以外的写入结果。
var ErrUnexpectedEvaluationSave = errors.New("parcel pricing: unexpected evaluation save outcome")

// ErrUnexpectedInForceOutcome 说明在用解析读口交回了封闭集合以外的结果。
var ErrUnexpectedInForceOutcome = errors.New("parcel pricing: unexpected in-force resolution outcome")

// EvaluatePricingOutcome 是评价请求的应用处理结果。
type EvaluatePricingOutcome uint8

const (
	EvaluatePricingOutcomeInvalid EvaluatePricingOutcome = iota
	EvaluationRecorded
	EvaluationExistingResult
	EvaluationConflict
	EvaluationRequestNotAccepted
	EvaluationUndecided
)

func (outcome EvaluatePricingOutcome) String() string {
	switch outcome {
	case EvaluationRecorded:
		return "RECORDED"
	case EvaluationExistingResult:
		return "EXISTING_RESULT"
	case EvaluationConflict:
		return "CONFLICT"
	case EvaluationRequestNotAccepted:
		return "NOT_ACCEPTED"
	case EvaluationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// EvaluatePricingCommand 携带一次评价请求。请求本体（方案版本、输入快照、证据层级、
// 重放引用）全部在领域对象内，编排不拆解它。
type EvaluatePricingCommand struct {
	Request domain.EvaluationRequest
}

type EvaluatePricingResult struct {
	outcome     EvaluatePricingOutcome
	evaluation  domain.PricingEvaluation
	hasRecord   bool
	handoffRef  string
	seriesNotes []string
}

func (result EvaluatePricingResult) Outcome() EvaluatePricingOutcome {
	return result.outcome
}

func (result EvaluatePricingResult) Evaluation() (domain.PricingEvaluation, bool) {
	return result.evaluation, result.hasRecord
}

// HandoffReference 非空说明评价已入册但意图还没交出去，重放会重发同一份。
func (result EvaluatePricingResult) HandoffReference() string {
	return result.handoffRef
}

// SeriesResolutionNotes 是本次形成评价前解析在用序列版本时留下的说明（无已登记版本 /
// 有版本未复核 / 在用版本无覆盖该时点的期次 / 种类不合）。同一份也进了评价的解释。
func (result EvaluatePricingResult) SeriesResolutionNotes() []string {
	return append([]string(nil), result.seriesNotes...)
}

type EvaluatePricingDeps struct {
	Store      ports.EvaluationStore
	Downstream ports.EvaluationHandoff
	Clock      ports.Clock
	// InForce 与 SeriesVersions 成对可选（ADR-0099 决定四）：形成评价前按方案绑定解析在用
	// 序列版本、按计价基准时点在该版本内解析期次，补齐输入快照里缺席的取值。两者都为 nil
	// 时保持既有行为——输入自带取值，或缺取值如实落待判断。只给一个是装配错误，构造时拒。
	InForce        ports.ReferenceSeriesInForceResolver
	SeriesVersions ports.ReferenceSeriesRegister
	// CatalogueInForce 与 Catalogues 成对可选（ADR-0109 Decision 三、四）：形成评价前按卡的目录绑定解析
	// 在用目录版本、按计价基准时点与输入的邮编路线解读数，补齐输入快照里缺席的读数。判据与序列那一对
	// 相同：两者都为 nil 时输入自带读数或缺读数如实落待判断；只给一个是装配错误。
	CatalogueInForce ports.ReferenceCatalogueInForceResolver
	Catalogues       ports.ReferenceCatalogueRegister
}

type EvaluatePricingHandler struct {
	deps EvaluatePricingDeps
}

// ErrSeriesResolutionHalfWired 说明在用解析只装了一半：只能解析在用版本却读不到期次，或
// 反过来，两种都会让评价在编排层静默退回「输入自带取值」的旧行为。构造器签名不改（装配
// 点与替身都靠它），所以在受理时拒——一次评价都不会带着半套装配形成。
var ErrSeriesResolutionHalfWired = errors.New("parcel pricing: InForce and SeriesVersions must be wired together")

// ErrCatalogueResolutionHalfWired 是目录那一对的同形错误。
var ErrCatalogueResolutionHalfWired = errors.New("parcel pricing: CatalogueInForce and Catalogues must be wired together")

func NewEvaluatePricingHandler(deps EvaluatePricingDeps) *EvaluatePricingHandler {
	return &EvaluatePricingHandler{deps: deps}
}

// completion 是本编排交给共用补齐件的那四口与时钟（ADR-0152 决定三）。
func (handler *EvaluatePricingHandler) completion() readingCompletion {
	return readingCompletion{
		clock:            handler.deps.Clock,
		inForce:          handler.deps.InForce,
		seriesVersions:   handler.deps.SeriesVersions,
		catalogueInForce: handler.deps.CatalogueInForce,
		catalogues:       handler.deps.Catalogues,
	}
}

// Handle 把一次评价请求推进到入册结果：幂等按评价标识（同标识同语义即重放返原——
// 评价是纯计算，重复请求不重算不换结果；同标识异语义是冲突不顶替）→ EvaluatePricing
// 纯函数（失败评价同样是版本化结果，照样入册与交付——解释里带着失败原因，不是丢弃品）
// → 保存与意图。金额、精度、取整全在领域结果内，编排零算术。
func (handler *EvaluatePricingHandler) Handle(
	ctx context.Context,
	command EvaluatePricingCommand,
) (EvaluatePricingResult, error) {
	if command.Request.ID().String() == "" {
		return EvaluatePricingResult{outcome: EvaluationRequestNotAccepted}, nil
	}
	if err := handler.completion().halfWired(); err != nil {
		return EvaluatePricingResult{outcome: EvaluationUndecided}, err
	}

	existing, found, err := handler.deps.Store.FindByID(ctx, command.Request.ID())
	if err != nil {
		return EvaluatePricingResult{outcome: EvaluationUndecided}, nil
	}
	if found {
		return handler.settleAgainstExisting(ctx, command, existing), nil
	}

	request, notes, err := handler.completion().complete(ctx, command.Request)
	if err != nil {
		return EvaluatePricingResult{outcome: EvaluationUndecided}, nil
	}
	command.Request = request

	evaluation := domain.EvaluatePricing(command.Request)

	saved, err := handler.deps.Store.Save(ctx, evaluation)
	if err != nil {
		return EvaluatePricingResult{outcome: EvaluationUndecided}, nil
	}
	switch saved {
	case ports.EvaluationSaved:
		result := EvaluatePricingResult{
			outcome:     EvaluationRecorded,
			evaluation:  evaluation,
			hasRecord:   true,
			seriesNotes: notes,
		}
		result.handoffRef = handler.handOff(ctx, evaluation)
		return result, nil
	case ports.EvaluationAlreadyRecorded:
		winner, found, err := handler.deps.Store.FindByID(ctx, command.Request.ID())
		if err != nil || !found {
			return EvaluatePricingResult{outcome: EvaluationUndecided}, nil
		}
		return handler.settleAgainstExisting(ctx, command, winner), nil
	default:
		return EvaluatePricingResult{}, fmt.Errorf("%w: %d", ErrUnexpectedEvaluationSave, saved)
	}
}

// settleAgainstExisting 分辨重放与冒名：把本次请求重算一遍（纯函数，无副作用）后比
// 语义摘要——摘要含选中事实、金额与解释的全部语义，同摘要即同一次计算的重复请求，
// 异摘要即同标识装了不同内容，原评价不顶替。
//
// 请求没带序列取值时，借原评价冻结的那几期来比，不重新解析在用版本：重放使用原序列
// 取值（CONTEXT），而在用版本此刻可能已经换了一版——那不是调用方改了请求。
func (handler *EvaluatePricingHandler) settleAgainstExisting(
	ctx context.Context,
	command EvaluatePricingCommand,
	existing domain.PricingEvaluation,
) EvaluatePricingResult {
	request, err := borrowSeriesReadings(command.Request, existing)
	if err != nil {
		return EvaluatePricingResult{outcome: EvaluationConflict}
	}
	replayed := domain.EvaluatePricing(request)
	if replayed.SemanticDigest() != existing.SemanticDigest() {
		return EvaluatePricingResult{outcome: EvaluationConflict}
	}
	result := EvaluatePricingResult{
		outcome:    EvaluationExistingResult,
		evaluation: existing,
		hasRecord:  true,
	}
	result.handoffRef = handler.handOff(ctx, existing)
	return result
}

// missingSeriesBindings 列出方案绑定了、而请求的输入快照里没有取值的序列。重放请求一律
// 视为不缺：重放携带原输入，不重新解析在用（ADR-0099 决定四）。
func missingSeriesBindings(request domain.EvaluationRequest) []domain.ReferenceSeriesBinding {
	if _, replay := request.ReplayOf(); replay {
		return nil
	}
	present := make(map[string]struct{})
	for _, value := range request.Input().ReferenceSeriesValues() {
		present[value.ReadingKey()] = struct{}{}
	}
	missing := make([]domain.ReferenceSeriesBinding, 0)
	for _, binding := range request.Plan().Structures().ReferenceSeries() {
		if _, found := present[binding.ReadingKey()]; !found {
			missing = append(missing, binding)
		}
	}
	return missing
}

// withSeriesReadings 把补齐的取值装回请求。评价请求的其余部分（标识、方案、证据层级、回指）
// 原样保留——经领域的 WithInput 只换输入一格，不重新构造；输入快照按领域的 WithReferenceSeries
// 重立，缺一个都装不进去。
func withSeriesReadings(request domain.EvaluationRequest, readings []domain.ReferenceSeriesValue) (domain.EvaluationRequest, error) {
	if len(readings) == 0 {
		return request, nil
	}
	values := append(request.Input().ReferenceSeriesValues(), readings...)
	input, err := request.Input().WithReferenceSeries(values...)
	if err != nil {
		return domain.EvaluationRequest{}, err
	}
	return request.WithInput(input)
}

// borrowSeriesReadings 从原评价冻结的输入里借出请求缺的那几期取值，供重放比对。
func borrowSeriesReadings(request domain.EvaluationRequest, existing domain.PricingEvaluation) (domain.EvaluationRequest, error) {
	missing := missingSeriesBindings(request)
	if len(missing) == 0 {
		return request, nil
	}
	frozen := make(map[string]domain.ReferenceSeriesValue)
	for _, value := range existing.Input().ReferenceSeriesValues() {
		frozen[value.ReadingKey()] = value
	}
	readings := make([]domain.ReferenceSeriesValue, 0, len(missing))
	for _, binding := range missing {
		if value, found := frozen[binding.ReadingKey()]; found && value.Reference().ID() == binding.SeriesID() {
			readings = append(readings, value)
		}
	}
	return withSeriesReadings(request, readings)
}

// missingCatalogueLinks 列出卡绑定了、而请求的输入快照里没有读数的目录。重放一律视为不缺（重放携带原读数）。
func missingCatalogueLinks(request domain.EvaluationRequest) []domain.ReferenceCatalogueLink {
	if _, replay := request.ReplayOf(); replay {
		return nil
	}
	present := make(map[domain.CatalogueKind]struct{})
	for _, reading := range request.Input().CatalogueReadings() {
		present[reading.Kind()] = struct{}{}
	}
	missing := make([]domain.ReferenceCatalogueLink, 0)
	for _, link := range request.Plan().Structures().ReferenceCatalogues() {
		if _, found := present[link.Kind()]; !found {
			missing = append(missing, link)
		}
	}
	return missing
}

// handOff 交发布意图。失败不翻评价，留续办引用重发同一份。
func (handler *EvaluatePricingHandler) handOff(
	ctx context.Context,
	evaluation domain.PricingEvaluation,
) string {
	if err := handler.deps.Downstream.HandOffEvaluation(ctx, ports.EvaluationHandoffIntent{
		Evaluation: evaluation,
	}); err == nil {
		return ""
	}
	return "CONT-EVALUATION/" + evaluation.ID().String()
}
