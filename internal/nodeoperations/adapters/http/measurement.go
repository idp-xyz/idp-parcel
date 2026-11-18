package nodeopshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// ActualMeasurementHandler 是测量登记与作废两口共用的编排。
type ActualMeasurementHandler interface {
	Handle(context.Context, application.RecordActualMeasurementCommand) (application.RecordActualMeasurementResult, error)
	Invalidate(context.Context, application.InvalidateActualMeasurementCommand) (application.InvalidateActualMeasurementResult, error)
}

// MeasurementIntake 把一次已认证的作业事实请求译成测量命令。租户与节点来自认证，不来自载荷。
type MeasurementIntake interface {
	IntakeMeasurement(context.Context, *http.Request) (application.RecordActualMeasurementCommand, error)
	IntakeMeasurementInvalidation(context.Context, *http.Request) (application.InvalidateActualMeasurementCommand, error)
}

// ManualMeasurementIntake 把一次运营决定译成手工测量命令。录入人取出示的操作者，不取载荷。
type ManualMeasurementIntake interface {
	IntakeManualMeasurement(context.Context, *http.Request) (application.RecordActualMeasurementCommand, error)
	IntakeManualMeasurementInvalidation(context.Context, *http.Request) (application.InvalidateActualMeasurementCommand, error)
}

var _ MeasurementIntake = (*WorkFactReceptionIntake)(nil)
var _ MeasurementIntake = UnconfiguredIntake{}
var _ ManualMeasurementIntake = (*OperatorDecisionIntake)(nil)
var _ ManualMeasurementIntake = UnconfiguredIntake{}

func (intake *WorkFactReceptionIntake) IntakeMeasurement(ctx context.Context, request *http.Request) (application.RecordActualMeasurementCommand, error) {
	none := application.RecordActualMeasurementCommand{}
	tenant, node, err := intake.authenticateNode(ctx, request)
	if err != nil {
		return none, err
	}
	var document measurementDocument
	if err := decodeClosedDocument(request, &document); err != nil {
		return none, err
	}
	return measurementCommand(tenant, node, document, "", "")
}

func (intake *WorkFactReceptionIntake) IntakeMeasurementInvalidation(ctx context.Context, request *http.Request) (application.InvalidateActualMeasurementCommand, error) {
	none := application.InvalidateActualMeasurementCommand{}
	tenant, _, err := intake.authenticateNode(ctx, request)
	if err != nil {
		return none, err
	}
	var document measurementInvalidationDocument
	if err := decodeClosedDocument(request, &document); err != nil {
		return none, err
	}
	return invalidationCommand(tenant, document, "", "")
}

func (intake *WorkFactReceptionIntake) authenticateNode(ctx context.Context, request *http.Request) (domain.TenantID, domain.NodeReference, error) {
	device := strings.TrimSpace(request.Header.Get(headerWorkDevice))
	kind := strings.TrimSpace(request.Header.Get(headerWorkScopeKind))
	reference := strings.TrimSpace(request.Header.Get(headerWorkScopeRef))
	if device == "" || kind == "" || reference == "" {
		return domain.TenantID{}, domain.NodeReference{}, fmt.Errorf("%w: %s, %s and %s are required", ErrMalformedRequest, headerWorkDevice, headerWorkScopeKind, headerWorkScopeRef)
	}
	if kind != "NODE" {
		return domain.TenantID{}, domain.NodeReference{}, fmt.Errorf("%w: node work requires Parcel-Work-Scope-Kind NODE", ErrMalformedRequest)
	}
	return intake.authenticator.AuthenticateNodeReception(ctx, httpapi.BearerToken(request), device, kind, reference)
}

