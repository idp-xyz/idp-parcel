package settlementaccounting

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ppinbox "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/inbox"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 未决哨兵：每一枚是入口停在的一格「等谁」，恢复动作各不相同，所以不合成一枚——合成之后运维只知道「未决」，不知道
// 该去登记价卡、找人裁、等别的上下文的读口，还是等依赖恢复。生产装配把它们都登进 WithUndecidedSentinels：重投会
// 改变结果（卡登了 / 裁了 / 读口接上了 / 依赖回来了），不当毒丸、不当发布失败。
var (
	// ErrEvaluationRequestNotVisible 表示按信封引用在 SA 还读不回那份请求。请求与信封在 SA 同一事务落库，读不回只剩
	// 可见性滞后一种成因；读口自己答不出（库不可用）同格——等的都是提供方那一侧。
	ErrEvaluationRequestNotVisible = errors.New(
		"parcel pricing settlementaccounting adapter: evaluation request is not yet visible")
	// ErrPriceCardNotConfigured 是入口的「未配置」：（租户、范围、方向、目的、时点）下没有价卡。恢复动作是登记一张
	// 卡——实例半边（PAR-SET-03），本上下文不替租户配。
	ErrPriceCardNotConfigured = errors.New(
		"parcel pricing settlementaccounting adapter: no price card is in force for the evaluation request")
	// ErrPriceCardApplicabilityConflict 是入口的「适用冲突」：多于一版价卡同时适用。恢复动作是人裁——不种任何选择规则。
	ErrPriceCardApplicabilityConflict = errors.New(
		"parcel pricing settlementaccounting adapter: more than one price card applies to the evaluation request")
	// ErrPricingInputUnavailable 是入口的「输入不可得」（裁决 4）：造不出计价输入快照，缺哪几只读口随消息点名。恢复
	// 动作是等提供方那一侧（TF / NO / PS）的只读口接上——后继票，不是传输。
	ErrPricingInputUnavailable = errors.New(
		"parcel pricing settlementaccounting adapter: the pricing input for the evaluation request is not obtainable")
	// ErrEvaluationFormationUndecided 是入口的「未决」：依赖故障，形成与否未知，停在哪一口随消息带出。
	ErrEvaluationFormationUndecided = errors.New(
		"parcel pricing settlementaccounting adapter: evaluation formation is undecided")
)

// EvaluationFormer 是「按评价请求形成评价」入口在本适配器眼里的形；application 的 FormEvaluationFromRequestHandler
// 直接满足它。
type EvaluationFormer interface {
	Handle(ctx context.Context, command ppapplication.FormEvaluationFromRequestCommand) (ppapplication.FormEvaluationFromRequestResult, error)
}

// FormOnEvaluationRequestSubmittedAdapter 是 ppinbox.EvaluationRequestSubmittedConsumer 的真实处理方：按信封引用向
// SA 读口取那一份请求，译成命令交入口，再把入口的封闭结果折成消费两格——入账不重投 / 哨兵重投。
//
// 只译不判（票 sa-cc/11 红线）：形成与否、结果分格全在入口与 EvaluatePricingHandler；这里不看评价内容、不碰数字。
type FormOnEvaluationRequestSubmittedAdapter struct {
	requests EvaluationRequestSource
	former   EvaluationFormer
	evidence ppdomain.EvidenceKind
}

// NewFormOnEvaluationRequestSubmittedAdapter 构造期拒 nil；证据层级同样在构造期核——它是这条路今天形成的每一份评价
// 的属性，装配方不说、这里不替它说。
func NewFormOnEvaluationRequestSubmittedAdapter(
	requests EvaluationRequestSource,
	former EvaluationFormer,
	evidence ppdomain.EvidenceKind,
) (*FormOnEvaluationRequestSubmittedAdapter, error) {
	if requests == nil {
		return nil, fmt.Errorf("parcel pricing settlementaccounting adapter: evaluation request source is nil")
	}
	if former == nil {
		return nil, fmt.Errorf("parcel pricing settlementaccounting adapter: evaluation former is nil")
	}
	if !evidence.Declared() {
		return nil, fmt.Errorf("parcel pricing settlementaccounting adapter: evidence kind %q is not one of S / R / P", evidence)
	}
	return &FormOnEvaluationRequestSubmittedAdapter{requests: requests, former: former, evidence: evidence}, nil
}

var _ ppinbox.SubmittedEvaluationRequestHandler = (*FormOnEvaluationRequestSubmittedAdapter)(nil)

// HandleSubmittedEvaluationRequest 译引用 → 取请求 → 译命令 → 交入口 → 折结果。
//
// 已形成与已存在都答 nil：一封信封一份评价，第二封同请求的信封按回指命中先到者，入账不重投（票面做法 3）。入口
// 上抛的 error 是结构上到不了的答案（替身装错、封闭集之外的结果），原样上抛不译成未决。
func (adapter *FormOnEvaluationRequestSubmittedAdapter) HandleSubmittedEvaluationRequest(
	ctx context.Context,
	submitted ppinbox.SubmittedEvaluationRequest,
) error {
	tenant, err := sadomain.NewTenantID(submitted.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableReference, err)
	}
	requestID, err := sadomain.NewEvaluationRequestID(submitted.EvaluationRequestID)
	if err != nil {
		return fmt.Errorf("%w: evaluation request: %v", ErrUntranslatableReference, err)
	}

	record, found, err := adapter.requests.FindByID(ctx, saports.EvaluationRequestKey{TenantID: tenant, Request: requestID})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEvaluationRequestNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: evaluation request %s", ErrEvaluationRequestNotVisible, requestID)
	}

	command, err := TranslateEvaluationRequest(record, adapter.evidence)
	if err != nil {
		return err
	}
	result, err := adapter.former.Handle(ctx, command)
	if err != nil {
		return fmt.Errorf("form evaluation from request %s: %w", requestID, err)
	}
	switch result.Outcome {
	case ppapplication.RequestEvaluationFormed, ppapplication.RequestEvaluationExisting:
		return nil
	case ppapplication.RequestPriceCardNotConfigured:
		return fmt.Errorf("%w: tenant %s scope %s %s/%s at %s", ErrPriceCardNotConfigured,
			command.Tenant, command.Scope, command.Direction, command.Purpose, command.BasisAt.UTC().Format("2006-01-02T15:04:05Z07:00"))
	case ppapplication.RequestPriceCardApplicabilityConflict:
		return fmt.Errorf("%w: candidates %s", ErrPriceCardApplicabilityConflict, describeCandidates(result.Candidates))
	case ppapplication.RequestPricingInputUnavailable:
		return fmt.Errorf("%w: %s", ErrPricingInputUnavailable, strings.Join(result.Missing, "; "))
	case ppapplication.RequestEvaluationUndecided:
		return fmt.Errorf("%w: %s", ErrEvaluationFormationUndecided, result.Reason)
	case ppapplication.RequestEvaluationNotAccepted:
		return fmt.Errorf("%w: the translated command for evaluation request %s was not accepted by parcel-pricing", ErrUntranslatableAnswer, requestID)
	default:
		return fmt.Errorf("%w: formation outcome %q", ErrUntranslatableAnswer, result.Outcome)
	}
}

// describeCandidates 把冲突候选写成「标识@版本」逐条列出，供裁的人知道在哪几张之间裁。
func describeCandidates(candidates []ppdomain.VersionReference) string {
	described := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		described = append(described, candidate.ID()+"@"+candidate.Version())
	}
	return strings.Join(described, ", ")
}
