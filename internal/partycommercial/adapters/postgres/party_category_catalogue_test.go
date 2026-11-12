package postgres_test

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对真实 PostgreSQL 证业务参与方读口的类别、关系段数、按类别筛与分面计数，以及参与方关系册按双方标识筛
// （ADR-0378；票 business-party-redesign/08）。类别是领域判定 domain.PartyCategoriesAt 的 SQL 镜像：同一批登记既写进库
// 让读面去答，又原样交给领域判定去判，两边逐家对照。夹具里的租户、标识、名称、依据与范围都是 SYN- 合成值。

// categoryFacts 是判一家单位的类别要看的全部登记，各取最新修订：它作为持有方的各段关系、以它为所属客户参与方的
// 各客户账户、钉着它的各责任法人。读面看到的是库里的这些行，领域判定看到的是这里的这些对象。
type categoryFacts struct {
	held     []domain.PartyRelationshipRegistration
	accounts []domain.CustomerAccountRegistration
	entities []domain.LegalEntityRegistration
}

func (facts categoryFacts) judged(at time.Time) []string {
	held := make([]domain.PartyRelationship, 0, len(facts.held))
	for _, registration := range facts.held {
		held = append(held, registration.Relationship())
	}
	accounts := make([]domain.IdentityLifecycle, 0, len(facts.accounts))
	for _, registration := range facts.accounts {
		accounts = append(accounts, registration.Lifecycle())
	}
	entities := make([]domain.IdentityLifecycle, 0, len(facts.entities))
	for _, registration := range facts.entities {
		entities = append(entities, registration.Lifecycle())
	}
	categories := domain.PartyCategoriesAt(held, accounts, entities, at)
	words := make([]string, len(categories))
	for index, category := range categories {
		words[index] = string(category)
	}
	return words
}

func categoryRelationship(
	t *testing.T,
	id, holder, counterparty string,
	role domain.PartyRole,
	approved bool,
) domain.PartyRelationship {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(relationshipStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	relationship, err := domain.NewCandidateRelationship(domain.PartyRelationshipSpec{
		Holder:       pcValue(t, domain.NewPartyID, holder),
		Counterparty: pcValue(t, domain.NewPartyID, counterparty),
		Role:         role,
		Scope:        pcValue(t, domain.NewCommercialScopeReference, "SYN-SCOPE-1"),
		Basis:        pcValue(t, domain.NewRelationshipBasisReference, "SYN-CONTRACT-"+id),
		Effective:    interval,
	})
	if err != nil {
		t.Fatalf("new candidate relationship: %v", err)
	}
	if approved {
		relationship, err = relationship.Approve(pcValue(t, domain.NewApprovalReference, "SYN-APPROVAL-"+id), relationshipStartsAt)
		if err != nil {
			t.Fatalf("approve relationship: %v", err)
		}
	}
	return relationship
}

func categoryRelationshipRegistration(
	t *testing.T, id string, revision int, relationship domain.PartyRelationship,
) domain.PartyRelationshipRegistration {
	t.Helper()
	registration, err := domain.NewPartyRelationshipRegistration(
		pcTenant(t, identityPageTenant), pcValue(t, domain.NewRelationshipID, id), revision, relationship,
	)
	if err != nil {
		t.Fatalf("new relationship registration: %v", err)
	}
	return registration
}

// categoryCatalogue 是灌好夹具的读面，连同每家单位判类别用的登记。
type categoryCatalogue struct {
	tenant  domain.TenantID
	parties func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.BusinessPartyRow], error)
	relate  func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.PartyRelationshipRow], error)
	facts   map[string]*categoryFacts
}

