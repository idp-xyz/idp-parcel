// request_buy_evaluation.go 编排 UC-SA-002 步 2「请求评价」的 SA 半边（票 sa-cc/08）：把结算提交主要
// 范围、计算目的与合格来源引用三件登进评价请求登记册，并在同一事务里经 Outbox 向 parcel-pricing 发
// 一封只带引用的 `settlement-accounting.evaluation-request.submitted`（裁决 1）。步 2 的后半（提供方采用
// 来源形成计价输入快照）归 parcel-pricing；步 5 的形成在 form_supplier_expected_cost.go。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUnexpectedEvaluationRequestSave 说明评价请求登记面交回了封闭集合以外的写入结果。
var ErrUnexpectedEvaluationRequestSave = errors.New("settlement accounting: unexpected evaluation request save outcome")

// EvaluationRequestOutcome 是一次请求评价的应用处理结果。`已存在`是业务答案不是失败（裁决 2：同自然键
// 重复提交交回原 ID）；`未受理`与`未决`分格（ADR-0029 按恢复动作分格）：前者改请求，后者等依赖。
type EvaluationRequestOutcome uint8

const (
	EvaluationRequestOutcomeInvalid EvaluationRequestOutcome = iota
	EvaluationRequested
	EvaluationRequestExisting
	EvaluationRequestNotAccepted
	EvaluationRequestUndecided
)

func (outcome EvaluationRequestOutcome) String() string {
	switch outcome {
	case EvaluationRequested:
		return "EVALUATION_REQUESTED"
	case EvaluationRequestExisting:
		return "EXISTING_EVALUATION_REQUEST"
	case EvaluationRequestNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case EvaluationRequestUndecided:
		return "EVALUATION_REQUEST_UNDECIDED"
	default:
		return ""
	}
}

// EvaluationRequestUndecidedReason 指名请求停在哪一步等谁。
type EvaluationRequestUndecidedReason uint8

const (
	EvaluationRequestUndecidedReasonNone EvaluationRequestUndecidedReason = iota
	EvaluationRequestRegistryUnavailable
	EvaluationRequestIdentityUnavailable
)

