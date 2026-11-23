package pricinghttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 价卡草稿撤下口。载荷只收草稿引用与原因；租户与撤下人来自操作者信封，载荷里出现身份格即按未知键拒。
// 撤下与发布挂同一张登记写面，不另套审批职责规则。

// PriceCardDraftWithdrawalIntake 把一次已认证的接入请求翻译成撤下命令。
type PriceCardDraftWithdrawalIntake interface {
	IntakePriceCardDraftWithdrawal(ctx context.Context, request *http.Request) (application.WithdrawPriceCardDraftCommand, error)
}

// PriceCardDraftWithdrawer 是撤下口转交的编排；事务边界在装配点。
type PriceCardDraftWithdrawer interface {
	Handle(ctx context.Context, command application.WithdrawPriceCardDraftCommand) (application.WithdrawPriceCardDraftResult, error)
}

// NewWithdrawPriceCardDraftEndpoint 交回撤下口（POST /pricing-price-card-draft-withdrawals）。每一格都是答案、取 200：
// 撤下改的是册上已有的那一行，不新落行。`已撤下`带回草稿（这次写下的，或册上本来就是已撤下的那一份）。
func NewWithdrawPriceCardDraftEndpoint(intake PriceCardDraftWithdrawalIntake, withdrawer PriceCardDraftWithdrawer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakePriceCardDraftWithdrawal(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := withdrawer.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if result.Outcome.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer := priceCardDraftProgressResponse{Outcome: result.Outcome.String()}
		if draft, has := result.Draft, result.HasDraft; has {
			row := priceCardDraftBodyOf(draft)
			answer.Draft = &row
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

// PriceCardDraftWithdrawalPayload 是撤下口的在线载荷：草稿引用加原因。没有身份格，也没有时刻。
type PriceCardDraftWithdrawalPayload struct {
	PlanID      string `json:"planId"`
	PlanVersion string `json:"planVersion"`
	Reason      string `json:"reason"`
}

// DecodePriceCardDraftWithdrawalPayload 封闭解码。未知键（含自报的撤下人、租户或时刻）与尾随内容都答坏报文。
// 原因空不空留给用例：空原因是不受理，不是报文形状坏了。
func DecodePriceCardDraftWithdrawalPayload(body io.Reader) (domain.VersionReference, string, error) {
	var payload PriceCardDraftWithdrawalPayload
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return domain.VersionReference{}, "", fmt.Errorf("%w: price card draft withdrawal: %v", ErrMalformedRequest, err)
	}
	if decoder.More() {
		return domain.VersionReference{}, "", fmt.Errorf("%w: price card draft withdrawal: trailing content", ErrMalformedRequest)
	}
	plan, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, payload.PlanID, payload.PlanVersion)
	if err != nil {
		return domain.VersionReference{}, "", fmt.Errorf("%w: price card draft withdrawal: %v", ErrMalformedRequest, err)
	}
	return plan, payload.Reason, nil
}

var _ PriceCardDraftWithdrawalIntake = (*OperatorRegistryIntake)(nil)

// IntakePriceCardDraftWithdrawal 是操作者渠道对撤下的译法：先认证、后解载荷。租户与撤下人取认证出的身份，不取载荷里的自报。
func (intake *OperatorRegistryIntake) IntakePriceCardDraftWithdrawal(ctx context.Context, request *http.Request) (application.WithdrawPriceCardDraftCommand, error) {
	operator, body, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.WithdrawPriceCardDraftCommand{}, err
	}
	plan, reason, err := DecodePriceCardDraftWithdrawalPayload(body)
	if err != nil {
		return application.WithdrawPriceCardDraftCommand{}, err
	}
	if operator.Tenant.String() == "" || operator.Operator == "" {
		return application.WithdrawPriceCardDraftCommand{}, ErrOperatorIdentityMissing
	}
	withdrawer, err := domain.NewOperatorSubject(operator.Operator, nil)
	if err != nil {
		return application.WithdrawPriceCardDraftCommand{}, fmt.Errorf("pricing http: withdrawer: %w", err)
	}
	return application.WithdrawPriceCardDraftCommand{
		Tenant: operator.Tenant, Plan: plan, Withdrawer: withdrawer, Reason: reason,
	}, nil
}

var _ PriceCardDraftWithdrawalIntake = UnconfiguredIntake{}

// IntakePriceCardDraftWithdrawal 不读请求：撤下人与租户都要从信封来。
func (UnconfiguredIntake) IntakePriceCardDraftWithdrawal(context.Context, *http.Request) (application.WithdrawPriceCardDraftCommand, error) {
	return application.WithdrawPriceCardDraftCommand{}, ErrAccessChannelNotConfigured
}
