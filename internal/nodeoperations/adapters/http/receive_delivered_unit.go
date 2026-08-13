// Package nodeopshttp 是 node-operations 的 HTTP 入站适配器，按 ADR-0022 把应用结果
// 映射成响应：状态码只回答服务端有没有形成答案，业务判别（待识别、未形成、未决、
// 已有结果、来源冲突）一律进响应体的 `outcome`。
//
// 包名与目录名不一致是刻意的。目录按 ADR-0018 的落位结论叫 `adapters/http`，而把包名
// 也叫 `http` 会遮住标准库。
package nodeopshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

// ErrMalformedRequest 表示这次请求构造不出命令，且重发同样的内容不会改变结果。
//
// ReceptionIntake 的实现用它包裹调用方的错，本包据以回 4xx；不带它的失败一律是 5xx。
// 这条分法直接决定离线设备的动作——4xx 出队交给人，5xx 留在队里重发。
var ErrMalformedRequest = errors.New("node operations http: malformed request")

// ReceptionIntake 把一次已认证的接入请求翻译成一条收寄命令。
//
// 它是接口而不是本包内的解析代码：租户与节点身份按 ADR-0003 只能来自认证结果（采信
// 自报租户号会穿透最高数据隔离边界），真实接入渠道的认证方式属 PAR-INT-01 待提供；
// ADR-0029 要求越权探针一律以「未找到」作答，那也是认证层的话。三项未决期间本包不带
// 任何实现，包括「开发用」的采信头部版本。
//
// 与信封相反，**事实身份与发生时间必须从请求体收**（ADR-0023）：SourceID 是设备签发
// 的幂等身份、OccurredAt 是设备记录的业务时间——服务器代铸任何一个，离线补传的重放
// 就会被误判成新事实。实现不得用服务端时钟或随机数顶替这两样。
type ReceptionIntake interface {
	IntakeReception(ctx context.Context, request *http.Request) (application.ReceiveDeliveredUnitCommand, error)
}

// ReceptionHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type ReceptionHandler interface {
	Handle(
		ctx context.Context,
		command application.ReceiveDeliveredUnitCommand,
	) (application.ReceiveDeliveredUnitResult, error)
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

// NewReceiveDeliveredUnitEndpoint 交回 `UC-NO-002` 收寄编排的 HTTP 入口。
func NewReceiveDeliveredUnitEndpoint(intake ReceptionIntake, handler ReceptionHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeReception(request.Context(), request)
		if err != nil {
			if errors.Is(err, ErrMalformedRequest) {
				writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
				return
			}
			writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
			return
		}

		result, err := handler.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeOutcome(response, result)
	})
}

// receptionResponse 是本端点的封闭响应形状。`outcome` 取应用结果枚举的原名，传输层
// 不合并、不改名，也没有「其他」这一格。待识别带候选、未形成带拒收原因、未决带续办
// 引用——少了它们调用方只知道被挡，不知道下一步找身份、找交付方还是等依赖。
type receptionResponse struct {
	Outcome               string   `json:"outcome"`
	IntakeVersion         string   `json:"intakeVersion,omitempty"`
	Unit                  string   `json:"unit,omitempty"`
	Node                  string   `json:"node,omitempty"`
	Association           string   `json:"association,omitempty"`
	ControlBasis          string   `json:"controlBasis,omitempty"`
	Candidates            []string `json:"candidates,omitempty"`
	RefusalReason         string   `json:"refusalReason,omitempty"`
	ServiceMarkers        []string `json:"serviceMarkers,omitempty"`
	ContinuationReference string   `json:"continuationReference,omitempty"`
}

// problemResponse 刻意不带自由文本消息。底层失败的措辞会捎带租户、节点或实物的存在
// 性；调用方要分流靠稳定的 `code`，要细节去查带关联标识的服务端记录（ADR-0029 同源
// 纪律：错误不泄露他租户的存在）。
type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
}

func writeOutcome(response http.ResponseWriter, result application.ReceiveDeliveredUnitResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		// 应用层交回了一个没有名字的结果：编程错误，不是业务答案，不能带 `outcome`
		// 上线——空字符串会被设备当成一种新的业务结果。
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := receptionResponse{Outcome: outcome}
	if record, present := result.Record(); present {
		if record.Intake.Version().String() != "" {
			body.IntakeVersion = record.Intake.Version().String()
			body.Unit = record.Intake.Unit().String()
			body.Node = record.Intake.Node().String()
			if association, associated := record.Intake.Association(); associated {
				body.Association = association.String()
			}
		}
		if record.Control.Active() {
			body.ControlBasis = record.Control.Basis().String()
		}
		for _, candidate := range record.Candidates {
			body.Candidates = append(body.Candidates, candidate.String())
		}
		body.RefusalReason = record.RefusalReason
		body.ServiceMarkers = append(body.ServiceMarkers, record.ServiceMarkers...)
	}
	if continuation := result.ContinuationReference(); continuation != "" {
		body.ContinuationReference = continuation
	}

	// ADR-0022：状态码只报有没有形成答案。待识别、未形成、未决、已有结果与来源冲突
	// 都是形成了的答案（2xx 带结果体）；只有首次收寄成立用 201 区分「新事实已登记」。
	status := http.StatusOK
	if result.Outcome() == application.NodeIntakeFormed {
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
