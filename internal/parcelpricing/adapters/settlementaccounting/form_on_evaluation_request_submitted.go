package settlementaccounting

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ppinbox "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/inbox"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 未决哨兵：每一枚是入口停在的一格「等谁」，恢复动作各不相同，所以不合成一枚。生产装配把它们都登进
// WithUndecidedSentinels：重投会改变结果（请求看得见了 / 依赖回来了），不当毒丸、不当发布失败。
//
// 入口答得出但答不出评价的那三格——没有在用价卡、多于一版同时适用、造不出计价输入快照——不在这里：重投不会让它们
// 变好，经「评价未形成」信封交给结算记停因，入账（ADR-0229 决定三）。补登之后由结算按停因另起下一代请求。
var (
	// ErrEvaluationRequestNotVisible 表示按信封引用在 SA 还读不回那份请求。请求与信封在 SA 同一事务落库，读不回只剩
	// 可见性滞后一种成因；读口自己答不出（库不可用）同格——等的都是提供方那一侧。
	ErrEvaluationRequestNotVisible = errors.New(
		"parcel pricing settlementaccounting adapter: evaluation request is not yet visible")
	// ErrEvaluationFormationUndecided 是入口的「未决」：依赖故障，形成与否未知，停在哪一口随消息带出。交「评价未形成」
	// 信封暂时失败也落这一格：入账跟着回滚，下一次重投再交。
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
	unformed ppports.EvaluationRequestUnformedHandoff
	evidence ppdomain.EvidenceKind
}

// NewFormOnEvaluationRequestSubmittedAdapter 构造期拒 nil；证据层级同样在构造期核——它是这条路今天形成的每一份评价
// 的属性，装配方不说、这里不替它说。「评价未形成」交接口缺席时，答不出评价的那几格就没有地方去，所以同样必备。
func NewFormOnEvaluationRequestSubmittedAdapter(
	requests EvaluationRequestSource,
	former EvaluationFormer,
	unformed ppports.EvaluationRequestUnformedHandoff,
	evidence ppdomain.EvidenceKind,
) (*FormOnEvaluationRequestSubmittedAdapter, error) {
	if requests == nil {
		return nil, fmt.Errorf("parcel pricing settlementaccounting adapter: evaluation request source is nil")
	}
	if former == nil {
		return nil, fmt.Errorf("parcel pricing settlementaccounting adapter: evaluation former is nil")
	}
	if unformed == nil {
		return nil, fmt.Errorf("parcel pricing settlementaccounting adapter: evaluation request unformed handoff is nil")
	}
	if !evidence.Declared() {
		return nil, fmt.Errorf("parcel pricing settlementaccounting adapter: evidence kind %q is not one of S / R / P", evidence)
	}
	return &FormOnEvaluationRequestSubmittedAdapter{requests: requests, former: former, unformed: unformed, evidence: evidence}, nil
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
	return foldFormation(ctx, adapter.unformed, command, requestID.String(), result)
}

// foldFormation 把入口的封闭结果折成消费两格：已形成 / 已存在答 nil 入账不重投；答不出评价的三格交「评价未形成」后入账；
// 其余各格一枚哨兵或原样上抛。BUY 与 SELL 两路共用这一套：结果格与恢复动作都是入口的，与请求出自哪一本册无关，两套
// 折法只会各自走样。
func foldFormation(
	ctx context.Context,
	unformed ppports.EvaluationRequestUnformedHandoff,
	command ppapplication.FormEvaluationFromRequestCommand,
	requestID string,
	result ppapplication.FormEvaluationFromRequestResult,
) error {
	switch result.Outcome {
	case ppapplication.RequestEvaluationFormed, ppapplication.RequestEvaluationExisting:
		return nil
	case ppapplication.RequestPriceCardNotConfigured:
		return handOffUnformed(ctx, unformed, command, ppports.UnformedPriceCardNotConfigured, []string{
			fmt.Sprintf("no price card in force for scope %s %s/%s at %s",
				command.Scope, command.Direction, command.Purpose, command.BasisAt.UTC().Format("2006-01-02T15:04:05Z07:00")),
		})
	case ppapplication.RequestPriceCardApplicabilityConflict:
		return handOffUnformed(ctx, unformed, command, ppports.UnformedPriceCardApplicabilityConflict, []string{
			"candidates " + describeCandidates(result.Candidates),
		})
	case ppapplication.RequestPricingInputUnavailable:
		return handOffUnformed(ctx, unformed, command, ppports.UnformedPricingInputUnavailable, result.Missing)
	case ppapplication.RequestEvaluationUndecided:
		return fmt.Errorf("%w: %s", ErrEvaluationFormationUndecided, result.Reason)
	case ppapplication.RequestEvaluationNotAccepted:
		return fmt.Errorf("%w: the translated command for evaluation request %s was not accepted by parcel-pricing", ErrUntranslatableAnswer, requestID)
	default:
		return fmt.Errorf("%w: formation outcome %q", ErrUntranslatableAnswer, result.Outcome)
	}
}

// handOffUnformed 交「评价未形成」。被框架确定性拒收是硬失败：重交永远同一个结果，落 publish_failed 等人看；别的失败是
// 依赖暂时不可用，答未决、入账跟着回滚，下一次重投再交。
func handOffUnformed(
	ctx context.Context,
	unformed ppports.EvaluationRequestUnformedHandoff,
	command ppapplication.FormEvaluationFromRequestCommand,
	cause ppports.EvaluationRequestUnformedCause,
	detail []string,
) error {
	err := unformed.HandOffEvaluationRequestUnformed(ctx, ppports.EvaluationRequestUnformedIntent{
		Tenant:    command.Tenant,
		Request:   command.Request,
		Direction: command.Direction,
		Cause:     cause,
		Detail:    append([]string(nil), detail...),
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ppports.ErrEvaluationRequestUnformedHandoffRejected):
		return fmt.Errorf("evaluation request %s unformed (%s): %w", command.Request, cause, err)
	default:
		return fmt.Errorf("%w: hand off evaluation request %s unformed (%s): %v", ErrEvaluationFormationUndecided, command.Request, cause, err)
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
