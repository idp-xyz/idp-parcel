package accessidentity

import (
	"context"
	"log/slog"
	"strings"
)

// redactedClientCredential 是集成客户端凭据在一切渲染路径上的替身。
const redactedClientCredential = "[REDACTED INTEGRATION CLIENT CREDENTIAL]"

// IntegrationClientCredential 是集成客户端这一次出示的凭据：客户端凭据许可取得的访问令牌本体（ADR-0149 决定三），
// 与这次 TLS 连接上出示的客户端证书（DER；没有就为空）。证书只用来核对证书绑定令牌（RFC 8705），令牌不绑证书时
// 它不参与任何判断。
//
// 令牌只在核验那一刻存在：不落库、不进日志，做法同 OperatorCredential——String、GoString 与 LogValue 一律只交出
// 替身。
type IntegrationClientCredential struct {
	token       string
	certificate []byte
}

// NewIntegrationClientCredential 不在这里判空：缺令牌是核验方要答的一格，不是构造失败。证书按值拷进来，出示之后
// 调用方再改它手里那份，改不到核验方看见的这一份。
func NewIntegrationClientCredential(token string, certificate []byte) IntegrationClientCredential {
	return IntegrationClientCredential{
		token:       strings.TrimSpace(token),
		certificate: append([]byte(nil), certificate...),
	}
}

// Token 交出令牌本体，只给核验方用。
func (credential IntegrationClientCredential) Token() string { return credential.token }

// Certificate 交出这次连接出示的客户端证书的副本；没有出示就为空。
func (credential IntegrationClientCredential) Certificate() []byte {
	return append([]byte(nil), credential.certificate...)
}

func (IntegrationClientCredential) String() string { return redactedClientCredential }

func (IntegrationClientCredential) GoString() string { return redactedClientCredential }

func (IntegrationClientCredential) LogValue() slog.Value {
	return slog.StringValue(redactedClientCredential)
}

// VerifiedClientToken 是核验方对一次集成客户端出示的结论：令牌代表的客户端主体，以及令牌是否绑定在这次连接出示的
// 证书上且对得上。
//
// 「绑没绑」由核验方答而「要不要绑」由册答：前者是这枚令牌的事实，后者是租户对这个客户端的取值，铸造那一步把两者
// 对上（IntegrationClientBinding 头注）。
type VerifiedClientToken struct {
	subject          IntegrationClientSubject
	certificateBound bool
}

func NewVerifiedClientToken(subject IntegrationClientSubject, certificateBound bool) VerifiedClientToken {
	return VerifiedClientToken{subject: subject, certificateBound: certificateBound}
}

func (verified VerifiedClientToken) Subject() IntegrationClientSubject { return verified.subject }
func (verified VerifiedClientToken) CertificateBound() bool            { return verified.certificateBound }

// IntegrationClientCredentialVerifier 核验一次集成客户端出示，交回经核验的客户端主体与证书绑定情况。
//
// 它不复用 OperatorCredentialVerifier：两族的信任锚是两组部署参数（受众不同——操作者令牌发给管理台，客户端令牌发给
// 本产品的 API），共用一个接口，装配时把操作者那一只接到集成口上也编得过。失败分三格，各对一种恢复动作，与操作者族
// 同（ADR-0029）：ErrAccessChannelNotConfigured、ErrCredentialRejected、ErrCredentialVerifierUnavailable。
type IntegrationClientCredentialVerifier interface {
	VerifyIntegrationClientCredential(ctx context.Context, credential IntegrationClientCredential) (VerifiedClientToken, error)
}

// UnconfiguredIntegrationClientCredentialVerifier 是集成客户端族发行方参数未设时的核验方：对任何出示都答
// ErrAccessChannelNotConfigured，缺令牌也一样。
type UnconfiguredIntegrationClientCredentialVerifier struct{}

func (UnconfiguredIntegrationClientCredentialVerifier) VerifyIntegrationClientCredential(
	context.Context,
	IntegrationClientCredential,
) (VerifiedClientToken, error) {
	return VerifiedClientToken{}, ErrAccessChannelNotConfigured
}
