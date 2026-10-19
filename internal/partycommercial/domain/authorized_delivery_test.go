package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证授权交付关系在领域里的两半：适用范围怎样写进 CommercialScopeReference，以及授权交付判断的三格
// （PC CONTEXT「参与方关系」；CONTEXT-MAP `party-commercial → node-operations`；ADR-0212 决定一、三、九）。

var (
	deliveryFrom    = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	deliveryTo      = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	deliveryOpened  = time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	deliveryTenant  = "SYN-TENANT-01"
	deliveryAccount = "SYN-ACCOUNT-01"
	deliveryOwner   = "SYN-PARTY-SHIPPER-01"
	deliverer       = "SYN-PARTY-DELIVERER-01"
	deliveryNode    = "SYN-NODE-SHA-HUB"
)

func deliveryScope(t *testing.T, account, node string) domain.AuthorizedDeliveryScope {
	t.Helper()
	var nodeRef *domain.LogisticsNodeReference
	if node != "" {
		ref := commercialValue(t, domain.NewLogisticsNodeReference, node)
		nodeRef = &ref
	}
	scope, err := domain.NewAuthorizedDeliveryScope(commercialValue(t, domain.NewCustomerAccountID, account), nodeRef)
	if err != nil {
		t.Fatalf("new authorized delivery scope: %v", err)
	}
	return scope
}

func deliverySpec(t *testing.T, holder, counterparty string, scope domain.CommercialScopeReference) domain.PartyRelationshipSpec {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(deliveryFrom, deliveryTo)
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	return domain.PartyRelationshipSpec{
		Holder:       commercialValue(t, domain.NewPartyID, holder),
		Counterparty: commercialValue(t, domain.NewPartyID, counterparty),
		Role:         domain.AuthorizedDeliveringPartyRole,
		Scope:        scope,
		Basis:        commercialValue(t, domain.NewRelationshipBasisReference, "SYN-DELIVERY-MANDATE-01"),
		Effective:    interval,
	}
}

func candidateDelivery(t *testing.T, holder, counterparty, account, node string) domain.PartyRelationship {
	t.Helper()
	relationship, err := domain.NewCandidateRelationship(
		deliverySpec(t, holder, counterparty, deliveryScope(t, account, node).Reference()),
	)
	if err != nil {
		t.Fatalf("new candidate delivery relationship: %v", err)
	}
	return relationship
}

func effectiveDelivery(t *testing.T, holder, counterparty, account, node string) domain.PartyRelationship {
	t.Helper()
	relationship, err := candidateDelivery(t, holder, counterparty, account, node).
		Approve(commercialValue(t, domain.NewApprovalReference, "SYN-APPROVAL-DELIVERY-01"), deliveryFrom)
	if err != nil {
		t.Fatalf("approve delivery relationship: %v", err)
	}
	return relationship
}

func deliveryRegistration(t *testing.T, tenant, id string, relationship domain.PartyRelationship) domain.PartyRelationshipRegistration {
	t.Helper()
	registration, err := domain.NewPartyRelationshipRegistration(
		commercialValue(t, domain.NewTenantID, tenant),
		commercialValue(t, domain.NewRelationshipID, id),
		1,
		relationship,
	)
	if err != nil {
		t.Fatalf("new relationship registration: %v", err)
	}
	return registration
}

func deliveryAccountOf(t *testing.T, tenant, account, owner string) domain.CustomerAccount {
	t.Helper()
	built, err := domain.RehydrateCustomerAccount(
		commercialValue(t, domain.NewTenantID, tenant),
		commercialValue(t, domain.NewCustomerAccountID, account),
		commercialValue(t, domain.NewPartyID, owner),
	)
	if err != nil {
		t.Fatalf("rehydrate customer account: %v", err)
	}
	return built
}

func deliveryQuery(t *testing.T, party, account, node string, at time.Time) domain.AuthorizedDeliveryQuery {
	t.Helper()
	return domain.AuthorizedDeliveryQuery{
		Tenant:          commercialValue(t, domain.NewTenantID, deliveryTenant),
		DeliveringParty: commercialValue(t, domain.NewPartyID, party),
		CustomerAccount: commercialValue(t, domain.NewCustomerAccountID, account),
		Node:            commercialValue(t, domain.NewLogisticsNodeReference, node),
		At:              at,
	}
}