func (UnconfiguredIntake) IntakeMeasurement(context.Context, *http.Request) (application.RecordActualMeasurementCommand, error) {
	return application.RecordActualMeasurementCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeMeasurementInvalidation(context.Context, *http.Request) (application.InvalidateActualMeasurementCommand, error) {
	return application.InvalidateActualMeasurementCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeManualMeasurement(context.Context, *http.Request) (application.RecordActualMeasurementCommand, error) {
	return application.RecordActualMeasurementCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeManualMeasurementInvalidation(context.Context, *http.Request) (application.InvalidateActualMeasurementCommand, error) {
	return application.InvalidateActualMeasurementCommand{}, ErrAccessChannelNotConfigured
}

func (intake *OperatorDecisionIntake) IntakeManualMeasurement(ctx context.Context, request *http.Request) (application.RecordActualMeasurementCommand, error) {
	none := application.RecordActualMeasurementCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionManualMeasurementEntry)
	if err != nil {
		return none, err
	}
	var document measurementDocument
	if err := decodeClosedDocument(request, &document); err != nil {
		return none, err
	}
	if strings.TrimSpace(document.OriginalDocument) == "" {
		return none, fmt.Errorf("%w: originalDocument is required", ErrMalformedRequest)
	}
	node, err := domain.NewNodeReference(document.Node)
	if err != nil {
		return none, fmt.Errorf("%w: node: %v", ErrMalformedRequest, err)
	}
	return measurementCommand(operator.Tenant, node, document, operator.Operator, document.OriginalDocument)
}

func (intake *OperatorDecisionIntake) IntakeManualMeasurementInvalidation(ctx context.Context, request *http.Request) (application.InvalidateActualMeasurementCommand, error) {
	none := application.InvalidateActualMeasurementCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionManualMeasurementEntry)
	if err != nil {
		return none, err
	}
	var document measurementInvalidationDocument
	if err := decodeClosedDocument(request, &document); err != nil {
		return none, err
	}
	if strings.TrimSpace(document.OriginalDocument) == "" {
		return none, fmt.Errorf("%w: originalDocument is required", ErrMalformedRequest)
	}
	return invalidationCommand(operator.Tenant, document, operator.Operator, document.OriginalDocument)
}

// measurementDocument 是一次称重量方的载荷。重量与长宽高都必填，没有「只报其中一项」的写法：缺了哪一格，
// 口上都拒成载荷不成形。复测是带新来源标识再报一条完整的。
type measurementDocument struct {
	SourceID         string `json:"sourceId"`
	ObjectKind       string `json:"objectKind"`
	ObjectID         string `json:"objectId"`
	Node             string `json:"node,omitempty"`
	Location         string `json:"location"`
	Basis            string `json:"basis"`
	PerformedBy      string `json:"performedBy"`
	EvidenceRef      string `json:"evidenceRef"`
	OccurredAt       string `json:"occurredAt"`
	WeightValue      string `json:"weightValue"`
	WeightUnit       string `json:"weightUnit"`
	Length           string `json:"length"`
	Width            string `json:"width"`
	Height           string `json:"height"`
	DimensionUnit    string `json:"dimensionUnit"`
	OriginalDocument string `json:"originalDocument,omitempty"`
}

type measurementInvalidationDocument struct {
	SourceID         string `json:"sourceId"`
	TargetSourceID   string `json:"targetSourceId"`
	Reason           string `json:"reason"`
	PerformedBy      string `json:"performedBy"`
	EvidenceRef      string `json:"evidenceRef"`
	OccurredAt       string `json:"occurredAt"`
	OriginalDocument string `json:"originalDocument,omitempty"`
}

func measurementCommand(
	tenant domain.TenantID,
	node domain.NodeReference,
	document measurementDocument,
	enteredBy string,
	original string,
) (application.RecordActualMeasurementCommand, error) {
	none := application.RecordActualMeasurementCommand{}
	if err := boundedSourceID(document.SourceID); err != nil {
		return none, err
	}
	occurredAt, err := time.Parse(time.RFC3339, document.OccurredAt)
	if err != nil {
		return none, fmt.Errorf("%w: occurredAt is not an RFC 3339 instant: %v", ErrMalformedRequest, err)
	}
	// 重量与长宽高是必填字段：缺格在口上就拒成载荷不成形，到不了编排，与到领域才形成不了的 NOT_ACCEPTED 分开。
	// 缺的是哪一格只在服务端的错误文本里，这一族的响应形状不带 detail。数字是不是正数、单位在不在产品内置目录上，
	// 仍由领域与编排判。
	for _, field := range []struct{ name, value string }{
		{"weightValue", document.WeightValue},
		{"weightUnit", document.WeightUnit},
		{"length", document.Length},
		{"width", document.Width},
		{"height", document.Height},
		{"dimensionUnit", document.DimensionUnit},
	} {
		if err := requiredField(field.name, field.value); err != nil {
			return none, err
		}
	}
	return application.RecordActualMeasurementCommand{
		TenantID: tenant.String(), SourceID: document.SourceID, PerformedBy: document.PerformedBy,
		Evidence: document.EvidenceRef, OccurredAt: occurredAt, ObjectKind: document.ObjectKind,
		ObjectID: document.ObjectID, Node: node.String(), Location: document.Location, Basis: document.Basis,
		WeightValue: document.WeightValue, WeightUnit: document.WeightUnit,
		Length: document.Length, Width: document.Width, Height: document.Height, DimensionUnit: document.DimensionUnit,
		EnteredBy: enteredBy, OriginalDocument: original,
	}, nil
}

func invalidationCommand(
	tenant domain.TenantID,
	document measurementInvalidationDocument,
	enteredBy string,
	original string,
) (application.InvalidateActualMeasurementCommand, error) {
	none := application.InvalidateActualMeasurementCommand{}
	if err := boundedSourceID(document.SourceID); err != nil {
		return none, err
	}
	if strings.TrimSpace(document.TargetSourceID) == "" {
		return none, fmt.Errorf("%w: targetSourceId is required", ErrMalformedRequest)
	}
	occurredAt, err := time.Parse(time.RFC3339, document.OccurredAt)
	if err != nil {
		return none, fmt.Errorf("%w: occurredAt is not an RFC 3339 instant: %v", ErrMalformedRequest, err)
	}
	return application.InvalidateActualMeasurementCommand{
		TenantID: tenant.String(), SourceID: document.SourceID, TargetSourceID: document.TargetSourceID,
		Reason: document.Reason, PerformedBy: document.PerformedBy, Evidence: document.EvidenceRef,
		OccurredAt: occurredAt, EnteredBy: enteredBy, OriginalDocument: original,
	}, nil
}

func boundedSourceID(sourceID string) error {
	if strings.TrimSpace(sourceID) == "" {
		return fmt.Errorf("%w: sourceId is blank; the fact identity is not minted here", ErrMalformedRequest)
	}
	if len(sourceID) > eventing.MaxSubjectLength || strings.IndexFunc(sourceID, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: sourceId exceeds %d bytes or carries a control character", ErrMalformedRequest, eventing.MaxSubjectLength)
	}
	return nil
}

// NewRecordMeasurementEndpoint 交回作业端的测量登记口。
func NewRecordMeasurementEndpoint(intake MeasurementIntake, handler ActualMeasurementHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeMeasurement(request.Context(), request)
		if err != nil {
			if deviceNotInUse(response, err) {
				return
			}
			writeIntakeProblem(response, err)
			return
		}
		result, err := handler.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeMeasurementOutcome(response, result.Outcome().String(), result.Outcome() == application.ActualMeasurementRecorded)
	})
}

