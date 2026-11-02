package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件对真实 PostgreSQL 证案件单据号这一格的落库与读回（ADR-0355 决定三、七、八、十一；迁移 0032）：号与无号情形随
// 案件同一行落下、两个读口与查阅面原样读回；迁移之前的旧行读成「上线前开立」；没落定的案件不落库；（租户，单据号）唯一
// 是最后一道保险；判撞那一问只在事务里答。断言一律在事务闭包外。

// Covers: ADR-0355 决定三、七——案件的单据号或无号情形随行落下，按范围键、按案件标识两个读口都原样读回；旧行（迁移
// 之前建立、两列皆空）读成上线前开立，不补号。
func TestACaseNumberAndItsAbsenceAreReadBackByBothReads(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()

	numberedKey := caseKey(t, "tenant-a")
	numbered := establishedCase(t, numberedKey, "case-1", nil)
	unconfiguredKey := caseKey(t, "tenant-b")
	unconfigured, err := unnumberedCase(t, unconfiguredKey, "case-2", nil).Unnumbered(domain.NumberingTimeZoneNotConfigured)
	if err != nil {
		t.Fatal(err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.cases.Save(txCtx, numberedKey, numbered); err != nil {
			return err
		}
		_, err := fixture.cases.Save(txCtx, unconfiguredKey, unconfigured)
		return err
	})
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.customs_case
			(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at)
		 VALUES ('tenant-c', 'jurisdiction/US', 'IMPORT', 'IMPORT_STANDARD', 'obligation/full', 'case-3',
		         '[{"parcel":"parcel-1","customer":"customer-1","sourceRef":"source/submission-1"}]', '[]', $1)`,
		crgBaseAt); err != nil {
		t.Fatalf("种一行迁移之前的旧案件：%v", err)
	}

	byKey, _, err := fixture.cases.FindByKey(ctx, numberedKey)
	if err != nil {
		t.Fatal(err)
	}
	if number, present := byKey.DocumentNumber(); !present || number != "CC-case-1" {
		t.Fatalf("按范围键读回的号 = %q %v", number, present)
	}
	byID, _, err := fixture.cases.FindByID(ctx, numberedKey.TenantID, byKey.ID())
	if err != nil {
		t.Fatal(err)
	}
	if number, _ := byID.DocumentNumber(); number != "CC-case-1" {
		t.Fatalf("按标识读回的号 = %q", number)
	}

	missing, _, err := fixture.cases.FindByKey(ctx, unconfiguredKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := missing.DocumentNumber(); present || missing.DocumentNumberAbsence() != domain.NumberingTimeZoneNotConfigured {
		t.Fatalf("时区未配置的案件读回 = %v", missing.DocumentNumberAbsence())
	}

	legacy, found, err := fixture.cases.FindByKey(ctx, caseKey(t, "tenant-c"))
	if err != nil || !found {
		t.Fatalf("旧行：%v %v", err, found)
	}
	if _, present := legacy.DocumentNumber(); present || legacy.DocumentNumberAbsence() != domain.IssuedBeforeNumbering {
		t.Fatalf("旧行读回 = %v，要上线前开立", legacy.DocumentNumberAbsence())
	}

	views, err := adapter.NewOperationsRecordViews(fixture.db(t))
	if err != nil {
		t.Fatal(err)
	}
	for tenant, want := range map[string][2]string{
		"tenant-a": {"CC-case-1", ""},
		"tenant-b": {"", "NUMBERING_TIME_ZONE_NOT_CONFIGURED"},
		"tenant-c": {"", "ISSUED_BEFORE_NUMBERING"},
	} {
		records, err := views.ListCustomsCases(ctx, crgValue(t, domain.NewTenantID, tenant), "", 10)
		if err != nil || len(records) != 1 {
			t.Fatalf("%s 查阅面：%v %+v", tenant, err, records)
		}
		if records[0].DocumentNumber != want[0] || records[0].DocumentNumberAbsence != want[1] {
			t.Fatalf("%s 查阅面单据号 = %q / %q，要 %q / %q", tenant,
				records[0].DocumentNumber, records[0].DocumentNumberAbsence, want[0], want[1])
		}
	}
}

// Covers: 没落定单据号的案件不落库——落成两列皆空就会读成「上线前开立」；「上线前开立」也不收，那只是旧行的读法。
func TestACaseWithoutASettledNumberIsNotSaved(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()
	key := caseKey(t, "tenant-a")

	beforeNumbering, err := unnumberedCase(t, key, "case-2", nil).Unnumbered(domain.IssuedBeforeNumbering)
	if err != nil {
		t.Fatal(err)
	}
	for name, customsCase := range map[string]domain.CustomsCase{
		"unsettled":        unnumberedCase(t, key, "case-1", nil),
		"before-numbering": beforeNumbering,
	} {
		err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			_, err := fixture.cases.Save(txCtx, key, customsCase)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "no settled document number") {
			t.Fatalf("%s：没落定单据号的案件被收下了：%v", name, err)
		}
	}
	if _, found, err := fixture.cases.FindByKey(ctx, key); err != nil || found {
		t.Fatalf("拒收之后库里有了案件：%v %v", err, found)
	}
}

// Covers: ADR-0355 决定八——（租户，单据号）唯一由案件表担保：同租户同号的第二个案件以唯一约束报错（不吞成「已有记录」），
// 另一租户同号照常；判撞那一问在事务里答，事务外即拒。
func TestCaseNumbersAreUniquePerTenantAndTheUsedQueryNeedsATransaction(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()

	first := caseKey(t, "tenant-a")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.cases.Save(txCtx, first, establishedCase(t, first, "case-1", nil))
		return err
	})

	second := first
	second.Obligation = crgValue(t, domain.NewObligationScopeReference, "obligation/follow-up")
	clash, err := unnumberedCase(t, second, "case-2", nil).Numbered("CC-case-1")
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := fixture.cases.Save(txCtx, second, clash)
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "customs_case_document_number_unique" {
		t.Fatalf("同租户同号的第二个案件没被唯一约束拦下：%v", err)
	}

	foreign := caseKey(t, "tenant-b")
	sameNumberElsewhere, err := unnumberedCase(t, foreign, "case-3", nil).Numbered("CC-case-1")
	if err != nil {
		t.Fatal(err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.cases.Save(txCtx, foreign, sameNumberElsewhere)
		return err
	})

	var usedHere, usedFree, usedForeignTenant bool
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		var err error
		if usedHere, err = fixture.cases.CaseNumberUsed(txCtx, first.TenantID, "CC-case-1"); err != nil {
			return err
		}
		if usedFree, err = fixture.cases.CaseNumberUsed(txCtx, first.TenantID, "CC-case-9"); err != nil {
			return err
		}
		usedForeignTenant, err = fixture.cases.CaseNumberUsed(txCtx, crgValue(t, domain.NewTenantID, "tenant-z"), "CC-case-1")
		return err
	})
	if !usedHere || usedFree || usedForeignTenant {
		t.Fatalf("判撞答错：本租户已用 %v、未用的号 %v、别的租户 %v", usedHere, usedFree, usedForeignTenant)
	}
	// 判撞是建案事务里的一问：事务外答出来的「未用」挡不住同一时刻另一笔建案，读口照写口一样拒。
	if _, err := fixture.cases.CaseNumberUsed(ctx, first.TenantID, "CC-case-1"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外判撞应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 迁移 0032 的库内第二道网：号不能是空白或带首尾空白，无号情形只收时区未配置且与号互斥。
func TestTheCaseTableRefusesMalformedNumberColumns(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()
	for name, columns := range map[string][2]any{
		"blank":             {" ", nil},
		"padded":            {" CC-1", nil},
		"unknown-absence":   {nil, "ISSUED_BEFORE_NUMBERING"},
		"number-and-reason": {"CC-1", "NUMBERING_TIME_ZONE_NOT_CONFIGURED"},
	} {
		_, err := fixture.pool.Exec(ctx,
			`INSERT INTO customs_compliance.customs_case
				(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at,
				 document_number, document_number_absence)
			 VALUES ('tenant-a', 'jurisdiction/US', 'IMPORT', 'IMPORT_STANDARD', $1, $1,
			         '[{"parcel":"parcel-1","customer":"customer-1","sourceRef":"source/submission-1"}]', '[]', $2, $3, $4)`,
			"obligation/"+name, crgBaseAt, columns[0], columns[1])
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Fatalf("%s：坏列被收下了：%v", name, err)
		}
	}
}

// db 交回连在本用例那个库上的框架 DB。
func (fixture *crgFixture) db(t *testing.T) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(fixture.pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	return db
}
