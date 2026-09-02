package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证渠道账号使用授权登记册（0018、ADR-0093）：撤销以新修订
// 追加而前一修订原样留册、按账号回查取每笔授权的最新修订、跨租户读不到、无事务拒。

var channelUseStartsAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newChannelAccountUseAuthorizations(t *testing.T) (*adapter.ChannelAccountUseAuthorizations, bentoapp.Transactor) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := adapter.NewChannelAccountUseAuthorizations(db)
	if err != nil {
		t.Fatalf("构造账号使用授权登记册：%v", err)
	}
	return registry, db.Transactor()
}

func channelUseFixture(
	t *testing.T,
	tenant, id, account, grantee string,
	revision int,
) domain.ChannelAccountUseAuthorizationRegistration {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(channelUseStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	authorization, err := domain.PublishChannelAccountUseAuthorization(
		pcValue(t, domain.NewChannelAccountID, account),
		pcValue(t, domain.NewPartyID, "holder-1"),
		pcValue(t, domain.NewPartyID, grantee),
		pcValue(t, domain.NewChannelProductReference, "CH-SG-POST"),
		pcValue(t, domain.NewCommercialScopeReference, "scope-a"),
		interval,
		domain.ChannelAccountTechnicallyAvailable,
		domain.ChannelAccountBusinessAuthorized,
		channelUseStartsAt,
	)
	if err != nil {
		t.Fatalf("publish authorization: %v", err)
	}
	registration, err := domain.NewChannelAccountUseAuthorizationRegistration(
		pcTenant(t, tenant),
		pcValue(t, domain.NewChannelAccountUseAuthorizationID, id),
		revision,
		authorization,
	)
	if err != nil {
		t.Fatalf("new registration: %v", err)
	}
	return registration
}

func mustSaveChannelAccountUse(
	t *testing.T,
	transactor bentoapp.Transactor,
	registry *adapter.ChannelAccountUseAuthorizations,
	registration domain.ChannelAccountUseAuthorizationRegistration,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := registry.SaveChannelAccountUse(txCtx, registration)
		if err != nil {
			return err
		}
		if outcome != ports.ChannelAccountUseSaved {
			return fmt.Errorf("save outcome = %s", outcome)
		}
		return nil
	})
}

// Covers: ADR-0093 决定一——撤销以新修订追加，前一修订原样留在册上。
//
// 这条在库上要能证两半：最新修订读回来是已撤销且带得出撤销时刻与依据；而前一修订仍在，
// 因为它是那段时间里确实有效的授权证据（CONTEXT「不删除已经形成的授权证据和交易快照」）。
func TestRevocationLandsAsANewRevisionAndLeavesThePriorOneStanding(t *testing.T) {
	registry, transactor := newChannelAccountUseAuthorizations(t)
	ctx := t.Context()

	first := channelUseFixture(t, "tenant-1", "cauth-1", "ACCT-1", "operator-1", 1)
	mustSaveChannelAccountUse(t, transactor, registry, first)

	revokedAt := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	revoked, err := first.Authorization().Revoke(
		pcValue(t, domain.NewChannelAccountRevocationBasisReference, "holder-notice-1"),
		revokedAt,
	)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	second, err := first.Succeed(revoked)
	if err != nil {
		t.Fatalf("succeed: %v", err)
	}
	mustSaveChannelAccountUse(t, transactor, registry, second)

	loaded, found, err := registry.LoadLatest(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewChannelAccountUseAuthorizationID, "cauth-1"))
	if err != nil || !found {
		t.Fatalf("load latest = (%v, %v)", found, err)
	}
	if loaded.Revision() != 2 {
		t.Fatalf("revision = %d, want 2", loaded.Revision())
	}
	if loaded.Authorization().Status() != domain.ChannelAccountUseAuthorizationRevoked {
		t.Fatalf("status = %q, want REVOKED", loaded.Authorization().Status())
	}
	at, wasRevoked := loaded.Authorization().RevokedAt()
	if !wasRevoked || !at.Equal(revokedAt) {
		t.Fatalf("revokedAt = (%v, %v), want %v", at, wasRevoked, revokedAt)
	}
	if loaded.Authorization().RevokedOn().String() != "holder-notice-1" {
		t.Fatalf("revokedOn = %q", loaded.Authorization().RevokedOn())
	}
	// 撤销不回溯：撤销时点之前仍有依据，之后没有。
	if !loaded.Authorization().AllowsUseAt(revokedAt.Add(-time.Hour)) {
		t.Fatal("撤销回溯了：撤销时点之前被判成无授权")
	}
	if loaded.Authorization().AllowsUseAt(revokedAt) {
		t.Fatal("撤销当刻仍允许发起新的业务使用")
	}

	// 前一修订原样留册：重放同内容答已登记而不是冲突，证明那一行没被改写过。
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := registry.SaveChannelAccountUse(txCtx, first)
		if err != nil {
			return err
		}
		if outcome != ports.ChannelAccountUseAlreadyRegistered {
			return fmt.Errorf("重放前一修订 outcome = %s，want ALREADY_REGISTERED", outcome)
		}
		return nil
	})
}

