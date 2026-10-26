package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证交接版本留存的段声明（ADR-0307 决定二，迁移 0053）：三格随行往返，按键、按前版找后继、按范围
// 各条读路都带着；没声明的格落 NULL、读回空串；空白串进不了库。段号都是合成串，证据只记 `S`。

// Covers: AT-TF-329 的留存那一半（ADR-0307 决定二）——待确认那一版的段声明原样落库、各条读路读回；更正行带着前版那一份
// 落下（承继由编排做，这里证它落得下、读得回）；没声明的那一格落 NULL。
func TestAHandoverSegmentDeclarationRoundTripsThroughEveryReadPath(t *testing.T) {
	repository, transactor, pool := newTransportHandovers(t)
	ctx := t.Context()
	declaration := ports.HandoverSegmentDeclaration{Segment: "SYN-SEG-NEXT", ServiceAction: domain.SegmentServesLinehaul.String()}
	pending := refusedRecord(t, "HRV-D-1")
	pending.SegmentDeclaration = declaration
	mustSaveHandover(t, transactor, ctx, repository, pending)

	found, exists, err := repository.FindByKey(ctx, handoverKeyFixture(t, "tenant-1", "HRV-D-1"))
	if err != nil || !exists || found.SegmentDeclaration != declaration {
		t.Fatalf("按键读回段声明 = %+v（%v %v），want %+v", found.SegmentDeclaration, exists, err, declaration)
	}
	var planned *string
	if err := pool.QueryRow(ctx,
		`SELECT declared_planned_segment FROM transport_fulfillment.transport_handover WHERE handover_version = 'HRV-D-1'`,
	).Scan(&planned); err != nil || planned != nil {
		t.Fatalf("没声明的计划段落了 %v（%v），want NULL", planned, err)
	}

	corrected, err := found.Handover.Correct(domain.HandoverCorrection{
		Verdict:           domain.ObjectHandedOver,
		ReleasingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-out"),
		ReceivingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-in"),
		Rule:              handoverValue(t, domain.NewHandoverRuleReference, "rule/v1"),
		Version:           handoverValue(t, domain.NewHandoverResultVersion, "HRV-D-2"),
		CorrectedAt:       handoverJudgedAtFixture.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	mustSaveHandover(t, transactor, ctx, repository, ports.TransportHandoverRecord{
		Key:                handoverKeyFixture(t, "tenant-1", "HRV-D-2"),
		ContentDigest:      "digest-corrected",
		Handover:           gateReadingOn(t, corrected),
		RecordedAt:         handoverJudgedAtFixture.Add(time.Hour),
		SegmentDeclaration: found.SegmentDeclaration,
	})
	successor, exists, err := repository.FindSuccessor(ctx, handoverKeyFixture(t, "tenant-1", "HRV-D-1"))
	if err != nil || !exists || successor.SegmentDeclaration != declaration {
		t.Fatalf("按前版找后继读回段声明 = %+v（%v %v），want %+v", successor.SegmentDeclaration, exists, err, declaration)
	}

	mustSaveHandover(t, transactor, ctx, repository, handedOverRecord(t, "HRV-D-PLAIN"))
	scoped, err := repository.ListByScope(ctx, pending.Key.TenantID, pending.Key.Scope)
	if err != nil || len(scoped) != 3 {
		t.Fatalf("按范围读：%v rows=%d", err, len(scoped))
	}
	for _, record := range scoped {
		want := declaration
		if record.Key.Version.String() == "HRV-D-PLAIN" {
			want = ports.HandoverSegmentDeclaration{}
		}
		if record.SegmentDeclaration != want {
			t.Fatalf("按范围读回 %s 的段声明 = %+v，want %+v", record.Key.Version, record.SegmentDeclaration, want)
		}
	}
}

// Covers: ADR-0307 决定二的库面——空白段声明绕过编排直插被 CHECK 拒：「没有声明」只有 NULL 一种写法。断言落到 SQLSTATE 与
// 约束名上，理由同节点声明那一条（TestABlankHandoverNodeIsRefusedByTheDatabase）。
func TestABlankSegmentDeclarationIsRefusedByTheDatabase(t *testing.T) {
	_, _, pool := newTransportHandovers(t)
	blanks := map[string][3]any{
		"declared_segment":                {"  ", nil, nil},
		"declared_planned_segment":        {nil, " ", nil},
		"declared_segment_service_action": {nil, nil, ""},
	}
	for column, declared := range blanks {
		t.Run(column, func(t *testing.T) {
			_, err := pool.Exec(t.Context(),
				`INSERT INTO transport_fulfillment.transport_handover
					(tenant_id, object_ref, scope_ref, handover_version,
					 released_by, received_by, verdict,
					 releasing_evidence, receiving_evidence, rule_ref, basis_ref,
					 corrects_version, corrected_at, judged_at, content_digest, recorded_at,
					 declared_segment, declared_planned_segment, declared_segment_service_action)
				 VALUES ('tenant-1', 'parcel-1', 'scope-1', 'HRV-BLANK-' || $4::text,
				         'node-1', 'carrier-1', 'PENDING_CONFIRMATION',
				         NULL, NULL, NULL, 'count-unclear',
				         NULL, NULL, now(), 'digest-1', now(),
				         $1, $2, $3)`,
				declared[0], declared[1], declared[2], column)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) {
				t.Fatalf("空白 %s 直插 err=%v，want 库面 CHECK 拒", column, err)
			}
			if pgErr.Code != "23514" || pgErr.ConstraintName != "transport_handover_segment_declaration_not_blank" {
				t.Fatalf("空白 %s 直插被 %s / %q 拒，want 23514 / transport_handover_segment_declaration_not_blank",
					column, pgErr.Code, pgErr.ConstraintName)
			}
		})
	}
}
