package domain

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

var ErrInvalidNotificationChannel = errors.New("invalid notification channel")

// NotificationChannelEndpoint 是一个通知渠道引用对到的推送端点：通知策略只指名渠道引用，往哪里推、带哪个凭据由租户
// 另行登记。凭据只记它在运行环境里的变量名。
type NotificationChannelEndpoint struct {
	channel    NotificationChannelReference
	endpoint   *url.URL
	credential string
}

// NewNotificationChannelEndpoint 只收绝对的 https 端点；回环地址上的 http 也收，给本机的合成客户系统用——别处用明文推
// 就是把客户的异常通知交给链路上的任何一方。
func NewNotificationChannelEndpoint(channel NotificationChannelReference, endpoint, credential string) (NotificationChannelEndpoint, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if !channel.valid() || err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return NotificationChannelEndpoint{}, fmt.Errorf("%w: channel or endpoint", ErrInvalidNotificationChannel)
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if ip := net.ParseIP(parsed.Hostname()); parsed.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return NotificationChannelEndpoint{}, fmt.Errorf("%w: plain http is only for a loopback endpoint", ErrInvalidNotificationChannel)
		}
	default:
		return NotificationChannelEndpoint{}, fmt.Errorf("%w: scheme %q", ErrInvalidNotificationChannel, parsed.Scheme)
	}
	return NotificationChannelEndpoint{channel: channel, endpoint: parsed, credential: strings.TrimSpace(credential)}, nil
}

func (channel NotificationChannelEndpoint) Channel() NotificationChannelReference {
	return channel.channel
}

func (channel NotificationChannelEndpoint) Endpoint() string { return channel.endpoint.String() }

// Credential 是凭据在运行环境里的变量名；空串表示这个渠道不带凭据。
func (channel NotificationChannelEndpoint) Credential() string { return channel.credential }
