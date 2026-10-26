package networkhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 最低成本并列的授权裁决口（ADR-0309 决定二）：授权角色在并列的那几家之间择一、形成这件包裹的首个计划。与授权改路决定同族、
// 同一种授予（ADR-0309 决定五），认证与准入判法一致，只是载荷与答复另成一份：这里不指名建议，指名包裹。

// InitialRouteTieDecisionIntake 把已认证的裁决请求翻译成命令。
type InitialRouteTieDecisionIntake interface {
	IntakeInitialRouteTieDecision(ctx context.Context, request *http.Request) (application.DecideInitialRouteCommand, error)
}

var _ InitialRouteTieDecisionIntake = (*OperatorDecisionIntake)(nil)

// InitialRouteTieDecider 是本口转交的裁决编排；事务边界在它那一侧（判断键锁随事务释放）。
type InitialRouteTieDecider interface {
	Decide(ctx context.Context, command application.DecideInitialRouteCommand) (application.DecideInitialRouteResult, error)
}

// InitialRouteTieDecisionPayload 是裁决口的线格式。载荷只有内容、没有身份（封闭解码）：租户与决定方取认证结果，载荷里自报
// 操作者一律坏报文。
type InitialRouteTieDecisionPayload struct {
	DeclaredParcelID string `json:"declaredParcelId"`
	Candidate        string `json:"candidate"`
	Basis            string `json:"basis"`
}

// IntakeInitialRouteTieDecision 译并列裁决（`/route-initial-route-decisions`）。决定方取认证出的操作者，记成
// `OPERATOR/<发行方>#<sub>`，与授权改路决定同一写法。
func (intake *OperatorDecisionIntake) IntakeInitialRouteTieDecision(
	ctx context.Context,
	request *http.Request,
) (application.DecideInitialRouteCommand, error) {
	none := application.DecideInitialRouteCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionAuthorizedReroute)
	if err != nil {
		return none, err
	}
	var payload InitialRouteTieDecisionPayload
	if err := decodeClosed(request.Body, &payload); err != nil {
		return none, err
	}
	command := application.DecideInitialRouteCommand{Tenant: operator.Tenant}
	if command.Parcel, err = domain.NewDeclaredParcelID(payload.DeclaredParcelID); err != nil {
		return none, fmt.Errorf("%w: declaredParcelId: %v", ErrMalformedRequest, err)
	}
	if command.Candidate, err = domain.NewCandidateID(payload.Candidate); err != nil {
		return none, fmt.Errorf("%w: candidate: %v", ErrMalformedRequest, err)
	}
	if command.Basis, err = domain.NewRerouteDecisionBasisReference(payload.Basis); err != nil {
		return none, fmt.Errorf("%w: basis: %v", ErrMalformedRequest, err)
	}
	if command.DecidedBy, err = domain.NewAuthorizedRoleReference("OPERATOR/" + operator.Operator); err != nil {
		return none, fmt.Errorf("network http: decider: %w", err)
	}
	return command, nil
}

// NewDecideInitialRouteEndpoint 交回并列裁决口的 HTTP 入口：方法门 → Intake → 编排 → 逐名转写。
func NewDecideInitialRouteEndpoint(intake InitialRouteTieDecisionIntake, decider InitialRouteTieDecider) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeInitialRouteTieDecision(request.Context(), request)
		if err != nil {
			writeDecisionIntakeProblem(response, err)
			return
		}
		result, err := decider.Decide(request.Context(), command)
		if err != nil {
			// 裁决成没成未知，或撞上不变量（基线对不上之类）：状态码只报「没形成答案」（ADR-0022），成因去查服务端记录。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeInitialRouteTieAnswer(response, result)
	})
}

