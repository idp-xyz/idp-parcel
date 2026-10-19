package postgres_test

import (
	"context"
	"errors"
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

// 本文件对真实 PostgreSQL 证授权交付判断读口：角色落得进参与方关系册（角色封闭集那条 CHECK），判断按每段关系的
// 最新修订答是、否、未配置，别的租户的关系不算，读不动交回错误。

var (
	deliveryStartsAt = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	deliveryOpenedAt = time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
)

func deliveryRelationship(
	t *testing.T,
	holder, counterparty, account, node string,
	startsAt, endsAt time.Time,
	approved bool,
) domain.PartyRelationship {
	t.Helper()
	var nodeRef *domain.LogisticsNodeReference
	if node != "" {
		ref := pcValue(t, domain.NewLogisticsNodeReference, node)
		nodeRef = &ref
	}
	scope, err := domain.NewAuthorizedDeliveryScope(pcValue(t, domain.NewCustomerAccountID, account), nodeRef)
	if err != nil {
		t.Fatalf("new delivery scope: %v", err)
	}
	interval, err := domain.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	relationship, err := domain.NewCandidateRelationship(domain.PartyRelationshipSpec{
		Holder:       pcValue(t, domain.NewPartyID, holder),
		Counterparty: pcValue(t, domain.NewPartyID, counterparty),
		Role:         domain.AuthorizedDeliveringPartyRole,
		Scope:        scope.Reference(),
		Basis:        pcValue(t, domain.NewRelationshipBasisReference, "SYN-MANDATE-"+holder),
		Effective:    interval,
	})
	if err != nil {
		t.Fatalf("new candidate delivery relationship: %v", err)
	}
	if !approved {
		return relationship
	}
	relationship, err = relationship.Approve(pcValue(t, domain.NewApprovalReference, "SYN-APPROVAL-"+holder), startsAt)
	if err != nil {
		t.Fatalf("approve delivery relationship: %v", err)
	}
	return relationship
}

func saveDeliveryRelationship(
	t *testing.T,
	registrations *adapter.PartyIdentityRegistrations,
	transactor bentoapp.Transactor,
	tenant, id string,
	revision int,
	relationship domain.PartyRelationship,
) {
	t.Helper()
	registration, err := domain.NewPartyRelationshipRegistration(
		pcTenant(t, tenant), pcValue(t, domain.NewRelationshipID, id), revision, relationship,
	)
	if err != nil {
		t.Fatalf("new relationship registration: %v", err)
	}
	if err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := registrations.SaveRelationship(txCtx, registration)
		if err != nil {
			return err
		}
		if outcome != ports.PartyRegistrySaved {
			t.Errorf("save %s r%d outcome = %s", id, revision, outcome)
		}
		return nil
	}); err != nil {
		t.Fatalf("save %s r%d: %v", id, revision, err)
	}
}

func newAuthorizedDeliveryJudgments(t *testing.T, db *bentopg.DB) *adapter.AuthorizedDeliveryJudgments {
	t.Helper()
	judgments, err := adapter.NewAuthorizedDeliveryJudgments(db)
	if err != nil {
		t.Fatalf("构造授权交付判断读口：%v", err)
	}
	return judgments
}

func authorizedDeliveryQuery(t *testing.T, tenant, party, account, node string, at time.Time) domain.AuthorizedDeliveryQuery {
	t.Helper()
	return domain.AuthorizedDeliveryQuery{
		Tenant:          pcTenant(t, tenant),
		DeliveringParty: pcValue(t, domain.NewPartyID, party),
		CustomerAccount: pcValue(t, domain.NewCustomerAccountID, account),
		Node:            pcValue(t, domain.NewLogisticsNodeReference, node),
		At:              at,
	}
}

