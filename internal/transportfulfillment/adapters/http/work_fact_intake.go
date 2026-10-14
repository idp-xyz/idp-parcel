package tfhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 作业事实口的身份不进事实载荷（ADR-0023：载荷是设备签发的事实内容）。设备与作业范围从这三个头来。
const (
	headerWorkDevice    = "Parcel-Device-Id"
	headerWorkScopeKind = "Parcel-Work-Scope-Kind"
	headerWorkScopeRef  = "Parcel-Work-Scope-Reference"
)

// ErrDeviceNotInUse 表示令牌与授予都过了，设备未登记、已停用或登在别的作业范围上（403）。
var ErrDeviceNotInUse = errors.New("transport fulfillment http: device is not registered for this work scope or has been deactivated")

const codeDeviceNotInUse = "DEVICE_NOT_IN_USE"

// WorkFactAuthenticator 以作业事实登记能力面铸信封，交回租户。失败只交回本包的格。
type WorkFactAuthenticator interface {
	AuthenticateWorkFact(ctx context.Context, bearerToken, deviceID, scopeKind, scopeReference string) (domain.TenantID, error)
}

// WorkFactCommandIntake 是作业事实各口的操作者渠道 Intake（ADR-0149 决定二、票 operator-channel/10 余下半边）。
// 首登与更正共用这一份认证与授予。不实现关段、建派送任务、有效时间判断、承运商首次有效收寄判断——那四口归运营决定。
type WorkFactCommandIntake struct {
	authenticator  WorkFactAuthenticator
	movementSource string
}

func NewWorkFactCommandIntake(authenticator WorkFactAuthenticator, movementSource string) (*WorkFactCommandIntake, error) {
	if authenticator == nil {
		return nil, errors.New("transport fulfillment http: work fact intake needs an authenticator")
	}
	if _, err := domain.NewMovementSourceReference(movementSource); err != nil {
		return nil, fmt.Errorf("transport fulfillment http: work fact intake: %w", err)
	}
	return &WorkFactCommandIntake{authenticator: authenticator, movementSource: movementSource}, nil
}

var (
	_ PickupRegistrationIntake      = (*WorkFactCommandIntake)(nil)
	_ PickupAttemptIntake           = (*WorkFactCommandIntake)(nil)
	_ PickupCorrectionIntake        = (*WorkFactCommandIntake)(nil)
	_ HandoverRegistrationIntake    = (*WorkFactCommandIntake)(nil)
	_ HandoverCorrectionIntake      = (*WorkFactCommandIntake)(nil)
	_ MovementFactIntake            = (*WorkFactCommandIntake)(nil)
	_ DeliveryDispatchTriggerIntake = (*WorkFactCommandIntake)(nil)
	_ DeliveryRegistrationIntake    = (*WorkFactCommandIntake)(nil)
	_ DeliveryProofCorrectionIntake = (*WorkFactCommandIntake)(nil)
	_ DeliveryAttemptIntake         = (*WorkFactCommandIntake)(nil)
)

func (intake *WorkFactCommandIntake) tenantFrom(ctx context.Context, request *http.Request) (domain.TenantID, error) {
	device := strings.TrimSpace(request.Header.Get(headerWorkDevice))
	kind := strings.TrimSpace(request.Header.Get(headerWorkScopeKind))
	reference := strings.TrimSpace(request.Header.Get(headerWorkScopeRef))
	if device == "" || kind == "" || reference == "" {
		return domain.TenantID{}, fmt.Errorf("%w: %s, %s and %s are required", ErrMalformedRequest, headerWorkDevice, headerWorkScopeKind, headerWorkScopeRef)
	}
	// 收寄口只认 NODE。这一面若也认 NODE，登在节点上的设备就能拿节点授予打揽收、交接和交付。
	if kind != "OFFSITE" {
		return domain.TenantID{}, fmt.Errorf("%w: transport fulfillment work facts require Parcel-Work-Scope-Kind OFFSITE", ErrMalformedRequest)
	}
	return intake.authenticator.AuthenticateWorkFact(ctx, httpapi.BearerToken(request), device, kind, reference)
}

