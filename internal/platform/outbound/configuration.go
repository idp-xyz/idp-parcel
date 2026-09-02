package outbound

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrOutboundNotConfigured 是构造期的拒绝，与 NotConfigured 那一格是两件事：这里拒的是
// 「装配写错了」，那一格答的是「渠道还没配」。压成一件会让一次装配错误在生产上表现为
// 一句诚实的「未配置」，而现场看不出那是配置没给还是代码给错了。
var ErrOutboundNotConfigured = errors.New("outbound: channel configuration is incomplete")

// CallTimeout 是一次出向调用的超时上限。
//
// **它没有默认值，零值也不可用。** ADR-0090 决定五要求的是「未给出超时即不得发起调用」，
// 不是「未给出就用某个数」。默认值在这里格外坏：一个猜出来的超时会直接改变一次调用落在
// 哪一格——太短会把本该 Accepted 的慢应答拖成 AnswerUndetermined，太长会把本该
// ProvenNotAccepted 的快速失败拖成 AnswerUndetermined，而那一格恰恰是不得重发的那一格。
//
// 真实取值属实例半边（`PAR-INT-02`），等真实渠道接入时由装配交入。
type CallTimeout struct {
	duration time.Duration
}

func NewCallTimeout(duration time.Duration) (CallTimeout, error) {
	if duration <= 0 {
		return CallTimeout{}, fmt.Errorf("%w: call timeout must be given and positive", ErrOutboundNotConfigured)
	}
	return CallTimeout{duration: duration}, nil
}

func (timeout CallTimeout) Configured() bool {
	return timeout.duration > 0
}

func (timeout CallTimeout) Duration() time.Duration {
	return timeout.duration
}

// ChannelConfiguration 是发起一次出向调用之前必须齐备的三样：去哪儿、拿哪份凭证、等多久。
//
// 三样都只定**位置**与**上限**。凭证这一项存的是「到哪里去取」而不是凭证本身，渠道的账号、
// 密钥与字段名一个都不在这里（ADR-0088 决定五）。
//
// 零值即未配置。生产装配在真渠道就位之前交入零值，适配器据此答 NotConfigured 并如实拒绝
// 发起调用，而不是静默失败或留一个空实现——未配置格沿用 ADR-0052／0054／0055 的既有形状，
// 本包不另立形。
//
// **它保证不了的那一格要说清**：本包不发起调用，因此拦不住一个绕过本类型、把地址写死在
// 自己包里的适配器。它能保证的只有一条——凡是收本类型作入参的适配器，配置不齐时手上没有
// 任何地址可去。评审出向适配器时先看它走不走这条入参，这一层守不进结构，只能靠看。
type ChannelConfiguration struct {
	endpoint   string
	credential string
	timeout    CallTimeout
}

// NewChannelConfiguration 三样缺一即拒。三样分开收而不是收一个配置结构体，是为了让漏给
// 其中一样在调用点就少一个实参，而不是在运行期表现为一个字段恰好为零。
func NewChannelConfiguration(endpointReference, credentialReference string, timeout CallTimeout) (ChannelConfiguration, error) {
	if strings.TrimSpace(endpointReference) == "" {
		return ChannelConfiguration{}, fmt.Errorf("%w: endpoint reference", ErrOutboundNotConfigured)
	}
	if strings.TrimSpace(credentialReference) == "" {
		return ChannelConfiguration{}, fmt.Errorf("%w: credential reference", ErrOutboundNotConfigured)
	}
	if !timeout.Configured() {
		return ChannelConfiguration{}, fmt.Errorf("%w: call timeout", ErrOutboundNotConfigured)
	}
	return ChannelConfiguration{
		endpoint:   endpointReference,
		credential: credentialReference,
		timeout:    timeout,
	}, nil
}

// Configured 是适配器发起调用之前该问的那一句。答 false 时唯一允许的下一步是交回
// NotConfigured。
func (configuration ChannelConfiguration) Configured() bool {
	return strings.TrimSpace(configuration.endpoint) != "" &&
		strings.TrimSpace(configuration.credential) != "" &&
		configuration.timeout.Configured()
}

func (configuration ChannelConfiguration) EndpointReference() string {
	return configuration.endpoint
}

func (configuration ChannelConfiguration) CredentialReference() string {
	return configuration.credential
}

func (configuration ChannelConfiguration) Timeout() CallTimeout {
	return configuration.timeout
}
