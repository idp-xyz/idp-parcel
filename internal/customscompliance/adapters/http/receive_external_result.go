// Package customshttp 是 customs-compliance 的 HTTP 入站适配器，按 ADR-0022 把应用
// 结果映射成响应：状态码只回答服务端有没有形成答案，业务判别一律进响应体的 `outcome`。
//
// 包名与目录名不一致与 shipmenthttp 同理：目录按 ADR-0018 叫 `adapters/http`，包名
// 叫 `http` 会遮住标准库。
package customshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// ErrMalformedRequest 表示这次请求构造不出命令，且重发同样的内容不会改变结果。
// 4xx/5xx 的分法决定通道侧的动作——4xx 出队交给人，5xx 留在队里重发。
var ErrMalformedRequest = errors.New("customs compliance http: malformed request")

// ResultIntake 把一次已认证的通道请求翻译成一条外部结果接收命令。它是接口而非解析
// 代码：租户归属只能来自通道认证结果（监管回执经集成客户端族进来，ADR-0149；哪家来源送回执是租户取值
// `PAR-INT-03`；采信报文自称的租户号会穿透 ADR-0003 的隔离边界）；来源标识、层、原文语义与
// 发生时间照 ADR-0023 的同一条纪律从报文体收——服务端不代铸外部事实的身份与时间。
// 真渠道是集成客户端族的 IntegrationClientIntake（ADR-0149；票 operator-channel/11）：租户取册上的绑定，
// 不采信载荷自报。UnconfiguredIntake 仍在，供发行方参数未设时整口答未配置，以及还没换渠道的其他口。
type ResultIntake interface {
	IntakeResult(ctx context.Context, request *http.Request) (application.ReceiveExternalResultCommand, error)
}

// ResultHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type ResultHandler interface {
	Handle(
		ctx context.Context,
		command application.ReceiveExternalResultCommand,
	) (application.ReceiveExternalResultResult, error)
}

const (
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeMalformedRequest = "MALFORMED_REQUEST"
	codeIntakeFailed     = "INTAKE_FAILED"
	codeNoAnswerFormed   = "NO_ANSWER_FORMED"
	codeUnnamedOutcome   = "UNNAMED_OUTCOME"
	// codeHandoffEnvelopeRejected：记录能落、交发布意图时信封被框架确定性拒收（来源标识长到把分区键顶过上限之类）。
	// 与 MALFORMED_REQUEST 同一句判据——重发同样的内容不会改变结果——所以同是 400 出队交给人，不另开 422：
	// ADR-0022 否决的正是「按语义贴近码」那条逐例裁量口，差别只进 code（票 sa-cc/34 裁决 4）。
	codeHandoffEnvelopeRejected = "HANDOFF_ENVELOPE_REJECTED"
)

// NewReceiveExternalResultEndpoint 交回 `UC-CC-006` 外部结果接收编排的 HTTP 入口。
func NewReceiveExternalResultEndpoint(intake ResultIntake, handler ResultHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeResult(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}

		result, err := handler.Handle(request.Context(), command)
		if err != nil {
			// 编排返 error 就是没形成答案；只有信封被确定性拒收这一格是调用方的错，留队重发只会把同一份再拒一次。
			if errors.Is(err, application.ErrExternalResultHandoffRejected) {
				writeProblem(response, http.StatusBadRequest, codeHandoffEnvelopeRejected)
				return
			}
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeResultOutcome(response, result)
	})
}

// resultResponse 是本端点的封闭响应形状。`outcome` 取应用结果枚举的原名；归属不上
// 与同层冲突都是已入册的答案（留存不猜/留存双方），通道方要知道的是记录成立与否和
// 续办引用，不是被翻译过的监管语义。
type resultResponse struct {
	Outcome               string `json:"outcome"`
	UndecidedReason       string `json:"undecidedReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
	HandoffReference      string `json:"handoffReference,omitempty"`
	// SourceID 只在记录在场时交回：人工录入的标识由服务端加了前缀，录入的人要知道库里记的是哪一个。
	SourceID string `json:"sourceId,omitempty"`
}

// OperatorDecisionCustomsExternalResultEntry 是关务人员照海关回执人工录入一条外部监管结果（dox/decisions.md「产品形态」）。
const OperatorDecisionCustomsExternalResultEntry OperatorDecision = "CUSTOMS_EXTERNAL_RESULT_ENTRY"

// ManualSourcePrefix 是人工录入的外部结果在来源标识上的前缀。要更正一条人工录入的结果，correctsSourceId 填带前缀的
// 那一个；更正接口送来的结果，照填接口的标识。
const ManualSourcePrefix = "MANUAL/"

// manualExternalResultDocument 是手工录入口的线格式：与集成客户端口同一份形状，另加 original——所凭原件的编号或存档
// 引用，必填。
type manualExternalResultDocument struct {
	externalResultDocument
	Original string `json:"original"`
}

// manualResultIntake 让操作员渠道的运营决定口也能交出接收命令：按人工录入这一种决定认证，载荷的读法与集成客户端口是
// 同一份，录入人记成 `OPERATOR/<发行方>#<sub>`、原件引用随命令走。接收时刻由服务端给。
type manualResultIntake struct {
	intake *OperatorDecisionIntake
	now    func() time.Time
}

// ManualResultIntake 交回手工录入口的 Intake。
func ManualResultIntake(intake *OperatorDecisionIntake, now func() time.Time) ResultIntake {
	return manualResultIntake{intake: intake, now: now}
}

func (manual manualResultIntake) IntakeResult(ctx context.Context, request *http.Request) (application.ReceiveExternalResultCommand, error) {
	none := application.ReceiveExternalResultCommand{}
	operator, err := manual.intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionCustomsExternalResultEntry)
	if err != nil {
		return none, err
	}
	var document manualExternalResultDocument
	if err := decodeClosedPayload(request.Body, &document); err != nil {
		return none, err
	}
	if strings.TrimSpace(document.Original) == "" {
		return none, fmt.Errorf("%w: original is required for a manually entered result", ErrMalformedRequest)
	}
	command, err := document.command(operator.Tenant, manual.now().UTC())
	if err != nil {
		return none, err
	}
	// 人工录入的来源标识自成一个名字空间：操作员多半拿回执号当标识，而回执号也是连接器最自然的来源标识。两边共用一个
	// 空间时，人先录了一条、接口随后送来同号而内容略有出入的那一条，会被答成来源冲突而整条丢掉——库里留下的是人抄的
	// 那一份，没有任何人看得见。分开之后两条各自成行，由同层判法去比（后续事件、更正或显式的同层冲突）。
	command.SourceID = ManualSourcePrefix + strings.TrimSpace(command.SourceID)
	command.EnteredBy = "OPERATOR/" + operator.Operator
	command.Original = strings.TrimSpace(document.Original)
	return command, nil
}

func writeResultOutcome(response http.ResponseWriter, result application.ReceiveExternalResultResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := resultResponse{Outcome: outcome}
	if reason := result.UndecidedReason().String(); reason != "" {
		body.UndecidedReason = reason
	}
	if continuation := result.ContinuationReference(); continuation != "" {
		body.ContinuationReference = continuation
	}
	if handoff := result.ResultHandoffReference(); handoff != "" {
		body.HandoffReference = handoff
	}
	if record, has := result.Record(); has {
		body.SourceID = record.Key.SourceID
	}

	status := http.StatusOK
	if result.Outcome() == application.ResultRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
	// Detail 只在目录查询参数被拒时在场（writePageQueryProblem），是给操作者看的理由散文，调用方不据此分支。
	Detail string `json:"detail,omitempty"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