// seedPartyCategories 灌一册够把类别的每条判据都走到的单位。各家是什么类别不在这里写死——由 categoryFacts.judged 交给
// 领域判定去判；这里只摆事实：
//
//	SYN-CAT-A     对我方持有已生效的客户关系
//	SYN-CAT-AA    对我方持有候选的转售关系（标识以 SYN-CAT-A 开头，给「按整个标识筛」做对照）
//	SYN-CAT-B     只有一个未停用的客户账户
//	SYN-CAT-C     候选的客户关系，另有一个客户账户（两条都成立）
//	SYN-CAT-D     只剩一段已撤销的供应商关系；这家单位自己也已停用
//	SYN-CAT-E     候选的供应商关系
//	SYN-CAT-F     只有一个生效时点未到的客户账户
//	SYN-CAT-G     只有一个已停用的客户账户
//	SYN-CAT-H     什么都没有，只被我方的一段关系指为相对方；生效时点未到
//	SYN-CAT-I     只被一个已停用的责任法人钉着
//	SYN-CAT-J     已生效的供应商与承运商代理两段关系
//	SYN-CAT-OURS  被一个未停用的责任法人钉着，是上面各段关系的相对方，自己又对 H 持有已生效的渠道账号持有关系
func seedPartyCategories(t *testing.T) categoryCatalogue {
	t.Helper()
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()
	tenant := pcTenant(t, identityPageTenant)
	facts := map[string]*categoryFacts{}
	of := func(party string) *categoryFacts {
		if facts[party] == nil {
			facts[party] = &categoryFacts{}
		}
		return facts[party]
	}

	saveParty := func(registration domain.BusinessPartyRegistration) {
		of(registration.Party().ID().String())
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveBusinessParty(txCtx, registration)
		})
	}
	saveRelationship := func(registration domain.PartyRelationshipRegistration) {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveRelationship(txCtx, registration)
		})
	}
	// relate 登一段关系并记到持有方名下；revoked 为真时再登一笔撤销，记下的是撤销后的那一笔。
	relate := func(id, holder, counterparty string, role domain.PartyRole, approved, revoked bool) {
		relationship := categoryRelationship(t, id, holder, counterparty, role, approved)
		latest := categoryRelationshipRegistration(t, id, 1, relationship)
		saveRelationship(latest)
		if revoked {
			ended, err := relationship.Revoke(
				pcValue(t, domain.NewRelationshipBasisReference, "SYN-REVOCATION-"+id), relationshipStartsAt.Add(48*time.Hour))
			if err != nil {
				t.Fatalf("revoke %s: %v", id, err)
			}
			latest = categoryRelationshipRegistration(t, id, 2, ended)
			saveRelationship(latest)
		}
		of(holder).held = append(of(holder).held, latest)
	}
	saveAccount := func(registration domain.CustomerAccountRegistration) {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveCustomerAccount(txCtx, registration)
		})
	}
	// account 登一个客户账户并记到所属客户参与方名下；deactivated 为真时再登一笔停用。
	account := func(registration domain.CustomerAccountRegistration, party string, deactivated bool) {
		saveAccount(registration)
		if deactivated {
			retired, err := registration.Deactivate(pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS-DEACT"), identityDeactivateAt)
			if err != nil {
				t.Fatalf("deactivate account: %v", err)
			}
			saveAccount(retired)
			registration = retired
		}
		of(party).accounts = append(of(party).accounts, registration)
	}
	saveEntity := func(registration domain.LegalEntityRegistration) {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveLegalEntity(txCtx, registration)
		})
	}
	// entity 登一个钉着这家单位的责任法人；deactivated 为真时再登一笔停用。
	entity := func(id, party string, deactivated bool) {
		registration := synLegalEntityFixture(t, id, party)
		saveEntity(registration)
		if deactivated {
			retired, err := registration.Deactivate(pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS-DEACT"), identityDeactivateAt)
			if err != nil {
				t.Fatalf("deactivate entity: %v", err)
			}
			saveEntity(retired)
			registration = retired
		}
		of(party).entities = append(of(party).entities, registration)
	}

	for _, id := range []string{"A", "AA", "B", "C", "E", "F", "G", "I", "J", "OURS"} {
		saveParty(synPartyFixture(t, "SYN-CAT-"+id, "SYN 类别单位 "+id, identityEffectiveFrom))
	}
	saveParty(synPartyFixture(t, "SYN-CAT-H", "SYN 类别单位 H", time.Now().UTC().Add(72*time.Hour)))
	retiring := synPartyFixture(t, "SYN-CAT-D", "SYN 类别单位 D", identityEffectiveFrom)
	saveParty(retiring)
	retired, err := retiring.Deactivate(pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS-DEACT"), identityDeactivateAt)
	if err != nil {
		t.Fatalf("deactivate party: %v", err)
	}
	saveParty(retired)

	relate("SYN-CREL-A", "SYN-CAT-A", "SYN-CAT-OURS", domain.CustomerRole, true, false)
	relate("SYN-CREL-AA", "SYN-CAT-AA", "SYN-CAT-OURS", domain.ResellerRole, false, false)
	relate("SYN-CREL-C", "SYN-CAT-C", "SYN-CAT-OURS", domain.CustomerRole, false, false)
	relate("SYN-CREL-D", "SYN-CAT-D", "SYN-CAT-OURS", domain.SupplierRole, true, true)
	relate("SYN-CREL-E", "SYN-CAT-E", "SYN-CAT-OURS", domain.SupplierRole, false, false)
	relate("SYN-CREL-J1", "SYN-CAT-J", "SYN-CAT-OURS", domain.SupplierRole, true, false)
	relate("SYN-CREL-J2", "SYN-CAT-J", "SYN-CAT-OURS", domain.CarrierAgentRole, true, false)
	relate("SYN-CREL-O", "SYN-CAT-OURS", "SYN-CAT-H", domain.AccountHolderRole, true, false)

	account(customerAccountRegistrationFixture(t, identityPageTenant, "SYN-CACC-B", "SYN-CAT-B"), "SYN-CAT-B", false)
	account(customerAccountRegistrationFixture(t, identityPageTenant, "SYN-CACC-C", "SYN-CAT-C"), "SYN-CAT-C", false)
	account(futureCustomerAccountRegistrationFixture(t, identityPageTenant, "SYN-CACC-F", "SYN-CAT-F"), "SYN-CAT-F", false)
	account(customerAccountRegistrationFixture(t, identityPageTenant, "SYN-CACC-G", "SYN-CAT-G"), "SYN-CAT-G", true)

	entity("SYN-CLE-I", "SYN-CAT-I", true)
	entity("SYN-CLE-OURS", "SYN-CAT-OURS", false)

	return categoryCatalogue{
		tenant: tenant,
		parties: func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.BusinessPartyRow], error) {
			return catalogue.ListBusinessParties(ctx, tenant, limit, query)
		},
		relate: func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.PartyRelationshipRow], error) {
			return catalogue.ListPartyRelationships(ctx, tenant, limit, query)
		},
		facts: facts,
	}
}

