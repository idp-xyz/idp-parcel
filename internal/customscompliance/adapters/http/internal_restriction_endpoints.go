package customshttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是内部合规限制的两个运营决定口（UC-CC-011，ADR-0334 决定七）：建立一份限制，与限制责任来源对限制自己的覆盖项追加确认或
// 解除决定。两口各按自己那一种决定认证——能建立的不因此能解除。

const (
	// OperatorDecisionInternalRestrictionEstablishment 与 OperatorDecisionInternalRestrictionDecision 字面与操作者册的决定种类相同。
	OperatorDecisionInternalRestrictionEstablishment OperatorDecision = "INTERNAL_RESTRICTION_ESTABLISHMENT"
	OperatorDecisionInternalRestrictionDecision      OperatorDecision = "INTERNAL_RESTRICTION_DECISION"
)

// InternalRestrictionEstablisher 与 InternalRestrictionDecider 是两口转交的编排（application.ManageInternalRestrictionHandler 的两半）。
type InternalRestrictionEstablisher interface {
	Establish(ctx context.Context, command application.EstablishInternalRestrictionCommand) (application.InternalRestrictionResult, error)
}

type InternalRestrictionDecider interface {
	Decide(ctx context.Context, command application.DecideInternalRestrictionCommand) (application.InternalRestrictionResult, error)
}

// InternalRestrictionEstablishmentIntake 译建立请求（`/customs-internal-restrictions`），InternalRestrictionDecisionIntake 译决定请求
// （`/customs-internal-restriction-decisions`）。
type InternalRestrictionEstablishmentIntake interface {
	IntakeInternalRestrictionEstablishment(ctx context.Context, request *http.Request) (application.EstablishInternalRestrictionCommand, error)
}

type InternalRestrictionDecisionIntake interface {
	IntakeInternalRestrictionDecision(ctx context.Context, request *http.Request) (application.DecideInternalRestrictionCommand, error)
}

var (
	_ InternalRestrictionEstablishmentIntake = (*OperatorDecisionIntake)(nil)
	_ InternalRestrictionDecisionIntake      = (*OperatorDecisionIntake)(nil)
)

func NewEstablishInternalRestrictionEndpoint(intake InternalRestrictionEstablishmentIntake, handler InternalRestrictionEstablisher) http.Handler {
	return decisionEndpoint(intake.IntakeInternalRestrictionEstablishment, handler.Establish, writeInternalRestriction)
}

func NewDecideInternalRestrictionEndpoint(intake InternalRestrictionDecisionIntake, handler InternalRestrictionDecider) http.Handler {
	return decisionEndpoint(intake.IntakeInternalRestrictionDecision, handler.Decide, writeInternalRestriction)
}

// CoverageItemPayload 是一个覆盖项的线格式：一个明确范围上的一个受限动作，建立时另带它开始时的适用性。
type CoverageItemPayload struct {
	Scope    string `json:"scope"`
	Action   string `json:"action"`
	Standing string `json:"standing,omitempty"`
}

// InternalRestrictionEstablishmentPayload 是建立口的线格式。决定方是认证出来的操作者、决定方式恒为人工决定（规则化形成不经运营口），
// 这两格不在载荷里；限制责任来源、依据与解除条件是租户取值，由请求方给，产品不代填。
type InternalRestrictionEstablishmentPayload struct {
	RestrictionID string                `json:"restrictionId"`
	Source        string                `json:"source"`
	Basis         string                `json:"basis"`
	Condition     string                `json:"condition"`
	StartedAt     string                `json:"startedAt"`
	Coverage      []CoverageItemPayload `json:"coverage"`
}

// InternalRestrictionDecisionPayload 是决定口的线格式。授权引用只被保存、不核验（ADR-0334 越权风险点 1：对 party-commercial 核决定方当前
// 授权不在本记录）。
type InternalRestrictionDecisionPayload struct {
	RestrictionID string                `json:"restrictionId"`
	DecisionID    string                `json:"decisionId"`
	Kind          string                `json:"kind"`
	Source        string                `json:"source"`
	Covers        []CoverageItemPayload `json:"covers"`
	Evidence      string                `json:"evidence"`
	Authority     string                `json:"authority"`
	DecidedAt     string                `json:"decidedAt"`
}

// IntakeInternalRestrictionEstablishment 按内部合规限制建立这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeInternalRestrictionEstablishment(ctx context.Context, request *http.Request) (application.EstablishInternalRestrictionCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionInternalRestrictionEstablishment)
	if err != nil {
		return application.EstablishInternalRestrictionCommand{}, err
	}
	var payload InternalRestrictionEstablishmentPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.EstablishInternalRestrictionCommand{}, err
	}
	startedAt, err := optionalInstant(payload.StartedAt, "startedAt")
	if err != nil {
		return application.EstablishInternalRestrictionCommand{}, err
	}
	coverage := make([]domain.CoverageSpec, 0, len(payload.Coverage))
	for _, item := range payload.Coverage {
		spec, err := item.coverageSpec()
		if err != nil {
			return application.EstablishInternalRestrictionCommand{}, err
		}
		coverage = append(coverage, spec)
	}
	return application.EstablishInternalRestrictionCommand{
		TenantID: operator.Tenant,
		Spec: domain.InternalRestrictionSpec{
			ID:        optionalReference(payload.RestrictionID, domain.NewInternalRestrictionID),
			Source:    optionalReference(payload.Source, domain.NewRestrictionSource),
			Basis:     optionalReference(payload.Basis, domain.NewRestrictionBasis),
			Mode:      domain.ManualDecision,
			DecidedBy: optionalReference(operator.Operator, domain.NewRestrictionDecider),
			Condition: optionalReference(payload.Condition, domain.NewRestrictionReleaseCondition),
			Coverage:  coverage,
			StartedAt: startedAt,
		},
	}, nil
}

