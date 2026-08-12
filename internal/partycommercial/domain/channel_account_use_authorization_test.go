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
