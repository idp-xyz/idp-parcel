package customshttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 技术回执接收口（ADR-0342 决定四）：报关服务商经集成客户端渠道送来对一次发送的异步技术回执。认证复用「外部结果」那一种授予——
// 送回执的就是送外部结果的那一方，两份都是它对同一份申报的答复；回执不进外部结果册（ADR-0242 决定一）。

// TechnicalReceiptIntake 是本口的 Intake。
type TechnicalReceiptIntake interface {
	IntakeTechnicalReceipt(ctx context.Context, request *http.Request) (application.ReceiveTechnicalReceiptCommand, error)
}

// TechnicalReceiptHandler 是本口转交的编排。
type TechnicalReceiptHandler interface {
	Handle(ctx context.Context, command application.ReceiveTechnicalReceiptCommand) (application.TechnicalReceiptResult, error)
}

var _ TechnicalReceiptIntake = (*IntegrationClientIntake)(nil)

// technicalReceiptDocument：sourceId 是通道给这份回执的来源身份，messageId 是它给原发送的消息号，outcome 只收 ACKNOWLEDGED 与
// FAILED，occurredAt 是通道写下这份回执的时刻。
type technicalReceiptDocument struct {
	SourceID   string `json:"sourceId"`
	MessageID  string `json:"messageId"`
	Outcome    string `json:"outcome"`
	OccurredAt string `json:"occurredAt"`
}

func (intake *IntegrationClientIntake) IntakeTechnicalReceipt(
	ctx context.Context,
	request *http.Request,
) (application.ReceiveTechnicalReceiptCommand, error) {
	none := application.ReceiveTechnicalReceiptCommand{}
	tenant, err := intake.authenticator.Authenticate(ctx, presentedClientCredential(request), FactCustomsExternalResult)
	if err != nil {
		return none, err
	}
	var document technicalReceiptDocument
	if err := decodeClosedDocument(request.Body, &document); err != nil {
		return none, err
	}
	var outcome domain.AttemptResult
	switch document.Outcome {
	case "ACKNOWLEDGED":
		outcome = domain.AttemptAcknowledged
	case "FAILED":
		outcome = domain.AttemptFailed
	default:
		return none, fmt.Errorf("%w: outcome must be ACKNOWLEDGED or FAILED", ErrMalformedRequest)
	}
	occurredAt, err := time.Parse(time.RFC3339, strings.TrimSpace(document.OccurredAt))
	if err != nil {
		return none, fmt.Errorf("%w: occurredAt is not an RFC 3339 instant: %v", ErrMalformedRequest, err)
	}
	// 内容摘要只取回执本身的几格：同一来源身份再到，比的是它说的话变没变，不比到达时刻。
	sum := sha256.Sum256([]byte(strings.Join([]string{document.MessageID, document.Outcome, occurredAt.UTC().Format(time.RFC3339Nano)}, "\x1f")))
	return application.ReceiveTechnicalReceiptCommand{
		TenantID:       tenant,
		Source:         document.SourceID,
		ChannelMessage: document.MessageID,
		Outcome:        outcome,
		OccurredAt:     occurredAt,
		ReceivedAt:     intake.clock.Now().UTC(),
		Digest:         "TR-1:" + hex.EncodeToString(sum[:]),
	}, nil
}

// NewReceiveTechnicalReceiptEndpoint 交回技术回执接收口。
func NewReceiveTechnicalReceiptEndpoint(intake TechnicalReceiptIntake, handler TechnicalReceiptHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeTechnicalReceipt(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := handler.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeTechnicalReceiptOutcome(response, result)
	})
}

type technicalReceiptResponse struct {
	Outcome  string `json:"outcome"`
	Version  string `json:"version,omitempty"`
	Sequence int    `json:"sequence,omitempty"`
	Current  string `json:"current,omitempty"`
}

// writeTechnicalReceiptOutcome：记下答 201；已有、冲突、待关联、未受理都是答案（200）；读写不动是没形成答案。答复带回执关联到的
// 尝试与它此刻的技术判断。
func writeTechnicalReceiptOutcome(response http.ResponseWriter, result application.TechnicalReceiptResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.TechnicalReceiptUndecided {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	version, sequence := result.Attempt()
	body := technicalReceiptResponse{Outcome: name, Version: version.String(), Sequence: sequence, Current: result.Current().String()}
	status := http.StatusOK
	if result.Outcome() == application.TechnicalReceiptRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
