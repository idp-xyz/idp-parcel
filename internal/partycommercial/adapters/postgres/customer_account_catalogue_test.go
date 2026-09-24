package postgres_test

import (
	"context"
	"net/url"
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