// Covers: 授权交付方是参与方关系封闭集里的一种角色，外部名随 String 出去；登记册与两道登记口都按这个名认它。
func TestAuthorizedDeliveringPartyIsARelationshipRole(t *testing.T) {
	if got := domain.AuthorizedDeliveringPartyRole.String(); got != "AUTHORIZED_DELIVERING_PARTY" {
		t.Fatalf("role name = %q, want AUTHORIZED_DELIVERING_PARTY", got)
	}
	relationship := effectiveDelivery(t, deliverer, deliveryOwner, deliveryAccount, "")
	if !relationship.AppliesAt(deliveryOpened) {
		t.Fatal("effective delivery relationship does not apply inside its interval")
	}
}

// Covers: 适用范围的写法——收到客户账户、可再收到节点，写成一种只有一个拼法的串，读回得到同一个范围；拼法唯一，
// 内容摘要与重放判据才稳。
func TestAuthorizedDeliveryScopeHasOneSpelling(t *testing.T) {
	cases := map[string]struct {
		account, node, want string
	}{
		"只收到客户账户":    {account: deliveryAccount, want: "customerAccount=SYN-ACCOUNT-01"},
		"再收到节点":      {account: deliveryAccount, node: deliveryNode, want: "customerAccount=SYN-ACCOUNT-01&node=SYN-NODE-SHA-HUB"},
		"标识自带分隔符照转义": {account: "A&B=C", node: "N/1 2", want: "customerAccount=A%26B%3DC&node=N%2F1+2"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reference := deliveryScope(t, tc.account, tc.node).Reference()
			if got := reference.String(); got != tc.want {
				t.Fatalf("reference = %q, want %q", got, tc.want)
			}
			parsed, err := domain.ParseAuthorizedDeliveryScope(reference)
			if err != nil {
				t.Fatalf("parse %q: %v", reference, err)
			}
			if got := parsed.CustomerAccount().String(); got != tc.account {
				t.Fatalf("customer account = %q, want %q", got, tc.account)
			}
			node, narrowed := parsed.Node()
			if narrowed != (tc.node != "") || (narrowed && node.String() != tc.node) {
				t.Fatalf("node = %q/%v, want %q", node, narrowed, tc.node)
			}
		})
	}
}

// Covers: 不是那一种拼法的串一律拒——换序、重复、缺账户、空值、多键、未转义的不透明范围；拒在构造门，
// 而不是留到判断时才读不懂。
func TestAuthorizedDeliveryScopeRejectsOtherSpellings(t *testing.T) {
	for _, raw := range []string{
		"SYN-SCOPE-01",
		"node=SYN-NODE-SHA-HUB&customerAccount=SYN-ACCOUNT-01",
		"customerAccount=SYN-ACCOUNT-01&customerAccount=SYN-ACCOUNT-02",
		"node=SYN-NODE-SHA-HUB",
		"customerAccount=",
		"customerAccount=SYN-ACCOUNT-01&node=",
		"customerAccount=SYN-ACCOUNT-01&channel=SYN-CH-01",
		"customerAccount=SYN%2DACCOUNT%2D01",
		"customerAccount=+",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := domain.ParseAuthorizedDeliveryScope(commercialValue(t, domain.NewCommercialScopeReference, raw))
			if !errors.Is(err, domain.ErrInvalidAuthorizedDeliveryScope) {
				t.Fatalf("error = %v, want ErrInvalidAuthorizedDeliveryScope", err)
			}
		})
	}
}

