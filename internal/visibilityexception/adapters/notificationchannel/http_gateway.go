// Package notificationchannel 是客户异常通知渠道的连接器：产品随附的那一种形态，把通知以 JSON POST 到租户登记的客户
// 系统端点。线格式是本产品定的公开形状（见 HTTPGateway 头注）；短信、邮件或某家消息平台的私有格式要另一种连接器，不在
// 这里猜。
package notificationchannel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// sendTimeout 是一次推送等答复的上限。
const sendTimeout = 15 * time.Second

// Subject 是这份通知说的是哪件包裹上的哪一类信号。通知对象自己只带发作期标识，客户系统认的是包裹。
type Subject struct {
	Parcel domain.TrackedParcelReference
	Kind   domain.ExceptionSignalKindReference
}

// HTTPGateway 实现 ports.NotificationChannelGateway。租户与通知的主题在构造时钉住（渠道口的签名上没有它们），所以它
// 逐请求构造。线格式：
//   - 请求：`POST <端点>`，`Content-Type: application/json`，`Idempotency-Key: <租户>/<发作期>/<客户>/<披露决定时间>`，
//     登了凭据时另带 `Authorization: Bearer <凭据变量的值>`；正文见 notificationBody。content 是披露内容快照的引用，
//     不是给客户看的文案——文案由客户系统按引用取自己那一侧的模板。
//   - 答复：任一 2xx 即渠道已收下；其余状态码、连不上、超时都算这一次没提交成。
//
// 幂等键取披露身份而不取通知标识：推送成了、随后落库没成的那一笔会整笔回滚，重发时通知标识是新签的，披露身份不变，
// 客户系统凭它认出是同一份。
type HTTPGateway struct {
	directory ports.NotificationChannelDirectory
	tenant    domain.TenantID
	subject   Subject
	client    *http.Client
	getenv    func(string) string
}

var _ ports.NotificationChannelGateway = (*HTTPGateway)(nil)

// NewHTTPGateway 用给定的 client（nil 取带超时的默认值）与环境读取函数（nil 取 os.Getenv）。不跟随重定向：第一跳已经
// 收到了整份通知，跟过去会把它送到渠道登记之外、未必是 https 的地址。
func NewHTTPGateway(
	directory ports.NotificationChannelDirectory,
	tenant domain.TenantID,
	subject Subject,
	client *http.Client,
	getenv func(string) string,
) (*HTTPGateway, error) {
	if directory == nil || tenant.String() == "" || subject.Parcel.String() == "" || subject.Kind.String() == "" {
		return nil, errors.New("notification channel: directory, tenant and subject are required")
	}
	if client == nil {
		client = &http.Client{Timeout: sendTimeout}
	}
	own := *client
	own.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if getenv == nil {
		getenv = os.Getenv
	}
	return &HTTPGateway{directory: directory, tenant: tenant, subject: subject, client: &own, getenv: getenv}, nil
}

type notificationBody struct {
	Tenant         string `json:"tenant"`
	NotificationID string `json:"notificationId"`
	Customer       string `json:"customer"`
	Parcel         string `json:"parcel"`
	SignalKind     string `json:"signalKind"`
	EpisodeID      string `json:"episodeId"`
	Content        string `json:"content"`
	Deadline       string `json:"deadline"`
}

func (gateway *HTTPGateway) SubmitToChannel(ctx context.Context, notification *domain.CustomerNotification) error {
	if notification == nil {
		return errors.New("notification channel: notification is nil")
	}
	channel, registered, err := gateway.directory.FindChannel(ctx, gateway.tenant, notification.Channel())
	if err != nil {
		return fmt.Errorf("notification channel: %w", err)
	}
	if !registered {
		return fmt.Errorf("%w: %s", ports.ErrNotificationChannelNotConfigured, notification.Channel().String())
	}
	var token string
	if name := channel.Credential(); name != "" {
		if token = gateway.getenv(name); token == "" {
			return fmt.Errorf("%w: %s", ports.ErrNotificationChannelCredentialMissing, name)
		}
	}
	disclosure := notification.Disclosure()
	body, err := json.Marshal(notificationBody{
		Tenant:         gateway.tenant.String(),
		NotificationID: notification.ID().String(),
		Customer:       notification.Customer().String(),
		Parcel:         gateway.subject.Parcel.String(),
		SignalKind:     gateway.subject.Kind.String(),
		EpisodeID:      disclosure.Episode().String(),
		Content:        notification.Content().String(),
		Deadline:       notification.Deadline().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("notification channel: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, channel.Endpoint(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notification channel: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", gateway.tenant.String()+"/"+disclosure.Episode().String()+"/"+
		disclosure.Customer().String()+"/"+disclosure.DecidedAt().UTC().Format(time.RFC3339Nano))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := gateway.client.Do(request)
	if err != nil {
		return fmt.Errorf("notification channel: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("notification channel: HTTP %d", response.StatusCode)
	}
	return nil
}
