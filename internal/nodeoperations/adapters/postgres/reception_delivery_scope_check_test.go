package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 16 证交付范围核对态落册（ADR-0212 决定五，票 ps-external-mark-relations/11）与来源文档里的依据引用（决定
// 七）：往返原样、库面形状、迁移不回填，复核目录两册各带它。夹具全为合成引用（S 级）。

func scopeCheck(t *testing.T, state domain.DeliveryScopeCheckState, reason domain.DeliveryScopeUnverifiedReason,
	detail domain.DeliveryScopeInapplicability) domain.DeliveryScopeCheck {
	t.Helper()
	check, err := domain.NewDeliveryScopeCheck(state, reason, detail)
	if err != nil {
		t.Fatalf("核对态：%v", err)
	}
	return check
}

// Covers: ADR-0212 决定五的存储面——已证、未证（带原因、带细分）、未配置与不带各自往返原样；来源文档里的依据引用原样读回。
func TestTheDeliveryScopeCheckAndBasisAreReadBackUnchanged(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	const tenant, mark = "SYN-TENANT-PSXM11", "SYN-MARK-PSXM11"
	verified := formedRecord(t, tenant, "SYN-SCAN-PSXM11-VERIFIED")
	verified.DeliveryScopeCheck = scopeCheck(t, domain.DeliveryScopeVerified, domain.ScopeUnverifiedReasonAbsent, domain.ScopeInapplicabilityAbsent)
	verified.Source = deliverySource(t, "unit-1", mark, "RECEIVED")
	verified.Source.ScopeBasis = ref(t, func(raw string) (domain.DeliveryScopeBasisReference, error) {
		return domain.NewDeliveryScopeBasisReference(domain.BasisDeliveryPreAdvice, raw)
	}, "PREADV-PSXM11")
	noBasis := recheckablePending(t, tenant, "SYN-SCAN-PSXM11-NO-BASIS", "SYN-INTAKEV-PSXM11-NO-BASIS", mark)
	noBasis.IdentityCheck = ports.IdentityChecked
	noBasis.DeliveryScopeCheck = scopeCheck(t, domain.DeliveryScopeUnverified, domain.ScopeNoBasis, domain.ScopeInapplicabilityAbsent)
	outside := recheckablePending(t, tenant, "SYN-SCAN-PSXM11-OUTSIDE", "SYN-INTAKEV-PSXM11-OUTSIDE", mark)
	outside.IdentityCheck = ports.IdentityChecked
	outside.DeliveryScopeCheck = scopeCheck(t, domain.DeliveryScopeUnverified, domain.ScopeBasisNotApplicable, domain.ScopeOutsideWindow)
	unconfigured := recheckablePending(t, tenant, "SYN-SCAN-PSXM11-UNCONFIGURED", "SYN-INTAKEV-PSXM11-UNCONFIGURED", mark)
	unconfigured.IdentityCheck = ports.IdentityChecked
	unconfigured.DeliveryScopeCheck = scopeCheck(t, domain.DeliveryScopeUnconfigured, domain.ScopeUnverifiedReasonAbsent, domain.ScopeInapplicabilityAbsent)
	absent := recheckablePending(t, tenant, "SYN-SCAN-PSXM11-ABSENT", "SYN-INTAKEV-PSXM11-ABSENT", mark)
	records := []ports.ReceptionRecord{verified, noBasis, outside, unconfigured, absent}
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range records {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})
	for _, record := range records {
		found, exists, err := repository.FindByKey(ctx, record.Key)
		if err != nil || !exists {
			t.Fatalf("取回 %s：exists=%v err=%v", record.Key.SourceID, exists, err)
		}
		if found.DeliveryScopeCheck != record.DeliveryScopeCheck {
			t.Errorf("%s 读回核对态 %+v, want %+v", record.Key.SourceID, found.DeliveryScopeCheck, record.DeliveryScopeCheck)
		}
		if found.Source.ScopeBasis != record.Source.ScopeBasis {
			t.Errorf("%s 读回依据 %+v, want %+v", record.Key.SourceID, found.Source.ScopeBasis, record.Source.ScopeBasis)
		}
	}
}

