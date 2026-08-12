package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

var (
	relationshipFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	relationshipTo   = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	withinRelation   = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
)

func party(t *testing.T, id, name string) domain.BusinessParty {
	t.Helper()
	return partyInTenant(t, "tenant-1", id, name)
}

func partyInTenant(t *testing.T, tenant, id, name string) domain.BusinessParty {
	t.Helper()
	built, err := domain.NewBusinessParty(
		commercialValue(t, domain.NewTenantID, tenant),
		commercialValue(t, domain.NewPartyID, id),
		commercialValue(t, domain.NewPartyName, name),
	)
	if err != nil {
		t.Fatalf("new business party: %v", err)
	}
	return built
}

func candidateRelationship(t *testing.T, holder, counterparty string, role domain.PartyRole) domain.PartyRelationship {
	t.Helper()
	built, err := domain.NewCandidateRelationship(domain.PartyRelationshipSpec{
		Holder:       commercialValue(t, domain.NewPartyID, holder),
		Counterparty: commercialValue(t, domain.NewPartyID, counterparty),
		Role:         role,
		Scope:        commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
		Basis:        commercialValue(t, domain.NewRelationshipBasisReference, "basis-1"),
		Effective:    mustInterval(t),
	})
	if err != nil {
		t.Fatalf("new candidate relationship: %v", err)
	}
	return built
}

func effectiveRelationship(t *testing.T, holder, counterparty string, role domain.PartyRole) domain.PartyRelationship {
	t.Helper()
	live, err := candidateRelationship(t, holder, counterparty, role).
		Approve(commercialValue(t, domain.NewApprovalReference, "approval-rel"), relationshipFrom)
	if err != nil {
		t.Fatalf("approve relationship: %v", err)
	}
	return live
}

// Covers: CONTEXT 业务参与方「不因一次交易角色而被永久定义为货主、代理商或承运商」—
// 角色只存在于关系里，参与方本身不带分类字段。
func TestABusinessPartyCarriesNoRole(t *testing.T) {
	partyType := reflect.TypeOf(domain.BusinessParty{})
	for index := 0; index < partyType.NumField(); index++ {
		name := strings.ToLower(partyType.Field(index).Name)
		for _, forbidden := range []string{"role", "type", "kind", "category", "classification"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("BusinessParty carries %q, which classifies a party permanently",
					partyType.Field(index).Name)
			}
		}
	}
}

// Covers: CONTEXT「同一参与方可以在不同交易中承担不同角色」— 同一参与方可以同时是一个
// 关系里的客户与另一个关系里的供应商。
func TestOnePartyHoldsDifferentRolesAtTheSameTime(t *testing.T) {
	asCustomer := effectiveRelationship(t, "party-1", "operator-1", domain.CustomerRole)
	asSupplier := effectiveRelationship(t, "party-1", "operator-1", domain.SupplierRole)

	for _, relationship := range []domain.PartyRelationship{asCustomer, asSupplier} {
		if !relationship.AppliesAt(withinRelation) {
			t.Fatalf("relationship %q does not apply inside its interval", relationship.Role())
		}
	}
	if asCustomer.Role() == asSupplier.Role() {
		t.Fatal("two relationships collapsed into one role")
	}
}