// NewInvalidateMeasurementEndpoint 交回作业端的测量作废口。
func NewInvalidateMeasurementEndpoint(intake MeasurementIntake, handler ActualMeasurementHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeMeasurementInvalidation(request.Context(), request)
		if err != nil {
			if deviceNotInUse(response, err) {
				return
			}
			writeIntakeProblem(response, err)
			return
		}
		result, err := handler.Invalidate(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeMeasurementOutcome(response, result.Outcome().String(), result.Outcome() == application.MeasurementInvalidationRecorded)
	})
}

// NewRecordManualMeasurementEndpoint 交回手工登记口。
func NewRecordManualMeasurementEndpoint(intake ManualMeasurementIntake, handler ActualMeasurementHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeManualMeasurement(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := handler.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeMeasurementOutcome(response, result.Outcome().String(), result.Outcome() == application.ActualMeasurementRecorded)
	})
}

// NewInvalidateManualMeasurementEndpoint 交回手工作废口。
func NewInvalidateManualMeasurementEndpoint(intake ManualMeasurementIntake, handler ActualMeasurementHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeManualMeasurementInvalidation(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := handler.Invalidate(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeMeasurementOutcome(response, result.Outcome().String(), result.Outcome() == application.MeasurementInvalidationRecorded)
	})
}

func deviceNotInUse(response http.ResponseWriter, err error) bool {
	if !errors.Is(err, ErrDeviceNotInUse) {
		return false
	}
	writeProblem(response, http.StatusForbidden, codeDeviceNotInUse)
	return true
}

func writeMeasurementOutcome(response http.ResponseWriter, outcome string, created bool) {
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(response, status, map[string]string{"outcome": outcome})
}
