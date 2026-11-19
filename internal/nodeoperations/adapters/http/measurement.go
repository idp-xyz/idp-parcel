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

const measurementNotNode = "node work requires " + httpapi.HeaderWorkScopeKind + " NODE"

var _ MeasurementIntake = (*WorkFactReceptionIntake)(nil)
var _ MeasurementIntake = UnconfiguredIntake{}
var _ ManualMeasurementIntake = (*OperatorDecisionIntake)(nil)
var _ ManualMeasurementIntake = UnconfiguredIntake{}

func (intake *WorkFactReceptionIntake) IntakeMeasurement(ctx context.Context, request *http.Request) (application.RecordActualMeasurementCommand, error) {
	none := application.RecordActualMeasurementCommand{}
	tenant, node, err := intake.authenticateNode(ctx, request, measurementNotNode)
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
	tenant, node, err := intake.authenticateNode(ctx, request, measurementNotNode)
	if err != nil {
		return none, err
	}
	var document measurementInvalidationDocument
	if err := decodeClosedDocument(request, &document); err != nil {
		return none, err
	}
	// 作业端设备只能作废在认证出的这个节点上登记的测量；这条规则由编排判，这里只把节点交进命令。
	return invalidationCommand(tenant, application.InvalidationAtNode(node), document, "", "")
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
	var document manualMeasurementDocument
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
	return measurementCommand(operator.Tenant, node, document.measurementDocument, operator.Operator, document.OriginalDocument)
}

func (intake *OperatorDecisionIntake) IntakeManualMeasurementInvalidation(ctx context.Context, request *http.Request) (application.InvalidateActualMeasurementCommand, error) {
	none := application.InvalidateActualMeasurementCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionManualMeasurementEntry)
	if err != nil {
		return none, err
	}
	var document manualMeasurementInvalidationDocument
	if err := decodeClosedDocument(request, &document); err != nil {
		return none, err
	}
	if strings.TrimSpace(document.OriginalDocument) == "" {
		return none, fmt.Errorf("%w: originalDocument is required", ErrMalformedRequest)
	}
	// 手工作废是运营决定：持授予的操作者可以作废本租户任何节点的测量，所以交「不限节点」，不是漏传。
	return invalidationCommand(operator.Tenant, application.InvalidationAcrossTenantNodes(), document.measurementInvalidationDocument, operator.Operator, document.OriginalDocument)
}

// measurementDocument 是作业端一次称重量方的载荷。重量与长宽高都必填，没有「只报其中一项」的写法：缺了哪一格，
// 口上都拒成载荷不成形。复测是带新来源标识再报一条完整的。节点取认证结果、原件号只属于手工录入，所以这里没有这两格：
// 封闭解码把作业端载荷里的 node 与 originalDocument 当未知键拒掉，不是悄悄忽略。
type measurementDocument struct {
	SourceID      string `json:"sourceId"`
	ObjectKind    string `json:"objectKind"`
	ObjectID      string `json:"objectId"`
	Location      string `json:"location"`
	Basis         string `json:"basis"`
	PerformedBy   string `json:"performedBy"`
	EvidenceRef   string `json:"evidenceRef"`
	OccurredAt    string `json:"occurredAt"`
	WeightValue   string `json:"weightValue"`
	WeightUnit    string `json:"weightUnit"`
	Length        string `json:"length"`
	Width         string `json:"width"`
	Height        string `json:"height"`
	DimensionUnit string `json:"dimensionUnit"`
}

// manualMeasurementDocument 是手工录入的载荷：作业端载荷之上多两格，节点与原件号都由录入人写，不来自认证。
type manualMeasurementDocument struct {
	measurementDocument
	Node             string `json:"node"`
	OriginalDocument string `json:"originalDocument"`
}

// measurementInvalidationDocument 是作业端一次作废的载荷；没有 originalDocument，理由同 measurementDocument。
type measurementInvalidationDocument struct {
	SourceID       string `json:"sourceId"`
	TargetSourceID string `json:"targetSourceId"`
	Reason         string `json:"reason"`
	PerformedBy    string `json:"performedBy"`
	EvidenceRef    string `json:"evidenceRef"`
	OccurredAt     string `json:"occurredAt"`
}

