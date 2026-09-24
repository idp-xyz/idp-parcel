package accessidentity

import (
	"context"
	"log/slog"
	"strings"
)

// redactedCredential 是凭据在一切渲染路径上的替身。
const redactedCredential = "[REDACTED OPERATOR CREDENTIAL]"

// OperatorCredential 是操作者出示的令牌本体（ADR-0100 决定二第二条）。
//
// 它只在核验那一刻存在：不落库、不进日志。「不进日志」做进了类型——String、GoString 与
// LogValue 一律只交出替身，于是 fmt 的各个动词、slog 的两种处理器、以及把它顺手包进错误信息，
// 都印不出令牌；靠「写日志的人记得别打它」守不住，因为漏打一次就再也收不回来。
type OperatorCredential struct{ token string }

// NewOperatorCredential 不在这里判空：缺令牌是核验方要答的一格（令牌缺失与过期、校验不过同属
// 「持令牌的人去登录或换令牌」，ADR-0100 决定四），不是构造失败。
func NewOperatorCredential(token string) OperatorCredential {
	return OperatorCredential{token: strings.TrimSpace(token)}
}

// Token 交出令牌本体，只给核验方用。
func (credential OperatorCredential) Token() string { return credential.token }

func (OperatorCredential) String() string { return redactedCredential }

func (OperatorCredential) GoString() string { return redactedCredential }

func (OperatorCredential) LogValue() slog.Value { return slog.StringValue(redactedCredential) }

// OperatorCredentialVerifier 核验一次操作者出示，交回经核验的操作者主体。
//
// 它不复用 CredentialVerifier：那个接口拿登记行上的受控引用去核一次出示、只答对不对，前提是
// 先按出示认领的键查到了那一行；操作者族在核验之前没有行可查——操作者主体（发行方 + sub）
// 本身就是核验的产物，册要拿它去查（ADR-0100 决定二、三）。
//
// 失败分三格，各对一种恢复动作（ADR-0029）：ErrAccessChannelNotConfigured（发行方参数未设，
// 找配部署参数的人）、ErrCredentialRejected（令牌缺失、过期或校验不过，持令牌的人去登录或
// 换令牌）、ErrCredentialVerifierUnavailable（取不回发行方的公钥集，运维去救依赖）。
type OperatorCredentialVerifier interface {
	VerifyOperatorCredential(ctx context.Context, credential OperatorCredential) (OperatorSubject, error)
}

// UnconfiguredOperatorCredentialVerifier 是发行方参数未设时的核验方：对任何出示都答
// ErrAccessChannelNotConfigured，缺令牌也一样——整族未配置时，令牌带没带都不是该找的原因。
type UnconfiguredOperatorCredentialVerifier struct{}

func (UnconfiguredOperatorCredentialVerifier) VerifyOperatorCredential(
	context.Context,
	OperatorCredential,
) (OperatorSubject, error) {
	return OperatorSubject{}, ErrAccessChannelNotConfigured
}
