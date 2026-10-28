package tfhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 监管运输处置承接口（UC-TF-001，ADR-0265、ADR-0337）。运输运营对关务协作事项形成承接决定，是管理台上作出的生产决定，挂
// 操作者渠道的「运营决定」能力面、按承接监管运输处置这一种认证（ADR-0151 定族判据）。路径取读面册名前缀
// `transport-fulfillment-`，理由同委托提交口。

// RegulatoryDispositionAcceptanceIntake 把已认证的运营写请求翻译成承接命令。
type RegulatoryDispositionAcceptanceIntake interface {
	IntakeRegulatoryDispositionAcceptance(ctx context.Context, request *http.Request) (application.AcceptRegulatoryDispositionCommand, error)
}

// RegulatoryDispositionAcceptor 是本适配器转交的应用编排。
type RegulatoryDispositionAcceptor interface {
	Handle(ctx context.Context, command application.AcceptRegulatoryDispositionCommand) (application.AcceptDispositionResult, error)
}

// RegulatoryDispositionAcceptancePayload 是承接口的线格式。载荷只有内容、没有身份（封闭解码）。`itemObjects`、`origin`、
// `destination`、`movementAction` 是据关务协作事项抄录的事项本体（关务一侧今天没有按引用读回的提供方，ADR-0337 决定三）；
// 决定时刻照 ADR-0023 由提交方给。
type RegulatoryDispositionAcceptancePayload struct {
	Item              string   `json:"item"`
	Basis             string   `json:"basis"`
	MovementAction    string   `json:"movementAction"`
	ItemObjects       []string `json:"itemObjects"`
	Origin            string   `json:"origin"`
	Destination       string   `json:"destination"`
	Decision          string   `json:"decision"`
	AcceptedObjects   []string `json:"acceptedObjects"`
	DeclineBasis      string   `json:"declineBasis"`
	MovementAuthority string   `json:"movementAuthority"`
	DecidedAt         string   `json:"decidedAt"`
}

// Command 把载荷连同信封给的身份（租户，与作出决定的操作者，ADR-0303 操作证据，载荷里不收）翻成承接命令。引用的形状
// 由编排与领域答`未受理`（200）；决定取值不在封闭集合里是坏报文（400），时刻解不出同。
func (payload RegulatoryDispositionAcceptancePayload) Command(operator OperatorIdentity) (application.AcceptRegulatoryDispositionCommand, error) {
	if operator.Tenant.String() == "" {
		return application.AcceptRegulatoryDispositionCommand{}, ErrOperatorIdentityMissing
	}
	decision, err := parseDispositionAcceptanceKind(payload.Decision)
	if err != nil {
		return application.AcceptRegulatoryDispositionCommand{}, err
	}
	decidedAt, err := parseOptionalInstant("decidedAt", payload.DecidedAt)
	if err != nil {
		return application.AcceptRegulatoryDispositionCommand{}, err
	}
	// 引用串在这里译成领域引用：空串译不出来，等价于缺席，交给编排答`未受理`——零值引用的 String() 为空，编排的受理门认得。
	item, _ := domain.NewCollaborationItemReference(payload.Item)
	basis, _ := domain.NewDispositionBasisReference(payload.Basis)
	return application.AcceptRegulatoryDispositionCommand{
		TenantID:          operator.Tenant,
		Item:              item,
		Basis:             basis,
		MovementAction:    payload.MovementAction,
		ItemObjects:       payload.ItemObjects,
		Origin:            payload.Origin,
		Destination:       payload.Destination,
		Decision:          decision,
		AcceptedObjects:   payload.AcceptedObjects,
		DeclineBasis:      payload.DeclineBasis,
		MovementAuthority: payload.MovementAuthority,
		DecidedAt:         decidedAt,
		DecidedBy:         operator.Operator,
	}, nil
}

func parseDispositionAcceptanceKind(raw string) (domain.DispositionAcceptanceKind, error) {
	for _, kind := range []domain.DispositionAcceptanceKind{domain.DispositionAccepted, domain.DispositionPartiallyAccepted, domain.DispositionDeclined} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return domain.DispositionAcceptanceKindInvalid, fmt.Errorf("%w: decision is not one of ACCEPTED, PARTIALLY_ACCEPTED, DECLINED", ErrMalformedRequest)
}

// NewAcceptRegulatoryDispositionEndpoint 交回监管运输处置承接的 HTTP 入口。
func NewAcceptRegulatoryDispositionEndpoint(intake RegulatoryDispositionAcceptanceIntake, acceptor RegulatoryDispositionAcceptor) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.AcceptDispositionResult, error, bool) {
		command, err := intake.IntakeRegulatoryDispositionAcceptance(request.Context(), request)
		if err != nil {
			return application.AcceptDispositionResult{}, err, false
		}
		result, err := acceptor.Handle(request.Context(), command)
		return result, err, true
	}, writeRegulatoryDispositionAcceptanceOutcome)
}

// regulatoryDispositionAcceptanceResponse 是承接口的封闭响应形状。`SUPPLEMENT_REQUIRED` 带一个原因、指名缺哪一项（ADR-0265
// 决定四），续办是协作事项补齐后再交一次；`NODE_ONLY_DISPOSITION` 续办在节点作业。`continuationReference` 非空说明回执意图还没
// 交出去，重放会重发同一份（ADR-0043）。
type regulatoryDispositionAcceptanceResponse struct {
	Outcome               string                         `json:"outcome"`
	UndecidedReason       string                         `json:"undecidedReason,omitempty"`
	SupplementReason      string                         `json:"supplementReason,omitempty"`
	ContinuationReference string                         `json:"continuationReference,omitempty"`
	Decision              *regulatoryDispositionDecision `json:"decision,omitempty"`
}

type regulatoryDispositionDecision struct {
	Item              string   `json:"item"`
	Basis             string   `json:"basis"`
	Kind              string   `json:"kind"`
	AcceptedObjects   []string `json:"acceptedObjects"`
	DeclineBasis      string   `json:"declineBasis,omitempty"`
	MovementAuthority string   `json:"movementAuthority,omitempty"`
	DecidedAt         string   `json:"decidedAt"`
}

func writeRegulatoryDispositionAcceptanceOutcome(response http.ResponseWriter, result application.AcceptDispositionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := regulatoryDispositionAcceptanceResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		SupplementReason:      result.SupplementReason().String(),
		ContinuationReference: result.HandoffReference(),
	}
	if record, present := result.Acceptance(); present {
		decision := record.Decision
		echo := &regulatoryDispositionDecision{
			Item:            decision.Item().String(),
			Basis:           decision.Basis().String(),
			Kind:            decision.Kind().String(),
			AcceptedObjects: []string{},
			DeclineBasis:    decision.DeclineBasis(),
			DecidedAt:       rfc3339(decision.DecidedAt()),
		}
		for _, object := range decision.AcceptedObjects() {
			echo.AcceptedObjects = append(echo.AcceptedObjects, object.String())
		}
		if authority, present := decision.MovementAuthority(); present {
			echo.MovementAuthority = authority.String()
		}
		body.Decision = echo
	}
	// ADR-0022：只有这次真落了承接决定用 201；已有决定的重放、冲突、待补充、纯节点动作、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.DispositionDecided {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
