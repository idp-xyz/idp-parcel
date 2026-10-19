package postgres_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 证交出门禁读数（ADR-0213 决定四，TF 迁移 0041）：随`已交接`那一版往返，各条读路都带着；
// 拒收不带；`已交接`缺读数、拒收带读数、明细形状不对都被 CHECK 拒；存量回填那一形读回未决。节点与对象都是合成串，
// 证据只记 `S`。

// gateReadingOn 给一版`已交接`带上一份属于它的交出门禁读数：没声明交出节点取不问关务就有的那一份，声明了就是一份
// 「不受门禁」的问询答复。别的裁决原样交回。
func gateReadingOn(t *testing.T, handover domain.TransportHandover) domain.TransportHandover {
	t.Helper()
	if !handover.TransfersControl() {
		return handover
	}
	reading, determined := handover.UnaskedGateReading()
	if !determined {
		reading = askedGateReading(t, handover, domain.GateNotGuarded)
	}
	placed, err := handover.WithGateReading(reading)
	if err != nil {
		t.Fatalf("挂交出门禁读数：%v", err)
	}
	return placed
}

func askedGateReading(t *testing.T, handover domain.TransportHandover, answer domain.GateAnswer) domain.HandoverGateReading {
	t.Helper()
	node, _ := handover.ReleasingNode()
	onward, _ := handover.OnwardNode()
	judgement, basis := "AADJ-"+handover.Object().String(), domain.GateBasis{
		Action: "CROSS_CUSTOMS_MOVEMENT", Cause: "CUSTOMS_SCOPE_UNRESOLVED", Sources: []string{"SYN-SOURCE-1"},
	}
	if answer == domain.GateUnreadable {
		judgement, basis = "", domain.GateBasis{Cause: "CUSTOMS_ADMISSION_UNAVAILABLE"}
	}
	finding, err := domain.NewGateFinding(handover.Object(), answer, judgement, []domain.GateBasis{basis})
	if err != nil {
		t.Fatalf("那一格：%v", err)
	}
	reading, err := domain.FormHandoverGateReading(domain.HandoverGateReadingSpec{Node: node, Onward: onward, Finding: finding})
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	return reading
}