// Covers: 票 08 完成判据「对照测试」——领域判定与读面 SQL 对同一批登记答出同样的类别。夹具覆盖：只凭在用客户关系是客户、
// 只凭未停用客户账户是客户、两者都有、只剩已撤销的关系不是、候选关系算、生效时点未到的账户算、已停用的账户不算、被未停用
// 的责任法人钉着是我方公司、被已停用的钉着不是、我方公司同时持有角色、只作相对方不得角色、一类都不属于。
func TestBusinessPartyCategoriesMirrorTheDomainJudgment(t *testing.T) {
	seeded := seedPartyCategories(t)
	page, err := seeded.parties(rawPageQuery(t, ports.BusinessPartyPages, "sort=partyId"), 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Rows) != len(seeded.facts) {
		t.Fatalf("读回 %d 家，夹具里有 %d 家", len(page.Rows), len(seeded.facts))
	}
	now := time.Now()
	for _, row := range page.Rows {
		want := seeded.facts[row.PartyID].judged(now)
		if !reflect.DeepEqual(row.Categories, want) {
			t.Errorf("%s 的类别：读面答 %v，领域判 %v", row.PartyID, row.Categories, want)
		}
	}

	// 领域判定自己的期望值在 domain 包的单测里钉着；这里另点几家，免得两边错成一样而对照照过。
	got := map[string][]string{}
	counts := map[string]ports.PartyRelationshipCounts{}
	for _, row := range page.Rows {
		got[row.PartyID] = row.Categories
		counts[row.PartyID] = row.RelationshipCounts
	}
	for party, want := range map[string][]string{
		"SYN-CAT-A":    {"CUSTOMER"},
		"SYN-CAT-C":    {"CUSTOMER"},
		"SYN-CAT-D":    {"NO_ROLE"},
		"SYN-CAT-F":    {"CUSTOMER"},
		"SYN-CAT-G":    {"NO_ROLE"},
		"SYN-CAT-H":    {"NO_ROLE"},
		"SYN-CAT-I":    {"NO_ROLE"},
		"SYN-CAT-J":    {"SUPPLIER", "CARRIER_AGENT"},
		"SYN-CAT-OURS": {"ACCOUNT_HOLDER", "OUR_COMPANY"},
	} {
		if !reflect.DeepEqual(got[party], want) {
			t.Errorf("%s 的类别 = %v, want %v", party, got[party], want)
		}
	}

	// 关系段数只数在用的：已撤销的那一段（D）两侧都不计；候选的另记一格。
	tally := func(live, candidate int) ports.RelationshipTally {
		return ports.RelationshipTally{Live: live, Candidate: candidate}
	}
	for party, want := range map[string]ports.PartyRelationshipCounts{
		"SYN-CAT-A":    {AsHolder: tally(1, 0)},
		"SYN-CAT-C":    {AsHolder: tally(1, 1)},
		"SYN-CAT-D":    {},
		"SYN-CAT-J":    {AsHolder: tally(2, 0)},
		"SYN-CAT-H":    {AsCounterparty: tally(1, 0)},
		"SYN-CAT-OURS": {AsHolder: tally(1, 0), AsCounterparty: tally(6, 3)},
	} {
		if counts[party] != want {
			t.Errorf("%s 的关系段数 = %+v, want %+v", party, counts[party], want)
		}
	}
}