// Covers: 完成判据「领域与真库用例」的真库半边——本人答是；生效关系覆盖答是；关系未生效、已到期、已撤销、范围不覆盖
// 这个客户账户或节点，都答否；本租户空册答未配置；别的租户的关系不算。撤销是同一段关系的下一修订，判断按最新修订答。
func TestAuthorizedDeliveryJudgmentAgainstTheRelationshipRegister(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrations, err := adapter.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatalf("构造身份登记册：%v", err)
	}
	transactor := db.Transactor()
	judgments := newAuthorizedDeliveryJudgments(t, db)
	ctx := t.Context()

	const (
		tenant  = "SYN-TENANT-01"
		account = "SYN-ACCOUNT-01"
		owner   = "SYN-PARTY-SHIPPER-01"
		hub     = "SYN-NODE-SHA-HUB"
		gate    = "SYN-NODE-SZX-GATE"
	)
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveCustomerAccount(txCtx, customerAccountRegistrationFixture(t, tenant, account, owner))
	})

	judge := func(tenant, party, node string, at time.Time) domain.AuthorizedDeliveryAnswer {
		t.Helper()
		answer, err := judgments.JudgeAuthorizedDelivery(ctx, authorizedDeliveryQuery(t, tenant, party, account, node, at))
		if err != nil {
			t.Fatalf("judge %s/%s: %v", tenant, party, err)
		}
		return answer
	}

	if got := judge(tenant, "SYN-PARTY-DELIVERER-01", hub, deliveryOpenedAt); got != domain.DeliveryAuthorizationNotConfigured {
		t.Fatalf("空册 = %s, want NOT_CONFIGURED", got)
	}
	if got := judge(tenant, owner, hub, deliveryOpenedAt); got != domain.DeliveryAuthorized {
		t.Fatalf("空册时本人 = %s, want AUTHORIZED", got)
	}

	// 别的租户登了一条覆盖同名账户的关系：本租户仍是空册，那条不算。
	saveDeliveryRelationship(t, registrations, transactor, "SYN-TENANT-02", "SYN-REL-DELIVERY-T2", 1,
		deliveryRelationship(t, "SYN-PARTY-DELIVERER-01", owner, account, "", deliveryStartsAt, time.Time{}, true))
	if got := judge(tenant, "SYN-PARTY-DELIVERER-01", hub, deliveryOpenedAt); got != domain.DeliveryAuthorizationNotConfigured {
		t.Fatalf("别的租户的关系让本租户不再是空册：%s, want NOT_CONFIGURED", got)
	}

	saveDeliveryRelationship(t, registrations, transactor, tenant, "SYN-REL-DELIVERY-01", 1,
		deliveryRelationship(t, "SYN-PARTY-DELIVERER-01", owner, account, "", deliveryStartsAt, time.Time{}, true))
	saveDeliveryRelationship(t, registrations, transactor, tenant, "SYN-REL-DELIVERY-02", 1,
		deliveryRelationship(t, "SYN-PARTY-DELIVERER-02", owner, account, "", deliveryStartsAt, time.Time{}, false))
	revokedLive := deliveryRelationship(t, "SYN-PARTY-DELIVERER-03", owner, account, "", deliveryStartsAt, time.Time{}, true)
	saveDeliveryRelationship(t, registrations, transactor, tenant, "SYN-REL-DELIVERY-03", 1, revokedLive)
	revoked, err := revokedLive.Revoke(pcValue(t, domain.NewRelationshipBasisReference, "SYN-REVOCATION-03"), deliveryStartsAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	saveDeliveryRelationship(t, registrations, transactor, tenant, "SYN-REL-DELIVERY-03", 2, revoked)
	saveDeliveryRelationship(t, registrations, transactor, tenant, "SYN-REL-DELIVERY-04", 1,
		deliveryRelationship(t, "SYN-PARTY-DELIVERER-04", owner, account, "", deliveryStartsAt, deliveryStartsAt.Add(30*24*time.Hour), true))
	saveDeliveryRelationship(t, registrations, transactor, tenant, "SYN-REL-DELIVERY-05", 1,
		deliveryRelationship(t, "SYN-PARTY-DELIVERER-05", owner, "SYN-ACCOUNT-02", "", deliveryStartsAt, time.Time{}, true))
	saveDeliveryRelationship(t, registrations, transactor, tenant, "SYN-REL-DELIVERY-06", 1,
		deliveryRelationship(t, "SYN-PARTY-DELIVERER-06", owner, account, hub, deliveryStartsAt, time.Time{}, true))
	saveDeliveryRelationship(t, registrations, transactor, tenant, "SYN-REL-DELIVERY-07", 1,
		deliveryRelationship(t, "SYN-PARTY-DELIVERER-07", owner, account, "", deliveryOpenedAt.Add(24*time.Hour), time.Time{}, true))

	cases := []struct {
		name  string
		party string
		node  string
		want  domain.AuthorizedDeliveryAnswer
	}{
		{name: "生效关系覆盖客户账户", party: "SYN-PARTY-DELIVERER-01", node: hub, want: domain.DeliveryAuthorized},
		{name: "候选关系未生效", party: "SYN-PARTY-DELIVERER-02", node: hub, want: domain.DeliveryNotAuthorized},
		{name: "最新修订已撤销", party: "SYN-PARTY-DELIVERER-03", node: hub, want: domain.DeliveryNotAuthorized},
		{name: "区间已到期", party: "SYN-PARTY-DELIVERER-04", node: hub, want: domain.DeliveryNotAuthorized},
		{name: "范围是别的客户账户", party: "SYN-PARTY-DELIVERER-05", node: hub, want: domain.DeliveryNotAuthorized},
		{name: "范围收到的节点就是开立节点", party: "SYN-PARTY-DELIVERER-06", node: hub, want: domain.DeliveryAuthorized},
		{name: "范围收到的节点不是开立节点", party: "SYN-PARTY-DELIVERER-06", node: gate, want: domain.DeliveryNotAuthorized},
		{name: "开立时刻早于生效起点", party: "SYN-PARTY-DELIVERER-07", node: hub, want: domain.DeliveryNotAuthorized},
		{name: "没有关系的交付方", party: "SYN-PARTY-DELIVERER-99", node: hub, want: domain.DeliveryNotAuthorized},
		{name: "本人", party: owner, node: gate, want: domain.DeliveryAuthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := judge(tenant, tc.party, tc.node, deliveryOpenedAt); got != tc.want {
				t.Fatalf("answer = %s, want %s", got, tc.want)
			}
		})
	}

	// 租户二那条关系在租户二自己的判断里才算：租户二还没登这个账户时相对方核不上，答否；登了就答是。
	if got := judge("SYN-TENANT-02", "SYN-PARTY-DELIVERER-01", hub, deliveryOpenedAt); got != domain.DeliveryNotAuthorized {
		t.Fatalf("租户二没登账户，相对方核不上 = %s, want NOT_AUTHORIZED", got)
	}
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveCustomerAccount(txCtx, customerAccountRegistrationFixture(t, "SYN-TENANT-02", account, owner))
	})
	if got := judge("SYN-TENANT-02", "SYN-PARTY-DELIVERER-01", hub, deliveryOpenedAt); got != domain.DeliveryAuthorized {
		t.Fatalf("租户二自己的关系 = %s, want AUTHORIZED", got)
	}
}