// Covers: 授权交付方关系的范围必须是那种写法，构造门就拒；别的角色的范围照旧是不透明引用，不受这一格约束。
func TestAuthorizedDeliveryRelationshipNeedsAStructuredScope(t *testing.T) {
	opaque := commercialValue(t, domain.NewCommercialScopeReference, "SYN-SCOPE-01")
	if _, err := domain.NewCandidateRelationship(deliverySpec(t, deliverer, deliveryOwner, opaque)); !errors.Is(err, domain.ErrInvalidPartyRelationship) {
		t.Fatalf("opaque scope error = %v, want ErrInvalidPartyRelationship", err)
	}

	customer := deliverySpec(t, deliveryOwner, "SYN-PARTY-OPERATOR-01", opaque)
	customer.Role = domain.CustomerRole
	if _, err := domain.NewCandidateRelationship(customer); err != nil {
		t.Fatalf("customer relationship with opaque scope: %v", err)
	}

	relationship := candidateDelivery(t, deliverer, deliveryOwner, deliveryAccount, deliveryNode)
	scope, ok := relationship.AuthorizedDeliveryScope()
	if !ok {
		t.Fatal("authorized delivery relationship carries no delivery scope")
	}
	if scope.CustomerAccount().String() != deliveryAccount {
		t.Fatalf("scope account = %q, want %q", scope.CustomerAccount(), deliveryAccount)
	}
	if _, ok := effectiveRelationship(t, "party-1", "operator-1", domain.CustomerRole).AuthorizedDeliveryScope(); ok {
		t.Fatal("customer relationship answered a delivery scope")
	}
}

// Covers: 完成判据「领域用例」——本人答是；生效关系覆盖答是；关系未生效、已到期、已撤销、范围不覆盖这个客户账户或节点
// 都答否；本租户空册答未配置；别的租户的关系不算。另补：已替代、相对方不是账户所属参与方都答否，账户查无时
// 不能是本人。
func TestAuthorizedDeliveryJudgment(t *testing.T) {
	account := deliveryAccountOf(t, deliveryTenant, deliveryAccount, deliveryOwner)
	held := func(relationships ...domain.PartyRelationship) []domain.PartyRelationshipRegistration {
		registrations := make([]domain.PartyRelationshipRegistration, 0, len(relationships))
		for index, relationship := range relationships {
			registrations = append(registrations, deliveryRegistration(t, deliveryTenant, "SYN-REL-DELIVERY-"+string(rune('A'+index)), relationship))
		}
		return registrations
	}
	revoked, err := effectiveDelivery(t, deliverer, deliveryOwner, deliveryAccount, "").
		Revoke(commercialValue(t, domain.NewRelationshipBasisReference, "SYN-REVOCATION-01"), deliveryFrom.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	expired, err := effectiveDelivery(t, deliverer, deliveryOwner, deliveryAccount, "").Expire(deliveryTo)
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	superseded, err := effectiveDelivery(t, deliverer, deliveryOwner, deliveryAccount, "").
		SupersededBy(commercialValue(t, domain.NewPartyID, "SYN-PARTY-DELIVERER-02"), deliveryFrom.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}

	cases := map[string]struct {
		query    domain.AuthorizedDeliveryQuery
		evidence domain.AuthorizedDeliveryEvidence
		want     domain.AuthorizedDeliveryAnswer
	}{
		"本人送站，空册也答是": {
			query:    deliveryQuery(t, deliveryOwner, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account},
			want:     domain.DeliveryAuthorized,
		},
		"生效关系覆盖客户账户": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(effectiveDelivery(t, deliverer, deliveryOwner, deliveryAccount, "")), TenantConfigured: true},
			want:     domain.DeliveryAuthorized,
		},
		"生效关系覆盖客户账户与开立节点": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(effectiveDelivery(t, deliverer, deliveryOwner, deliveryAccount, deliveryNode)), TenantConfigured: true},
			want:     domain.DeliveryAuthorized,
		},
		"候选关系未生效": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(candidateDelivery(t, deliverer, deliveryOwner, deliveryAccount, "")), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
		"开立时刻早于生效起点": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryFrom.Add(-time.Hour)),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(effectiveDelivery(t, deliverer, deliveryOwner, deliveryAccount, "")), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
		"开立时刻落在区间终点之后": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryTo.Add(time.Hour)),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(effectiveDelivery(t, deliverer, deliveryOwner, deliveryAccount, "")), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
		"已到期": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(expired), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
		"已撤销": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(revoked), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
		"已替代": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(superseded), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
		"范围是别的客户账户": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(effectiveDelivery(t, deliverer, deliveryOwner, "SYN-ACCOUNT-02", "")), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
		"范围收到别的节点": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(effectiveDelivery(t, deliverer, deliveryOwner, deliveryAccount, "SYN-NODE-SZX-GATE")), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
		"关系的持有方是别的交付方": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(effectiveDelivery(t, "SYN-PARTY-DELIVERER-02", deliveryOwner, deliveryAccount, "")), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
		"相对方不是账户所属参与方": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account, Held: held(effectiveDelivery(t, deliverer, "SYN-PARTY-SHIPPER-02", deliveryAccount, "")), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
		"本租户空册": {
			query:    deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: account},
			want:     domain.DeliveryAuthorizationNotConfigured,
		},
		"别的租户的关系不算": {
			query: deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{
				Account:          account,
				Held:             []domain.PartyRelationshipRegistration{deliveryRegistration(t, "SYN-TENANT-02", "SYN-REL-DELIVERY-X", effectiveDelivery(t, deliverer, deliveryOwner, deliveryAccount, ""))},
				TenantConfigured: true,
			},
			want: domain.DeliveryNotAuthorized,
		},
		"账户查无，空册答未配置": {
			query:    deliveryQuery(t, deliveryOwner, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{},
			want:     domain.DeliveryAuthorizationNotConfigured,
		},
		"账户是别的租户的，不能是本人": {
			query:    deliveryQuery(t, deliveryOwner, deliveryAccount, deliveryNode, deliveryOpened),
			evidence: domain.AuthorizedDeliveryEvidence{Account: deliveryAccountOf(t, "SYN-TENANT-02", deliveryAccount, deliveryOwner), TenantConfigured: true},
			want:     domain.DeliveryNotAuthorized,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := domain.JudgeAuthorizedDelivery(tc.query, tc.evidence)
			if err != nil {
				t.Fatalf("judge: %v", err)
			}
			if got != tc.want {
				t.Fatalf("answer = %s, want %s", got, tc.want)
			}
		})
	}
}

