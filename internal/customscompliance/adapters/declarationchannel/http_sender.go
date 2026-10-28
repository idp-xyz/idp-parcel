// Package declarationchannel 是申报发送通道的连接器（ADR-0342 决定三）：产品随附的那一种形态，把申报包以 JSON POST 到通道登记的
// 端点，读同步答复。通道的线格式是本产品定的公开形状（见 HTTPSender 头注）；某个报关服务商或单一窗口的私有格式要另一种连接器，
// 不在这里猜。
package declarationchannel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// sendTimeout 是一次发送等同步答复的上限。超了按「发出去了、没拿到答复」记待确认，不当失败。
const sendTimeout = 15 * time.Second

// maxAnswerBytes 是读同步答复的上限：答复只有几个键，读更多只会让坏掉的对端把内存吃满。
const maxAnswerBytes = 64 << 10

// HTTPSender 实现 application.DeclarationSender。线格式：
//   - 请求：`POST <端点>`，`Content-Type: application/json`，`Idempotency-Key: <租户>/<版本>/<尝试序号>`，登了凭据时另带
//     `Authorization: Bearer <凭据变量的值>`；正文是申报包（declarationBody）。
//   - 答复：2xx 或 4xx，正文 `{"outcome":"ACCEPTED"|"REJECTED","messageId":"…"}`。ACCEPTED 记已回执，REJECTED 记失败；
//     messageId 是通道给这次发送的消息号，异步技术回执凭它关联回来。别的状态码或读不懂的正文，记没拿到答复。
type HTTPSender struct {
	client *http.Client
	getenv func(string) string
}

var _ application.DeclarationSender = (*HTTPSender)(nil)

// NewHTTPSender 用给定的 client（nil 取带超时的默认值）与环境读取函数（nil 取 os.Getenv）。
func NewHTTPSender(client *http.Client, getenv func(string) string) *HTTPSender {
	if client == nil {
		client = &http.Client{Timeout: sendTimeout}
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	return &HTTPSender{client: client, getenv: getenv}
}

type declarationBody struct {
	Tenant    string   `json:"tenant"`
	Version   string   `json:"version"`
	Sequence  int      `json:"sequence"`
	Unit      string   `json:"unit"`
	Procedure string   `json:"procedure"`
	Members   []string `json:"members"`
	Dossier   string   `json:"dossier"`
	Roles     string   `json:"roles"`
	Target    string   `json:"target"`
}

type answerBody struct {
	Outcome   string `json:"outcome"`
	MessageID string `json:"messageId"`
}

func (sender *HTTPSender) SendDeclaration(
	ctx context.Context,
	channel domain.DeclarationChannel,
	pkg application.DeclarationPackage,
) (application.ChannelAnswer, error) {
	var token string
	if name := channel.Credential(); name != "" {
		if token = sender.getenv(name); token == "" {
			return application.ChannelAnswer{}, fmt.Errorf("%w: %s", application.ErrDeclarationCredentialMissing, name)
		}
	}
	body, err := json.Marshal(declarationBody{
		Tenant: pkg.Tenant, Version: pkg.Version, Sequence: pkg.Sequence, Unit: pkg.Unit, Procedure: pkg.Procedure,
		Members: pkg.Members, Dossier: pkg.Dossier, Roles: pkg.Roles, Target: pkg.Target,
	})
	if err != nil {
		return application.ChannelAnswer{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, channel.Endpoint(), bytes.NewReader(body))
	if err != nil {
		return application.ChannelAnswer{}, fmt.Errorf("%w: %v", application.ErrDeclarationNotSent, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", pkg.IdempotencyKey)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := sender.client.Do(request)
	if err != nil {
		if dialFailed(err) {
			return application.ChannelAnswer{}, fmt.Errorf("%w: %v", application.ErrDeclarationNotSent, err)
		}
		// 拨通之后断的、等答复超时的：请求可能已经到了对端，记待确认。
		return application.ChannelAnswer{Response: err.Error()}, nil
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes))
	status := fmt.Sprintf("HTTP %d", response.StatusCode)
	if response.StatusCode < 200 || response.StatusCode >= 500 || (response.StatusCode >= 300 && response.StatusCode < 400) {
		return application.ChannelAnswer{Response: status}, nil
	}
	var answer answerBody
	if err := json.Unmarshal(raw, &answer); err != nil {
		return application.ChannelAnswer{Response: status + " unreadable answer"}, nil
	}
	switch answer.Outcome {
	case "ACCEPTED":
		return application.ChannelAnswer{Answered: true, Accepted: true, Message: answer.MessageID, Response: status}, nil
	case "REJECTED":
		return application.ChannelAnswer{Answered: true, Message: answer.MessageID, Response: status}, nil
	default:
		return application.ChannelAnswer{Response: status + " unknown outcome"}, nil
	}
}

// dialFailed 认「连都没连上」：这时请求确知没离开本进程。
func dialFailed(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}
