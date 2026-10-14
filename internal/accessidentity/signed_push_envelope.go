package accessidentity

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// SignedPushMinter 把一份签名推送铸成它所绑定的集成客户端的信封（ADR-0149 决定三第三条；票 operator-channel/12）。信封与
// 客户端凭据令牌那条入口铸的是同一个 IntegrationClientEnvelope：消费方只认集成客户端的身份，不认它是经哪条入口进来的。
type SignedPushMinter struct {
	sources   PushSourceRegistry
	keys      PushSigningKeyResolver
	clients   IntegrationClientRegistry
	admission AdmissionScope
	ledger    PushReplayLedger
	now       func() time.Time
}

// NewSignedPushMinter 的 admission 不许缺，取法同 NewIntegrationClientMinter。
func NewSignedPushMinter(
	sources PushSourceRegistry,
	keys PushSigningKeyResolver,
	clients IntegrationClientRegistry,
	admission AdmissionScope,
	ledger PushReplayLedger,
	now func() time.Time,
) (*SignedPushMinter, error) {
	if sources == nil || keys == nil || clients == nil || admission == nil || ledger == nil || now == nil {
		return nil, ErrNilDependency
	}
	return &SignedPushMinter{sources: sources, keys: keys, clients: clients, admission: admission, ledger: ledger, now: now}, nil
}

// MintSignedPush 铸造一份签名推送的信封：认推送源 → 验签名 → 判窗口 → 查所绑的集成客户端、按所交的事实类型核授予、判准入
// 范围 → 记重放账。
//
// 签名判在窗口之前，理由在 ErrPushTimestampOutsideWindow；重放账记在最后，前面任何一步拒了都不入账，补齐之后原样再发照常
// 收下。租户、客户端与来源身份只取自册上的绑定，推送里自报的一概不读。
func (minter *SignedPushMinter) MintSignedPush(
	ctx context.Context,
	source PushSourceID,
	push SignedPush,
	request IntegrationClientRequest,
) (IntegrationClientEnvelope, error) {
	if _, err := ParseExternalFactType(string(request.FactType)); err != nil {
		return IntegrationClientEnvelope{}, fmt.Errorf("mint signed push envelope: request names no fact type: %w", err)
	}
	standing, found, err := minter.sources.FindPushSource(ctx, source)
	if err != nil {
		return IntegrationClientEnvelope{}, fmt.Errorf("%w: %w", ErrPushSourceRegistryUnavailable, err)
	}
	if !found {
		return IntegrationClientEnvelope{}, fmt.Errorf("%w: push source is not registered", ErrPushSignatureRejected)
	}
	at := minter.now()
	if err := minter.verifySignature(ctx, standing, push, at); err != nil {
		return IntegrationClientEnvelope{}, err
	}
	binding := standing.binding
	if push.signedAt.Before(at.Add(-binding.window)) || push.signedAt.After(at.Add(binding.clockSkew)) {
		return IntegrationClientEnvelope{}, ErrPushTimestampOutsideWindow
	}
	client, found, err := minter.clients.FindIntegrationClient(ctx, binding.client)
	if err != nil {
		return IntegrationClientEnvelope{}, fmt.Errorf("%w: %w", ErrIntegrationClientRegistryUnavailable, err)
	}
	if !found || client.binding.tenantID != binding.tenantID {
		return IntegrationClientEnvelope{}, ErrIntegrationClientNotGranted
	}
	envelope, err := admitIntegrationClient(ctx, minter.admission, client, binding.client, request, at)
	if err != nil {
		return IntegrationClientEnvelope{}, err
	}
	receipt, err := NewPushReceipt(binding.tenantID, binding.source, push, push.signedAt.Add(binding.window))
	if err != nil {
		return IntegrationClientEnvelope{}, err
	}
	first, err := minter.ledger.RecordPushReceipt(ctx, receipt)
	if err != nil {
		return IntegrationClientEnvelope{}, fmt.Errorf("%w: %w", ErrPushReplayLedgerUnavailable, err)
	}
	if !first {
		return IntegrationClientEnvelope{}, ErrPushReplayed
	}
	return envelope, nil
}

// verifySignature 拿推送源此刻生效、且签名头里有同形态签名的每一把钥去验，任一项验过即成立。签名头里没有那一形的钥不去
// 受限存储取：取一把用不上的钥，只会让存储那一侧的故障挡住一份本可以验过的推送。
//
// 某一把取不回或取回的东西不成那一形，先记下、继续试其余的：轮换期两把同形态并存时，旧钥的本体已经从存储撤了，新钥
// 仍该验得过。一把都没验过、又有取用失败，答依赖故障而不是签名不过——答签名不过会让推送方去换一把其实没对上存储的密钥。
// 部署没给受限存储则立刻交回，换一把引用也取不回。
func (minter *SignedPushMinter) verifySignature(ctx context.Context, standing PushSourceStanding, push SignedPush, at time.Time) error {
	var unresolved error
	for _, recorded := range standing.keys {
		key := recorded.key
		if !recorded.EffectiveAt(at) || !push.carries(key.form) {
			continue
		}
		material, err := minter.keys.ResolvePushSigningKey(ctx, key.reference)
		if errors.Is(err, ErrAccessChannelNotConfigured) {
			return err
		}
		if err != nil {
			unresolved = fmt.Errorf("%w: key %s: %w", ErrPushSigningKeyUnavailable, key.keyID, err)
			continue
		}
		verified, err := push.verifiedBy(key.form, material)
		if err != nil {
			unresolved = fmt.Errorf("%w: key %s: %w", ErrPushSigningKeyUnavailable, key.keyID, err)
			continue
		}
		if verified {
			return nil
		}
	}
	if unresolved != nil {
		return unresolved
	}
	return fmt.Errorf("%w: no signature verifies under a key effective at receipt", ErrPushSignatureRejected)
}
