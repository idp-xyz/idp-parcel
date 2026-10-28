package nodeopshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是节点监管协作的两口（UC-NO-001；ADR-0338）：承接决定是节点作业责任角色在管理台上作的判断，挂操作者渠道「运营决定」能力面的
// 节点监管协作承接一种；执行事实是节点现场当场做了什么的记录，挂「作业事实登记」能力面、判自己那一种事实的准入范围。两口的身份格各只
// 从自己的信封来，载荷封闭。

// OperatorDecisionNodeCollaborationAcceptance 是节点作业责任角色对关务协作事项形成承接决定。
const OperatorDecisionNodeCollaborationAcceptance OperatorDecision = "NODE_COLLABORATION_ACCEPTANCE"

// CollaborationExecutionAuthenticator 以作业事实登记能力面铸协作执行事实的信封并判这一种事实的准入范围，交回租户与节点。
type CollaborationExecutionAuthenticator interface {
	AuthenticateCollaborationExecution(ctx context.Context, bearerToken, deviceID, scopeKind, scopeReference string) (domain.TenantID, domain.NodeReference, string, error)
}

// CollaborationAcceptanceIntake 与 CollaborationExecutionIntake 各把一次已认证的请求译成命令。
type (
	CollaborationAcceptanceIntake interface {
		IntakeCollaborationAcceptance(ctx context.Context, request *http.Request) (application.AcceptCollaborationCommand, error)
	}
	CollaborationExecutionIntake interface {
		IntakeCollaborationExecution(ctx context.Context, request *http.Request) (application.RecordExecutionCommand, error)
	}
)

// CollaborationHandler 是两口转交的编排，装配负责把每次调用包进一笔事务。
type CollaborationHandler interface {
	Accept(ctx context.Context, command application.AcceptCollaborationCommand) (application.CollaborationResult, error)
	RecordExecution(ctx context.Context, command application.RecordExecutionCommand) (application.CollaborationResult, error)
}

// acceptCollaborationPayload 是承接决定的线格式：节点、事项、决定、承接的实物与动作、授权依据、拒接或部分承接的业务依据、待补充的
// 缺口、决定时刻。租户取认证结果，没有操作者格。decidedAt 是作出决定的业务时刻，执行事实不得早于它。
type acceptCollaborationPayload struct {
	Node            string     `json:"node"`
	Item            string     `json:"item"`
	Decision        string     `json:"decision"`
	AcceptedUnits   []string   `json:"acceptedUnits"`
	AcceptedActions []string   `json:"acceptedActions"`
	Authority       string     `json:"authority"`
	Basis           string     `json:"basis"`
	Gaps            []string   `json:"gaps"`
	DecidedAt       *time.Time `json:"decidedAt"`
}

// executeCollaborationPayload 是执行事实的线格式。没有租户与节点——那两格取自信封。
type executeCollaborationPayload struct {
	Item        string     `json:"item"`
	Unit        string     `json:"unit"`
	Action      string     `json:"action"`
	Evidence    string     `json:"evidence"`
	PerformedAt *time.Time `json:"performedAt"`
}

var decisionKindsByName = map[string]domain.AcceptanceDecisionKind{
	domain.CollaborationAccepted.String():          domain.CollaborationAccepted,
	domain.CollaborationDeclined.String():          domain.CollaborationDeclined,
	domain.CollaborationPartiallyAccepted.String(): domain.CollaborationPartiallyAccepted,
	domain.CollaborationPendingSupplement.String(): domain.CollaborationPendingSupplement,
}

var actionKindsByName = map[string]domain.CollaborationActionKind{
	domain.UnsealAction.String():  domain.UnsealAction,
	domain.IsolateAction.String(): domain.IsolateAction,
	domain.PresentAction.String(): domain.PresentAction,
	domain.TallyAction.String():   domain.TallyAction,
	domain.ObserveAction.String(): domain.ObserveAction,
}

