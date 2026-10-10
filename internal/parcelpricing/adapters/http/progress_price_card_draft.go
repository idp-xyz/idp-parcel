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

// 价卡草稿批准口与发布口（ADR-0101 决定五；票 price-card-import/04）。载荷只收草稿引用（方案标识与方案版本）；租户与
// 批准者来自操作者信封，载荷里出现身份格即按未知键拒。发布不记谁按的键——批准者已记在草稿上。

// PriceCardDraftApprovalIntake 把一次已认证的接入请求翻译成批准命令：草稿引用 + 信封里的租户与批准者主体。
type PriceCardDraftApprovalIntake interface {
	IntakePriceCardDraftApproval(ctx context.Context, request *http.Request) (application.ApprovePriceCardDraftCommand, error)
}

// PriceCardDraftPublicationIntake 把一次已认证的接入请求翻译成发布命令：草稿引用 + 信封里的租户。
type PriceCardDraftPublicationIntake interface {
	IntakePriceCardDraftPublication(ctx context.Context, request *http.Request) (application.PublishPriceCardDraftCommand, error)
}

// PriceCardDraftApprover 是批准口转交的编排；事务边界在装配点。
type PriceCardDraftApprover interface {
	Handle(ctx context.Context, command application.ApprovePriceCardDraftCommand) (application.ApprovePriceCardDraftResult, error)
}

// PriceCardDraftPublisher 是发布口转交的编排；登记与草稿推进同一笔事务，边界在装配点。
type PriceCardDraftPublisher interface {
	Handle(ctx context.Context, command application.PublishPriceCardDraftCommand) (application.PublishPriceCardDraftResult, error)
}

// NewApprovePriceCardDraftEndpoint 交回批准口（POST /pricing-price-card-draft-approvals）。每一格都是答案、取 200：
// 批准改的是册上已有的那一行，不新落行（ADR-0022）；只有`已批准`带回推进后的草稿。
func NewApprovePriceCardDraftEndpoint(intake PriceCardDraftApprovalIntake, approver PriceCardDraftApprover) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakePriceCardDraftApproval(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := approver.Handle(request.Context(), command)
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

// NewPublishPriceCardDraftEndpoint 交回发布口（POST /pricing-price-card-draft-publications）。`registration` 是登记用例
// 的答复原名，与登记口、登记 CLI 同源、不改名；登记新落了一版取 201，其余答案取 200。登记与否未知（依赖故障）不是答案，
// 答「没形成答案」，同登记口。
func NewPublishPriceCardDraftEndpoint(intake PriceCardDraftPublicationIntake, publisher PriceCardDraftPublisher) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakePriceCardDraftPublication(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := publisher.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if result.Outcome.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer := priceCardDraftProgressResponse{Outcome: result.Outcome.String()}
		status := http.StatusOK
		if registration, handed := result.Registration, result.HasRegistration; handed {
			if registration.String() == "" {
				writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
				return
			}
			answer.Registration = registration.String()
			if registration == application.PriceCardRecorded {
				status = http.StatusCreated
			}
		}
		if draft, has := result.Draft, result.HasDraft; has {
			row := priceCardDraftBodyOf(draft)
			answer.Draft = &row
		}
		writeJSON(response, status, answer)
	})
}

// priceCardDraftProgressResponse 是批准口与发布口的答复：`outcome` 是本口的那一格，`registration` 只在发布真交给了登记
// 用例时在场，`draft` 只在推进成功时在场，行形与录入口、查阅读口同。
type priceCardDraftProgressResponse struct {
	Outcome      string              `json:"outcome"`
	Registration string              `json:"registration,omitempty"`
	Draft        *priceCardDraftBody `json:"draft,omitempty"`
}

// PriceCardDraftReferencePayload 是两口的在线载荷：只有草稿引用。封闭解码，未知键（含任何身份格）与尾随内容都答坏报文。
type PriceCardDraftReferencePayload struct {
	PlanID      string `json:"planId"`
	PlanVersion string `json:"planVersion"`
}

