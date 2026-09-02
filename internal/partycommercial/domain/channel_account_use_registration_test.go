package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: CONTEXT「渠道账号使用授权」生命周期——「撤销和自然到期均终止后续新使用，但不删除
// 已经形成的授权证据和交易快照」，落在登记面上就是撤销以新修订追加，绝不原地改写。
//
// 这条不变量 SQL 守不住。主键（租户+登记标识+修订）只守键唯一，一条修订二把账号或授权双方
// 整个换掉照样入得了库，而它在册上看起来仍是同一笔授权的后继——「追加」于是成了偷换的伪装。
// 下游按登记标识取最新修订，取到的会是一份从没有人授权过的关系。
func TestASucceedingRevisionCannotSwapThePartiesItClaimsToContinue(t *testing.T) {
	tenant := commercialValue(t, domain.NewTenantID, "tenant-1")
	id := commercialValue(t, domain.NewChannelAccountUseAuthorizationID, "cauth-1")
	basis := commercialValue(t, domain.NewChannelAccountRevocationBasisReference, "revocation-1")

	first, err := domain.NewChannelAccountUseAuthorizationRegistration(
		tenant, id, 1, publishedChannelAccountUse(t),
	)
	if err != nil {
		t.Fatalf("first revision: %v", err)
	}

	t.Run("a revocation revision continues the same grant", func(t *testing.T) {
		revoked, err := first.Authorization().Revoke(basis, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("revoke: %v", err)
		}
		second, err := first.Succeed(revoked)
		if err != nil {
			t.Fatalf("succeed: %v", err)
		}
		if second.Revision() != 2 {
			t.Fatalf("revision = %d, want 2", second.Revision())
		}
		// 前一修订不因后继而改变——它是那段时间里确实有效的授权证据。
		if first.Authorization().Status() != domain.ChannelAccountUseAuthorizationPublished {
			t.Fatal("追加后继修订回头改写了前一修订")
		}
	})

	t.Run("a revision that swaps the grantee is refused", func(t *testing.T) {
		stranger, err := domain.PublishChannelAccountUseAuthorization(
			first.Authorization().Account(),
			first.Authorization().Grantor(),
			commercialValue(t, domain.NewPartyID, "operator-2"),
			first.Authorization().Channel(),
			first.Authorization().Scope(),
			first.Authorization().Effective(),
			domain.ChannelAccountTechnicallyAvailable,
			domain.ChannelAccountBusinessAuthorized,
			time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatalf("publish stranger: %v", err)
		}
		if _, err := first.Succeed(stranger); !errors.Is(err, domain.ErrInvalidChannelAccountUseRegistration) {
			t.Fatalf("error = %v, want ErrInvalidChannelAccountUseRegistration；换掉被授权人的修订入了册", err)
		}
	})

	t.Run("a revision that swaps the account is refused", func(t *testing.T) {
		elsewhere, err := domain.PublishChannelAccountUseAuthorization(
			commercialValue(t, domain.NewChannelAccountID, "channel-acct-2"),
			first.Authorization().Grantor(),
			first.Authorization().Grantee(),
			first.Authorization().Channel(),
			first.Authorization().Scope(),
			first.Authorization().Effective(),
			domain.ChannelAccountTechnicallyAvailable,
			domain.ChannelAccountBusinessAuthorized,
			time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatalf("publish elsewhere: %v", err)
		}
		if _, err := first.Succeed(elsewhere); !errors.Is(err, domain.ErrInvalidChannelAccountUseRegistration) {
			t.Fatalf("error = %v, want ErrInvalidChannelAccountUseRegistration；换掉账号的修订入了册", err)
		}
	})
}