// Covers: ports.ChannelAccountUseAuthorizationRegistry.LoadAuthorizedAccountUse 的分组语义。
//
// 同一个渠道账号先后授给两方是正当的（前一笔已撤销，后一笔在生效）。整体取最新一行会让其中
// 一笔在册上消失，而调用方要的正是逐笔对时点判 AllowsUseAt——读口不代答「此刻许不许用」。
func TestAccountLookupReturnsTheLatestRevisionOfEveryAuthorizationOnThatAccount(t *testing.T) {
	registry, transactor := newChannelAccountUseAuthorizations(t)
	ctx := t.Context()

	older := channelUseFixture(t, "tenant-2", "cauth-older", "ACCT-SHARED", "operator-1", 1)
	mustSaveChannelAccountUse(t, transactor, registry, older)
	revoked, err := older.Authorization().Revoke(
		pcValue(t, domain.NewChannelAccountRevocationBasisReference, "holder-notice-2"),
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	succeeded, err := older.Succeed(revoked)
	if err != nil {
		t.Fatalf("succeed: %v", err)
	}
	mustSaveChannelAccountUse(t, transactor, registry, succeeded)

	newer := channelUseFixture(t, "tenant-2", "cauth-newer", "ACCT-SHARED", "operator-2", 1)
	mustSaveChannelAccountUse(t, transactor, registry, newer)

	found, err := registry.LoadAuthorizedAccountUse(
		ctx, pcTenant(t, "tenant-2"), pcValue(t, domain.NewChannelAccountID, "ACCT-SHARED"))
	if err != nil {
		t.Fatalf("load by account: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("按账号回查得 %d 笔，want 2（每笔授权各取最新修订）", len(found))
	}
	byID := map[string]domain.ChannelAccountUseAuthorizationRegistration{}
	for _, registration := range found {
		byID[registration.ID().String()] = registration
	}
	if got := byID["cauth-older"]; got.Revision() != 2 ||
		got.Authorization().Status() != domain.ChannelAccountUseAuthorizationRevoked {
		t.Fatalf("已撤销那笔 = rev %d / %s，want rev 2 / REVOKED", got.Revision(), got.Authorization().Status())
	}
	if got := byID["cauth-newer"]; got.Revision() != 1 ||
		got.Authorization().Status() != domain.ChannelAccountUseAuthorizationPublished {
		t.Fatalf("在生效那笔 = rev %d / %s，want rev 1 / PUBLISHED", got.Revision(), got.Authorization().Status())
	}

	// 跨租户读不到：同一个账号标识在别的租户下是别人的事实。
	other, err := registry.LoadAuthorizedAccountUse(
		ctx, pcTenant(t, "tenant-3"), pcValue(t, domain.NewChannelAccountID, "ACCT-SHARED"))
	if err != nil {
		t.Fatalf("load by account (other tenant): %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("跨租户读到 %d 笔", len(other))
	}
}

func TestChannelAccountUseWritesRefuseToRunOutsideATransaction(t *testing.T) {
	registry, _ := newChannelAccountUseAuthorizations(t)
	if _, err := registry.SaveChannelAccountUse(t.Context(),
		channelUseFixture(t, "tenant-1", "cauth-1", "ACCT-1", "operator-1", 1),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 SaveChannelAccountUse 应返回 ErrTransactionRequired，实得：%v", err)
	}
}
