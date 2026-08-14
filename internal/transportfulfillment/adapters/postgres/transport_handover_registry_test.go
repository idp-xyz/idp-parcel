package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证交接判断登记：三裁决往返、更正是新行且原行不删、
// 逐格完备性入库内 CHECK、撞键译`已登记`、作用域隔离、无事务拒。

var handoverJudgedAtFixture = time.Date(2026, 9, 11, 14, 0, 0, 0, time.UTC)

func TestAHandedOverJudgmentRoundTrips(t *testing.T) {
	repository, transactor, _ := newTransportHandovers(t)
	ctx := t.Context()

	record := handedOverRecord(t, "HRV-000000000001")
	mustSaveHandover(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, handoverKeyFixture(t, "tenant-1", "HRV-000000000001"))
	if err != nil || !exists {
		t.Fatalf("取回交接：%v exists=%v", err, exists)
	}
	if found.Handover.Verdict() != domain.ObjectHandedOver ||
		found.Handover.ReleasedBy().String() != "node-1" ||
		found.Handover.ReceivedBy().String() != "carrier-1" ||
		!found.Handover.JudgedAt().Equal(handoverJudgedAtFixture) ||
		found.ContentDigest != record.ContentDigest {
		t.Fatalf("交接往返变形：%+v", found.Handover)
	}
	releasing, hasReleasing := found.Handover.ReleasingEvidence()
	receiving, hasReceiving := found.Handover.ReceivingEvidence()
	rule, hasRule := found.Handover.Rule()
	if !hasReleasing || !hasReceiving || !hasRule ||
		releasing.String() != "seal-out" || receiving.String() != "seal-in" || rule.String() != "rule/v1" {
		t.Fatal("已交接的双方证据或适用规则没有随行保全")
	}
	if _, hasBasis := found.Handover.Basis(); hasBasis {
		t.Fatal("已交接读回却带了拒收/待确认依据")
	}
	if !found.Handover.TransfersControl() {
		t.Fatal("已交接读回却不转移控制")
	}
}

// TestARefusedJudgmentKeepsItsBasis 证拒收那一格：依据随行，且它不转移控制。
func TestARefusedJudgmentKeepsItsBasis(t *testing.T) {
	repository, transactor, _ := newTransportHandovers(t)
	ctx := t.Context()

	mustSaveHandover(t, transactor, ctx, repository, refusedRecord(t, "HRV-000000000002"))

	found, exists, err := repository.FindByKey(ctx, handoverKeyFixture(t, "tenant-1", "HRV-000000000002"))
	if err != nil || !exists {
		t.Fatalf("取回拒收：%v exists=%v", err, exists)
	}
	basis, hasBasis := found.Handover.Basis()
	if !hasBasis || basis.String() != "seal-broken" {
		t.Fatalf("拒收依据没有随行保全：%+v", found.Handover)
	}
	if found.Handover.TransfersControl() {
		t.Fatal("拒收却转移了控制")
	}
}