// Covers: 读不动交回错误，不折成任何一种答复；问法缺格同样交回错误、不碰库。
func TestAuthorizedDeliveryJudgmentFailsLoudly(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	judgments := newAuthorizedDeliveryJudgments(t, db)

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	answer, err := judgments.JudgeAuthorizedDelivery(cancelled,
		authorizedDeliveryQuery(t, "SYN-TENANT-01", "SYN-PARTY-DELIVERER-01", "SYN-ACCOUNT-01", "SYN-NODE-SHA-HUB", deliveryOpenedAt))
	if err == nil || answer != domain.AuthorizedDeliveryAnswerInvalid {
		t.Fatalf("读不动 = (%s, %v), want 无答复与错误", answer, err)
	}

	incomplete := authorizedDeliveryQuery(t, "SYN-TENANT-01", "SYN-PARTY-DELIVERER-01", "SYN-ACCOUNT-01", "SYN-NODE-SHA-HUB", time.Time{})
	answer, err = judgments.JudgeAuthorizedDelivery(t.Context(), incomplete)
	if !errors.Is(err, domain.ErrInvalidAuthorizedDeliveryQuery) || answer != domain.AuthorizedDeliveryAnswerInvalid {
		t.Fatalf("缺开立时刻 = (%s, %v), want ErrInvalidAuthorizedDeliveryQuery", answer, err)
	}

	if _, err := adapter.NewAuthorizedDeliveryJudgments(nil); err == nil {
		t.Fatal("nil db 构造成功，漏装要在构造期暴露")
	}
}