var gapKindsByName = map[string]domain.SupplementGapKind{
	domain.MissingObjectGap.String():        domain.MissingObjectGap,
	domain.MissingActionGap.String():        domain.MissingActionGap,
	domain.MissingNodeGap.String():          domain.MissingNodeGap,
	domain.MissingAuthorityGap.String():     domain.MissingAuthorityGap,
	domain.MissingWorkConditionGap.String(): domain.MissingWorkConditionGap,
}

func parseActionKind(field, value string) (domain.CollaborationActionKind, error) {
	if kind, ok := actionKindsByName[value]; ok {
		return kind, nil
	}
	return domain.CollaborationActionKindInvalid, fmt.Errorf("%w: %s is not one of UNSEAL, ISOLATE, PRESENT, TALLY, OBSERVE", ErrMalformedRequest, field)
}

// IntakeCollaborationAcceptance 译节点监管协作承接（`/node-operations/collaboration-acceptances`）。载荷只在形状上把关（缺格、空白、
// NUL、词表外、重复）；决定三格的业务取舍（拒接要依据、待补充要缺口、承接要实物与动作）归领域，不在这里先答一遍。
func (intake *OperatorDecisionIntake) IntakeCollaborationAcceptance(ctx context.Context, request *http.Request) (application.AcceptCollaborationCommand, error) {
	none := application.AcceptCollaborationCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionNodeCollaborationAcceptance)
	if err != nil {
		return none, err
	}
	var payload acceptCollaborationPayload
	if err := decodeClosedDocument(request, &payload); err != nil {
		return none, err
	}
	for name, value := range map[string]string{"node": payload.Node, "item": payload.Item, "decision": payload.Decision, "authority": payload.Authority} {
		if err := requiredField(name, value); err != nil {
			return none, err
		}
	}
	if strings.ContainsRune(payload.Basis, 0) {
		return none, fmt.Errorf("%w: basis carries NUL", ErrMalformedRequest)
	}
	if payload.DecidedAt == nil || payload.DecidedAt.IsZero() {
		return none, fmt.Errorf("%w: decidedAt is required", ErrMalformedRequest)
	}
	command := application.AcceptCollaborationCommand{
		TenantID:      operator.Tenant,
		Node:          payload.Node,
		Item:          payload.Item,
		AcceptedUnits: payload.AcceptedUnits,
		Authority:     payload.Authority,
		Basis:         strings.TrimSpace(payload.Basis),
		DecidedAt:     *payload.DecidedAt,
		Operator:      operator.Operator,
	}
	decision, ok := decisionKindsByName[payload.Decision]
	if !ok {
		return none, fmt.Errorf("%w: decision is not one of ACCEPTED, PARTIALLY_ACCEPTED, DECLINED, PENDING_SUPPLEMENT", ErrMalformedRequest)
	}
	command.Decision = decision
	for index, unit := range payload.AcceptedUnits {
		if err := requiredField(fmt.Sprintf("acceptedUnits[%d]", index), unit); err != nil {
			return none, err
		}
	}
	for index, raw := range payload.AcceptedActions {
		action, err := parseActionKind(fmt.Sprintf("acceptedActions[%d]", index), raw)
		if err != nil {
			return none, err
		}
		command.AcceptedActions = append(command.AcceptedActions, action)
	}
	for index, raw := range payload.Gaps {
		gap, ok := gapKindsByName[raw]
		if !ok {
			return none, fmt.Errorf("%w: gaps[%d] is not one of OBJECT, ACTION, NODE, AUTHORITY, WORK_CONDITION", ErrMalformedRequest, index)
		}
		command.Gaps = append(command.Gaps, gap)
	}
	return command, nil
}

// WorkFactCollaborationIntake 是作业事实登记面在执行事实口的 Intake（CollaborationExecutionIntake 的实现）。设备与作业范围从三个头来，作业范围种类只认 NODE：执行事实在节点现场登记。
type WorkFactCollaborationIntake struct {
	authenticator CollaborationExecutionAuthenticator
}

