package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// execer 是池或旁路连接共有的最小写面。
type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// seedAdoptedFunds 在 settlement-accounting 的表里造一条已采用的确认到账事实（首版 <ref>/v1）。真实到账的证据引用要指向
// 结算已采用的事实，本入口的用例都先造它；造得出的形状与结算的采用口落下的行一致（身份行加版本行）。
func seedAdoptedFunds(t *testing.T, db execer, ref, currency string, amountMinor int64) {
	t.Helper()
	ctx := t.Context()
	if _, err := db.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact (tenant_id, fact_id, recorded_at)
		 VALUES ('SYN-T1', $1, now())`, ref); err != nil {
		t.Fatalf("造资金事实身份行 %s：%v", ref, err)
	}
	if _, err := db.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact_version
			(tenant_id, fact_id, version, source_ref, kind, currency, amount_minor, occurred_at,
			 content_digest, recorded_at)
		 VALUES ('SYN-T1', $1, $2, 'SYN-SOURCE-BANK-1', 'RECEIPT_CONFIRMED', $3, $4,
		         '2026-08-20T00:00:00Z', 'digest-' || $2, now())`,
		ref, ref+"/v1", currency, amountMinor); err != nil {
		t.Fatalf("造资金事实版本 %s：%v", ref, err)
	}
}

// setAsideAsPrincipal 把已采用的事实整条认作代收本金（结算一侧的映射，目标就是事实自己）。
func setAsideAsPrincipal(t *testing.T, db execer, ref string) {
	t.Helper()
	if _, err := db.Exec(t.Context(),
		`INSERT INTO settlement_accounting.funds_mapping
			(tenant_id, mapping_id, fact_id, target_kind, target_ref, basis, mapped_at, content_digest, recorded_at)
		 VALUES ('SYN-T1', 'SYN-MAP-' || $1, $1, 'COLLECTED_PRINCIPAL', $1, 'SYN-CHANNEL-REMITTANCE-ADVICE',
		         '2026-08-21T00:00:00Z', 'digest', now())`, ref); err != nil {
		t.Fatalf("造认作代收本金的映射 %s：%v", ref, err)
	}
}

// seedPrincipalFunds 造一条已采用且已认作代收本金的事实：真实到账登记的前提。
func seedPrincipalFunds(t *testing.T, db execer, ref, currency string, amountMinor int64) {
	t.Helper()
	seedAdoptedFunds(t, db, ref, currency, amountMinor)
	setAsideAsPrincipal(t, db, ref)
}

// correctAdoptedFunds 追加结算一侧的外部更正：同一事实的新版本回指前版，只有金额变（ADR-0285 照采）。
func correctAdoptedFunds(t *testing.T, db execer, ref, corrects, version string, amountMinor int64) {
	t.Helper()
	if _, err := db.Exec(t.Context(),
		`INSERT INTO settlement_accounting.external_funds_fact_version
			(tenant_id, fact_id, version, source_ref, kind, currency, amount_minor, occurred_at,
			 corrects, corrected_at, content_digest, recorded_at)
		 SELECT tenant_id, fact_id, $3, source_ref, kind, currency, $4, occurred_at,
		        $2, occurred_at + interval '1 day', 'digest-' || $3, now()
		   FROM settlement_accounting.external_funds_fact_version
		  WHERE tenant_id = 'SYN-T1' AND fact_id = $1 AND version = $2`,
		ref, corrects, version, amountMinor); err != nil {
		t.Fatalf("追加更正版本 %s：%v", version, err)
	}
}

// settlementFunds 把 settlement-accounting 真实的采用、更正与映射编排接在同一个库上（装配同 cmd/parcel-settlement-register
// 的 buildRegistrar：每一口都接真）。纵向用例经它种结算一侧的事实，认作与映射的互斥、事实锁与「被整条认作」的读法
// 因此是结算自己落下的，不是测试手写的一份形状。
type settlementFunds struct {
	db      *bentopg.DB
	handler *saapplication.MapExternalFundsHandler
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now().UTC() }

