package accessidentity

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNilDependency 表示铸造器缺件，建不成。
var ErrNilDependency = errors.New("access identity: minter dependency is nil")

// ErrRequestKeyNotDerived 表示渠道的推导口没交出可用的来源请求键。
//
// 它不折进 ErrCredentialRejected：凭据是好的，缺的是这次请求里推导所依赖的那个字段，
// 恢复动作是接入方按渠道契约补齐请求，不是换凭据（分格判据同 ADR-0029）。
var ErrRequestKeyNotDerived = errors.New("access identity: source request key was not derived")

// Minter 铸造来源信封。
//
// **签名里没有 *http.Request，也没有任何请求体形状。** 身份三要素只能来自 registry 交回
// 的那一行登记，请求那一侧只经 ChannelRequest 流向推导口、且只影响来源请求键。这是
// ADR-0003 最高隔离边界在铸造这一步的落法：不是「实现里记得别读请求里的租户」，而是
// 读得到的地方压根没有租户可读。
//
// 它做的是「已核验的渠道 → 四要素信封」这一段。凭据怎么核由 CredentialVerifier 的实现
// 方决定，本包连出示材料长什么样都不知道（ChannelCredentialProof 只交出查找键）——凭据
// 形态属 ADR-0072 Decision 二挡住的那一半。
type Minter struct {
	registry ChannelRegistry
	verifier CredentialVerifier
}

func NewMinter(registry ChannelRegistry, verifier CredentialVerifier) (*Minter, error) {
	if registry == nil || verifier == nil {
		return nil, ErrNilDependency
	}
	return &Minter{registry: registry, verifier: verifier}, nil
}

// MintSubmission 铸造一次提交的来源信封。
func (minter *Minter) MintSubmission(
	ctx context.Context,
	proof ChannelCredentialProof,
	request ChannelRequest,
) (SubmissionEnvelope, error) {

	registration, err := minter.authenticate(ctx, proof)
	if err != nil {
		return SubmissionEnvelope{}, err
	}

	requestKey, err := registration.derivation.DeriveSubmissionRequestKey(ctx, request)
	if err != nil {
		return SubmissionEnvelope{}, fmt.Errorf("mint submission envelope: %w", err)
	}

	envelope, err := seal(registration, requestKey)
	if err != nil {
		return SubmissionEnvelope{}, err
	}
	return SubmissionEnvelope{envelope: envelope}, nil
}

// MintWithdrawal 铸造一次撤回**自身**的来源信封。
//
// 它铸的不是被撤那份委托的身份——那一份由消费方按客户指名的既有委托去取回。两者必须
// 分开，合用一个会让撤回被判成原提交的重放（WithdrawalIntake 注释里的原句）。
func (minter *Minter) MintWithdrawal(
	ctx context.Context,
	proof ChannelCredentialProof,
	request ChannelRequest,
) (WithdrawalEnvelope, error) {

	registration, err := minter.authenticate(ctx, proof)
	if err != nil {
		return WithdrawalEnvelope{}, err
	}

	requestKey, err := registration.derivation.DeriveWithdrawalRequestKey(ctx, request)
	if err != nil {
		return WithdrawalEnvelope{}, fmt.Errorf("mint withdrawal envelope: %w", err)
	}

	envelope, err := seal(registration, requestKey)
	if err != nil {
		return WithdrawalEnvelope{}, err
	}
	return WithdrawalEnvelope{envelope: envelope}, nil
}

// authenticate 定位登记行并核验凭据，两步的失败分成两格交回。
//
// 顺序不能反：先核验后查册的话，册里没有这一行时根本没有受控引用可拿去核验，只能先
// 答一个含糊的失败，两格就又折回一格了。
func (minter *Minter) authenticate(
	ctx context.Context,
	proof ChannelCredentialProof,
) (ChannelRegistration, error) {

	// 显式判 nil 接口：少了这一步，缺凭证的调用会在 ClaimedChannel() 上 panic，
	// 而 panic 与「这次出示没说清认领哪一行」是两回事，前者还会把装配缺陷报成崩溃。
	if proof == nil {
		return ChannelRegistration{}, ErrInvalidChannelKey
	}
	key := proof.ClaimedChannel()
	if key.value == "" {
		return ChannelRegistration{}, ErrInvalidChannelKey
	}

	registration, found, err := minter.registry.FindChannel(ctx, key)
	if err != nil {
		// 读不动登记册是依赖故障，与「册里没有这一行」分开交回：前者要运维去救，
		// 后者要接入方去配。把它折进未配置会让人去配一个其实已经配好的渠道。
		return ChannelRegistration{}, fmt.Errorf("look up access channel: %w", err)
	}
	if !found {
		return ChannelRegistration{}, ErrAccessChannelNotConfigured
	}

	accepted, err := minter.verifier.VerifyCredential(ctx, registration.credential, proof)
	if err != nil {
		return ChannelRegistration{}, fmt.Errorf("verify access credential: %w", err)
	}
	if !accepted {
		return ChannelRegistration{}, ErrCredentialRejected
	}
	return registration, nil
}

// seal 用登记行的身份三要素加上推导出的请求键封出信封。
//
// 它是本包**唯一**产生非零 SourceEnvelope 的地方，所以「身份不来自请求」这条不变式
// 只需要在这一处成立。
func seal(registration ChannelRegistration, requestKey string) (SourceEnvelope, error) {
	trimmed := strings.TrimSpace(requestKey)
	if trimmed == "" {
		return SourceEnvelope{}, ErrRequestKeyNotDerived
	}
	return SourceEnvelope{
		tenantID:          registration.tenantID,
		customerAccountID: registration.customerAccountID,
		source:            registration.source,
		requestKey:        trimmed,
	}, nil
}