// Covers: 核对态的库面形状——绕过适配器直接写库的一行也过不了：取值集外、原因或细分集外、未证不带原因、非未证带原因、细分只随
// 「依据不适用」、核对态只在形成与待识别两格上、未核对的待识别不带。合形的几种照常可写，不带核对态的待识别（迁移之前的形状）也可写。
func TestTheReceptionTableRefusesAMisshapenDeliveryScopeCheck(t *testing.T) {
	_, _, pool := newReceptionsWithPool(t)
	const insert = `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict,
		 service_markers, identity_check, source_document, delivery_scope_check, delivery_scope_unverified_reason,
		 delivery_scope_inapplicability, recorded_at)
		VALUES ('SYN-TENANT-PSXM11', $1, 'syn-digest', $2, $3::jsonb, $3::jsonb, '[]'::jsonb, false, '[]'::jsonb, $4,
		        '{"shape":"NRS-2"}'::jsonb, $5, $6, $7, now())`
	for name, testCase := range map[string]scopeCheckRow{
		"取值集外":        pendingScopeRow(ptr("PROVEN"), nil, nil),
		"原因集外":        pendingScopeRow(ptr("UNVERIFIED"), ptr("NO_PRE_ADVICE"), nil),
		"细分集外":        pendingScopeRow(ptr("UNVERIFIED"), ptr("BASIS_NOT_APPLICABLE"), ptr("TOO_LATE")),
		"未证不带原因":      pendingScopeRow(ptr("UNVERIFIED"), nil, nil),
		"已证带原因":       pendingScopeRow(ptr("VERIFIED"), ptr("NO_BASIS"), nil),
		"未配置带原因":      pendingScopeRow(ptr("UNCONFIGURED"), ptr("NO_BASIS"), nil),
		"细分不随依据不适用":   pendingScopeRow(ptr("UNVERIFIED"), ptr("NO_BASIS"), ptr("OUTSIDE_WINDOW")),
		"依据不适用不带细分":   pendingScopeRow(ptr("UNVERIFIED"), ptr("BASIS_NOT_APPLICABLE"), nil),
		"原因没有核对态":     pendingScopeRow(nil, ptr("NO_BASIS"), nil),
		"未收寄带核对态":     {kind: "INTAKE_NOT_FORMED", check: ptr("VERIFIED")},
		"未核对的待识别带核对态": {kind: "PENDING_IDENTIFICATION", intake: ptr("{}"), identity: ptr("UNCHECKED"), check: ptr("VERIFIED")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pool.Exec(t.Context(), insert, "SYN-SCAN-PSXM11-"+name, testCase.kind, testCase.intake,
				testCase.identity, testCase.check, testCase.reason, testCase.applicable)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("写入 err = %v, want CHECK 违例（23514）", err)
			}
		})
	}
	for name, testCase := range map[string]scopeCheckRow{
		"已证":    pendingScopeRow(ptr("VERIFIED"), nil, nil),
		"未证":    pendingScopeRow(ptr("UNVERIFIED"), ptr("NUMBER_NOT_PRE_ADVISED"), nil),
		"依据不适用": pendingScopeRow(ptr("UNVERIFIED"), ptr("BASIS_NOT_APPLICABLE"), ptr("WITHDRAWN")),
		"未配置":   pendingScopeRow(ptr("UNCONFIGURED"), nil, nil),
		"不带":    pendingScopeRow(nil, nil, nil),
		"形成格已证": {kind: "INTAKE_FORMED", intake: ptr("{}"), check: ptr("VERIFIED")},
	} {
		if _, err := pool.Exec(t.Context(), insert, "SYN-SCAN-PSXM11-OK-"+name, testCase.kind, testCase.intake,
			testCase.identity, testCase.check, testCase.reason, testCase.applicable); err != nil {
			t.Fatalf("合形的一行（%s）被拒：%v", name, err)
		}
	}
}

// scopeCheckRow 是绕过适配器直接写的一行：种类、收寄与控制的 jsonb、身份核对态与交付范围核对态三列。
type scopeCheckRow struct {
	kind                                        string
	intake, identity, check, reason, applicable *string
}

// pendingScopeRow 是一行核对过的待识别，交付范围核对态三列由调用方给。
func pendingScopeRow(check, reason, applicable *string) scopeCheckRow {
	return scopeCheckRow{kind: "PENDING_IDENTIFICATION", intake: ptr("{}"), identity: ptr("CHECKED"), check: check, reason: reason, applicable: applicable}
}