func (reason EvaluationRequestUndecidedReason) String() string {
	switch reason {
	case EvaluationRequestRegistryUnavailable:
		return "EVALUATION_REQUEST_REGISTRY_UNAVAILABLE"
	case EvaluationRequestIdentityUnavailable:
		return "EVALUATION_REQUEST_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// RequestBuyEvaluationCommand 携带一次 BUY 评价请求。发生项 / 费用项目 / 供应商协议引用由调用方（结算作业或上游编排）交进来，
// 本编排不替它从 TF 登记册推——推导出来的匹配不是「合格来源引用」（票面红线）。计算目的不在命令上：
// 本编排请求的只有 BUY 供应商成本这一格，词表由领域封闭。金额、币种、换算一格都没有——它们整组
// 出自评价（ADR-0107 / ADR-0013），命令带得了它们就有了第二套数字。
type RequestBuyEvaluationCommand struct {
	TenantID    domain.TenantID
	Scope       domain.PrimaryScopeReference
	Occurrence  domain.TransportChargeOccurrence
	FeeItem     domain.FeeItemReference
	Agreement   domain.SupplierAgreementReference
	RequestedBy domain.RequesterReference
}

type RequestBuyEvaluationResult struct {
	outcome      EvaluationRequestOutcome
	reason       EvaluationRequestUndecidedReason
	record       ports.EvaluationRequestRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result RequestBuyEvaluationResult) Outcome() EvaluationRequestOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result RequestBuyEvaluationResult) UndecidedReason() EvaluationRequestUndecidedReason {
	return result.reason
}

// Request 在已请求与`已存在`两格交回登记下的那份——`已存在`交回的是先到者，ID 是它的。
func (result RequestBuyEvaluationResult) Request() (ports.EvaluationRequestRecord, bool) {
	return result.record, result.hasRecord
}

func (result RequestBuyEvaluationResult) ContinuationReference() string {
	return result.continuation
}

// EvaluationRequestHandoffReference 非空说明请求已登记但信封还没交出去，重放会重发同一份。
func (result RequestBuyEvaluationResult) EvaluationRequestHandoffReference() string {
	return result.handoff
}

// RequestBuyEvaluationDeps 是请求评价编排的依赖。四口都必备，缺任何一口这条编排都走不完：登记面与
// 信封同事务缺一不可（裁决 1），铸造 ID 是请求的身份（裁决 2）。
type RequestBuyEvaluationDeps struct {
	Registry   ports.EvaluationRequestRegistry
	Identity   ports.EvaluationRequestIdentityFactory
	Downstream ports.EvaluationRequestHandoff
	Clock      ports.Clock
}

type RequestBuyEvaluationHandler struct {
	deps RequestBuyEvaluationDeps
}

// NewRequestBuyEvaluationHandler 构造期拒 nil：漏装一口在这里就报出来，而不是等第一次请求时在
// 解引用处 panic——那种 panic 被路由层兜成没有稳定 code 的 500，读的人分不出是进程坏了还是装配漏了。
//
// 缺件一律包 ErrNilDependency（同包 NewApplyPreAcceptanceControlHandler 那张表的形），哪一口缺在文本里
// 点名：装配方按 errors.Is 就能把「装配漏了」与别的构造错误分开，裸 fmt.Errorf 做不到这一点。
func NewRequestBuyEvaluationHandler(deps RequestBuyEvaluationDeps) (*RequestBuyEvaluationHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"evaluation request registry", deps.Registry == nil},
		{"evaluation request identity factory", deps.Identity == nil},
		{"evaluation request downstream handoff", deps.Downstream == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &RequestBuyEvaluationHandler{deps: deps}, nil
}

// Handle 把一次 BUY 评价请求推进到已登记并已发信封：受理 → 按自然键找先到者（有则`已存在`交回原 ID）
// → 铸造 ID → 登记 → 交信封。并发下另一方先落自然键时，Save 答`已存在`、读回赢家作答。信封投递失败
// 不翻结果：请求已登记是真的，只是那封信还没出去——留续办引用，重放时重发同一份。
func (handler *RequestBuyEvaluationHandler) Handle(
	ctx context.Context,
	command RequestBuyEvaluationCommand,
) (RequestBuyEvaluationResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" || command.RequestedBy.String() == "" {
		return RequestBuyEvaluationResult{outcome: EvaluationRequestNotAccepted}, nil
	}
	sources := domain.EligibleSourceReferences{
		Occurrence: command.Occurrence,
		FeeItem:    command.FeeItem,
		Agreement:  command.Agreement,
	}
	naturalKey, err := domain.EvaluationRequestNaturalKeyOf(command.Scope, domain.BuySupplierCost, sources)
	if err != nil {
		return RequestBuyEvaluationResult{outcome: EvaluationRequestNotAccepted}, nil
	}

	// 先按自然键找先到者，再铸 ID：重放不该消耗一个新 ID——那个 ID 谁都不会引用，却会让「铸了却没登」
	// 在日志里长得像一次丢失。这一查与登记之间的窄窗留给 Save 的`已存在`分支兜底。
	existing, found, err := handler.deps.Registry.FindByNaturalKey(ctx, command.TenantID, naturalKey)
	if err != nil {
		return evaluationRequestUndecided(EvaluationRequestRegistryUnavailable, naturalKey.SourceDigest), nil
	}
	if found {
		return handler.existingRequest(ctx, existing), nil
	}

	id, err := handler.deps.Identity.MintEvaluationRequestID(ctx)
	if err != nil {
		return evaluationRequestUndecided(EvaluationRequestIdentityUnavailable, naturalKey.SourceDigest), nil
	}
	now := handler.deps.Clock.Now()
	request, err := domain.SubmitEvaluationRequest(domain.EvaluationRequestSpec{
		ID:          id,
		Scope:       command.Scope,
		Purpose:     domain.BuySupplierCost,
		Sources:     sources,
		RequestedAt: now,
		RequestedBy: command.RequestedBy,
	})
	if err != nil {
		return RequestBuyEvaluationResult{outcome: EvaluationRequestNotAccepted}, nil
	}

	record := ports.EvaluationRequestRecord{
		Key:        ports.EvaluationRequestKey{TenantID: command.TenantID, Request: id},
		Request:    request,
		RecordedAt: now,
	}
	saved, err := handler.deps.Registry.Save(ctx, record)
	if err != nil {
		return evaluationRequestUndecided(EvaluationRequestRegistryUnavailable, id.String()), nil
	}
	switch saved {
	case ports.EvaluationRequestSaved:
		result := RequestBuyEvaluationResult{outcome: EvaluationRequested, record: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.EvaluationRequestAlreadyRequested:
		winner, found, err := handler.deps.Registry.FindByNaturalKey(ctx, command.TenantID, naturalKey)
		if err != nil || !found {
			return evaluationRequestUndecided(EvaluationRequestRegistryUnavailable, naturalKey.SourceDigest), nil
		}
		return handler.existingRequest(ctx, winner), nil
	default:
		return RequestBuyEvaluationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedEvaluationRequestSave, saved)
	}
}

// existingRequest 按先到者作答并再交一次同一份意图：重放交的是同一封（同租户、同请求 ID），Outbox 按
// 认领键吞掉第二次——「`已存在`不重发」在真库上就是这样成立的；不在这里跳过交接，是为了让上一次
// 交接失败留下的那封在重放时补上（与 map_external_funds.go 的 existingFact 同形）。
func (handler *RequestBuyEvaluationHandler) existingRequest(
	ctx context.Context,
	record ports.EvaluationRequestRecord,
) RequestBuyEvaluationResult {
	return RequestBuyEvaluationResult{
		outcome:   EvaluationRequestExisting,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record),
	}
}

// handOff 把已登记的请求交给 parcel-pricing。投递失败不翻结果，留续办引用重放时重发同一份。
func (handler *RequestBuyEvaluationHandler) handOff(
	ctx context.Context,
	record ports.EvaluationRequestRecord,
) string {
	if err := handler.deps.Downstream.HandOffEvaluationRequest(ctx, ports.EvaluationRequestIntent{Record: record}); err == nil {
		return ""
	}
	return evaluationRequestContinuation("EVALUATION_REQUEST_HANDOFF",
		record.Key.TenantID.String(), record.Key.Request.String())
}

func evaluationRequestUndecided(reason EvaluationRequestUndecidedReason, subject string) RequestBuyEvaluationResult {
	return RequestBuyEvaluationResult{
		outcome:      EvaluationRequestUndecided,
		reason:       reason,
		continuation: evaluationRequestContinuation(reason.String(), subject),
	}
}

func evaluationRequestContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}
