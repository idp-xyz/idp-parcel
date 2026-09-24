package postgres_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// customerAccountQuery 按本册声明解一份查询串，与端点解的是同一份声明；空串即缺省序的第一页。
func customerAccountQuery(t *testing.T, raw string) cataloguepage.Query {
	t.Helper()
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query %q: %v", raw, err)
	}
	query, err := ports.CustomerAccountCatalogue.Decode(values)
	if err != nil {
		t.Fatalf("decode query %q: %v", raw, err)
	}
	return query
}

// 本文件对真实 PostgreSQL 16 证货主客户账户目录读口（票 admin-write-faces/04）：生命周期
// 三格各有实例可显、名称从参与方册左连接且悬空如实缺席、跨租户零行、limit 非正拒。

// futureCustomerAccountRegistrationFixture 造一份生效时点在未来的账户登记：`已登记`那一格要
// 有实例可显，就得有一笔登了但还没到生效时点的账户（判据同 futurePartyRegistrationFixture）。
func futureCustomerAccountRegistrationFixture(t *testing.T, tenant, accountID, partyID string) domain.CustomerAccountRegistration {
	t.Helper()
	account, err := domain.RehydrateCustomerAccount(
		pcTenant(t, tenant),
		pcValue(t, domain.NewCustomerAccountID, accountID),
		pcValue(t, domain.NewPartyID, partyID),
	)
	if err != nil {
		t.Fatalf("rehydrate account: %v", err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(time.Now().UTC().Add(72 * time.Hour))
	if err != nil {
		t.Fatalf("account lifecycle: %v", err)
	}
	registration, err := domain.NewCustomerAccountRegistration(
		account, 1, pcValue(t, domain.NewIdentityBasisReference, "basis-"+accountID), lifecycle,
	)
	if err != nil {
		t.Fatalf("account registration: %v", err)
	}
	return registration
}

// Covers: 票 04 完成判据——身份三级里唯一登记成功却无处可看的那一级，目录上生命周期三格
// 各有实例：未来生效的登了未生效（REGISTERED）、已过生效时点的生效（EFFECTIVE）、带停用两件
// 的停用（DEACTIVATED）。钉法照 TestBusinessPartyCatalogueShowsAllThreeLifecycleCells。
//
// 多钉一格是本册特有的：账户钉在一个参与方身份上、名称从参与方册转写，所以「参与方册查无此人」
// 要如实缺席（HasPartyName 为假）而不是拿空串冒充名称——这一格是写入用例把门失败才会出现的
// 悬空，读面不遮掩。夹具里那个悬空账户直接写库绕过应用层把门，为的正是造出这一格。
func TestCustomerAccountCatalogueShowsAllThreeLifecycleCellsAndTheDanglingParty(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()

	customer := partyRegistrationFixture(t, "tenant-1", "party-cust", "货主客户参与方")
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, customer)
	})

	effective := customerAccountRegistrationFixture(t, "tenant-1", "account-effective", "party-cust")
	retiring := customerAccountRegistrationFixture(t, "tenant-1", "account-retired", "party-cust")
	future := futureCustomerAccountRegistrationFixture(t, "tenant-1", "account-future", "party-cust")
	dangling := customerAccountRegistrationFixture(t, "tenant-1", "account-dangling", "party-nobody")
	for _, registration := range []domain.CustomerAccountRegistration{effective, retiring, future, dangling} {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveCustomerAccount(txCtx, registration)
		})
	}

	deactivated, err := retiring.Deactivate(
		pcValue(t, domain.NewIdentityBasisReference, "basis-deact"), identityDeactivateAt)
	if err != nil {
		t.Fatalf("deactivate account: %v", err)
	}
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveCustomerAccount(txCtx, deactivated)
	})

	page, err := catalogue.ListCustomerAccounts(ctx, pcTenant(t, "tenant-1"), 10, customerAccountQuery(t, ""))
	if err != nil {
		t.Fatalf("list customer accounts: %v", err)
	}
	rows := page.Rows
	if len(rows) != 4 || page.Total != 4 || page.Next != "" {
		t.Fatalf("rows = %d, want 4（三格各一，外加一个悬空参与方的）", len(rows))
	}
	byAccount := make(map[string]ports.CustomerAccountRow, len(rows))
	for _, row := range rows {
		byAccount[row.AccountID] = row
	}

	if got := byAccount["account-effective"]; got.Status != "EFFECTIVE" || got.Revision != 1 ||
		got.CustomerPartyID != "party-cust" || !got.HasPartyName ||
		got.CustomerPartyName != "货主客户参与方" || got.HasDeactivation ||
		got.Basis != "basis-account-effective" || !got.EffectiveFrom.Equal(identityEffectiveFrom) {
		t.Fatalf("已生效那一行 = %+v", got)
	}
	if got := byAccount["account-future"]; got.Status != "REGISTERED" || got.HasDeactivation || !got.HasPartyName {
		t.Fatalf("未来生效那一行 = %+v", got)
	}
	// 停用行：上列的是停用那一笔修订（修订 2），停用两件都在——只有其一的行说不出
	// 「依据什么停用」或「何时起停用」，库上那条 paired 约束守的也是这个。
	retired := byAccount["account-retired"]
	if retired.Status != "DEACTIVATED" || retired.Revision != 2 ||
		!retired.HasDeactivation || retired.DeactivationBasis != "basis-deact" ||
		!retired.DeactivatedAt.Equal(identityDeactivateAt) {
		t.Fatalf("已停用那一行 = %+v", retired)
	}
	// 悬空参与方：账户行照常上列（它是登记册上的事实），名称如实缺席。
	if got := byAccount["account-dangling"]; got.HasPartyName || got.CustomerPartyName != "" ||
		got.CustomerPartyID != "party-nobody" || got.Status != "EFFECTIVE" {
		t.Fatalf("悬空参与方那一行 = %+v", got)
	}

	foreign, err := catalogue.ListCustomerAccounts(ctx, pcTenant(t, "tenant-b"), 10, customerAccountQuery(t, ""))
	if err != nil || len(foreign.Rows) != 0 || foreign.Total != 0 {
		t.Fatalf("跨租户 = (%d 行、共 %d, %v)；隔离边界按 ADR-0003 在 SQL 条件上", len(foreign.Rows), foreign.Total, err)
	}
	if _, err := catalogue.ListCustomerAccounts(ctx, pcTenant(t, "tenant-1"), 0, customerAccountQuery(t, "")); err == nil {
		t.Fatal("limit 为 0 时读面静默答了一页")
	}
}

