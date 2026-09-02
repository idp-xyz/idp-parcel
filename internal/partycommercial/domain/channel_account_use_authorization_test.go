package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: `AT-PC-009`「渠道账号技术可用但无业务授权 → 不发布账号使用授权」。
//
// 技术可达与业务授权分轴（ADR-0039）：前者成立不能顶替后者；字段不全是另一格错误。
func TestTechnicallyAvailableAccountDoesNotPublishWithoutBusinessAuthorization(t *testing.T) {
	account := commercialValue(t, domain.NewChannelAccountID, "channel-acct-1")
	grantor := commercialValue(t, domain.NewPartyID, "holder-1")
	grantee := commercialValue(t, domain.NewPartyID, "operator-1")
	channel := commercialValue(t, domain.NewChannelProductReference, "channel-product-1")
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")
	interval := mustInterval(t)
	publishedAt := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	t.Run("technically available but business unauthorized refuses publication", func(t *testing.T) {
		published, err := domain.PublishChannelAccountUseAuthorization(
			account, grantor, grantee, channel, scope, interval,
			domain.ChannelAccountTechnicallyAvailable,
			domain.ChannelAccountBusinessUnauthorized,
			publishedAt,
		)
		if !errors.Is(err, domain.ErrChannelAccountBusinessUnauthorized) {
			t.Fatalf("error = %v, want ErrChannelAccountBusinessUnauthorized", err)
		}
		if errors.Is(err, domain.ErrInvalidChannelAccountUseAuthorization) {
			t.Fatal("业务未授权被压成了字段不全")
		}
		if published.Status() == domain.ChannelAccountUseAuthorizationPublished {
			t.Fatal("无业务授权仍发布了账号使用授权")
		}
	})

	t.Run("unanswered business standing is unauthorized", func(t *testing.T) {
		_, err := domain.PublishChannelAccountUseAuthorization(
			account, grantor, grantee, channel, scope, interval,
			domain.ChannelAccountTechnicallyAvailable,
			domain.ChannelAccountBusinessStandingInvalid,
			publishedAt,
		)
		if !errors.Is(err, domain.ErrChannelAccountBusinessUnauthorized) {
			t.Fatalf("error = %v, want ErrChannelAccountBusinessUnauthorized", err)
		}
	})

	t.Run("business authorized may publish even when also technically available", func(t *testing.T) {
		published, err := domain.PublishChannelAccountUseAuthorization(
			account, grantor, grantee, channel, scope, interval,
			domain.ChannelAccountTechnicallyAvailable,
			domain.ChannelAccountBusinessAuthorized,
			publishedAt,
		)
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
		if published.Status() != domain.ChannelAccountUseAuthorizationPublished {
			t.Fatalf("status = %q, want PUBLISHED", published.Status())
		}
		if !published.AllowsUseAt(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
			t.Fatal("已发布授权在有效期内不允许使用")
		}
	})

	t.Run("incomplete fields stay distinct from business unauthorized", func(t *testing.T) {
		_, err := domain.PublishChannelAccountUseAuthorization(
			domain.ChannelAccountID{}, grantor, grantee, channel, scope, interval,
			domain.ChannelAccountTechnicallyAvailable,
			domain.ChannelAccountBusinessAuthorized,
			publishedAt,
		)
		if !errors.Is(err, domain.ErrInvalidChannelAccountUseAuthorization) {
			t.Fatalf("error = %v, want ErrInvalidChannelAccountUseAuthorization", err)
		}
		if errors.Is(err, domain.ErrChannelAccountBusinessUnauthorized) {
			t.Fatal("字段不全与业务未授权互相 Is")
		}
	})

	t.Run("channel mapping candidacy is not a use authorization", func(t *testing.T) {
		// 产品—渠道映射只给候选；即使技术上「有渠道」，也不能冒充使用授权已发布。
		product := mustServiceProduct(t)
		mapping, err := domain.NewProductChannelMapping(product, []domain.ChannelProductReference{channel}, interval)
		if err != nil {
			t.Fatalf("new mapping: %v", err)
		}
		if len(mapping.CandidatesAt(publishedAt)) == 0 {
			t.Fatal("fixture mapping has no candidates")
		}
		_, err = domain.PublishChannelAccountUseAuthorization(
			account, grantor, grantee, channel, scope, interval,
			domain.ChannelAccountTechnicallyAvailable,
			domain.ChannelAccountBusinessUnauthorized,
			publishedAt,
		)
		if !errors.Is(err, domain.ErrChannelAccountBusinessUnauthorized) {
			t.Fatalf("mapping candidacy leaked into use-authorization publication: %v", err)
		}
	})
}

