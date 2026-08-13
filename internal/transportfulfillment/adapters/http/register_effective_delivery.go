// Package tfhttp 是 transport-fulfillment 的 HTTP 入站适配器，按 ADR-0022 把应用结果
// 映射成响应：状态码只回答服务端有没有形成答案，业务判别（未生效、冲突、未决……）
// 一律进响应体的 `outcome`。
//
// 包名与目录名不一致是刻意的。目录按 ADR-0018 的落位结论叫 `adapters/http`，而把包名
// 也叫 `http` 会遮住标准库。
package tfhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// ErrMalformedRequest 表示这次请求构造不出命令，且重发同样的内容不会改变结果。
//
// DeliveryIntake 的实现用它包裹调用方的错，本包据以回 4xx；不带它的失败一律是 5xx。
// 这条分法直接决定离线设备的动作——4xx 出队交给人，5xx 留在队里重发。
var ErrMalformedRequest = errors.New("transport fulfillment http: malformed request")

// DeliveryIntake 把已认证的接入请求翻译成首登/更正命令。
//
// 它是接口而不是本包内的解析代码：租户身份按 ADR-0003 只能来自认证结果，真实接入渠道
// 的认证方式属 PAR-INT-01 待提供；ADR-0029 要求越权探针一律以「未找到」作答，那也是
// 认证层的话。未决期间本包不带任何实现，包括「开发用」的采信头部版本。
//
// 与信封相反，**事实内容必须从请求体收**（ADR-0023）：对象、尝试、POD 证据引用与
// 更正时间都是设备/派送端记录的事实——服务器代铸任何一样，离线补传的重放就会被误判
// 成新事实。
type DeliveryIntake interface {
	IntakeRegistration(ctx context.Context, request *http.Request) (application.RegisterEffectiveDeliveryCommand, error)
	IntakeCorrection(ctx context.Context, request *http.Request) (application.CorrectDeliveryProofCommand, error)
}

// DeliveryHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type DeliveryHandler interface {
	Register(
		ctx context.Context,
		command application.RegisterEffectiveDeliveryCommand,
	) (application.RegisterEffectiveDeliveryResult, error)
	Correct(
		ctx context.Context,
		command application.CorrectDeliveryProofCommand,
	) (application.RegisterEffectiveDeliveryResult, error)
}

// 传输层错误码。它们不是业务原因目录：业务原因走 `outcome`，这里只说明为什么没有
// `outcome`。
const (
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeMalformedRequest = "MALFORMED_REQUEST"
	codeIntakeFailed     = "INTAKE_FAILED"
	codeNoAnswerFormed   = "NO_ANSWER_FORMED"
	codeUnnamedOutcome   = "UNNAMED_OUTCOME"
)

// NewRegisterEffectiveDeliveryEndpoint 交回交付生效首登的 HTTP 入口。
func NewRegisterEffectiveDeliveryEndpoint(intake DeliveryIntake, handler DeliveryHandler) http.Handler {
	return endpoint(func(request *http.Request) (application.RegisterEffectiveDeliveryResult, error, bool) {
		command, err := intake.IntakeRegistration(request.Context(), request)
		if err != nil {
			return application.RegisterEffectiveDeliveryResult{}, err, false
		}
		result, err := handler.Register(request.Context(), command)
		return result, err, true
	})
}

// NewCorrectDeliveryProofEndpoint 交回 POD 更正的 HTTP 入口。首登与更正是两个端点：
// 它们的命令形状与恢复动作不同，合并成一个入口就得靠请求体里的模式字段分路——那是
// 给「第三种模式」开的门。
func NewCorrectDeliveryProofEndpoint(intake DeliveryIntake, handler DeliveryHandler) http.Handler {
	return endpoint(func(request *http.Request) (application.RegisterEffectiveDeliveryResult, error, bool) {
		command, err := intake.IntakeCorrection(request.Context(), request)
		if err != nil {
			return application.RegisterEffectiveDeliveryResult{}, err, false
		}
		result, err := handler.Correct(request.Context(), command)
		return result, err, true
	})
}

// endpoint 收拢两个入口共同的传输层纪律：方法门、4xx/5xx 分法与结果映射。
func endpoint(
	invoke func(*http.Request) (application.RegisterEffectiveDeliveryResult, error, bool),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		result, err, intakeDone := invoke(request)
		if err != nil {
			if !intakeDone {
				if errors.Is(err, ErrMalformedRequest) {
					writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
					return
				}
				writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
				return
			}
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeOutcome(response, result)
	})
}

// deliveryResponse 是两个端点共用的封闭响应形状。`outcome` 取应用结果枚举的原名，
// 传输层不合并、不改名，也没有「其他」这一格。更正答案带 `corrects`——版本链透出
// 前版引用，调用方据此核对更正落在了哪一版上。
type deliveryResponse struct {
	Outcome               string `json:"outcome"`
	DeliveryVersion       string `json:"deliveryVersion,omitempty"`
	Object                string `json:"object,omitempty"`
	Attempt               string `json:"attempt,omitempty"`
	Proof                 string `json:"proof,omitempty"`
	Corrects              string `json:"corrects,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
}

// problemResponse 刻意不带自由文本消息。底层失败的措辞会捎带租户、对象或尝试的存在
// 性；调用方要分流靠稳定的 `code`，要细节去查带关联标识的服务端记录（ADR-0029 同源
// 纪律：错误不泄露他租户的存在）。
type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
}

func writeOutcome(response http.ResponseWriter, result application.RegisterEffectiveDeliveryResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		// 应用层交回了一个没有名字的结果：编程错误，不是业务答案——空字符串会被
		// 设备当成一种新的业务结果。
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := deliveryResponse{Outcome: outcome}
	if record, present := result.Record(); present {
		body.DeliveryVersion = record.Delivery.Version().String()
		body.Object = record.Delivery.Object().String()
		body.Attempt = record.Delivery.Attempt().String()
		body.Proof = record.Delivery.Proof().String()
		if predecessor, corrected := record.Delivery.Corrects(); corrected {
			body.Corrects = predecessor.String()
		}
	}
	if continuation := result.ContinuationReference(); continuation != "" {
		body.ContinuationReference = continuation
	}

	// ADR-0022：状态码只报有没有新落一版。首登与更正都持久化了新版本用 201；
	// 未生效（NOT_EFFECTIVE）、冲突、已有结果、未决、未受理都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.DeliveryRegistered ||
		result.Outcome() == application.DeliveryCorrected {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
