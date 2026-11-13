package customshttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是监管限制的两个运营决定口：关务人员依据一条已接收的外部监管结果，把监管机构的决定登成对明确动作的限制，并依据
// 责任来源的后续结果解除它。两口各按自己那一种决定认证——能建立的不因此能解除。限制的范围与生效时刻、解除的时刻都取自
// 所引的那条结果，不在载荷里。

const (
	// OperatorDecisionRegulatoryRestrictionEstablishment 与 OperatorDecisionRegulatoryRestrictionRelease 字面与操作者册的决定
	// 种类相同。
	OperatorDecisionRegulatoryRestrictionEstablishment OperatorDecision = "REGULATORY_RESTRICTION_ESTABLISHMENT"
	OperatorDecisionRegulatoryRestrictionRelease       OperatorDecision = "REGULATORY_RESTRICTION_RELEASE"
)

// RegulatoryRestrictionEstablisher 与 RegulatoryRestrictionReleaser 是两口转交的编排（application.ManageRestrictionHandler 带
// 依据核对的两个入口）。
type RegulatoryRestrictionEstablisher interface {
	EstablishOnResult(ctx context.Context, command application.EstablishRestrictionOnResultCommand) (application.RestrictionResult, error)
}

type RegulatoryRestrictionReleaser interface {
	ReleaseOnResult(ctx context.Context, command application.ReleaseRestrictionOnResultCommand) (application.RestrictionResult, error)
}

type RegulatoryRestrictionEstablishmentIntake interface {
	IntakeRegulatoryRestrictionEstablishment(ctx context.Context, request *http.Request) (application.EstablishRestrictionOnResultCommand, error)
}

type RegulatoryRestrictionReleaseIntake interface {
	IntakeRegulatoryRestrictionRelease(ctx context.Context, request *http.Request) (application.ReleaseRestrictionOnResultCommand, error)
}

var (
	_ RegulatoryRestrictionEstablishmentIntake = (*OperatorDecisionIntake)(nil)
	_ RegulatoryRestrictionReleaseIntake       = (*OperatorDecisionIntake)(nil)
)

func NewEstablishRegulatoryRestrictionEndpoint(intake RegulatoryRestrictionEstablishmentIntake, handler RegulatoryRestrictionEstablisher) http.Handler {
	return decisionEndpoint(intake.IntakeRegulatoryRestrictionEstablishment, handler.EstablishOnResult, writeRegulatoryRestriction)
}

func NewReleaseRegulatoryRestrictionEndpoint(intake RegulatoryRestrictionReleaseIntake, handler RegulatoryRestrictionReleaser) http.Handler {
	return decisionEndpoint(intake.IntakeRegulatoryRestrictionRelease, handler.ReleaseOnResult, writeRegulatoryRestriction)
}

// RegulatoryRestrictionEstablishmentPayload 是建立口的线格式。decision 是所依据的那条外部监管结果的来源标识；constrains 是
// 这条限制约束的方向性动作，取封闭词。没有范围与生效时刻两格：它们取自所引的结果。
type RegulatoryRestrictionEstablishmentPayload struct {
	RestrictionID string   `json:"restrictionId"`
	Decision      string   `json:"decision"`
	Constrains    []string `json:"constrains"`
}

// RegulatoryRestrictionReleasePayload 是解除口的线格式。result 是解除所依据的那条外部监管结果的来源标识。
type RegulatoryRestrictionReleasePayload struct {
	RestrictionID string `json:"restrictionId"`
	Result        string `json:"result"`
}