func (intake *WorkFactCommandIntake) IntakePickupRegistration(ctx context.Context, request *http.Request) (application.RegisterOffsitePickupCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.RegisterOffsitePickupCommand{}, err
	}
	var payload OffsitePickupRegistrationPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.RegisterOffsitePickupCommand{}, err
	}
	return payload.Command(tenant)
}

func (intake *WorkFactCommandIntake) IntakePickupAttempt(ctx context.Context, request *http.Request) (application.PerformOffsitePickupCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.PerformOffsitePickupCommand{}, err
	}
	var payload OffsitePickupAttemptPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.PerformOffsitePickupCommand{}, err
	}
	return payload.Command(tenant)
}

func (intake *WorkFactCommandIntake) IntakePickupCorrection(ctx context.Context, request *http.Request) (application.CorrectOffsitePickupCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.CorrectOffsitePickupCommand{}, err
	}
	var payload OffsitePickupCorrectionPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.CorrectOffsitePickupCommand{}, err
	}
	return payload.Command(tenant)
}

func (intake *WorkFactCommandIntake) IntakeHandoverRegistration(ctx context.Context, request *http.Request) (application.RegisterTransportHandoverCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.RegisterTransportHandoverCommand{}, err
	}
	var payload HandoverRegistrationPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.RegisterTransportHandoverCommand{}, err
	}
	return payload.Command(tenant)
}

func (intake *WorkFactCommandIntake) IntakeHandoverCorrection(ctx context.Context, request *http.Request) (application.CorrectTransportHandoverCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.CorrectTransportHandoverCommand{}, err
	}
	var payload HandoverCorrectionPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.CorrectTransportHandoverCommand{}, err
	}
	return payload.Command(tenant)
}

func (intake *WorkFactCommandIntake) IntakeMovementFact(ctx context.Context, request *http.Request) (application.RecordMovementFactCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.RecordMovementFactCommand{}, err
	}
	var payload MovementFactPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.RecordMovementFactCommand{}, err
	}
	return payload.Command(tenant, intake.movementSource)
}

func (intake *WorkFactCommandIntake) IntakeDeliveryDispatchTrigger(ctx context.Context, request *http.Request) (application.TriggerDeliveryDispatchCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.TriggerDeliveryDispatchCommand{}, err
	}
	var payload DeliveryDispatchTriggerPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.TriggerDeliveryDispatchCommand{}, err
	}
	return payload.Command(tenant)
}

func (intake *WorkFactCommandIntake) IntakeRegistration(ctx context.Context, request *http.Request) (application.RegisterEffectiveDeliveryCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.RegisterEffectiveDeliveryCommand{}, err
	}
	var payload EffectiveDeliveryPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.RegisterEffectiveDeliveryCommand{}, err
	}
	return payload.Command(tenant)
}

func (intake *WorkFactCommandIntake) IntakeCorrection(ctx context.Context, request *http.Request) (application.CorrectDeliveryProofCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.CorrectDeliveryProofCommand{}, err
	}
	var payload DeliveryProofCorrectionPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.CorrectDeliveryProofCommand{}, err
	}
	return payload.Command(tenant)
}

func (intake *WorkFactCommandIntake) IntakeDeliveryAttempt(ctx context.Context, request *http.Request) (application.RecordDeliveryAttemptCommand, error) {
	tenant, err := intake.tenantFrom(ctx, request)
	if err != nil {
		return application.RecordDeliveryAttemptCommand{}, err
	}
	var payload DeliveryAttemptPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.RecordDeliveryAttemptCommand{}, err
	}
	return payload.Command(tenant)
}

// HandoverCorrectionPayload 是交接更正的线格式，逐格镜像更正命令去掉租户。
type HandoverCorrectionPayload struct {
	Object             string `json:"object"`
	Scope              string `json:"scope"`
	PredecessorVersion string `json:"predecessorVersion"`
	Verdict            string `json:"verdict"`
	ReleasingEvidence  string `json:"releasingEvidence"`
	ReceivingEvidence  string `json:"receivingEvidence"`
	Rule               string `json:"rule"`
	Basis              string `json:"basis"`
	NewVersion         string `json:"newVersion"`
	CorrectedAt        string `json:"correctedAt"`
}