func NewWorkFactCollaborationIntake(authenticator CollaborationExecutionAuthenticator) (*WorkFactCollaborationIntake, error) {
	if authenticator == nil {
		return nil, errors.New("node operations http: work fact collaboration intake needs an authenticator")
	}
	return &WorkFactCollaborationIntake{authenticator: authenticator}, nil
}

var _ CollaborationExecutionIntake = (*WorkFactCollaborationIntake)(nil)

// IntakeCollaborationExecution 译执行事实登记（`/node-operations/collaboration-executions`）。先认证、后解载荷：授予不过的调用方看
// 不到载荷校验的结果。动作取封闭五值，放行、查验结论与处置决定不在其中。
func (intake *WorkFactCollaborationIntake) IntakeCollaborationExecution(ctx context.Context, request *http.Request) (application.RecordExecutionCommand, error) {
	none := application.RecordExecutionCommand{}
	device := strings.TrimSpace(request.Header.Get(headerWorkDevice))
	kind := strings.TrimSpace(request.Header.Get(headerWorkScopeKind))
	reference := strings.TrimSpace(request.Header.Get(headerWorkScopeRef))
	if device == "" || kind == "" || reference == "" {
		return none, fmt.Errorf("%w: %s, %s and %s are required", ErrMalformedRequest, headerWorkDevice, headerWorkScopeKind, headerWorkScopeRef)
	}
	if kind != "NODE" {
		return none, fmt.Errorf("%w: a collaboration execution fact is recorded at a node; Parcel-Work-Scope-Kind must be NODE", ErrMalformedRequest)
	}
	tenant, node, operator, err := intake.authenticator.AuthenticateCollaborationExecution(ctx, httpapi.BearerToken(request), device, kind, reference)
	if err != nil {
		return none, err
	}
	var payload executeCollaborationPayload
	if err := decodeClosedDocument(request, &payload); err != nil {
		return none, err
	}
	for name, value := range map[string]string{"item": payload.Item, "unit": payload.Unit, "action": payload.Action, "evidence": payload.Evidence} {
		if err := requiredField(name, value); err != nil {
			return none, err
		}
	}
	if payload.PerformedAt == nil || payload.PerformedAt.IsZero() {
		return none, fmt.Errorf("%w: performedAt is required", ErrMalformedRequest)
	}
	action, err := parseActionKind("action", payload.Action)
	if err != nil {
		return none, err
	}
	return application.RecordExecutionCommand{
		TenantID:    tenant,
		Node:        node,
		Item:        payload.Item,
		Unit:        payload.Unit,
		Action:      action,
		Evidence:    payload.Evidence,
		PerformedAt: *payload.PerformedAt,
		Operator:    operator,
	}, nil
}

var (
	_ CollaborationAcceptanceIntake = (*OperatorDecisionIntake)(nil)
	_ CollaborationAcceptanceIntake = UnconfiguredIntake{}
	_ CollaborationExecutionIntake  = UnconfiguredIntake{}
)

// IntakeCollaborationAcceptance 同 IntakeReception：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeCollaborationAcceptance(context.Context, *http.Request) (application.AcceptCollaborationCommand, error) {
	return application.AcceptCollaborationCommand{}, ErrAccessChannelNotConfigured
}

// IntakeCollaborationExecution 同 IntakeReception：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeCollaborationExecution(context.Context, *http.Request) (application.RecordExecutionCommand, error) {
	return application.RecordExecutionCommand{}, ErrAccessChannelNotConfigured
}

// NewAcceptCollaborationEndpoint 交回承接决定的 HTTP 入口。按 ADR-0022 映射：只有这次真落了决定（含待补充一轮）答 201；已有决定的重放、
// 同一事项的不同决定（冲突）与未受理是形成了的答案，答 200 带 outcome。库读写失败答 5xx：没形成答案，设备或页面留着重发。
func NewAcceptCollaborationEndpoint(intake CollaborationAcceptanceIntake, handler CollaborationHandler) http.Handler {
	return collaborationEndpoint(
		func(ctx context.Context, request *http.Request) (application.CollaborationResult, error, bool) {
			command, err := intake.IntakeCollaborationAcceptance(ctx, request)
			if err != nil {
				return application.CollaborationResult{}, err, false
			}
			result, err := handler.Accept(ctx, command)
			return result, err, true
		},
		func(outcome application.CollaborationOutcome) bool {
			return outcome == application.CollaborationDecided || outcome == application.CollaborationPendingSupplement
		},
	)
}