// IntakeRegulatoryRestrictionEstablishment 按监管限制建立这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeRegulatoryRestrictionEstablishment(ctx context.Context, request *http.Request) (application.EstablishRestrictionOnResultCommand, error) {
	none := application.EstablishRestrictionOnResultCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionRegulatoryRestrictionEstablishment)
	if err != nil {
		return none, err
	}
	var payload RegulatoryRestrictionEstablishmentPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return none, err
	}
	id, err := domain.NewRestrictionID(payload.RestrictionID)
	if err != nil {
		return none, fmt.Errorf("%w: restrictionId: %v", ErrMalformedRequest, err)
	}
	constrains := make([]domain.GuardedAction, 0, len(payload.Constrains))
	for _, word := range payload.Constrains {
		action, err := guardedActionFromWord(word)
		if err != nil {
			return none, err
		}
		constrains = append(constrains, action)
	}
	return application.EstablishRestrictionOnResultCommand{
		TenantID:   operator.Tenant,
		ID:         id,
		Decision:   payload.Decision,
		Constrains: constrains,
	}, nil
}

// IntakeRegulatoryRestrictionRelease 按监管限制解除这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeRegulatoryRestrictionRelease(ctx context.Context, request *http.Request) (application.ReleaseRestrictionOnResultCommand, error) {
	none := application.ReleaseRestrictionOnResultCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionRegulatoryRestrictionRelease)
	if err != nil {
		return none, err
	}
	var payload RegulatoryRestrictionReleasePayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return none, err
	}
	id, err := domain.NewRestrictionID(payload.RestrictionID)
	if err != nil {
		return none, fmt.Errorf("%w: restrictionId: %v", ErrMalformedRequest, err)
	}
	return application.ReleaseRestrictionOnResultCommand{TenantID: operator.Tenant, ID: id, Result: payload.Result}, nil
}

func guardedActionFromWord(raw string) (domain.GuardedAction, error) {
	for _, action := range []domain.GuardedAction{
		domain.OutboundRelease, domain.LoadingDeparture, domain.CrossCustomsMovement, domain.FinalDelivery,
	} {
		if action.String() == raw {
			return action, nil
		}
	}
	return domain.GuardedActionInvalid, fmt.Errorf("%w: constrains=%q is not a guarded action word", ErrMalformedRequest, raw)
}

// regulatoryRestrictionResponse 是两口的封闭响应形状。限制的内容只在这次形成、重放命中或已解除时交回。
type regulatoryRestrictionResponse struct {
	Outcome       string   `json:"outcome"`
	RestrictionID string   `json:"restrictionId,omitempty"`
	Decision      string   `json:"decision,omitempty"`
	Scope         string   `json:"scope,omitempty"`
	Constrains    []string `json:"constrains,omitempty"`
	EffectiveAt   string   `json:"effectiveAt,omitempty"`
	ReleasedBy    string   `json:"releasedBy,omitempty"`
	ReleasedAt    string   `json:"releasedAt,omitempty"`
	// HandoffReference 非空说明限制已入册而发布意图没交出去，凭它续办。
	HandoffReference string `json:"handoffReference,omitempty"`
}

// writeRegulatoryRestriction 逐名转写。建立或解除真落了才 201；其余业务答案 200。未决只在限制册或外部结果册读写不动时
// 出现，是没形成答案——答 5xx，不让操作者以为这次决定已经有了着落。
func writeRegulatoryRestriction(response http.ResponseWriter, result application.RestrictionResult) {
	if result.Outcome() == application.RestrictionUndecided {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := regulatoryRestrictionResponse{Outcome: outcome, HandoffReference: result.HandoffReference()}
	if restriction, present := result.Restriction(); present {
		body.RestrictionID = restriction.ID().String()
		body.Decision = restriction.Decision().String()
		body.Scope = restriction.Scope().String()
		for _, action := range restriction.Constrains() {
			body.Constrains = append(body.Constrains, action.String())
		}
		body.EffectiveAt = restriction.EffectiveAt().UTC().Format("2006-01-02T15:04:05Z07:00")
		if release, at, released := restriction.Release(); released {
			body.ReleasedBy = release.String()
			body.ReleasedAt = at.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
	}
	status := http.StatusOK
	if result.Outcome() == application.RestrictionEstablished || result.Outcome() == application.RestrictionReleased {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
