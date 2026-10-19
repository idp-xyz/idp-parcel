package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证交接的三格节点声明（ADR-0213）：随行往返、缺席读回缺席、更正行照前版承继、按范围读与
// 查阅册都带着它们、空白串进不了库。节点代码都是合成串，证据只记 `S`。

func handedOverRecordWithNodes(t *testing.T, version string, nodes domain.HandoverNodeDeclarations) ports.TransportHandoverRecord {
	t.Helper()
	record := handedOverRecord(t, version)
	handover, err := domain.FormTransportHandover(domain.TransportHandoverSpec{
		TenantID:          record.Handover.TenantID(),
		Object:            record.Handover.Object(),
		Scope:             record.Handover.Scope(),
		ReleasedBy:        record.Handover.ReleasedBy(),
		ReceivedBy:        record.Handover.ReceivedBy(),
		Verdict:           domain.ObjectHandedOver,
		ReleasingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-out"),
		ReceivingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-in"),
		Rule:              handoverValue(t, domain.NewHandoverRuleReference, "rule/v1"),
		Version:           record.Handover.Version(),
		JudgedAt:          record.Handover.JudgedAt(),
		Nodes:             nodes,
	})
	if err != nil {
		t.Fatalf("构造带节点的已交接夹具：%v", err)
	}
	record.Handover = handover
	return record
}