// NewRecordCollaborationExecutionEndpoint 交回执行事实登记的 HTTP 入口。首次登记答 201；已有事实的重放、内容冲突、拒接或待补充的事项、
// 越出承接范围与未受理都是形成了的答案，答 200 带 outcome——设备据此出队，恢复动作分别是重新承接、补齐缺口与扩承接范围，重发不会变好。
func NewRecordCollaborationExecutionEndpoint(intake CollaborationExecutionIntake, handler CollaborationHandler) http.Handler {
	return collaborationEndpoint(
		func(ctx context.Context, request *http.Request) (application.CollaborationResult, error, bool) {
			command, err := intake.IntakeCollaborationExecution(ctx, request)
			if err != nil {
				return application.CollaborationResult{}, err, false
			}
			result, err := handler.RecordExecution(ctx, command)
			return result, err, true
		},
		func(outcome application.CollaborationOutcome) bool { return outcome == application.ExecutionRecorded },
	)
}

func collaborationEndpoint(
	run func(ctx context.Context, request *http.Request) (application.CollaborationResult, error, bool),
	created func(application.CollaborationOutcome) bool,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		result, err, reached := run(request.Context(), request)
		switch {
		case err != nil && !reached:
			writeIntakeProblem(response, err)
			return
		case err != nil:
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeCollaborationOutcome(response, result, created(result.Outcome()))
	})
}

// collaborationResponse 是两口共用的封闭响应形状。outcome 取应用结果枚举的原名；continuationReference 非空说明发布意图还没交出去，
// 重放会重发同一份（ADR-0043）。承接回显的是在册的那一轮（round），事实回显的是在册的那一条。
type collaborationResponse struct {
	Outcome               string   `json:"outcome"`
	ContinuationReference string   `json:"continuationReference,omitempty"`
	Item                  string   `json:"item,omitempty"`
	Node                  string   `json:"node,omitempty"`
	Round                 int      `json:"round,omitempty"`
	Decision              string   `json:"decision,omitempty"`
	AcceptedUnits         []string `json:"acceptedUnits,omitempty"`
	AcceptedActions       []string `json:"acceptedActions,omitempty"`
	Gaps                  []string `json:"gaps,omitempty"`
	DecidedAt             string   `json:"decidedAt,omitempty"`
	Unit                  string   `json:"unit,omitempty"`
	Action                string   `json:"action,omitempty"`
	PerformedAt           string   `json:"performedAt,omitempty"`
}

func writeCollaborationOutcome(response http.ResponseWriter, result application.CollaborationResult, created bool) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := collaborationResponse{Outcome: outcome, ContinuationReference: result.HandoffReference()}
	if record, present := result.Acceptance(); present {
		acceptance := record.Acceptance
		body.Item = acceptance.Item().String()
		body.Node = acceptance.Node().String()
		body.Round = record.Round
		body.Decision = acceptance.Decision().String()
		body.DecidedAt = rfc3339(acceptance.DecidedAt())
		for _, unit := range acceptance.AcceptedUnits() {
			body.AcceptedUnits = append(body.AcceptedUnits, unit.String())
		}
		for _, action := range acceptance.AcceptedActions() {
			body.AcceptedActions = append(body.AcceptedActions, action.String())
		}
		for _, gap := range acceptance.Gaps() {
			body.Gaps = append(body.Gaps, gap.String())
		}
	}
	if record, present := result.Fact(); present {
		fact := record.Fact
		body.Item = fact.Item().String()
		body.Node = fact.Node().String()
		body.Unit = fact.Unit().String()
		body.Action = fact.Action().String()
		body.PerformedAt = rfc3339(fact.PerformedAt())
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
