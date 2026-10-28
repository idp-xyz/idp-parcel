package tfhttp

import (
	"context"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 替代/退运旅程启动口（UC-TF-007，与 UC-TF-001 共用的独立旅程启动，ADR-0337）。授权业务角色采用备用伙伴或退运，是管理台上
// 作出的生产决定，挂操作者渠道的「运营决定」能力面、按启动替代或退运旅程这一种认证：能承接监管运输处置的不因此能开旅程。

// AlternateJourneyStartIntake 把已认证的运营写请求翻译成旅程启动命令。
type AlternateJourneyStartIntake interface {
	IntakeAlternateJourneyStart(ctx context.Context, request *http.Request) (application.StartAlternateJourneyCommand, error)
}

// AlternateJourneyStarter 是本适配器转交的应用编排。
type AlternateJourneyStarter interface {
	Handle(ctx context.Context, command application.StartAlternateJourneyCommand) (application.StartAlternateJourneyResult, error)
}

// AlternateJourneyStartPayload 是旅程启动口的线格式。载荷只有内容、没有身份（封闭解码）。`basisKind` 显式标出处置依据的来路
// （PS 处置决定或监管处置决定），不由依据串猜；启动时刻照 ADR-0023 由提交方给。
type AlternateJourneyStartPayload struct {
	Journey   string   `json:"journey"`
	Purpose   string   `json:"purpose"`
	Original  string   `json:"original"`
	BasisKind string   `json:"basisKind"`
	Basis     string   `json:"basis"`
	Members   []string `json:"members"`
	StartedAt string   `json:"startedAt"`
}

// Command 把载荷连同信封给的身份（租户，与作出决定的操作者，ADR-0303 操作证据，载荷里不收）翻成旅程启动命令。目的与来路不在
// 封闭集合里、时刻解不出是坏报文（400）；其余形状由编排与领域答`未受理`（200）。
func (payload AlternateJourneyStartPayload) Command(operator OperatorIdentity) (application.StartAlternateJourneyCommand, error) {
	if operator.Tenant.String() == "" {
		return application.StartAlternateJourneyCommand{}, ErrOperatorIdentityMissing
	}
	var purpose domain.JourneyPurpose
	switch payload.Purpose {
	case domain.AlternateJourneyPurpose.String():
		purpose = domain.AlternateJourneyPurpose
	case domain.ReturnJourneyPurpose.String():
		purpose = domain.ReturnJourneyPurpose
	default:
		return application.StartAlternateJourneyCommand{}, fmt.Errorf("%w: purpose is not one of ALTERNATE, RETURN", ErrMalformedRequest)
	}
	var basisKind domain.DispositionBasisKind
	switch payload.BasisKind {
	case domain.ServiceDispositionDecision.String():
		basisKind = domain.ServiceDispositionDecision
	case domain.RegulatoryDispositionDecision.String():
		basisKind = domain.RegulatoryDispositionDecision
	default:
		return application.StartAlternateJourneyCommand{}, fmt.Errorf("%w: basisKind is not one of SERVICE_DISPOSITION, REGULATORY_DISPOSITION", ErrMalformedRequest)
	}
	startedAt, err := parseOptionalInstant("startedAt", payload.StartedAt)
	if err != nil {
		return application.StartAlternateJourneyCommand{}, err
	}
	return application.StartAlternateJourneyCommand{
		TenantID:  operator.Tenant,
		Journey:   payload.Journey,
		Purpose:   purpose,
		Original:  payload.Original,
		BasisKind: basisKind,
		Basis:     payload.Basis,
		Members:   payload.Members,
		StartedAt: startedAt,
		StartedBy: operator.Operator,
	}, nil
}

// NewStartAlternateJourneyEndpoint 交回替代/退运旅程启动的 HTTP 入口。
func NewStartAlternateJourneyEndpoint(intake AlternateJourneyStartIntake, starter AlternateJourneyStarter) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.StartAlternateJourneyResult, error, bool) {
		command, err := intake.IntakeAlternateJourneyStart(request.Context(), request)
		if err != nil {
			return application.StartAlternateJourneyResult{}, err, false
		}
		result, err := starter.Handle(request.Context(), command)
		return result, err, true
	}, writeAlternateJourneyStartOutcome)
}

// alternateJourneyStartResponse 是旅程启动口的封闭响应形状。`continuationReference` 是旅程册未决时的续办引用（原样重交即可）；
// 两个 handoff 引用非空说明对应链的意图还没交出去，重放会重发同一份（ADR-0043）。
type alternateJourneyStartResponse struct {
	Outcome                    string                  `json:"outcome"`
	UndecidedReason            string                  `json:"undecidedReason,omitempty"`
	ContinuationReference      string                  `json:"continuationReference,omitempty"`
	DispositionHandoffContinue string                  `json:"dispositionHandoffReference,omitempty"`
	ExceptionHandoffContinue   string                  `json:"exceptionHandoffReference,omitempty"`
	Journey                    *alternateJourneyRecord `json:"journey,omitempty"`
}

type alternateJourneyRecord struct {
	Journey    string   `json:"journey"`
	Original   string   `json:"original"`
	Purpose    string   `json:"purpose"`
	BasisKind  string   `json:"basisKind"`
	Basis      string   `json:"basis"`
	Members    []string `json:"members"`
	StartedAt  string   `json:"startedAt"`
	RecordedAt string   `json:"recordedAt"`
}

func writeAlternateJourneyStartOutcome(response http.ResponseWriter, result application.StartAlternateJourneyResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := alternateJourneyStartResponse{
		Outcome:                    outcome,
		UndecidedReason:            result.UndecidedReason().String(),
		ContinuationReference:      result.ContinuationReference(),
		DispositionHandoffContinue: result.DispositionHandoffReference(),
		ExceptionHandoffContinue:   result.ExceptionHandoffReference(),
	}
	if record, present := result.Record(); present {
		echo := &alternateJourneyRecord{
			Journey:    record.Journey.Journey().String(),
			Original:   record.Journey.OriginalJourney().String(),
			Purpose:    record.Journey.Purpose().String(),
			BasisKind:  record.Journey.BasisKind().String(),
			Basis:      record.Journey.Basis().String(),
			Members:    []string{},
			StartedAt:  rfc3339(record.Journey.StartedAt()),
			RecordedAt: rfc3339(record.RecordedAt),
		}
		for _, member := range record.Journey.Members() {
			echo.Members = append(echo.Members, member.String())
		}
		body.Journey = echo
	}
	// ADR-0022：只有这次真启动了旅程用 201；已有旅程的重放、来源冲突、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.JourneyStarted {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