// Covers: AT-TF-217、AT-TF-220、AT-TF-221——声明的节点随行往返（按键、按前版找后继、按范围，各条读路都带着），没声明的
// 那格读回缺席；更正行照前版承继三格；查阅册逐格列出，没声明的列空。
func TestHandoverNodeDeclarationsRoundTripThroughEveryReadPath(t *testing.T) {
	stores := newReviewCatalogueStores(t)
	ctx := t.Context()
	nodes := domain.HandoverNodeDeclarations{
		Releasing: handoverValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SHA-HUB"),
		Onward:    handoverValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SZX-GW"),
	}
	first := handedOverRecordWithNodes(t, "HRV-N-1", nodes)
	mustSaveHandover(t, stores.transactor, ctx, stores.handovers, first)

	found, exists, err := stores.handovers.FindByKey(ctx, handoverKeyFixture(t, "tenant-1", "HRV-N-1"))
	if err != nil || !exists {
		t.Fatalf("取回交接：%v exists=%v", err, exists)
	}
	if found.Handover.Nodes() != nodes {
		t.Fatalf("按键读回节点 = %+v，want %+v", found.Handover.Nodes(), nodes)
	}
	if node, present := found.Handover.ReceivingNode(); present {
		t.Fatalf("没声明的接收节点读回了 %q", node)
	}

	corrected, err := found.Handover.Correct(domain.HandoverCorrection{
		Verdict:     domain.HandoverRefused,
		Basis:       handoverValue(t, domain.NewHandoverBasisReference, "count-mismatch"),
		Version:     handoverValue(t, domain.NewHandoverResultVersion, "HRV-N-2"),
		CorrectedAt: handoverJudgedAtFixture.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	mustSaveHandover(t, stores.transactor, ctx, stores.handovers, ports.TransportHandoverRecord{
		Key:           handoverKeyFixture(t, "tenant-1", "HRV-N-2"),
		ContentDigest: "digest-corrected",
		Handover:      corrected,
		RecordedAt:    handoverJudgedAtFixture.Add(time.Hour),
	})
	successor, exists, err := stores.handovers.FindSuccessor(ctx, handoverKeyFixture(t, "tenant-1", "HRV-N-1"))
	if err != nil || !exists {
		t.Fatalf("按前版找后继：%v exists=%v", err, exists)
	}
	if successor.Handover.Nodes() != nodes {
		t.Fatalf("更正行节点 = %+v，want 承继前版 %+v", successor.Handover.Nodes(), nodes)
	}

	scoped, err := stores.handovers.ListByScope(ctx, first.Key.TenantID, first.Key.Scope)
	if err != nil || len(scoped) != 2 {
		t.Fatalf("按范围读：%v rows=%d", err, len(scoped))
	}
	for _, record := range scoped {
		if record.Handover.Nodes() != nodes {
			t.Fatalf("按范围读回 %s 的节点 = %+v", record.Key.Version, record.Handover.Nodes())
		}
	}

	rows, err := stores.catalogue.ListTransportHandovers(ctx, first.Key.TenantID, 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列交接册：%v rows=%d", err, len(rows))
	}
	for _, row := range rows {
		if row.ReleasingNode != "SYN-NODE-SHA-HUB" || row.ReceivingNode != "" || row.OnwardNode != "SYN-NODE-SZX-GW" {
			t.Fatalf("册面 %s 的节点列 = %q / %q / %q", row.Version, row.ReleasingNode, row.ReceivingNode, row.OnwardNode)
		}
	}
}

// Covers: AT-TF-216——不声明节点的交接三列落 NULL、读回一格都没声明。迁移 0039 之前登下的行就是这一形。
func TestAHandoverWithoutNodeDeclarationsStoresNulls(t *testing.T) {
	repository, transactor, pool := newTransportHandovers(t)
	ctx := t.Context()
	mustSaveHandover(t, transactor, ctx, repository, handedOverRecord(t, "HRV-PLAIN-1"))

	var releasing, receiving, onward *string
	if err := pool.QueryRow(ctx,
		`SELECT releasing_node, receiving_node, onward_node FROM transport_fulfillment.transport_handover
		  WHERE handover_version = 'HRV-PLAIN-1'`).Scan(&releasing, &receiving, &onward); err != nil {
		t.Fatalf("读三列：%v", err)
	}
	if releasing != nil || receiving != nil || onward != nil {
		t.Fatalf("没声明的节点落了值：%v %v %v", releasing, receiving, onward)
	}
	found, _, err := repository.FindByKey(ctx, handoverKeyFixture(t, "tenant-1", "HRV-PLAIN-1"))
	if err != nil || found.Handover.Nodes().Declared() {
		t.Fatalf("读回 err=%v nodes=%+v，want 一格都没声明", err, found.Handover.Nodes())
	}
}

// Covers: AT-TF-222 的库面——空白节点串绕过领域直插被 CHECK 拒：「没有声明」只有 NULL 一种写法。断言落到
// SQLSTATE 与约束名上：只断有错，直插语句里别的列因约束变化失败时这里照样会过，证明不了是这道 CHECK 拦下的。
// 空白只取空格与空串：CHECK 照 TF 既有迁移用 btrim，只削空格；制表符一类的空白由领域 TrimSpace 挡在库前，不归库面证。
func TestABlankHandoverNodeIsRefusedByTheDatabase(t *testing.T) {
	_, _, pool := newTransportHandovers(t)
	blanks := map[string][3]any{
		"releasing_node": {"  ", nil, nil},
		"receiving_node": {nil, " ", nil},
		"onward_node":    {nil, nil, ""},
	}
	for column, nodes := range blanks {
		t.Run(column, func(t *testing.T) {
			_, err := pool.Exec(t.Context(),
				`INSERT INTO transport_fulfillment.transport_handover
					(tenant_id, object_ref, scope_ref, handover_version,
					 released_by, received_by, verdict,
					 releasing_evidence, receiving_evidence, rule_ref, basis_ref,
					 corrects_version, corrected_at, judged_at, content_digest, recorded_at,
					 releasing_node, receiving_node, onward_node)
				 VALUES ('tenant-1', 'parcel-1', 'scope-1', 'HRV-BLANK-' || $4::text,
				         'node-1', 'carrier-1', 'HANDED_OVER',
				         'seal-out', 'seal-in', 'rule/v1', NULL,
				         NULL, NULL, now(), 'digest-1', now(),
				         $1, $2, $3)`,
				nodes[0], nodes[1], nodes[2], column)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) {
				t.Fatalf("空白 %s 直插 err=%v，want 库面 CHECK 拒", column, err)
			}
			if pgErr.Code != "23514" || pgErr.ConstraintName != "transport_handover_nodes_not_blank" {
				t.Fatalf("空白 %s 直插被 %s / %q 拒，want 23514 / transport_handover_nodes_not_blank", column, pgErr.Code, pgErr.ConstraintName)
			}
		})
	}
}