// Covers: AT-TF-236、AT-TF-237、AT-TF-238、AT-TF-239、AT-TF-243、AT-TF-245 的库面——问了关务的读数连同关务判断标识与依据原文落进明细、整格另成
// 一列，按键、按前版找后继、按范围各条读路都原样装回，没声明去向读回仍写明；停在「交出节点未声明」「去向与交出节点相同」
// 的那几版读回同一停点；拒收那一版两列皆 NULL；查阅册列出整格。
func TestHandoverGateReadingsRoundTripThroughEveryReadPath(t *testing.T) {
	stores := newReviewCatalogueStores(t)
	ctx := t.Context()
	nodes := domain.HandoverNodeDeclarations{Releasing: handoverValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SZX-GATE")}
	first := handedOverRecordWithNodes(t, "HRV-G-1", nodes)
	blocked, err := first.Handover.WithGateReading(askedGateReading(t, first.Handover, domain.GateBlocked))
	if err != nil {
		t.Fatalf("挂阻断读数：%v", err)
	}
	first.Handover = blocked
	mustSaveHandover(t, stores.transactor, ctx, stores.handovers, first)

	querier, err := stores.db.ReadExecutor(ctx)
	if err != nil {
		t.Fatalf("读口：%v", err)
	}
	var answer string
	var detail []byte
	if err := querier.QueryRow(ctx,
		`SELECT gate_reading, gate_reading_detail FROM transport_fulfillment.transport_handover WHERE handover_version = 'HRV-G-1'`,
	).Scan(&answer, &detail); err != nil {
		t.Fatalf("读两列：%v", err)
	}
	var stored struct {
		Node    *string `json:"node"`
		Onward  *string `json:"onward"`
		Stop    *string `json:"stop"`
		Finding struct {
			Object    string  `json:"object"`
			Answer    string  `json:"answer"`
			Judgement *string `json:"judgement"`
			Bases     []struct {
				Action  string   `json:"action"`
				Cause   string   `json:"cause"`
				Sources []string `json:"sources"`
			} `json:"bases"`
		} `json:"finding"`
	}
	if err := json.Unmarshal(detail, &stored); err != nil {
		t.Fatalf("明细：%v", err)
	}
	if answer != "BLOCKED" || stored.Node == nil || *stored.Node != "SYN-NODE-SZX-GATE" || stored.Onward != nil || stored.Stop != nil ||
		stored.Finding.Judgement == nil || *stored.Finding.Judgement != "AADJ-parcel-1" || len(stored.Finding.Bases) != 1 ||
		stored.Finding.Bases[0].Action != "CROSS_CUSTOMS_MOVEMENT" || stored.Finding.Bases[0].Sources[0] != "SYN-SOURCE-1" {
		t.Fatalf("落库 %s %s", answer, detail)
	}

	found, exists, err := stores.handovers.FindByKey(ctx, handoverKeyFixture(t, "tenant-1", "HRV-G-1"))
	if err != nil || !exists {
		t.Fatalf("按键读回：%v exists=%v", err, exists)
	}
	reading, read := found.Handover.GateReading()
	if !read || reading.Answer() != domain.GateBlocked || !reading.OnwardNotDeclared() {
		t.Fatalf("按键读回的读数 = %+v", reading)
	}
	if finding, _ := reading.Finding(); len(finding.Bases()) != 1 || finding.Bases()[0].Cause != "CUSTOMS_SCOPE_UNRESOLVED" {
		t.Fatalf("依据原文没装回：%+v", finding.Bases())
	}

	corrected, err := found.Handover.Correct(domain.HandoverCorrection{
		Verdict:           domain.ObjectHandedOver,
		ReleasingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-out-2"),
		ReceivingEvidence: handoverValue(t, domain.NewHandoverEvidenceReference, "seal-in-2"),
		Rule:              handoverValue(t, domain.NewHandoverRuleReference, "rule/v1"),
		Version:           handoverValue(t, domain.NewHandoverResultVersion, "HRV-G-2"),
		CorrectedAt:       handoverJudgedAtFixture.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	unreadable, err := domain.FormHandoverGateReading(domain.HandoverGateReadingSpec{Stop: domain.HandoverGateRegistryUnavailable})
	if err != nil {
		t.Fatalf("读不动停点：%v", err)
	}
	if corrected, err = corrected.WithGateReading(unreadable); err != nil {
		t.Fatalf("挂读不动读数：%v", err)
	}
	mustSaveHandover(t, stores.transactor, ctx, stores.handovers, ports.TransportHandoverRecord{
		Key:           handoverKeyFixture(t, "tenant-1", "HRV-G-2"),
		ContentDigest: "digest-corrected",
		Handover:      corrected,
		RecordedAt:    handoverJudgedAtFixture.Add(time.Hour),
	})
	successor, exists, err := stores.handovers.FindSuccessor(ctx, handoverKeyFixture(t, "tenant-1", "HRV-G-1"))
	if err != nil || !exists {
		t.Fatalf("按前版找后继：%v exists=%v", err, exists)
	}
	if reading, _ := successor.Handover.GateReading(); reading.Answer() != domain.GateUnreadable {
		t.Fatalf("更正版读数 = %s", reading.Answer())
	}

	plain := handedOverRecord(t, "HRV-G-3")
	mustSaveHandover(t, stores.transactor, ctx, stores.handovers, plain)
	mustSaveHandover(t, stores.transactor, ctx, stores.handovers, refusedRecord(t, "HRV-G-4"))
	gateNode := handoverValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SZX-GATE")
	mustSaveHandover(t, stores.transactor, ctx, stores.handovers,
		handedOverRecordWithNodes(t, "HRV-G-5", domain.HandoverNodeDeclarations{Releasing: gateNode, Onward: gateNode}))
	scoped, err := stores.handovers.ListByScope(ctx, first.Key.TenantID, first.Key.Scope)
	if err != nil || len(scoped) != 5 {
		t.Fatalf("按范围读：%v rows=%d", err, len(scoped))
	}
	want := map[string]string{"HRV-G-1": "BLOCKED", "HRV-G-2": "UNREADABLE", "HRV-G-3": "UNDECIDED", "HRV-G-4": "", "HRV-G-5": "UNDECIDED"}
	wantStop := map[string]domain.HandoverGateStop{
		"HRV-G-3": domain.HandoverReleasingNodeNotDeclared, "HRV-G-5": domain.HandoverOnwardSameAsReleasing,
	}
	for _, record := range scoped {
		reading, read := record.Handover.GateReading()
		got := ""
		if read {
			got = reading.Answer().String()
		}
		if got != want[record.Key.Version.String()] {
			t.Fatalf("按范围读回 %s 的读数 = %q", record.Key.Version, got)
		}
		if want, stopped := wantStop[record.Key.Version.String()]; stopped {
			if stop, _ := reading.Stop(); stop != want {
				t.Fatalf("%s 的停点 = %s，want %s", record.Key.Version, stop, want)
			}
		}
	}

	rows, err := stores.catalogue.ListTransportHandovers(ctx, first.Key.TenantID, 10)
	if err != nil || len(rows) != 5 {
		t.Fatalf("上列交接册：%v rows=%d", err, len(rows))
	}
	for _, row := range rows {
		if row.GateReading != want[row.Version] {
			t.Fatalf("册面 %s 的读数列 = %q", row.Version, row.GateReading)
		}
	}
}

// Covers: AT-TF-239、AT-TF-244 的库面半边——迁移 0041 的库面：`已交接`缺读数、拒收带读数、整格越出封闭集、整格与明细一缺一在、
// 明细停点与答复都在或都不在、问了关务的读数与这一行的交出节点、去向、对象对不上，都被 CHECK 拒；断言落到 SQLSTATE 与
// 约束名上。存量回填那一形（未决、停点「登记于门禁读数之前」）读回成形。
func TestTheDatabaseGuardsTheHandoverGateReadingShape(t *testing.T) {
	repository, _, pool := newTransportHandovers(t)
	const backfill = `{"node": null, "onward": null, "stop": "RECORDED_BEFORE_GATE_READING", "finding": null}`
	insertDeclared := func(version string, verdict, rule, basis, answer, detail, releasing, onward any) error {
		_, err := pool.Exec(t.Context(),
			`INSERT INTO transport_fulfillment.transport_handover
				(tenant_id, object_ref, scope_ref, handover_version,
				 released_by, received_by, verdict,
				 releasing_evidence, receiving_evidence, rule_ref, basis_ref,
				 corrects_version, corrected_at, judged_at, content_digest, recorded_at,
				 gate_reading, gate_reading_detail, releasing_node, onward_node)
			 VALUES ('tenant-1', 'parcel-1', 'scope-1', $1,
			         'node-1', 'carrier-1', $2,
			         'seal-out', 'seal-in', $3, $4,
			         NULL, NULL, $5, 'digest-1', $5,
			         $6, $7::jsonb, $8, $9)`,
			version, verdict, rule, basis, handoverJudgedAtFixture, answer, detail, releasing, onward)
		return err
	}
	insert := func(version string, verdict, rule, basis, answer, detail any) error {
		return insertDeclared(version, verdict, rule, basis, answer, detail, nil, nil)
	}
	for name, tc := range map[string]struct {
		verdict, rule, basis, answer, detail any
		constraint                           string
	}{
		"handed over without a reading": {"HANDED_OVER", "rule/v1", nil, nil, nil, "transport_handover_gate_reading_on_handed_over"},
		"refused with a reading":        {"REFUSED", nil, "seal-broken", "UNDECIDED", backfill, "transport_handover_gate_reading_on_handed_over"},
		"answer out of the closed set":  {"HANDED_OVER", "rule/v1", nil, "MAYBE", backfill, "transport_handover_gate_reading_closed"},
		"answer without detail":         {"HANDED_OVER", "rule/v1", nil, "UNDECIDED", nil, "transport_handover_gate_reading_detail_coupled"},
		"neither stop nor finding": {"HANDED_OVER", "rule/v1", nil, "UNDECIDED",
			`{"node": null, "onward": null, "stop": null, "finding": null}`, "transport_handover_gate_reading_detail_shape"},
		"both stop and finding": {"HANDED_OVER", "rule/v1", nil, "UNDECIDED",
			`{"stop": "RELEASING_NODE_NOT_DECLARED", "finding": {"object": "parcel-1"}}`, "transport_handover_gate_reading_detail_shape"},
		"keys missing": {"HANDED_OVER", "rule/v1", nil, "UNDECIDED", `{}`, "transport_handover_gate_reading_detail_shape"},
	} {
		t.Run(name, func(t *testing.T) {
			err := insert("HRV-CHECK-"+name, tc.verdict, tc.rule, tc.basis, tc.answer, tc.detail)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != tc.constraint {
				t.Fatalf("err = %v，want 23514 / %s", err, tc.constraint)
			}
		})
	}

	// 问了关务的读数与这一行的声明对不上：交出节点、去向、对象各错一格，或这一行根本没声明交出节点。
	asked := func(node, onward, object string) string {
		quote := func(value string) string {
			if value == "" {
				return "null"
			}
			return `"` + value + `"`
		}
		return `{"node": ` + quote(node) + `, "onward": ` + quote(onward) + `, "stop": null, "finding": {"object": "` + object +
			`", "answer": "NOT_GUARDED", "judgement": "AADJ-1", "bases": [{"action": "OUTBOUND_RELEASE", "cause": "OUTSIDE_CUSTOMS_BOOKS", "sources": []}]}}`
	}
	for name, tc := range map[string]struct {
		detail            string
		releasing, onward any
	}{
		"asked at another node":        {asked("SYN-NODE-SHA-HUB", "SYN-NODE-SIN-HUB", "parcel-1"), "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB"},
		"asked toward another onward":  {asked("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-GW", "parcel-1"), "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB"},
		"asked with an onward dropped": {asked("SYN-NODE-SZX-GATE", "", "parcel-1"), "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB"},
		"asked about another object":   {asked("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB", "parcel-2"), "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB"},
		"asked without a releasing":    {asked("", "", "parcel-1"), nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			err := insertDeclared("HRV-MATCH-"+name, "HANDED_OVER", "rule/v1", nil, "NOT_GUARDED", tc.detail, tc.releasing, tc.onward)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "transport_handover_gate_reading_matches_handover" {
				t.Fatalf("err = %v，want 23514 / transport_handover_gate_reading_matches_handover", err)
			}
		})
	}
	if err := insertDeclared("HRV-MATCHED", "HANDED_OVER", "rule/v1", nil, "NOT_GUARDED",
		asked("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB", "parcel-1"), "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB"); err != nil {
		t.Fatalf("与声明对得上的问询读数进不了库：%v", err)
	}
	if found, exists, err := repository.FindByKey(t.Context(), handoverKeyFixture(t, "tenant-1", "HRV-MATCHED")); err != nil || !exists {
		t.Fatalf("读回与声明对得上的那一行：%v exists=%v", err, exists)
	} else if reading, _ := found.Handover.GateReading(); reading.Answer() != domain.GateNotGuarded {
		t.Fatalf("读回的读数 = %s", reading.Answer())
	}

	if err := insert("HRV-BACKFILL", "HANDED_OVER", "rule/v1", nil, "UNDECIDED", backfill); err != nil {
		t.Fatalf("存量回填那一形进不了库：%v", err)
	}
	found, exists, err := repository.FindByKey(t.Context(), handoverKeyFixture(t, "tenant-1", "HRV-BACKFILL"))
	if err != nil || !exists {
		t.Fatalf("读回回填行：%v exists=%v", err, exists)
	}
	reading, _ := found.Handover.GateReading()
	if stop, _ := reading.Stop(); reading.Answer() != domain.GateUndecided || stop != domain.HandoverRecordedBeforeGateReading {
		t.Fatalf("回填行读数 = %s %s", reading.Answer(), stop)
	}

	if err := insert("HRV-MISMATCH", "HANDED_OVER", "rule/v1", nil, "BLOCKED", backfill); err != nil {
		t.Fatalf("整格与明细对不上的一行（库面不核，领域重建门核）：%v", err)
	}
	if _, _, err := repository.FindByKey(t.Context(), handoverKeyFixture(t, "tenant-1", "HRV-MISMATCH")); !errors.Is(err, domain.ErrInvalidHandoverGateReading) {
		t.Fatalf("整格与停点对不上的一行装回了：%v", err)
	}
}

// Covers: 迁移 0041 的存量回填——迁移之前登下的`已交接`（含迁移 0039 之后声明了交出节点的那一形）回填为未决「登记于门禁
// 读数之前」，读回成形、不按节点核；拒收那一行两列仍是 NULL。回填语句取迁移文件原文，不在这里另抄一份。
func TestTheMigrationBackfillsEarlierHandoversAsRecordedBeforeTheGateReading(t *testing.T) {
	repository, _, pool := newTransportHandovers(t)
	ctx := t.Context()

	assets, err := migrations.TransportFulfillment()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	var backfill string
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "0041_") {
			start := strings.Index(asset.SQL, "UPDATE transport_fulfillment.transport_handover")
			if start < 0 {
				t.Fatal("迁移 0041 里找不到回填那一句")
			}
			end := strings.Index(asset.SQL[start:], ";")
			if end < 0 {
				t.Fatal("迁移 0041 的回填那一句没有收尾")
			}
			backfill = asset.SQL[start : start+end]
		}
	}
	if backfill == "" {
		t.Fatal("没有迁移 0041")
	}

	for _, statement := range []string{
		`ALTER TABLE transport_fulfillment.transport_handover DROP CONSTRAINT transport_handover_gate_reading_on_handed_over`,
		`INSERT INTO transport_fulfillment.transport_handover
		     (tenant_id, object_ref, scope_ref, handover_version, released_by, received_by, verdict,
		      releasing_evidence, receiving_evidence, rule_ref, basis_ref, judged_at, content_digest, recorded_at, releasing_node)
		 VALUES ('tenant-1', 'parcel-1', 'scope-1', 'HRV-LEGACY', 'node-1', 'carrier-1', 'HANDED_OVER',
		         'seal-out', 'seal-in', 'rule/v1', NULL, now(), 'digest-1', now(), 'SYN-NODE-SZX-GATE'),
		        ('tenant-1', 'parcel-1', 'scope-1', 'HRV-LEGACY-REFUSED', 'node-1', 'carrier-1', 'REFUSED',
		         'seal-out', NULL, NULL, 'seal-broken', now(), 'digest-2', now(), NULL)`,
		backfill,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("执行 %q：%v", statement, err)
		}
	}

	found, exists, err := repository.FindByKey(ctx, handoverKeyFixture(t, "tenant-1", "HRV-LEGACY"))
	if err != nil || !exists {
		t.Fatalf("回填后的已交接装不回：%v exists=%v", err, exists)
	}
	reading, read := found.Handover.GateReading()
	if stop, _ := reading.Stop(); !read || reading.Answer() != domain.GateUndecided || stop != domain.HandoverRecordedBeforeGateReading {
		t.Fatalf("回填读数 = %s %s", reading.Answer(), stop)
	}
	refused, exists, err := repository.FindByKey(ctx, handoverKeyFixture(t, "tenant-1", "HRV-LEGACY-REFUSED"))
	if err != nil || !exists {
		t.Fatalf("拒收那一行装不回：%v exists=%v", err, exists)
	}
	if _, read := refused.Handover.GateReading(); read {
		t.Fatalf("拒收那一行被回填了读数")
	}
}