// Covers: 票 08 完成判据「按类别筛出的行与总数一致；categories 与别的维、与 q 组合」。类别是多值维：维内为或，一行的
// 类别里有所给的任一个值即匹配。
func TestBusinessPartyCatalogueFiltersByCategory(t *testing.T) {
	seeded := seedPartyCategories(t)
	assertFilteredPages(t, "业务参与方", ports.BusinessPartyPages, seeded.parties,
		func(row ports.BusinessPartyRow) string { return row.PartyID },
		[]pageCase{
			{"sort=partyId&categories=CUSTOMER", "SYN-CAT-A,SYN-CAT-B,SYN-CAT-C,SYN-CAT-F"},
			{"sort=partyId&categories=SUPPLIER", "SYN-CAT-E,SYN-CAT-J"},
			{"sort=partyId&categories=RESELLER", "SYN-CAT-AA"},
			{"sort=partyId&categories=CUSTOMER&categories=SUPPLIER", "SYN-CAT-A,SYN-CAT-B,SYN-CAT-C,SYN-CAT-E,SYN-CAT-F,SYN-CAT-J"},
			{"sort=partyId&categories=OUR_COMPANY", "SYN-CAT-OURS"},
			{"sort=partyId&categories=ACCOUNT_HOLDER", "SYN-CAT-OURS"},
			{"sort=partyId&categories=CARRIER_AGENT&categories=OUR_COMPANY", "SYN-CAT-J,SYN-CAT-OURS"},
			{"sort=partyId&categories=NO_ROLE", "SYN-CAT-D,SYN-CAT-G,SYN-CAT-H,SYN-CAT-I"},
			{"sort=partyId&categories=NO_ROLE&status=EFFECTIVE", "SYN-CAT-G,SYN-CAT-I"},
			{"sort=partyId&categories=NO_ROLE&status=REGISTERED&status=DEACTIVATED", "SYN-CAT-D,SYN-CAT-H"},
			{"sort=partyId&categories=CUSTOMER&q=SYN-CAT-C", "SYN-CAT-C"},
			{"sort=partyId&categories=AUTHORIZED_DELIVERING_PARTY", ""},
		})
}

// Covers: 票 08 完成判据「游标翻过多页不重不漏」与「拿旧条件的游标换了类别再翻，照 ADR-0144 拒」。
func TestBusinessPartyCategoryPagesTurnWithoutRepeatsOrGaps(t *testing.T) {
	seeded := seedPartyCategories(t)
	conditions := "sort=partyId&categories=CUSTOMER&categories=SUPPLIER"
	var seen []string
	firstNext := ""
	for raw := conditions; ; {
		page, err := seeded.parties(rawPageQuery(t, ports.BusinessPartyPages, raw), 2)
		if err != nil {
			t.Fatalf("list %q: %v", raw, err)
		}
		if page.Total != 6 {
			t.Fatalf("%q 的总数 = %d, want 6", raw, page.Total)
		}
		for _, row := range page.Rows {
			seen = append(seen, row.PartyID)
		}
		if page.Next == "" {
			break
		}
		if firstNext == "" {
			firstNext = page.Next
		}
		raw = conditions + "&after=" + url.QueryEscape(page.Next)
	}
	want := []string{"SYN-CAT-A", "SYN-CAT-B", "SYN-CAT-C", "SYN-CAT-E", "SYN-CAT-F", "SYN-CAT-J"}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("逐页翻过 = %v, want %v", seen, want)
	}

	// 这一份游标是在「客户或供应商」之下造的：换成别的类别再带上它，条件对不上，接入口拒。
	_, err := ports.BusinessPartyPages.DecodeRaw("sort=partyId&categories=NO_ROLE&after=" + url.QueryEscape(firstNext))
	var malformed *cataloguepage.MalformedQuery
	if !errors.As(err, &malformed) {
		t.Fatalf("换了类别还带旧游标：err = %v, want *MalformedQuery", err)
	}
}