// initialRouteTieResponse 是裁决口的封闭响应形状。`plan` 只随成立出现；`existing` 随已有结果出现，说明判断库里是计划还是无路由；
// `registeredReason` 只随没有并列登记且登记行存在时出现；`candidateReason` 只随此刻重判不成立的三格出现；`undecidedReason` 与
// `continuationReference` 只随未决出现。
type initialRouteTieResponse struct {
	Outcome               string                 `json:"outcome"`
	RegisteredReason      string                 `json:"registeredReason,omitempty"`
	CandidateReason       string                 `json:"candidateReason,omitempty"`
	UndecidedReason       string                 `json:"undecidedReason,omitempty"`
	ContinuationReference string                 `json:"continuationReference,omitempty"`
	Key                   *initialRouteTieKey    `json:"key,omitempty"`
	Plan                  *initialRouteTiePlan   `json:"plan,omitempty"`
	Existing              *initialRouteExistence `json:"existing,omitempty"`
}

type initialRouteTieKey struct {
	CustomerAccountID  string `json:"customerAccountId"`
	ShipmentRequestID  string `json:"shipmentRequestId"`
	AcceptanceBaseline string `json:"acceptanceBaseline"`
	DeclaredParcelID   string `json:"declaredParcelId"`
	ServicePurpose     string `json:"servicePurpose"`
}

// initialRouteTiePlan 透出首个计划：从 `effectiveFrom`（决定的时刻）起生效，时间一律 RFC 3339 UTC。
type initialRouteTiePlan struct {
	Version       string `json:"version"`
	Candidate     string `json:"candidate"`
	EffectiveFrom string `json:"effectiveFrom"`
	JudgedAt      string `json:"judgedAt"`
}

type initialRouteExistence struct {
	Conclusion  string `json:"conclusion"`
	PlanVersion string `json:"planVersion,omitempty"`
}

func writeInitialRouteTieAnswer(response http.ResponseWriter, result application.DecideInitialRouteResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := initialRouteTieResponse{
		Outcome:               outcome,
		CandidateReason:       result.CandidateReason(),
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference().String(),
	}
	if reason := result.RegisteredReason(); reason != ports.InitialRouteNotFormedReasonInvalid {
		body.RegisteredReason = reason.String()
	}
	if plan, present := result.Plan(); present {
		key := plan.Key()
		body.Key = tieKeyBody(key)
		body.Plan = &initialRouteTiePlan{
			Version:       plan.Version().String(),
			Candidate:     plan.SelectedCandidate().String(),
			EffectiveFrom: utcText(plan.EffectiveFrom()),
			JudgedAt:      utcText(plan.JudgedAt()),
		}
	}
	if existing, present := result.Existing(); present {
		body.Key = tieKeyBody(existing.Key)
		if existing.HasPlan {
			body.Existing = &initialRouteExistence{Conclusion: "ROUTE_FORMED", PlanVersion: existing.Plan.Version().String()}
		} else {
			body.Existing = &initialRouteExistence{Conclusion: "NO_CURRENT_ROUTE"}
		}
	}
	// ADR-0022：只有这一次真形成了首个计划用 201；已有结果、没有并列登记、拒与未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.InitialRouteTieDecided {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

func tieKeyBody(key domain.InitialRouteJudgmentKey) *initialRouteTieKey {
	return &initialRouteTieKey{
		CustomerAccountID:  key.CustomerAccountID.String(),
		ShipmentRequestID:  key.ShipmentRequestID.String(),
		AcceptanceBaseline: key.AcceptanceBaseline.String(),
		DeclaredParcelID:   key.DeclaredParcelID.String(),
		ServicePurpose:     key.ServicePurpose.String(),
	}
}

// InitialRouteTieCatalogueReader 是并列读面消费的读口（ADR-0309 决定六）。
type InitialRouteTieCatalogueReader interface {
	ListInitialRouteTies(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.InitialRouteTieCatalogueRow, error)
}

var _ InitialRouteTieCatalogueReader = ports.InitialRouteTieCatalogueRead(nil)

// outcomeInitialRouteTiesListed 是读面唯一的业务结果：空册如实答空列表。
const outcomeInitialRouteTiesListed = "INITIAL_ROUTE_TIES_LISTED"

// NewQueryInitialRouteTiesEndpoint 交回并列读面（GET /route-initial-route-ties）。复用本包目录查阅的 Intake：并列同属运营查阅，
// 授权边界同是租户。状态只答此刻的大概，裁决成不成立仍由决定口在判断键锁里判。
func NewQueryInitialRouteTiesEndpoint(intake CatalogueQueryIntake, reader InitialRouteTieCatalogueReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		rows, err := reader.ListInitialRouteTies(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]initialRouteTieBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, initialRouteTieBodyOf(row))
		}
		writeJSON(response, http.StatusOK, initialRouteTieListResponse{Outcome: outcomeInitialRouteTiesListed, Ties: bodies})
	})
}

