package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
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