// manualMeasurementInvalidationDocument 是手工作废的载荷：多一格原件号。
type manualMeasurementInvalidationDocument struct {
	measurementInvalidationDocument
	OriginalDocument string `json:"originalDocument"`
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
	scope application.MeasurementInvalidationScope,
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
		OccurredAt: occurredAt, EnteredBy: enteredBy, OriginalDocument: original, Scope: scope,
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

// measurementEndpoint 是四个测量口共用的骨架：只收 POST，译命令，交编排，答结果。作业端的两口在译命令失败时先问设备是否在用
// （deviceFace），手工的两口没有设备这一步。译命令与调编排都包成闭包、到请求时才取方法，装配时交进来的 intake 为空也只在
// 请求时才暴露。
func measurementEndpoint[Command, Result any](
	deviceFace bool,
	intake func(context.Context, *http.Request) (Command, error),
	handle func(context.Context, Command) (Result, error),
	answer func(Result) (outcome string, created bool),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake(request.Context(), request)
		if err != nil {
			if deviceFace && deviceNotInUse(response, err) {
				return
			}
			writeIntakeProblem(response, err)
			return
		}
		result, err := handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		outcome, created := answer(result)
		writeMeasurementOutcome(response, outcome, created)
	})
}

func answerMeasurementRecording(result application.RecordActualMeasurementResult) (string, bool) {
	return result.Outcome().String(), result.Outcome() == application.ActualMeasurementRecorded
}

func answerMeasurementInvalidation(result application.InvalidateActualMeasurementResult) (string, bool) {
	return result.Outcome().String(), result.Outcome() == application.MeasurementInvalidationRecorded
}

// NewRecordMeasurementEndpoint 交回作业端的测量登记口。
func NewRecordMeasurementEndpoint(intake MeasurementIntake, handler ActualMeasurementHandler) http.Handler {
	return measurementEndpoint(true,
		func(ctx context.Context, request *http.Request) (application.RecordActualMeasurementCommand, error) {
			return intake.IntakeMeasurement(ctx, request)
		},
		func(ctx context.Context, command application.RecordActualMeasurementCommand) (application.RecordActualMeasurementResult, error) {
			return handler.Handle(ctx, command)
		},
		answerMeasurementRecording)
}

// NewInvalidateMeasurementEndpoint 交回作业端的测量作废口。
func NewInvalidateMeasurementEndpoint(intake MeasurementIntake, handler ActualMeasurementHandler) http.Handler {
	return measurementEndpoint(true,
		func(ctx context.Context, request *http.Request) (application.InvalidateActualMeasurementCommand, error) {
			return intake.IntakeMeasurementInvalidation(ctx, request)
		},
		func(ctx context.Context, command application.InvalidateActualMeasurementCommand) (application.InvalidateActualMeasurementResult, error) {
			return handler.Invalidate(ctx, command)
		},
		answerMeasurementInvalidation)
}

// NewRecordManualMeasurementEndpoint 交回手工登记口。
func NewRecordManualMeasurementEndpoint(intake ManualMeasurementIntake, handler ActualMeasurementHandler) http.Handler {
	return measurementEndpoint(false,
		func(ctx context.Context, request *http.Request) (application.RecordActualMeasurementCommand, error) {
			return intake.IntakeManualMeasurement(ctx, request)
		},
		func(ctx context.Context, command application.RecordActualMeasurementCommand) (application.RecordActualMeasurementResult, error) {
			return handler.Handle(ctx, command)
		},
		answerMeasurementRecording)
}

// NewInvalidateManualMeasurementEndpoint 交回手工作废口。
func NewInvalidateManualMeasurementEndpoint(intake ManualMeasurementIntake, handler ActualMeasurementHandler) http.Handler {
	return measurementEndpoint(false,
		func(ctx context.Context, request *http.Request) (application.InvalidateActualMeasurementCommand, error) {
			return intake.IntakeManualMeasurementInvalidation(ctx, request)
		},
		func(ctx context.Context, command application.InvalidateActualMeasurementCommand) (application.InvalidateActualMeasurementResult, error) {
			return handler.Invalidate(ctx, command)
		},
		answerMeasurementInvalidation)
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