// facetOf 取答复里某一维的分面，排成「值 → 行数」。
func facetOf(t *testing.T, facets cataloguepage.Facets, dimension string) map[string]int64 {
	t.Helper()
	cells, present := facets[dimension]
	if !present {
		t.Fatalf("答复里没有 %s 的分面：%v", dimension, facets)
	}
	counts := make(map[string]int64, len(cells))
	for _, cell := range cells {
		counts[cell.Value] = cell.Count
	}
	return counts
}

// Covers: 票 08 完成判据「分面各格之和与逐格去筛得到的总数逐一对上」，以及 ADR-0378 决定五的几句：每一维照词表列出
// 每个值、为零的也列；条件取 q 与其余各维、不含该维自己；多值维一行记进多格。
func TestBusinessPartyFacetsAgreeWithFilteringEachValue(t *testing.T) {
	seeded := seedPartyCategories(t)
	list := func(raw string) cataloguepage.Result[ports.BusinessPartyRow] {
		t.Helper()
		page, err := seeded.parties(rawPageQuery(t, ports.BusinessPartyPages, raw), 50)
		if err != nil {
			t.Fatalf("list %q: %v", raw, err)
		}
		return page
	}
	join := func(conditions, extra string) string {
		if conditions == "" {
			return extra
		}
		return conditions + "&" + extra
	}

	// 每一组条件下：某一维每个值上的数，等于在「其余条件」之上只筛这一个值得到的总数。
	for _, tc := range []struct{ conditions, withoutStatus, withoutCategories string }{
		{"", "", ""},
		{"status=EFFECTIVE", "", "status=EFFECTIVE"},
		{"categories=NO_ROLE", "categories=NO_ROLE", ""},
		{"categories=CUSTOMER&categories=SUPPLIER&status=EFFECTIVE&q=SYN-CAT", "categories=CUSTOMER&categories=SUPPLIER&q=SYN-CAT", "status=EFFECTIVE&q=SYN-CAT"},
	} {
		facets := list(tc.conditions).Facets
		statuses := facetOf(t, facets, "status")
		if len(statuses) != 3 {
			t.Fatalf("%q 的 status 分面 = %v, want 词表上的三个值都在", tc.conditions, statuses)
		}
		for value, count := range statuses {
			if total := list(join(tc.withoutStatus, "status="+value)).Total; total != count {
				t.Errorf("%q：status=%s 的分面是 %d，逐格去筛得 %d", tc.conditions, value, count, total)
			}
		}
		categories := facetOf(t, facets, "categories")
		if len(categories) != len(domain.PartyCategoryWords()) {
			t.Fatalf("%q 的 categories 分面 = %v, want 词表上的每个值都在", tc.conditions, categories)
		}
		for value, count := range categories {
			if total := list(join(tc.withoutCategories, "categories="+value)).Total; total != count {
				t.Errorf("%q：categories=%s 的分面是 %d，逐格去筛得 %d", tc.conditions, value, count, total)
			}
		}
	}

	// 另点几格的数：不带条件时十二家里客户四家、无角色四家；J 同时是供应商与承运商代理，两格各记一次。
	whole := facetOf(t, list("").Facets, "categories")
	for value, want := range map[string]int64{
		"CUSTOMER": 4, "SUPPLIER": 2, "CARRIER_AGENT": 1, "RESELLER": 1, "ACCOUNT_HOLDER": 1,
		"AUTHORIZED_DELIVERING_PARTY": 0, "OUR_COMPANY": 1, "NO_ROLE": 4,
	} {
		if whole[value] != want {
			t.Errorf("不带条件时 categories=%s 的分面 = %d, want %d", value, whole[value], want)
		}
	}
	if got := facetOf(t, list("").Facets, "status"); got["REGISTERED"] != 1 || got["EFFECTIVE"] != 10 || got["DEACTIVATED"] != 1 {
		t.Errorf("不带条件时 status 的分面 = %v, want 已登记 1、已生效 10、已停用 1", got)
	}
	// 选了类别，类别那一维的分面不随之变（条件不含该维自己）；状态那一维随之变。
	if got := facetOf(t, list("categories=NO_ROLE").Facets, "categories"); !reflect.DeepEqual(got, whole) {
		t.Errorf("筛了无角色之后 categories 的分面 = %v, want 与不带条件时相同 %v", got, whole)
	}
	if got := facetOf(t, list("categories=NO_ROLE").Facets, "status"); got["REGISTERED"] != 1 || got["EFFECTIVE"] != 2 || got["DEACTIVATED"] != 1 {
		t.Errorf("筛了无角色之后 status 的分面 = %v, want 已登记 1、已生效 2、已停用 1", got)
	}
}