// Covers: 票 catalogue-read-pagination/02 完成判据「翻页期间插入新登记，已翻过的页不重出、未翻的页不漏行」——ADR-0144
// 否决偏移分页的理由就是这一格：新登的排在最前，偏移会把上一页末尾那行挤进下一页开头。缺省序 -registeredAt 下，
// 游标之后只取严格更早登记的行，翻页中途新登的账户落在已翻过的那头，后面各页照旧、不重不漏。
func TestCustomerAccountCataloguePagesByCursorWithoutRepeatsOrGapsWhileAccountsArrive(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	save := func(accountID string) {
		registration := customerAccountRegistrationFixture(t, "tenant-1", accountID, "SYN-PARTY-01")
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveCustomerAccount(txCtx, registration)
		})
	}
	// 逐笔各一个事务：登记时刻即事务起点，按保存先后递增，缺省序下后存的在前。
	for _, accountID := range []string{"SYN-ACC-1", "SYN-ACC-2", "SYN-ACC-3", "SYN-ACC-4", "SYN-ACC-5"} {
		save(accountID)
	}

	first, err := catalogue.ListCustomerAccounts(ctx, tenant, 2, customerAccountQuery(t, ""))
	if err != nil {
		t.Fatalf("第一页: %v", err)
	}
	if got := accountIDsOf(first.Rows); got != "SYN-ACC-5,SYN-ACC-4" || first.Total != 5 || first.Next == "" {
		t.Fatalf("第一页 = %s（共 %d，next %q）", got, first.Total, first.Next)
	}

	save("SYN-ACC-6")

	second, err := catalogue.ListCustomerAccounts(ctx, tenant, 2, customerAccountQuery(t, "after="+url.QueryEscape(first.Next)))
	if err != nil {
		t.Fatalf("第二页: %v", err)
	}
	if got := accountIDsOf(second.Rows); got != "SYN-ACC-3,SYN-ACC-2" || second.Total != 6 || second.Next == "" {
		t.Fatalf("第二页 = %s（共 %d）；新登的 SYN-ACC-6 该落在已翻过的那头，总数跟着它变", got, second.Total)
	}
	third, err := catalogue.ListCustomerAccounts(ctx, tenant, 2, customerAccountQuery(t, "after="+url.QueryEscape(second.Next)))
	if err != nil {
		t.Fatalf("第三页: %v", err)
	}
	if got := accountIDsOf(third.Rows); got != "SYN-ACC-1" || third.Next != "" {
		t.Fatalf("末页 = %s（next %q），want SYN-ACC-1 且 next 为空", got, third.Next)
	}
}