// DecodePriceCardDraftReferencePayload 解载荷并立方案版本引用；缺格或带首尾空白答坏报文。
func DecodePriceCardDraftReferencePayload(body io.Reader) (domain.VersionReference, error) {
	var payload PriceCardDraftReferencePayload
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return domain.VersionReference{}, fmt.Errorf("%w: price card draft reference: %v", ErrMalformedRequest, err)
	}
	if decoder.More() {
		return domain.VersionReference{}, fmt.Errorf("%w: price card draft reference: trailing content", ErrMalformedRequest)
	}
	plan, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, payload.PlanID, payload.PlanVersion)
	if err != nil {
		return domain.VersionReference{}, fmt.Errorf("%w: price card draft reference: %v", ErrMalformedRequest, err)
	}
	return plan, nil
}

var (
	_ PriceCardDraftApprovalIntake    = (*OperatorRegistryIntake)(nil)
	_ PriceCardDraftPublicationIntake = (*OperatorRegistryIntake)(nil)
)

// IntakePriceCardDraftApproval 是操作者渠道对批准的译法：先认证、后解载荷。租户与批准者取认证出的身份，批准者的主体
// 引用与录入者同一种写法（发行方 + sub），授予集取信封里铸造那一刻持有的授予名——批准门拿它们比录入者与规则要求的那一格。
func (intake *OperatorRegistryIntake) IntakePriceCardDraftApproval(ctx context.Context, request *http.Request) (application.ApprovePriceCardDraftCommand, error) {
	operator, body, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.ApprovePriceCardDraftCommand{}, err
	}
	plan, err := DecodePriceCardDraftReferencePayload(body)
	if err != nil {
		return application.ApprovePriceCardDraftCommand{}, err
	}
	if operator.Tenant.String() == "" || operator.Operator == "" {
		return application.ApprovePriceCardDraftCommand{}, ErrOperatorIdentityMissing
	}
	grants := make([]domain.OperatorGrant, 0, len(operator.Grants))
	for _, name := range operator.Grants {
		grant, err := domain.NewOperatorGrant(name)
		if err != nil {
			return application.ApprovePriceCardDraftCommand{}, fmt.Errorf("pricing http: operator grant %q: %w", name, err)
		}
		grants = append(grants, grant)
	}
	approver, err := domain.NewOperatorSubject(operator.Operator, grants)
	if err != nil {
		return application.ApprovePriceCardDraftCommand{}, fmt.Errorf("pricing http: approver: %w", err)
	}
	return application.ApprovePriceCardDraftCommand{Tenant: operator.Tenant, Plan: plan, Approver: approver}, nil
}

// IntakePriceCardDraftPublication 是操作者渠道对发布的译法：先认证、后解载荷，租户取认证出的身份。
func (intake *OperatorRegistryIntake) IntakePriceCardDraftPublication(ctx context.Context, request *http.Request) (application.PublishPriceCardDraftCommand, error) {
	operator, body, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.PublishPriceCardDraftCommand{}, err
	}
	plan, err := DecodePriceCardDraftReferencePayload(body)
	if err != nil {
		return application.PublishPriceCardDraftCommand{}, err
	}
	if operator.Tenant.String() == "" {
		return application.PublishPriceCardDraftCommand{}, ErrOperatorIdentityMissing
	}
	return application.PublishPriceCardDraftCommand{Tenant: operator.Tenant, Plan: plan}, nil
}

var (
	_ PriceCardDraftApprovalIntake    = UnconfiguredIntake{}
	_ PriceCardDraftPublicationIntake = UnconfiguredIntake{}
)

// IntakePriceCardDraftApproval 不读请求：批准者与租户都要从信封来，判据同录入口。
func (UnconfiguredIntake) IntakePriceCardDraftApproval(context.Context, *http.Request) (application.ApprovePriceCardDraftCommand, error) {
	return application.ApprovePriceCardDraftCommand{}, ErrAccessChannelNotConfigured
}

// IntakePriceCardDraftPublication 不读请求：租户要从信封来。
func (UnconfiguredIntake) IntakePriceCardDraftPublication(context.Context, *http.Request) (application.PublishPriceCardDraftCommand, error) {
	return application.PublishPriceCardDraftCommand{}, ErrAccessChannelNotConfigured
}