// Covers: 判断的输入缺任一格都不形成答案——缺格不是「否」，交回错误让调用方知道问法本身坏了。
func TestAuthorizedDeliveryJudgmentRefusesAnIncompleteQuery(t *testing.T) {
	breaks := map[string]func(*domain.AuthorizedDeliveryQuery){
		"no tenant":  func(query *domain.AuthorizedDeliveryQuery) { query.Tenant = domain.TenantID{} },
		"no party":   func(query *domain.AuthorizedDeliveryQuery) { query.DeliveringParty = domain.PartyID{} },
		"no account": func(query *domain.AuthorizedDeliveryQuery) { query.CustomerAccount = domain.CustomerAccountID{} },
		"no node":    func(query *domain.AuthorizedDeliveryQuery) { query.Node = domain.LogisticsNodeReference{} },
		"no instant": func(query *domain.AuthorizedDeliveryQuery) { query.At = time.Time{} },
	}
	for name, breakQuery := range breaks {
		t.Run(name, func(t *testing.T) {
			query := deliveryQuery(t, deliverer, deliveryAccount, deliveryNode, deliveryOpened)
			breakQuery(&query)
			if err := query.Validate(); !errors.Is(err, domain.ErrInvalidAuthorizedDeliveryQuery) {
				t.Fatalf("validate error = %v, want ErrInvalidAuthorizedDeliveryQuery", err)
			}
			answer, err := domain.JudgeAuthorizedDelivery(query, domain.AuthorizedDeliveryEvidence{})
			if !errors.Is(err, domain.ErrInvalidAuthorizedDeliveryQuery) {
				t.Fatalf("judge error = %v, want ErrInvalidAuthorizedDeliveryQuery", err)
			}
			if answer != domain.AuthorizedDeliveryAnswerInvalid {
				t.Fatalf("answer = %s, want none", answer)
			}
		})
	}
}