func newSettlementFunds(t *testing.T, pool *pgxpool.Pool) settlementFunds {
	t.Helper()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	facts, err := sapostgres.NewExternalFundsFacts(db)
	if err != nil {
		t.Fatalf("构造外部资金事实写口：%v", err)
	}
	mappings, err := sapostgres.NewFundsMappings(db)
	if err != nil {
		t.Fatalf("构造资金映射写口：%v", err)
	}
	applications, err := sapostgres.NewSettlementApplications(db)
	if err != nil {
		t.Fatalf("构造核销写口：%v", err)
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	factHandoff, err := sapostgres.NewOutboxExternalFundsFactHandoff(db, outboxStore, wallClock{})
	if err != nil {
		t.Fatalf("构造资金事实采用交接口：%v", err)
	}
	downstream, err := sapostgres.NewOutboxSettlementApplicationHandoff(db, outboxStore, wallClock{})
	if err != nil {
		t.Fatalf("构造核销交接口：%v", err)
	}
	postings, err := sapostgres.NewCustomerReceiptPostings(db)
	if err != nil {
		t.Fatalf("构造客户收款余额重算：%v", err)
	}
	claimTargets, err := sapostgres.NewClaimSettlementTargets(db)
	if err != nil {
		t.Fatalf("构造赔付与追偿认可目标口：%v", err)
	}
	handler, err := saapplication.NewMapExternalFundsHandler(saapplication.MapExternalFundsDeps{
		Facts:        facts,
		Mappings:     mappings,
		Applications: applications,
		Downstream:   downstream,
		FactHandoff:  factHandoff,
		Clock:        wallClock{},
		Books:        postings,
		Balances:     postings,
		Standings:    postings,
		ClaimTargets: claimTargets,
		FactLocks:    applications,
	})
	if err != nil {
		t.Fatalf("构造外部资金采用编排：%v", err)
	}
	return settlementFunds{db: db, handler: handler}
}

func (funds settlementFunds) within(t *testing.T, step func(ctx context.Context) error) {
	t.Helper()
	if err := funds.db.Transactor().WithinTransaction(t.Context(), step); err != nil {
		t.Fatalf("结算一侧的事务：%v", err)
	}
}

// adopt 经结算的采用口采用一条确认到账的事实（首版 <ref>/v1）。
func (funds settlementFunds) adopt(t *testing.T, ref, currency string, amountMinor int64) {
	t.Helper()
	tenant, err := sadomain.NewTenantID("SYN-T1")
	if err != nil {
		t.Fatal(err)
	}
	var outcome saapplication.FundsOutcome
	funds.within(t, func(ctx context.Context) error {
		result, err := funds.handler.AdoptFact(ctx, saapplication.AdoptFundsFactCommand{
			TenantID: tenant, Fact: ref, Source: "SYN-SOURCE-BANK-1", Kind: sadomain.FundsReceiptConfirmed,
			Currency: currency, AmountMinor: amountMinor, Version: ref + "/v1",
			OccurredAt: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		})
		outcome = result.Outcome()
		return err
	})
	if outcome != saapplication.FundsFactAdopted {
		t.Fatalf("采用 %s = %s，要已采用", ref, outcome)
	}
}

// correct 经结算的采用口追加外部更正：新版本回指前版，只有金额变。
func (funds settlementFunds) correct(t *testing.T, ref, corrects, version string, amountMinor int64) {
	t.Helper()
	tenant, err := sadomain.NewTenantID("SYN-T1")
	if err != nil {
		t.Fatal(err)
	}
	var outcome saapplication.FundsOutcome
	funds.within(t, func(ctx context.Context) error {
		result, err := funds.handler.CorrectFact(ctx, saapplication.CorrectFundsFactCommand{
			TenantID: tenant, Fact: ref, Corrects: corrects, Version: version, AmountMinor: amountMinor,
			CorrectedAt: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC),
		})
		outcome = result.Outcome()
		return err
	})
	if outcome != saapplication.FundsFactAdopted {
		t.Fatalf("更正 %s 到 %s = %s，要已采用", ref, version, outcome)
	}
}

// mapTo 经结算的映射编排把已采用的事实映到一个目标，返回编排的答案；目标等于事实自己时就是整条认作代收本金。
func (funds settlementFunds) mapTo(
	t *testing.T, kind sadomain.SettlementTargetKind, ref, target string,
) saapplication.FundsOutcome {
	t.Helper()
	tenant, err := sadomain.NewTenantID("SYN-T1")
	if err != nil {
		t.Fatal(err)
	}
	var outcome saapplication.FundsOutcome
	funds.within(t, func(ctx context.Context) error {
		result, err := funds.handler.Map(ctx, saapplication.MapFundsCommand{
			TenantID: tenant, Mapping: "SYN-MAP-" + kind.String() + "-" + ref, Fact: ref,
			TargetKind: kind, Target: target, Basis: "SYN-CHANNEL-REMITTANCE-ADVICE",
			MappedAt: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC),
		})
		outcome = result.Outcome()
		return err
	})
	return outcome
}

// seedReceivableAccount 登一个同币种的应收结算账户：事实映到账户时要有这个落点（结算账户登记册本身与本入口无关，写行即可）。
func seedReceivableAccount(t *testing.T, db execer, account, currency string) {
	t.Helper()
	if _, err := db.Exec(t.Context(),
		`INSERT INTO settlement_accounting.settlement_account
			(tenant_id, account_id, legal_entity_id, counterparty_id, direction, currency,
			 settlement_policy_id, responsibility_basis, registered_at)
		 VALUES ('SYN-T1', $1, 'SYN-ENTITY-1', 'SYN-CUST-1', 'RECEIVABLE', $2,
		         'SYN-POLICY-1', 'SYN-CONTRACT-1', now())`, account, currency); err != nil {
		t.Fatalf("登应收结算账户 %s：%v", account, err)
	}
}