// Covers: CONTEXT 候选关系 → 已生效「双方身份、角色、方向、范围、依据和有效期间完整，
// 并通过适用批准」— 缺任一项都建不成候选，未批准的候选不生效。
func TestRelationshipNeedsCompleteContentAndApproval(t *testing.T) {
	incomplete := map[string]func(*domain.PartyRelationshipSpec){
		"no holder":       func(spec *domain.PartyRelationshipSpec) { spec.Holder = domain.PartyID{} },
		"no counterparty": func(spec *domain.PartyRelationshipSpec) { spec.Counterparty = domain.PartyID{} },
		"no role":         func(spec *domain.PartyRelationshipSpec) { spec.Role = domain.PartyRoleInvalid },
		"no scope":        func(spec *domain.PartyRelationshipSpec) { spec.Scope = domain.CommercialScopeReference{} },
		"no basis":        func(spec *domain.PartyRelationshipSpec) { spec.Basis = domain.RelationshipBasisReference{} },
	}
	for name, breakSpec := range incomplete {
		t.Run(name, func(t *testing.T) {
			spec := domain.PartyRelationshipSpec{
				Holder:       commercialValue(t, domain.NewPartyID, "party-1"),
				Counterparty: commercialValue(t, domain.NewPartyID, "operator-1"),
				Role:         domain.CustomerRole,
				Scope:        commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
				Basis:        commercialValue(t, domain.NewRelationshipBasisReference, "basis-1"),
				Effective:    mustInterval(t),
			}
			breakSpec(&spec)
			if _, err := domain.NewCandidateRelationship(spec); !errors.Is(err, domain.ErrInvalidPartyRelationship) {
				t.Fatalf("error = %v, want ErrInvalidPartyRelationship", err)
			}
		})
	}

	t.Run("a candidate does not support new decisions", func(t *testing.T) {
		candidate := candidateRelationship(t, "party-1", "operator-1", domain.CustomerRole)
		if candidate.AppliesAt(withinRelation) {
			t.Fatal("an unapproved candidate relationship supported a decision")
		}
	})
}

// Covers: CONTEXT「关系撤销、到期或替代只影响其生效边界后的新决定，不删除历史关系或
// 改写既有业务快照」。
func TestEndingARelationshipStopsNewDecisionsButKeepsHistory(t *testing.T) {
	endings := map[string]struct {
		apply func(*testing.T, domain.PartyRelationship) (domain.PartyRelationship, error)
		want  domain.RelationshipStatus
	}{
		"revoked": {
			apply: func(t *testing.T, relationship domain.PartyRelationship) (domain.PartyRelationship, error) {
				t.Helper()
				return relationship.Revoke(commercialValue(t, domain.NewRelationshipBasisReference, "revoke-1"), withinRelation)
			},
			want: domain.RelationshipRevoked,
		},
		"expired": {
			apply: func(t *testing.T, relationship domain.PartyRelationship) (domain.PartyRelationship, error) {
				t.Helper()
				return relationship.Expire(relationshipTo)
			},
			want: domain.RelationshipExpired,
		},
	}

	for name, ending := range endings {
		t.Run(name, func(t *testing.T) {
			live := effectiveRelationship(t, "party-1", "operator-1", domain.CustomerRole)
			ended, err := ending.apply(t, live)
			if err != nil {
				t.Fatalf("end relationship: %v", err)
			}
			if ended.Status() != ending.want {
				t.Fatalf("status = %q, want %q", ended.Status(), ending.want)
			}
			if live.Status() != domain.RelationshipEffective {
				t.Fatal("ending mutated the effective value")
			}
			if ended.Holder() != live.Holder() || ended.Role() != live.Role() || ended.Basis() != live.Basis() {
				t.Fatal("ending rewrote the relationship's parties, role or basis")
			}
			if ended.AppliesAt(withinRelation.Add(time.Hour)) {
				t.Fatal("an ended relationship still supports new decisions")
			}
		})
	}
}

// Covers: CONTEXT「不能仅从名称、品牌或技术账号推断」— 同名不同身份仍是两个参与方。
func TestSameNameIsNotTheSameParty(t *testing.T) {
	first := party(t, "party-1", "同一个名字")
	second := party(t, "party-2", "同一个名字")

	if first.ID() == second.ID() {
		t.Fatal("two parties sharing a name collapsed into one identity")
	}
	if first == second {
		t.Fatal("party equality fell back to the name")
	}
}