// Covers: ADR-0212「既有行不回填」——核对态这三列加上之前已落的各种行读回都不带核对态，迁移不推定哪一行当时查过范围。从空库按序施加
// 本上下文的迁移，停在核对态那一份之前落旧形状的行，再施加那一份。
func TestTheDeliveryScopeCheckMigrationLeavesExistingRowsWithoutACheck(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA node_operations`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.NodeOperations()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0015_reception_delivery_scope_check"
	var check *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			check = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if check == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, service_markers,
		 service_result_check, identity_check, judgment_version, recorded_at)
		VALUES ('SYN-TENANT-PSXM11', 'SYN-SCAN-PSXM11-FORMED', 'syn-digest-1', 'INTAKE_FORMED',
		        '{"version":"SYN-INTAKEV-PSXM11-FORMED"}'::jsonb, '{}'::jsonb, '[]'::jsonb, false, '[]'::jsonb, 'CHECKED', NULL,
		        'SYN-INTAKEV-PSXM11-FORMED', now()),
		       ('SYN-TENANT-PSXM11', 'SYN-SCAN-PSXM11-PENDING', 'syn-digest-2', 'PENDING_IDENTIFICATION',
		        '{"version":"SYN-INTAKEV-PSXM11-PENDING"}'::jsonb, '{}'::jsonb, '["SYN-PARCEL-1"]'::jsonb, false, '[]'::jsonb,
		        NULL, 'CHECKED', 'SYN-INTAKEV-PSXM11-PENDING', now())`); err != nil {
		t.Fatalf("落旧形状的行：%v", err)
	}
	if _, err := conn.Exec(ctx, check.SQL); err != nil {
		t.Fatalf("施加 %s：%v", target, err)
	}
	var withCheck int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM node_operations.reception
		WHERE delivery_scope_check IS NOT NULL OR delivery_scope_unverified_reason IS NOT NULL
		   OR delivery_scope_inapplicability IS NOT NULL`).Scan(&withCheck); err != nil {
		t.Fatalf("读回：%v", err)
	}
	if withCheck != 0 {
		t.Fatalf("迁移给 %d 行既有判断推定了核对态, want 0——不回填", withCheck)
	}
}

// Covers: ADR-0212 决定五「复核目录带它」——收寄册与待识别册每一行带它记下的交付范围核对态；不带的行如实不带。
func TestCatalogueRowsCarryTheDeliveryScopeCheck(t *testing.T) {
	catalogue, receptions, _, transactor := newReviewCatalogue(t)
	ctx := t.Context()
	const tenant, mark = "SYN-TENANT-PSXM11", "SYN-MARK-PSXM11"
	formed := formedRecord(t, tenant, "SYN-SCAN-PSXM11-FORMED")
	formed.DeliveryScopeCheck = scopeCheck(t, domain.DeliveryScopeVerified, domain.ScopeUnverifiedReasonAbsent, domain.ScopeInapplicabilityAbsent)
	unverified := recheckablePending(t, tenant, "SYN-SCAN-PSXM11-UNVERIFIED", "SYN-INTAKEV-PSXM11-UNVERIFIED", mark)
	unverified.IdentityCheck = ports.IdentityChecked
	unverified.DeliveryScopeCheck = scopeCheck(t, domain.DeliveryScopeUnverified, domain.ScopeBasisNotApplicable, domain.ScopeNodeMismatch)
	legacy := recheckablePending(t, tenant, "SYN-SCAN-PSXM11-LEGACY", "SYN-INTAKEV-PSXM11-LEGACY", mark)
	saveReceptions(t, transactor, ctx, receptions, formed, unverified, legacy)

	tenantID := ref(t, domain.NewTenantID, tenant)
	listed, err := catalogue.ListReceptions(ctx, tenantID, 10)
	if err != nil {
		t.Fatalf("上列收寄册：%v", err)
	}
	unidentified, err := catalogue.ListUnidentifiedItems(ctx, tenantID, 10)
	if err != nil {
		t.Fatalf("上列待识别册：%v", err)
	}
	want := map[string]domain.DeliveryScopeCheck{
		formed.Key.SourceID:     formed.DeliveryScopeCheck,
		unverified.Key.SourceID: unverified.DeliveryScopeCheck,
		legacy.Key.SourceID:     {},
	}
	if len(listed) != 3 || len(unidentified) != 2 {
		t.Fatalf("收寄册 %d 行、待识别册 %d 行, want 3 与 2", len(listed), len(unidentified))
	}
	for _, row := range listed {
		if row.DeliveryScopeCheck != want[row.SourceID] {
			t.Errorf("收寄册 %s 核对态 = %+v, want %+v", row.SourceID, row.DeliveryScopeCheck, want[row.SourceID])
		}
	}
	for _, row := range unidentified {
		if row.DeliveryScopeCheck != want[row.SourceID] {
			t.Errorf("待识别册 %s 核对态 = %+v, want %+v", row.SourceID, row.DeliveryScopeCheck, want[row.SourceID])
		}
	}
}