// IntakeInternalRestrictionDecision 按内部合规限制决定这一种决定认证；决定方取自认证结果。
func (intake *OperatorDecisionIntake) IntakeInternalRestrictionDecision(ctx context.Context, request *http.Request) (application.DecideInternalRestrictionCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionInternalRestrictionDecision)
	if err != nil {
		return application.DecideInternalRestrictionCommand{}, err
	}
	var payload InternalRestrictionDecisionPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.DecideInternalRestrictionCommand{}, err
	}
	kind, err := decisionKindFrom(payload.Kind)
	if err != nil {
		return application.DecideInternalRestrictionCommand{}, err
	}
	decidedAt, err := optionalInstant(payload.DecidedAt, "decidedAt")
	if err != nil {
		return application.DecideInternalRestrictionCommand{}, err
	}
	covers := make([]domain.CoverageKey, 0, len(payload.Covers))
	for _, item := range payload.Covers {
		if strings.TrimSpace(item.Standing) != "" {
			return application.DecideInternalRestrictionCommand{}, fmt.Errorf("%w: a decision names coverage items, not their standing", ErrMalformedRequest)
		}
		spec, err := item.coverageSpec()
		if err != nil {
			return application.DecideInternalRestrictionCommand{}, err
		}
		covers = append(covers, domain.CoverageKey{Scope: spec.Scope, Action: spec.Action})
	}
	return application.DecideInternalRestrictionCommand{
		TenantID:      operator.Tenant,
		RestrictionID: optionalReference(payload.RestrictionID, domain.NewInternalRestrictionID),
		Spec: domain.RestrictionDecisionSpec{
			ID:        optionalReference(payload.DecisionID, domain.NewRestrictionDecisionID),
			Kind:      kind,
			Source:    optionalReference(payload.Source, domain.NewRestrictionSource),
			Covers:    covers,
			Evidence:  optionalReference(payload.Evidence, domain.NewRestrictionEvidence),
			DecidedBy: optionalReference(operator.Operator, domain.NewRestrictionDecider),
			Authority: optionalReference(payload.Authority, domain.NewRestrictionAuthority),
			At:        decidedAt,
		},
	}, nil
}

// coverageSpec 译一个覆盖项。动作与适用性给了却不在封闭集合里是坏报文；空着的格交零值，留给编排答未受理——两者续办不同，
// 不折成一格（与建案口的方向同一写法）。
func (item CoverageItemPayload) coverageSpec() (domain.CoverageSpec, error) {
	spec := domain.CoverageSpec{Scope: optionalReference(item.Scope, domain.NewDecisionScopeReference)}
	if raw := strings.TrimSpace(item.Action); raw != "" {
		action, ok := domain.ParseGuardedAction(raw)
		if !ok {
			return domain.CoverageSpec{}, fmt.Errorf("%w: action %q is not one of the guarded actions", ErrMalformedRequest, raw)
		}
		spec.Action = action
	}
	switch strings.TrimSpace(item.Standing) {
	case "":
	case domain.CoverageInForce.String():
		spec.Standing = domain.CoverageInForce
	case domain.CoverageUnconfirmed.String():
		spec.Standing = domain.CoverageUnconfirmed
	case domain.CoverageConflicting.String():
		spec.Standing = domain.CoverageConflicting
	default:
		return domain.CoverageSpec{}, fmt.Errorf("%w: standing %q is not IN_FORCE, UNCONFIRMED or CONFLICTING", ErrMalformedRequest, item.Standing)
	}
	return spec, nil
}

func decisionKindFrom(raw string) (domain.RestrictionDecisionKind, error) {
	switch strings.TrimSpace(raw) {
	case "":
		return domain.RestrictionDecisionKindInvalid, nil
	case domain.ConfirmCoverage.String():
		return domain.ConfirmCoverage, nil
	case domain.ReleaseCoverage.String():
		return domain.ReleaseCoverage, nil
	default:
		return domain.RestrictionDecisionKindInvalid, fmt.Errorf("%w: kind %q is neither CONFIRM nor RELEASE", ErrMalformedRequest, raw)
	}
}

// optionalInstant 译一个业务时刻：空着交零值（编排答未受理），给了却不是 RFC 3339 是坏报文。
func optionalInstant(raw, field string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s is not an RFC 3339 instant: %v", ErrMalformedRequest, field, err)
	}
	return at.UTC(), nil
}

// internalRestrictionResponse 是两口共用的封闭响应形状。
type internalRestrictionResponse struct {
	Outcome       string `json:"outcome"`
	RestrictionID string `json:"restrictionId,omitempty"`
	Decisions     int    `json:"decisions,omitempty"`
}

// writeInternalRestriction 逐名转写。形成了才 201（建立或决定真落了）；重放、冲突、被拒与未决都是形成了的业务答案，200。
// 未决（册读写不动）没有形成结论，响应体里不带限制标识。
func writeInternalRestriction(response http.ResponseWriter, result application.InternalRestrictionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := internalRestrictionResponse{Outcome: outcome}
	if restriction, present := result.Restriction(); present {
		body.RestrictionID = restriction.ID().String()
		body.Decisions = len(restriction.Decisions())
	}
	status := http.StatusOK
	if result.Outcome() == application.InternalRestrictionEstablished || result.Outcome() == application.InternalRestrictionDecided {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