// Covers: CONTEXT「货主客户账户必须明确关联其客户参与方；参与方、账户、法人和合同标识
// 不能互相替代」。
func TestCustomerAccountMustNameItsCustomerParty(t *testing.T) {
	customer := party(t, "party-1", "货主")
	account, err := domain.NewCustomerAccount(
		customer.Tenant(),
		commercialValue(t, domain.NewCustomerAccountID, "account-1"),
		customer,
	)
	if err != nil {
		t.Fatalf("new customer account: %v", err)
	}
	if account.CustomerParty().String() != "party-1" {
		t.Fatalf("account customer party = %q", account.CustomerParty())
	}

	if _, err := domain.NewCustomerAccount(
		customer.Tenant(),
		commercialValue(t, domain.NewCustomerAccountID, "account-2"),
		domain.BusinessParty{},
	); !errors.Is(err, domain.ErrInvalidCustomerAccount) {
		t.Fatalf("error = %v; an account exists without naming its customer party", err)
	}
}

// Covers: `AT-PC-001`「建立责任法人和客户参与方关系 → 稳定身份与时态关系分别形成，不把
// 客户账户当法人」。
//
// 责任法人今天仍是 `LegalEntityReference`（无独立法人聚合生命周期）；本条只钉已经存在的
// 半边：客户参与方身份、时态客户关系、货主账户与法人引用三者类型/绑定互不替代。
func TestLegalEntityReferenceAndCustomerPartyAreFormedSeparately(t *testing.T) {
	customerParty := party(t, "party-customer", "货主甲")
	legalParty := party(t, "party-legal", "责任法人乙")
	account, err := domain.NewCustomerAccount(
		customerParty.Tenant(),
		commercialValue(t, domain.NewCustomerAccountID, "account-customer"),
		customerParty,
	)
	if err != nil {
		t.Fatalf("new customer account: %v", err)
	}
	relationship := effectiveRelationship(t, customerParty.ID().String(), "operator-1", domain.CustomerRole)
	legal := commercialValue(t, domain.NewLegalEntityReference, "legal-entity-1")

	if !relationship.AppliesAt(withinRelation) {
		t.Fatal("客户时态关系没有形成生效")
	}
	if relationship.Holder() != customerParty.ID() {
		t.Fatal("关系持有方不是稳定的客户参与方身份")
	}
	if account.CustomerParty() != customerParty.ID() {
		t.Fatal("货主账户没有钉住客户参与方")
	}
	if customerParty.ID() == legalParty.ID() {
		t.Fatal("客户参与方与法人参与方塌成同一身份")
	}
	if reflect.TypeOf(account.ID()) == reflect.TypeOf(legal) {
		t.Fatal("客户账户标识与责任法人引用塌成同一类型，账户就能冒充法人")
	}
	if reflect.TypeOf(customerParty.ID()) == reflect.TypeOf(legal) {
		t.Fatal("参与方标识与责任法人引用塌成同一类型")
	}
}

// Covers: `AT-PC-014` 并列 F（ADR-0041）：参与方与货主账户携带 TenantID；账户不得绑定他租参与方。
// 不搅 AT-PC-028 的 Caller 探测轴。
func TestCustomerAccountRejectsCrossTenantPartyBinding(t *testing.T) {
	home := partyInTenant(t, "tenant-home", "party-home", "本租货主")
	away := partyInTenant(t, "tenant-away", "party-away", "他租货主")

	account, err := domain.NewCustomerAccount(
		home.Tenant(),
		commercialValue(t, domain.NewCustomerAccountID, "account-home"),
		home,
	)
	if err != nil {
		t.Fatalf("same-tenant account: %v", err)
	}
	if account.Tenant() != home.Tenant() {
		t.Fatal("账户没有带上租户")
	}
	if home.Tenant() == away.Tenant() {
		t.Fatal("夹具两租户塌成同一个")
	}

	_, err = domain.NewCustomerAccount(
		home.Tenant(),
		commercialValue(t, domain.NewCustomerAccountID, "account-cross"),
		away,
	)
	if !errors.Is(err, domain.ErrCrossTenantCustomerAccount) {
		t.Fatalf("error = %v, want ErrCrossTenantCustomerAccount", err)
	}
	if errors.Is(err, domain.ErrInvalidCustomerAccount) {
		t.Fatal("跨租户绑定被压成了字段不全")
	}
}