func (payload HandoverCorrectionPayload) Command(tenant domain.TenantID) (application.CorrectTransportHandoverCommand, error) {
	none := application.CorrectTransportHandoverCommand{}
	if tenant.String() == "" {
		return none, ErrOperatorIdentityMissing
	}
	verdict, err := handoverVerdictFromWord(payload.Verdict)
	if err != nil {
		return none, err
	}
	correctedAt, err := time.Parse(time.RFC3339, payload.CorrectedAt)
	if err != nil {
		return none, fmt.Errorf("%w: correctedAt: %v", ErrMalformedRequest, err)
	}
	return application.CorrectTransportHandoverCommand{
		TenantID:           tenant,
		Object:             payload.Object,
		Scope:              payload.Scope,
		PredecessorVersion: payload.PredecessorVersion,
		Verdict:            verdict,
		ReleasingEvidence:  payload.ReleasingEvidence,
		ReceivingEvidence:  payload.ReceivingEvidence,
		Rule:               payload.Rule,
		Basis:              payload.Basis,
		NewVersion:         payload.NewVersion,
		CorrectedAt:        correctedAt,
	}, nil
}

// OffsitePickupCorrectionPayload 是单对象揽收更正的线格式。
type OffsitePickupCorrectionPayload struct {
	Object             string `json:"object"`
	Attempt            string `json:"attempt"`
	PredecessorVersion string `json:"predecessorVersion"`
	Place              string `json:"place"`
	Control            string `json:"control"`
	ExecutedBy         string `json:"executedBy"`
	OccurredAt         string `json:"occurredAt"`
	CorrectedAt        string `json:"correctedAt"`
}

func (payload OffsitePickupCorrectionPayload) Command(tenant domain.TenantID) (application.CorrectOffsitePickupCommand, error) {
	none := application.CorrectOffsitePickupCommand{}
	if tenant.String() == "" {
		return none, ErrOperatorIdentityMissing
	}
	occurredAt, err := time.Parse(time.RFC3339, payload.OccurredAt)
	if err != nil {
		return none, fmt.Errorf("%w: occurredAt: %v", ErrMalformedRequest, err)
	}
	correctedAt, err := time.Parse(time.RFC3339, payload.CorrectedAt)
	if err != nil {
		return none, fmt.Errorf("%w: correctedAt: %v", ErrMalformedRequest, err)
	}
	return application.CorrectOffsitePickupCommand{
		TenantID:           tenant,
		Object:             payload.Object,
		Attempt:            payload.Attempt,
		PredecessorVersion: payload.PredecessorVersion,
		Place:              payload.Place,
		Control:            payload.Control,
		ExecutedBy:         payload.ExecutedBy,
		OccurredAt:         occurredAt,
		CorrectedAt:        correctedAt,
	}, nil
}

// DeliveryProofCorrectionPayload 是 POD 更正的线格式。
type DeliveryProofCorrectionPayload struct {
	Attempt     string `json:"attempt"`
	Object      string `json:"object"`
	NewProof    string `json:"newProof"`
	CorrectedAt string `json:"correctedAt"`
}

func (payload DeliveryProofCorrectionPayload) Command(tenant domain.TenantID) (application.CorrectDeliveryProofCommand, error) {
	none := application.CorrectDeliveryProofCommand{}
	if tenant.String() == "" {
		return none, ErrOperatorIdentityMissing
	}
	correctedAt, err := time.Parse(time.RFC3339, payload.CorrectedAt)
	if err != nil {
		return none, fmt.Errorf("%w: correctedAt: %v", ErrMalformedRequest, err)
	}
	return application.CorrectDeliveryProofCommand{
		TenantID:    tenant,
		Attempt:     payload.Attempt,
		Object:      payload.Object,
		NewProof:    payload.NewProof,
		CorrectedAt: correctedAt,
	}, nil
}
