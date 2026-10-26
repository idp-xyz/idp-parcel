package pricinghttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// ReferenceCatalogueReviewIntake 把一次已认证的接入请求翻译成目录版本复核命令。接口而非实现的理由与序列复核口同源：复核
// 责任方与他的授予集来自 ADR-0100 的操作者信封，不从载荷里取——租户的参考复核职责规则比的就是这两样（ADR-0262 决定一）。
type ReferenceCatalogueReviewIntake interface {
	IntakeReferenceCatalogueReview(ctx context.Context, request *http.Request) (application.ReviewReferenceCatalogueCommand, error)
}

// ReferenceCatalogueReviewer 是本端点转交的复核编排，事务边界在编排侧给出。
type ReferenceCatalogueReviewer interface {
	Handle(ctx context.Context, command application.ReviewReferenceCatalogueCommand) (application.ReviewReferenceCatalogueOutcome, error)
}

// NewReviewReferenceCatalogueEndpoint 交回计价参考目录版本复核的 HTTP 入口（POST /pricing-reference-catalogue-reviews；
// ADR-0109 决定二、ADR-0262 决定一）。
//
// 这一口存在的理由是授予：受控批量口的复核者只是批文里的一个串、带不出授予集，规则要求某一格授予时那一路一律答复核者
// 不合格；没有操作者面，要求授予的租户就根本无法复核目录（ADR-0262 越权风险点 4）。形照序列复核口：不与目录登记合口，
// 两种命令、两套答案代数。
func NewReviewReferenceCatalogueEndpoint(intake ReferenceCatalogueReviewIntake, reviewer ReferenceCatalogueReviewer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeReferenceCatalogueReview(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		outcome, err := reviewer.Handle(request.Context(), command)
		if err != nil {
			// 依赖故障不是业务答案（ADR-0022）：`未决`连同错误交回时只认错误。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		// `已追加`是新增一行，取 201；其余都是形成了的答案，取 200——未配置、须换人、复核者不合格是治理答案，不借 4xx 说话。
		writeRegistrationAnswer(response, outcome.String(), outcome == application.CatalogueReviewRecorded)
	})
}

// IntakeReferenceCatalogueReview 译目录复核：复核人与他的授予集取认证出的提交操作者，租户同。先认证、后解载荷。
func (intake *OperatorRegistryIntake) IntakeReferenceCatalogueReview(ctx context.Context, request *http.Request) (application.ReviewReferenceCatalogueCommand, error) {
	operator, body, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.ReviewReferenceCatalogueCommand{}, err
	}
	payload, err := DecodeReferenceCatalogueReviewPayload(body)
	if err != nil {
		return application.ReviewReferenceCatalogueCommand{}, err
	}
	grants, err := operatorGrants(operator.Grants)
	if err != nil {
		return application.ReviewReferenceCatalogueCommand{}, err
	}
	return payload.Command(operator.Tenant, operator.Operator, grants)
}

// ReferenceCatalogueReviewPayload 是目录复核口的在线载荷：受控批量口目录复核文档去掉租户与复核人两格——两者与授予集都从
// 认证结果来，载荷里出现即按未知键拒。reviewedAt 可缺：缺时由用例取时钟当下。
type ReferenceCatalogueReviewPayload struct {
	CatalogueID      string `json:"catalogueId"`
	CatalogueVersion string `json:"catalogueVersion"`
	Decision         string `json:"decision"`
	Basis            string `json:"basis"`
	ReviewedAt       string `json:"reviewedAt,omitempty"`
}

// DecodeReferenceCatalogueReviewPayload 封闭解码：未知键与尾随内容都答坏报文。
func DecodeReferenceCatalogueReviewPayload(body io.Reader) (ReferenceCatalogueReviewPayload, error) {
	var payload ReferenceCatalogueReviewPayload
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return ReferenceCatalogueReviewPayload{}, fmt.Errorf("%w: reference catalogue review payload: %v", ErrMalformedRequest, err)
	}
	if decoder.More() {
		return ReferenceCatalogueReviewPayload{}, fmt.Errorf("%w: reference catalogue review payload: trailing content", ErrMalformedRequest)
	}
	return payload, nil
}

// Command 只做形状翻译（时刻按 RFC 3339 解）；四件齐不齐、结论在不在封闭集、复核职责规则都留给用例与领域判。
func (payload ReferenceCatalogueReviewPayload) Command(tenant domain.TenantID, reviewer string, grants []domain.OperatorGrant) (application.ReviewReferenceCatalogueCommand, error) {
	if tenant.String() == "" || reviewer == "" {
		return application.ReviewReferenceCatalogueCommand{}, ErrOperatorIdentityMissing
	}
	command := application.ReviewReferenceCatalogueCommand{
		Tenant:           tenant,
		CatalogueID:      payload.CatalogueID,
		CatalogueVersion: payload.CatalogueVersion,
		Reviewer:         reviewer,
		ReviewerGrants:   grants,
		Decision:         domain.SeriesReviewDecision(payload.Decision),
		Basis:            payload.Basis,
	}
	if payload.ReviewedAt != "" {
		reviewedAt, err := time.Parse(time.RFC3339, payload.ReviewedAt)
		if err != nil {
			return application.ReviewReferenceCatalogueCommand{}, fmt.Errorf("%w: reviewedAt must be RFC 3339: %v", ErrMalformedRequest, err)
		}
		command.ReviewedAt = reviewedAt
	}
	return command, nil
}