type initialRouteTieListResponse struct {
	Outcome string                `json:"outcome"`
	Ties    []initialRouteTieBody `json:"ties"`
}

// initialRouteTieBody 逐字段透出一条并列。候选的结局与依据是入账那一刻的评估；`tied` 是并列的那几家与打平的金额；`state` 是
// PENDING / DECIDED / FORMED_OTHERWISE 原词；`decision` 只随已决定在场。
type initialRouteTieBody struct {
	CustomerAccountID  string                       `json:"customerAccountId"`
	ShipmentRequestID  string                       `json:"shipmentRequestId"`
	AcceptanceBaseline string                       `json:"acceptanceBaseline"`
	DeclaredParcelID   string                       `json:"declaredParcelId"`
	ServicePurpose     string                       `json:"servicePurpose"`
	Candidates         []rerouteSuggestionCandidate `json:"candidates"`
	Tied               []initialRouteTieCost        `json:"tied"`
	AcceptedAt         string                       `json:"acceptedAt"`
	SettledAt          string                       `json:"settledAt"`
	State              string                       `json:"state"`
	Decision           *initialRouteTieDecisionBody `json:"decision,omitempty"`
}

type initialRouteTieCost struct {
	Candidate   string `json:"candidate"`
	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`
}

type initialRouteTieDecisionBody struct {
	Candidate   string `json:"candidate"`
	PlanVersion string `json:"planVersion"`
	DecidedBy   string `json:"decidedBy"`
	Basis       string `json:"basis"`
	DecidedAt   string `json:"decidedAt"`
}

func initialRouteTieBodyOf(row ports.InitialRouteTieCatalogueRow) initialRouteTieBody {
	body := initialRouteTieBody{
		CustomerAccountID:  row.CustomerAccountID,
		ShipmentRequestID:  row.ShipmentRequestID,
		AcceptanceBaseline: row.AcceptanceBaseline,
		DeclaredParcelID:   row.DeclaredParcelID,
		ServicePurpose:     row.ServicePurpose,
		Candidates:         make([]rerouteSuggestionCandidate, 0, len(row.Candidates)),
		Tied:               make([]initialRouteTieCost, 0, len(row.Tied)),
		AcceptedAt:         utcText(row.AcceptedAt),
		SettledAt:          utcText(row.SettledAt),
		State:              string(row.State),
	}
	for _, candidate := range row.Candidates {
		body.Candidates = append(body.Candidates, rerouteSuggestionCandidate(candidate))
	}
	for _, cost := range row.Tied {
		body.Tied = append(body.Tied, initialRouteTieCost(cost))
	}
	if row.HasDecision {
		body.Decision = &initialRouteTieDecisionBody{
			Candidate:   row.Decision.Candidate,
			PlanVersion: row.Decision.PlanVersion,
			DecidedBy:   row.Decision.DecidedBy,
			Basis:       row.Decision.Basis,
			DecidedAt:   utcText(row.Decision.DecidedAt),
		}
	}
	return body
}