// Covers: CONTEXT「渠道账号使用授权」生命周期——「授权可以在到期前被显式撤销；撤销和
// 自然到期均终止后续新使用，但不删除已经形成的授权证据和交易快照」。
//
// 两个终止成因后果相同而来源不同，必须在类型上就分得开。合并成一个终止时刻之后，下游读到
// 的「现在不能用」既可能是持有人收回了授权、也可能只是这一版到期了——前者要去重新取得授权，
// 后者要去续期，两件事要人做的动作相反。这正是本仓反复记的那个形状：两种状态可观察签名相同。
func TestRevocationAndNaturalExpiryTerminateUseButStayDistinguishable(t *testing.T) {
	authorization := publishedChannelAccountUse(t)
	insideInterval := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	afterInterval := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)

	t.Run("revocation stops use inside the interval", func(t *testing.T) {
		revoked, err := authorization.Revoke(
			commercialValue(t, domain.NewChannelAccountRevocationBasisReference, "revocation-1"),
			time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatalf("revoke: %v", err)
		}
		if revoked.AllowsUseAt(insideInterval) {
			t.Fatal("撤销之后仍允许在有效期内发起新的业务使用")
		}
		if revoked.Status() != domain.ChannelAccountUseAuthorizationRevoked {
			t.Fatalf("status = %q, want REVOKED", revoked.Status())
		}
	})

	t.Run("natural expiry is not recorded as a revocation", func(t *testing.T) {
		// 到期同样终止后续新使用，但它不是撤销：状态仍停在已发布，撤销时刻缺席。
		if authorization.AllowsUseAt(afterInterval) {
			t.Fatal("有效期之外仍允许发起新的业务使用")
		}
		if authorization.Status() != domain.ChannelAccountUseAuthorizationPublished {
			t.Fatalf("status = %q, want PUBLISHED", authorization.Status())
		}
		if _, revoked := authorization.RevokedAt(); revoked {
			t.Fatal("自然到期被记成了撤销")
		}
	})
}

// Covers: CONTEXT「撤销和自然到期均终止**后续**新使用……已经形成的交易仍保留当时有效的
// 授权依据」。
//
// 撤销自其自身时点起生效，不回溯。判据同 SupplierAgreement.SupportsProcurementAt。把撤销
// 做成整段失效会让追溯答错方向：一笔在撤销之前正当形成的交易，事后复核时会被判成当时就无
// 授权，而 CONTEXT 明说那笔仍保留当时有效的依据。
func TestRevocationClosesFutureUseWithoutUnmakingPastUse(t *testing.T) {
	revokedAt := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	revoked, err := publishedChannelAccountUse(t).Revoke(
		commercialValue(t, domain.NewChannelAccountRevocationBasisReference, "revocation-1"),
		revokedAt,
	)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if !revoked.AllowsUseAt(revokedAt.Add(-24 * time.Hour)) {
		t.Fatal("撤销回溯了：撤销时点之前的使用被判成无授权")
	}
	if revoked.AllowsUseAt(revokedAt) {
		t.Fatal("撤销当刻仍允许发起新的业务使用")
	}
	if revoked.AllowsUseAt(revokedAt.Add(24 * time.Hour)) {
		t.Fatal("撤销之后仍允许发起新的业务使用")
	}
}

func publishedChannelAccountUse(t *testing.T) domain.ChannelAccountUseAuthorization {
	t.Helper()
	published, err := domain.PublishChannelAccountUseAuthorization(
		commercialValue(t, domain.NewChannelAccountID, "channel-acct-1"),
		commercialValue(t, domain.NewPartyID, "holder-1"),
		commercialValue(t, domain.NewPartyID, "operator-1"),
		commercialValue(t, domain.NewChannelProductReference, "channel-product-1"),
		commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		mustInterval(t),
		domain.ChannelAccountTechnicallyAvailable,
		domain.ChannelAccountBusinessAuthorized,
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return published
}

func mustServiceProduct(t *testing.T) domain.ServiceProduct {
	t.Helper()
	version := registerable(t, domain.ServiceProductObject, "product-channel-1", "v1", "sha256:product-channel-1")
	live, err := version.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	product, err := domain.NewServiceProduct(live, domain.NetworkServiceForm)
	if err != nil {
		t.Fatalf("new service product: %v", err)
	}
	return product
}