// Covers: ADR-0378 决定六——参与方关系册按持有方、相对方的整个标识筛（q 是包含匹配，会把标识互为前缀的别家带出来），
// 并给 status 的分面。
func TestPartyRelationshipCatalogueFiltersByEitherPartyAndCountsStatuses(t *testing.T) {
	seeded := seedPartyCategories(t)
	assertFilteredPages(t, "参与方关系", ports.PartyRelationshipPages, seeded.relate,
		func(row ports.PartyRelationshipRow) string { return row.RelationshipID },
		[]pageCase{
			{"sort=relationshipId&holderId=SYN-CAT-J", "SYN-CREL-J1,SYN-CREL-J2"},
			{"sort=relationshipId&holderId=SYN-CAT-A", "SYN-CREL-A"},
			{"sort=relationshipId&q=SYN-CAT-A", "SYN-CREL-A,SYN-CREL-AA"},
			{"sort=relationshipId&holderId=SYN-CAT-A&holderId=SYN-CAT-AA", "SYN-CREL-A,SYN-CREL-AA"},
			{"sort=relationshipId&counterpartyId=SYN-CAT-OURS",
				"SYN-CREL-A,SYN-CREL-AA,SYN-CREL-C,SYN-CREL-D,SYN-CREL-E,SYN-CREL-J1,SYN-CREL-J2"},
			{"sort=relationshipId&counterpartyId=SYN-CAT-OURS&status=CANDIDATE", "SYN-CREL-AA,SYN-CREL-C,SYN-CREL-E"},
			{"sort=relationshipId&holderId=SYN-CAT-OURS", "SYN-CREL-O"},
			{"sort=relationshipId&holderId=SYN-CAT-J&counterpartyId=SYN-CAT-OURS&role=SUPPLIER", "SYN-CREL-J1"},
			{"sort=relationshipId&holderId=SYN-CAT-H", ""},
		})

	facet := func(raw string) map[string]int64 {
		t.Helper()
		page, err := seeded.relate(rawPageQuery(t, ports.PartyRelationshipPages, raw), 50)
		if err != nil {
			t.Fatalf("list %q: %v", raw, err)
		}
		return facetOf(t, page.Facets, "status")
	}
	want := func(candidate, effective, revoked int64) map[string]int64 {
		return map[string]int64{"CANDIDATE": candidate, "EFFECTIVE": effective, "EXPIRED": 0, "REVOKED": revoked, "SUPERSEDED": 0}
	}
	if got := facet(""); !reflect.DeepEqual(got, want(3, 4, 1)) {
		t.Errorf("不带条件时 status 的分面 = %v, want %v", got, want(3, 4, 1))
	}
	// 状态那一维自己不进条件：筛了候选，分面照旧是整册的；按角色筛了，分面随之变。
	if got := facet("status=CANDIDATE"); !reflect.DeepEqual(got, want(3, 4, 1)) {
		t.Errorf("筛了候选之后 status 的分面 = %v, want %v", got, want(3, 4, 1))
	}
	if got := facet("role=SUPPLIER"); !reflect.DeepEqual(got, want(1, 1, 1)) {
		t.Errorf("筛了供应商之后 status 的分面 = %v, want %v", got, want(1, 1, 1))
	}
	if got := facet("counterpartyId=SYN-CAT-H"); !reflect.DeepEqual(got, want(0, 1, 0)) {
		t.Errorf("按相对方筛之后 status 的分面 = %v, want %v", got, want(0, 1, 0))
	}
}