// TestACorrectionLandsAsANewVersionRow 证更正是新版本新行：两行都在库、新行回指前身、
// 原判断读回来还是原来那个——「不删除原交接」的库面。
func TestACorrectionLandsAsANewVersionRow(t *testing.T) {
	repository, transactor, pool := newTransportHandovers(t)
	ctx := t.Context()

	first := handedOverRecord(t, "HRV-000000000001")
	mustSaveHandover(t, transactor, ctx, repository, first)

	corrected, err := first.Handover.Correct(domain.HandoverCorrection{
		Verdict:     domain.HandoverRefused,
		Basis:       handoverValue(t, domain.NewHandoverBasisReference, "count-mismatch"),
		Version:     handoverValue(t, domain.NewHandoverResultVersion, "HRV-000000000002"),
		CorrectedAt: handoverJudgedAtFixture.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	mustSaveHandover(t, transactor, ctx, repository, ports.TransportHandoverRecord{
		Key:           handoverKeyFixture(t, "tenant-1", "HRV-000000000002"),
		ContentDigest: "digest-corrected",
		Handover:      corrected,
		RecordedAt:    handoverJudgedAtFixture.Add(2 * time.Hour),
	})

	newVersion, exists, err := repository.FindByKey(ctx, handoverKeyFixture(t, "tenant-1", "HRV-000000000002"))
	if err != nil || !exists {
		t.Fatalf("取回更正版：%v exists=%v", err, exists)
	}
	predecessor, corrects := newVersion.Handover.Corrects()
	if !corrects || predecessor.String() != "HRV-000000000001" {
		t.Fatalf("前版引用没有随行保全：%+v", newVersion.Handover)
	}
	correctedAt, hasCorrectedAt := newVersion.Handover.CorrectedAt()
	if !hasCorrectedAt || !correctedAt.Equal(handoverJudgedAtFixture.Add(2*time.Hour)) {
		t.Fatal("更正时间没有随行保全")
	}

	original, exists, err := repository.FindByKey(ctx, handoverKeyFixture(t, "tenant-1", "HRV-000000000001"))
	if err != nil || !exists {
		t.Fatalf("原判断被更正冲掉了：%v exists=%v", err, exists)
	}
	if original.Handover.Verdict() != domain.ObjectHandedOver {
		t.Fatal("原判断的裁决被改写")
	}

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM transport_fulfillment.transport_handover`).Scan(&rows); err != nil {
		t.Fatalf("统计版本行：%v", err)
	}
	if rows != 2 {
		t.Fatalf("rows = %d, want 2（历史行只增不删）", rows)
	}
}

// TestHandedOverCompletenessIsPinnedInTheDatabase 证逐格完备性入库内 CHECK：绕过领域
// 直插一行缺接收证据的「已交接」被数据库拒（领域构造门是第一道，两道互补不互替）。
func TestHandedOverCompletenessIsPinnedInTheDatabase(t *testing.T) {
	_, _, pool := newTransportHandovers(t)
	ctx := t.Context()

	_, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_handover
			(tenant_id, object_ref, scope_ref, handover_version,
			 released_by, received_by, verdict,
			 releasing_evidence, receiving_evidence, rule_ref, basis_ref,
			 corrects_version, corrected_at, judged_at, content_digest, recorded_at)
		 VALUES ('tenant-1', 'parcel-1', 'scope-1', 'HRV-000000000001',
		         'node-1', 'carrier-1', 'HANDED_OVER',
		         'seal-out', NULL, 'rule/v1', NULL,
		         NULL, NULL, now(), 'digest-1', now())`)
	if err == nil {
		t.Fatal("一行缺接收证据的「已交接」进了库")
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_handover
			(tenant_id, object_ref, scope_ref, handover_version,
			 released_by, received_by, verdict,
			 releasing_evidence, receiving_evidence, rule_ref, basis_ref,
			 corrects_version, corrected_at, judged_at, content_digest, recorded_at)
		 VALUES ('tenant-1', 'parcel-1', 'scope-1', 'HRV-000000000003',
		         'node-1', 'carrier-1', 'REFUSED',
		         NULL, NULL, NULL, NULL,
		         NULL, NULL, now(), 'digest-3', now())`)
	if err == nil {
		t.Fatal("一行没有原因的「拒收」进了库")
	}
}

func TestASecondHandoverRegistrationKeepsTheFirst(t *testing.T) {
	repository, transactor, _ := newTransportHandovers(t)
	ctx := t.Context()

	mustSaveHandover(t, transactor, ctx, repository, handedOverRecord(t, "HRV-000000000001"))

	late := handedOverRecord(t, "HRV-000000000001")
	late.ContentDigest = "digest-late"
	mustWithinHandoverTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.Save(txCtx, late)
		if err != nil {
			return err
		}
		if outcome != ports.HandoverAlreadyRegistered {
			t.Fatalf("outcome = %d, want ALREADY_REGISTERED", outcome)
		}
		found, exists, err := repository.FindByKey(txCtx, handoverKeyFixture(t, "tenant-1", "HRV-000000000001"))
		if err != nil || !exists {
			t.Fatalf("撞键后同事务读回失败：%v exists=%v", err, exists)
		}
		if found.ContentDigest != "digest-handed-over" {
			t.Fatal("后到者覆盖了先到者的登记")
		}
		return nil
	})
}

func TestHandoverScopesAreInvisibleToEachOther(t *testing.T) {
	repository, transactor, _ := newTransportHandovers(t)
	ctx := t.Context()

	mustSaveHandover(t, transactor, ctx, repository, handedOverRecord(t, "HRV-000000000001"))

	_, exists, err := repository.FindByKey(ctx, handoverKeyFixture(t, "tenant-b", "HRV-000000000001"))
	if err != nil {
		t.Fatalf("他租户查询出错：%v", err)
	}
	if exists {
		t.Error("他租户读到了本租户的交接判断")
	}
}

func TestHandoverWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newTransportHandovers(t)

	_, err := repository.Save(t.Context(), handedOverRecord(t, "HRV-000000000001"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// ---- 夹具 ----

func newTransportHandovers(t *testing.T) (*adapter.TransportHandovers, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewTransportHandovers(db)
	if err != nil {
		t.Fatalf("构造交接登记库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustWithinHandoverTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	fn func(context.Context) error,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func mustSaveHandover(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.TransportHandovers,
	record ports.TransportHandoverRecord,
) {
	t.Helper()
	mustWithinHandoverTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.HandoverSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})
}

func handoverValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func handoverKeyFixture(t *testing.T, tenant, version string) ports.TransportHandoverKey {
	t.Helper()
	return ports.TransportHandoverKey{
		TenantID: handoverValue(t, domain.NewTenantID, tenant),
		Object:   handoverValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Scope:    handoverValue(t, domain.NewHandoverScopeReference, "scope-1"),
		Version:  handoverValue(t, domain.NewHandoverResultVersion, version),
	}
}

func handedOverRecord(t *testing.T, version string) ports.TransportHandoverRecord {
	t.Helper()
	handover, err := domain.FormTransportHandover(domain.TransportHandoverSpec{
		TenantID:          handoverValue(t, domain.NewTenantID, "tenant-1"),
		Object:            handoverValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Scope:             handoverValue(t, domain.NewHandoverScopeReference, "scope-1"),
		ReleasedBy:        handoverValue(t, domain.NewHandoverPartyReference, "node-1"),
		ReceivedBy:        handoverValue(t, domain.NewHandoverPartyReference, "carrier-1"),
		Verdict:           domain.ObjectHandedOver,
		ReleasingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-out"),
		ReceivingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-in"),
		Rule:              handoverValue(t, domain.NewHandoverRuleReference, "rule/v1"),
		Version:           handoverValue(t, domain.NewHandoverResultVersion, version),
		JudgedAt:          handoverJudgedAtFixture,
	})
	if err != nil {
		t.Fatalf("构造已交接夹具：%v", err)
	}
	return ports.TransportHandoverRecord{
		Key:           handoverKeyFixture(t, "tenant-1", version),
		ContentDigest: "digest-handed-over",
		Handover:      handover,
		RecordedAt:    handoverJudgedAtFixture,
	}
}

func refusedRecord(t *testing.T, version string) ports.TransportHandoverRecord {
	t.Helper()
	handover, err := domain.FormTransportHandover(domain.TransportHandoverSpec{
		TenantID:          handoverValue(t, domain.NewTenantID, "tenant-1"),
		Object:            handoverValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Scope:             handoverValue(t, domain.NewHandoverScopeReference, "scope-1"),
		ReleasedBy:        handoverValue(t, domain.NewHandoverPartyReference, "node-1"),
		ReceivedBy:        handoverValue(t, domain.NewHandoverPartyReference, "carrier-1"),
		Verdict:           domain.HandoverRefused,
		ReleasingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-out"),
		Basis:             handoverValue(t, domain.NewHandoverBasisReference, "seal-broken"),
		Version:           handoverValue(t, domain.NewHandoverResultVersion, version),
		JudgedAt:          handoverJudgedAtFixture,
	})
	if err != nil {
		t.Fatalf("构造拒收夹具：%v", err)
	}
	return ports.TransportHandoverRecord{
		Key:           handoverKeyFixture(t, "tenant-1", version),
		ContentDigest: "digest-refused",
		Handover:      handover,
		RecordedAt:    handoverJudgedAtFixture,
	}
}