// Covers: 完成判据「total 随筛选与 q 变」——总数与本页出自同一组条件（ADR-0144 决定五）：状态维内为或、维间为与，
// q 在账户号、客户参与方号与参与方名称上做不分大小写的字面包含；q 里的 % 不当通配符。另钉按账户号升序的次序。
func TestCustomerAccountCatalogueTotalFollowsFiltersAndKeyword(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	for _, party := range []domain.BusinessPartyRegistration{
		partyRegistrationFixture(t, "tenant-1", "SYN-PARTY-A", "SYN 华东货主"),
		partyRegistrationFixture(t, "tenant-1", "SYN-PARTY-B", "SYN 华南货主"),
	} {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveBusinessParty(txCtx, party)
		})
	}
	retiring := customerAccountRegistrationFixture(t, "tenant-1", "SYN-ACC-B2", "SYN-PARTY-B")
	for _, registration := range []domain.CustomerAccountRegistration{
		customerAccountRegistrationFixture(t, "tenant-1", "SYN-ACC-A1", "SYN-PARTY-A"),
		futureCustomerAccountRegistrationFixture(t, "tenant-1", "SYN-ACC-A2", "SYN-PARTY-A"),
		customerAccountRegistrationFixture(t, "tenant-1", "SYN-ACC-B1", "SYN-PARTY-B"),
		retiring,
	} {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveCustomerAccount(txCtx, registration)
		})
	}
	deactivated, err := retiring.Deactivate(pcValue(t, domain.NewIdentityBasisReference, "basis-deact"), identityDeactivateAt)
	if err != nil {
		t.Fatalf("deactivate account: %v", err)
	}
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveCustomerAccount(txCtx, deactivated)
	})

	cases := []struct {
		query string
		want  string
	}{
		{"sort=accountId", "SYN-ACC-A1,SYN-ACC-A2,SYN-ACC-B1,SYN-ACC-B2"},
		{"sort=accountId&status=EFFECTIVE", "SYN-ACC-A1,SYN-ACC-B1"},
		{"sort=accountId&status=EFFECTIVE&status=REGISTERED", "SYN-ACC-A1,SYN-ACC-A2,SYN-ACC-B1"},
		{"sort=accountId&customerPartyId=SYN-PARTY-B", "SYN-ACC-B1,SYN-ACC-B2"},
		{"sort=accountId&customerPartyId=SYN-PARTY-B&status=DEACTIVATED", "SYN-ACC-B2"},
		{"sort=accountId&q=" + url.QueryEscape("华东"), "SYN-ACC-A1,SYN-ACC-A2"},
		{"sort=accountId&q=acc-b", "SYN-ACC-B1,SYN-ACC-B2"},
		{"sort=accountId&q=%25", ""},
	}
	for _, tc := range cases {
		page, err := catalogue.ListCustomerAccounts(ctx, tenant, 10, customerAccountQuery(t, tc.query))
		if err != nil {
			t.Fatalf("%s: %v", tc.query, err)
		}
		if got := accountIDsOf(page.Rows); got != tc.want || page.Total != int64(len(page.Rows)) {
			t.Fatalf("%s = %s（共 %d），want %s 且总数与本页同一组条件", tc.query, got, page.Total, tc.want)
		}
	}
}

func accountIDsOf(rows []ports.CustomerAccountRow) string {
	ids := make([]string, len(rows))
	for index, row := range rows {
		ids[index] = row.AccountID
	}
	return strings.Join(ids, ",")
}
