package accessidentity

import (
	"context"
	"errors"
	"strings"
)

// ErrAccessChannelNotConfigured 表示这次出示所走的渠道没配置。客户渠道上是登记册里没有
// 所认领的那一行，恢复动作是去配置渠道参数（PAR-INT-01）；操作者渠道上是 parcel-api 的发行方
// 部署参数未设（ADR-0100 决定二），恢复动作是去配部署参数。两族都是换一份凭据或重试不会好。
//
// 它与 ErrCredentialRejected 分成两格而不是合成一句「认证失败」，判据同 ADR-0029：
// 恢复动作不同——这一格找的是配渠道的人，那一格找的是持凭据的人。合成一格会让两种
// 恢复动作在同一个答复下不可分辨，而不可分辨的代价由收到答复的那一方付。
var ErrAccessChannelNotConfigured = errors.New("access identity: access channel is not configured")

// ErrCredentialRejected 表示核验方拒绝了这次出示：客户渠道上是登记行在册而凭据不符，
// 操作者渠道上是令牌缺失、过期或校验不过。
//
// 它不说明哪一半不对，理由同 ADR-0029 的探针同答约束：细分会把「这个渠道标识存在」
// 这件事告诉一个还没证明自己是谁的调用方。
var ErrCredentialRejected = errors.New("access identity: presented credential rejected")

// ErrCredentialVerifierUnavailable 表示核验方此刻答不出这次出示对不对：它要的外部依赖取不回
// 或答得不成形（操作者族上是发行方的 JWKS）。
//
// 它与 ErrCredentialRejected 分成两格，判据同 ADR-0029：那一格要持凭据的人去换凭据，这一格要
// 运维去救依赖——凭据可能完全没毛病，换多少次也不会好；合成一格会让人去重新登录一个其实
// 有效的会话。
var ErrCredentialVerifierUnavailable = errors.New("access identity: credential verifier is unavailable")

// ErrInvalidChannelKey 表示这次出示没说清自己认领哪一行登记，还没走到核验那一步。
var ErrInvalidChannelKey = errors.New("access identity: claimed channel key is empty")

// ErrInvalidCredentialReference 表示受控引用是空的，那一行登记建不成。
var ErrInvalidCredentialReference = errors.New("access identity: credential reference is empty")

// ChannelKey 是一次出示所认领的登记行的键。
//
// **它自己不是身份。** 租户、客户账户与来源一律取自认领到的那一行，不取自这个键——
// 调用方能自由指定它，而 ADR-0003 的最高隔离边界不允许身份来自调用方能自由指定的
// 东西。命名上刻意不叫「凭据键」也不叫「标识」：它是登记册的查找键，属登记这一侧，
// 与凭据长什么样无关（凭据形态是 ADR-0072 Decision 二挡住的那一半）。
type ChannelKey struct{ value string }

func NewChannelKey(value string) (ChannelKey, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ChannelKey{}, ErrInvalidChannelKey
	}
	return ChannelKey{value: trimmed}, nil
}

func (key ChannelKey) String() string { return key.value }

// CredentialReference 是登记册里存的**受控引用**，不是凭据本体。凭据本体留在约定的
// 受限存储里，库中出现任何形似凭据的明文即违背票 01 的 S1 验收点。
//
// 它是个不透明串：本包不解读它，也不因它推断凭据是密钥、证书还是会话。
type CredentialReference struct{ value string }

func NewCredentialReference(value string) (CredentialReference, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return CredentialReference{}, ErrInvalidCredentialReference
	}
	return CredentialReference{value: trimmed}, nil
}

func (reference CredentialReference) String() string { return reference.value }

// ChannelCredentialProof 是渠道侧就一次出示材料交出的凭证。
//
// **本包不构造它、不拆它，也不知道它背后是 API 密钥、客户证书还是门户会话。** 只有
// 一个方法，且那个方法只交出登记行的查找键——凭据形态属 ADR-0072 Decision 二明文挡住
// 的那一半（原句：「登记册表结构**与凭据形态**在 PAR-INT-01 最低证据到位前不立」）。
// 本轮做的是「已核验的渠道 → 四要素信封」这一段，不是凭据验证本身。
//
// 把它做成单方法接口而不是带字段的结构，是因为一个 `{公开键, 秘密}` 的结构就已经替
// 三种渠道拟了同一种形态：标准文件投递与门户登录都没有「秘密」这一栏。
type ChannelCredentialProof interface {
	ClaimedChannel() ChannelKey
}

// CredentialVerifier 拿登记行上的受控引用去核对一次出示。
//
// 本仓没有它的生产实现，也不该在本轮有：核对怎么做取决于凭据形态，而那一半还锁着；
// 凭据本体外置另是 AGENTS.md 的红线。接口留在这里，是为了让铸造那一段能先成型、被
// 钉住，并且在装配时看得出还缺谁。
type CredentialVerifier interface {
	VerifyCredential(ctx context.Context, reference CredentialReference, proof ChannelCredentialProof) (bool, error)
}
