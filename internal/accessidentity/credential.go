package accessidentity

import (
	"context"
	"errors"
	"strings"
)

// ErrAccessChannelNotConfigured 表示登记册里没有出示凭据所对应的那一行。恢复动作是
// 去配置渠道参数（PAR-INT-01），换一份凭据或重试都不会好。
//
// 它与 ErrCredentialRejected 分成两格而不是合成一句「认证失败」，判据同 ADR-0029：
// 恢复动作不同——这一格找的是配渠道的人，那一格找的是持凭据的人。合成一格会让两种
// 恢复动作在同一个答复下不可分辨，而不可分辨的代价由收到答复的那一方付。
var ErrAccessChannelNotConfigured = errors.New("access identity: access channel is not configured")

// ErrCredentialRejected 表示登记行在册，但出示的凭据核验不通过。
//
// 它不说明哪一半不对（是密钥错还是已轮换），理由同 ADR-0029 的探针同答约束：细分会把
// 「这个渠道标识存在」这件事告诉一个还没证明自己是谁的调用方。
var ErrCredentialRejected = errors.New("access identity: presented credential rejected")

// ErrInvalidCredential 表示出示的材料连形状都不完整（缺公开标识或缺秘密部分），
// 还没走到核验那一步。
var ErrInvalidCredential = errors.New("access identity: presented credential is incomplete")

// CredentialKey 是凭据的公开部分，用来在登记册里定位那一行。
//
// **它自己不是身份。** 租户、客户账户与来源一律取自定位到的登记行，不取自这个键——
// 调用方能自由指定它，而 ADR-0003 的最高隔离边界不允许身份来自调用方能自由指定的东西。
// 命名上刻意叫「键」不叫「标识」，就是不想让下一个人顺手拿它当租户用。
type CredentialKey struct{ value string }

func NewCredentialKey(value string) (CredentialKey, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return CredentialKey{}, ErrInvalidCredential
	}
	return CredentialKey{value: trimmed}, nil
}

func (key CredentialKey) String() string { return key.value }

// CredentialReference 是登记册里存的**受控引用**，不是凭据本体。凭据本体留在约定的
// 受限存储里，库中出现任何形似凭据的明文即违背票 01 的 S1 验收点。
type CredentialReference struct{ value string }

func NewCredentialReference(value string) (CredentialReference, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return CredentialReference{}, ErrInvalidCredential
	}
	return CredentialReference{value: trimmed}, nil
}

func (reference CredentialReference) String() string { return reference.value }

// PresentedCredential 是接入方在一次请求里出示的材料：公开的定位键 + 秘密部分。
//
// 秘密部分只交给 CredentialVerifier，不进任何信封。String 有意写成脱敏常量而不是
// 默认的字段展开——一个能被 %v 打进日志的秘密，靠的是每个人每次都记得别打；把脱敏
// 做进类型，就不必靠记得。
type PresentedCredential struct {
	key    CredentialKey
	secret string
}

func NewPresentedCredential(key CredentialKey, secret string) (PresentedCredential, error) {
	if key.value == "" || strings.TrimSpace(secret) == "" {
		return PresentedCredential{}, ErrInvalidCredential
	}
	return PresentedCredential{key: key, secret: secret}, nil
}

// Key 交出公开的定位键。秘密部分没有访问器：包外拿不到它，也就无从把它写进别处。
func (credential PresentedCredential) Key() CredentialKey { return credential.key }

func (credential PresentedCredential) String() string {
	return "accessidentity.PresentedCredential{key:" + credential.key.value + " secret:REDACTED}"
}

// CredentialVerifier 拿受控引用去核对出示的材料。
//
// 本仓没有它的生产实现，也不该有：凭据本体外置是 AGENTS.md 的红线之一，核对发生在
// 受限存储那一侧。接口留在这里是为了让铸造流程能先成型并被钉住。
type CredentialVerifier interface {
	VerifyCredential(ctx context.Context, reference CredentialReference, presented PresentedCredential) (bool, error)
}
