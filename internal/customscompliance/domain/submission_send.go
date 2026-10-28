package domain

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// 申报发送通道、发送事实与技术回执（ADR-0342）。提交尝试行登记之后不改：越过发送边界另记一笔发送事实，后到的技术回执逐份
// 追加，尝试此刻的技术判断由这几份折出（ADR-0242 决定三「不改写」）。

var (
	ErrInvalidDeclarationChannel = errors.New("customs compliance: invalid declaration channel")
	ErrInvalidSendFact           = errors.New("customs compliance: invalid submission send fact")
	ErrInvalidTechnicalReceipt   = errors.New("customs compliance: invalid technical receipt")
)

// DeclarationChannel 是一个发送目标对应的申报通道：把申报包发到哪个端点、凭哪个凭据。端点与凭据是租户取值，产品不预填；
// 凭据只登引用（运行环境里的变量名），密钥本身不进库。
type DeclarationChannel struct {
	target     string
	endpoint   *url.URL
	credential string
}

// NewDeclarationChannel 只收绝对的 https 端点；回环地址上的 http 也收，给本机的合成报关服务商用——别处用明文发申报包就是把
// 申报资料交给链路上的任何一方。
func NewDeclarationChannel(target, endpoint, credential string) (DeclarationChannel, error) {
	target, credential = strings.TrimSpace(target), strings.TrimSpace(credential)
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if target == "" || err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return DeclarationChannel{}, fmt.Errorf("%w: target or endpoint", ErrInvalidDeclarationChannel)
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if ip := net.ParseIP(parsed.Hostname()); parsed.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return DeclarationChannel{}, fmt.Errorf("%w: plain http is only for a loopback endpoint", ErrInvalidDeclarationChannel)
		}
	default:
		return DeclarationChannel{}, fmt.Errorf("%w: scheme %q", ErrInvalidDeclarationChannel, parsed.Scheme)
	}
	return DeclarationChannel{target: target, endpoint: parsed, credential: credential}, nil
}

func (channel DeclarationChannel) Target() string { return channel.target }

func (channel DeclarationChannel) Endpoint() string { return channel.endpoint.String() }

// Credential 是凭据在运行环境里的变量名；空串表示这个通道不带凭据。
func (channel DeclarationChannel) Credential() string { return channel.credential }

// SameChannel 判两份登记是不是同一份内容。
func (channel DeclarationChannel) SameChannel(other DeclarationChannel) bool {
	return channel.target == other.target && channel.Endpoint() == other.Endpoint() && channel.credential == other.credential
}

// SubmissionSendFact 是一次提交尝试越过发送边界的那一笔：何时发出、通道同步答了什么、通道给的消息号。一份尝试至多一笔。
// 没拿到同步答复（超时、连接断在发出之后）是待确认，不是失败——可能已经发出去了（CC CONTEXT「提交尝试」）。
type SubmissionSendFact struct {
	version        SubmissionVersionID
	sequence       int
	sentAt         time.Time
	outcome        AttemptResult
	channelMessage string
	response       string
}

// RecordSubmissionSend 立一笔发送事实。结果只收越过边界之后的三种；通道没给消息号就留空，异步回执那时就关联不上，答待关联。
func RecordSubmissionSend(
	version SubmissionVersionID,
	sequence int,
	sentAt time.Time,
	outcome AttemptResult,
	channelMessage string,
	response string,
) (SubmissionSendFact, error) {
	if !version.valid() || sequence < 1 || sentAt.IsZero() ||
		!outcome.valid() || outcome == AttemptRegisteredPendingSend || strings.IndexByte(channelMessage, 0) >= 0 {
		return SubmissionSendFact{}, ErrInvalidSendFact
	}
	return SubmissionSendFact{
		version: version, sequence: sequence, sentAt: sentAt.UTC(), outcome: outcome,
		channelMessage: strings.TrimSpace(channelMessage), response: strings.TrimSpace(response),
	}, nil
}

func (fact SubmissionSendFact) Version() SubmissionVersionID { return fact.version }

func (fact SubmissionSendFact) Sequence() int { return fact.sequence }

func (fact SubmissionSendFact) SentAt() time.Time { return fact.sentAt }

func (fact SubmissionSendFact) Outcome() AttemptResult { return fact.outcome }

func (fact SubmissionSendFact) ChannelMessage() string { return fact.channelMessage }

// Response 是同步答复正文的摘要或引用，只作审计。
func (fact SubmissionSendFact) Response() string { return fact.response }

// TechnicalReceipt 是通道对一次尝试的一份技术回执（ADR-0242 决定一、二）：收没收到、校验过没过。来源身份是通道给这份回执的
// 消息号，同身份同内容重到答已有，同身份异内容是来源响应冲突。只收明确的两种：收到与校验未过；「还不知道」不是回执。
type TechnicalReceipt struct {
	source     string
	version    SubmissionVersionID
	sequence   int
	outcome    AttemptResult
	receivedAt time.Time
	digest     string
}

func ReceiveTechnicalReceipt(
	source string,
	version SubmissionVersionID,
	sequence int,
	outcome AttemptResult,
	receivedAt time.Time,
	digest string,
) (TechnicalReceipt, error) {
	source, digest = strings.TrimSpace(source), strings.TrimSpace(digest)
	if source == "" || digest == "" || !version.valid() || sequence < 1 || receivedAt.IsZero() ||
		(outcome != AttemptAcknowledged && outcome != AttemptFailed) {
		return TechnicalReceipt{}, ErrInvalidTechnicalReceipt
	}
	return TechnicalReceipt{source: source, version: version, sequence: sequence, outcome: outcome,
		receivedAt: receivedAt.UTC(), digest: digest}, nil
}

func (receipt TechnicalReceipt) Source() string { return receipt.source }

func (receipt TechnicalReceipt) Version() SubmissionVersionID { return receipt.version }

func (receipt TechnicalReceipt) Sequence() int { return receipt.sequence }

func (receipt TechnicalReceipt) Outcome() AttemptResult { return receipt.outcome }

func (receipt TechnicalReceipt) ReceivedAt() time.Time { return receipt.receivedAt }

func (receipt TechnicalReceipt) Digest() string { return receipt.digest }

// CurrentTechnicalJudgment 折出一次尝试此刻的技术判断：没有发送事实就是尝试行上的那一格（首次尝试是已登记待发送，受控重发
// 形成时已带结果）；有发送事实取它；之后到达的技术回执按到达先后逐份覆盖——后到的明确回执改变的是当前判断，不改写前面
// 那几份（ADR-0242 决定三）。回执须是这一次尝试的，别的尝试的混进来是调用方装错了。
func CurrentTechnicalJudgment(attempt SubmissionAttempt, send *SubmissionSendFact, receipts []TechnicalReceipt) (AttemptResult, error) {
	current := attempt.Result()
	if send != nil {
		if send.version != attempt.version || send.sequence != attempt.sequence {
			return AttemptResultInvalid, ErrInvalidSendFact
		}
		current = send.outcome
	}
	var latest time.Time
	for _, receipt := range receipts {
		if receipt.version != attempt.version || receipt.sequence != attempt.sequence {
			return AttemptResultInvalid, ErrInvalidTechnicalReceipt
		}
		if !receipt.receivedAt.Before(latest) {
			current, latest = receipt.outcome, receipt.receivedAt
		}
	}
	return current, nil
}
