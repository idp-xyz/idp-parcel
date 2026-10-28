package tfhttp

import (
	"context"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 监管运输处置承接与替代/退运旅程启动两口的运营决定面（票 docs-consistency/code/43，ADR-0337）。各成一种决定，与别的决定、也彼此
// 分开授予——授予的粒度等于权力的粒度。另立文件，理由同 operator_decision_intake_corrections.go。

const (
	OperatorDecisionRegulatoryDispositionAcceptance OperatorDecision = "REGULATORY_DISPOSITION_ACCEPTANCE"
	OperatorDecisionAlternateJourneyStart           OperatorDecision = "ALTERNATE_JOURNEY_START"
)

var (
	_ RegulatoryDispositionAcceptanceIntake = (*OperatorDecisionIntake)(nil)
	_ AlternateJourneyStartIntake           = (*OperatorDecisionIntake)(nil)
)

// IntakeRegulatoryDispositionAcceptance 译监管运输处置承接（`/transport-fulfillment-regulatory-disposition-acceptances`）。操作者随命令
// 带进去，作为操作证据记在承接决定上，只进审计（ADR-0303 的待遇）。
func (intake *OperatorDecisionIntake) IntakeRegulatoryDispositionAcceptance(ctx context.Context, request *http.Request) (application.AcceptRegulatoryDispositionCommand, error) {
	operator, err := intake.identityFor(ctx, request, OperatorDecisionRegulatoryDispositionAcceptance)
	if err != nil {
		return application.AcceptRegulatoryDispositionCommand{}, err
	}
	if strings.TrimSpace(operator.Operator) == "" {
		return application.AcceptRegulatoryDispositionCommand{}, ErrOperatorIdentityMissing
	}
	var payload RegulatoryDispositionAcceptancePayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.AcceptRegulatoryDispositionCommand{}, err
	}
	return payload.Command(operator)
}

// IntakeAlternateJourneyStart 译替代/退运旅程启动（`/transport-fulfillment-alternate-journey-starts`）。
func (intake *OperatorDecisionIntake) IntakeAlternateJourneyStart(ctx context.Context, request *http.Request) (application.StartAlternateJourneyCommand, error) {
	operator, err := intake.identityFor(ctx, request, OperatorDecisionAlternateJourneyStart)
	if err != nil {
		return application.StartAlternateJourneyCommand{}, err
	}
	if strings.TrimSpace(operator.Operator) == "" {
		return application.StartAlternateJourneyCommand{}, ErrOperatorIdentityMissing
	}
	var payload AlternateJourneyStartPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.StartAlternateJourneyCommand{}, err
	}
	return payload.Command(operator)
}
