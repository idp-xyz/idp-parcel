package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// wiredReceptionWithStore 交回真库连接池、框架 DB、生产装配的收寄编排与直读收寄册的适配器。
func wiredReceptionWithStore(t *testing.T) (*pgxpool.Pool, *bentopg.DB, nodeopshttp.ReceptionHandler, *nopostgres.Receptions) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	return pool, db, reception, receptions
}

// commandSource 是一条命令应留下的来源文档：逐格取自命令，外部标识观察不改写。
func commandSource(command nodeopsapp.ReceiveDeliveredUnitCommand, claim string) nodomain.DeliverySource {
	return nodomain.DeliverySource{
		Unit:        command.Unit,
		Mark:        command.Mark.Mark,
		Claim:       claim,
		Evidence:    command.Evidence,
		Refusal:     command.Refusal,
		DeliveredBy: command.DeliveredBy,
		Node:        command.Node,
		OccurredAt:  command.OccurredAt.UTC(),
		ScopeBasis:  command.ScopeBasis,
	}
}

// Covers: `AT-NO-047`「明确接收、明确拒收、只有扫描各来一次交付——每一次的来源都随判断保全，外部标识观察原样」的真库面
// （ADR-0199 决定一）：三种观察各经生产装配送一次站，落册后经收寄库读回，来源文档与送进去的逐格相同，在线口带两端空白的
// 号原样进了 jsonb；三行都带判断版本。只在证据上不同的重投答已有结果，在册的来源文档不变。夹具全为合成（S 级）。
func TestTheWiredReceptionKeepsTheDeliverySourceOfEveryObservation(t *testing.T) {
	pool, _, reception, receptions := wiredReceptionWithStore(t)

	received := receptionCommand(t, "SYN-DELIVERY-NRS02-RECEIVED", nodeopsapp.ExplicitReception)
	received.Mark = noports.ExternalMarkObservation{Mark: "  SYN-BARCODE-NRS02 \t"}
	refused := receptionCommand(t, "SYN-DELIVERY-NRS02-REFUSED", nodeopsapp.ExplicitRefusal)
	refused.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-NRS02-2")
	scanned := receptionCommand(t, "SYN-DELIVERY-NRS02-SCANNED", nodeopsapp.ScanOnlyObservation)
	scanned.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-NRS02-3")

	for _, delivery := range []struct {
		command nodeopsapp.ReceiveDeliveredUnitCommand
		claim   string
	}{{received, "RECEIVED"}, {refused, "REFUSED"}, {scanned, "SCAN_ONLY"}} {
		command := delivery.command
		if _, err := reception.Handle(t.Context(), command); err != nil {
			t.Fatalf("%s：%v", command.SourceID, err)
		}
		found, exists, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: command.TenantID, SourceID: command.SourceID})
		if err != nil || !exists {
			t.Fatalf("读回 %s：exists=%v err=%v", command.SourceID, exists, err)
		}
		if want := commandSource(command, delivery.claim); found.Source != want {
			t.Fatalf("%s 的来源文档读回 %+v, want %+v", command.SourceID, found.Source, want)
		}
		if found.JudgmentVersion.String() == "" {
			t.Fatalf("%s 没有判断版本", command.SourceID)
		}
		var mark string
		if err := pool.QueryRow(t.Context(),
			`SELECT source_document ->> 'mark' FROM node_operations.reception WHERE tenant_id = $1 AND source_id = $2`,
			command.TenantID.String(), command.SourceID).Scan(&mark); err != nil {
			t.Fatalf("直读 %s 的外部标识观察：%v", command.SourceID, err)
		}
		if mark != command.Mark.Mark {
			t.Fatalf("%s 在库里的外部标识观察 = %q, want 原样 %q", command.SourceID, mark, command.Mark.Mark)
		}
	}

	resent := received
	resent.Evidence = mustValue(t, nodomain.NewReceptionEvidenceReference, "SYN-SIGN-NRS02-OTHER")
	replay, err := reception.Handle(t.Context(), resent)
	if err != nil || replay.Outcome() != nodeopsapp.ReceptionExistingResult {
		t.Fatalf("只在证据上不同的重投 outcome = %v err = %v, want EXISTING_RESULT", replay.Outcome(), err)
	}
	kept, _, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: received.TenantID, SourceID: received.SourceID})
	if err != nil || kept.Source != commandSource(received, "RECEIVED") {
		t.Fatalf("重投之后在册的来源 %+v err = %v, want 首存的那一份", kept.Source, err)
	}
	if rows := receptionRows(t, pool, received.SourceID); rows != 1 {
		t.Fatalf("%s 在册 %d 行, want 1", received.SourceID, rows)
	}
}

// Covers: ADR-0199 决定一「既有行不回填」的生产装配面——来源保全之前落下的行（来源文档为空）照常重放：同一来源同一内容答
// 已有结果，不答冲突、不报错。未收寄那一行从未签过判断版本，当前有效版本缺席；待识别那一行的判断版本由迁移回填为它的收寄
// 结果版本，当前有效版本就是它。重放不给它们补来源文档，也不另落一行。
func TestTheWiredReceptionReplaysRowsWrittenBeforeSourcesWerePreserved(t *testing.T) {
	pool, _, reception, receptions := wiredReceptionWithStore(t)

	refusal := receptionCommand(t, "SYN-DELIVERY-NRS02-LEGACY-REFUSED", nodeopsapp.ExplicitRefusal)
	received := receptionCommand(t, "SYN-DELIVERY-NRS02-LEGACY-RECEIVED", nodeopsapp.ExplicitReception)
	received.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-NRS02-LEGACY")
	digestOf := func(command nodeopsapp.ReceiveDeliveredUnitCommand) string {
		t.Helper()
		_, digest, err := nodomain.CanonicalizeDeliveryPayload(command.Unit, command.Mark.Mark, uint8(command.Claim), command.Node, command.OccurredAt)
		if err != nil {
			t.Fatalf("算摘要：%v", err)
		}
		return digest
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, refusal_reason,
		 service_markers, recorded_at)
		VALUES ($1, $2, $3, 'INTAKE_NOT_FORMED', NULL, NULL, '[]'::jsonb, false, $4, '[]'::jsonb, $5)`,
		refusal.TenantID.String(), refusal.SourceID, digestOf(refusal), refusal.Refusal, receptionDeliveredAt); err != nil {
		t.Fatalf("落旧形状的拒收行：%v", err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict,
		 service_markers, judgment_version, recorded_at)
		VALUES ($1, $2, $3, 'PENDING_IDENTIFICATION',
		        jsonb_build_object('unit', $4::text, 'node', $5::text, 'deliveredBy', $6::text, 'evidence', $7::text,
		                           'version', 'SYN-INTAKEV-NRS02-LEGACY', 'receivedAt', $8::timestamptz),
		        jsonb_build_object('unit', $4::text, 'node', $5::text, 'kind', 'NODE_INTAKE',
		                           'basis', 'NODE-INTAKE/SYN-INTAKEV-NRS02-LEGACY', 'establishedAt', $8::timestamptz),
		        '[]'::jsonb, false, '[]'::jsonb, 'SYN-INTAKEV-NRS02-LEGACY', $8)`,
		received.TenantID.String(), received.SourceID, digestOf(received), received.Unit.String(), received.Node.String(),
		received.DeliveredBy.String(), received.Evidence.String(), receptionDeliveredAt); err != nil {
		t.Fatalf("落旧形状的待识别行：%v", err)
	}

	for _, replay := range []struct {
		command  nodeopsapp.ReceiveDeliveredUnitCommand
		recorded nodeopsapp.ReceptionOutcome
		current  string
	}{
		{refusal, nodeopsapp.NodeIntakeNotFormed, ""},
		{received, nodeopsapp.UnitPendingIdentification, "SYN-INTAKEV-NRS02-LEGACY"},
	} {
		command := replay.command
		result, err := reception.Handle(t.Context(), command)
		if err != nil || result.Outcome() != nodeopsapp.ReceptionExistingResult {
			t.Fatalf("%s 重放 outcome = %v err = %v, want EXISTING_RESULT", command.SourceID, result.Outcome(), err)
		}
		if result.RecordedOutcome() != replay.recorded || result.CurrentVersion().String() != replay.current {
			t.Fatalf("%s 原判断种类 %q 当前有效版本 %q, want %q %q", command.SourceID,
				result.RecordedOutcome(), result.CurrentVersion(), replay.recorded, replay.current)
		}
		kept, _, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: command.TenantID, SourceID: command.SourceID})
		if err != nil || kept.Source.Preserved() {
			t.Fatalf("%s 重放之后读回来源 %+v err = %v, want 仍是来源未保全", command.SourceID, kept.Source, err)
		}
		if rows := receptionRows(t, pool, command.SourceID); rows != 1 {
			t.Fatalf("%s 在册 %d 行, want 1", command.SourceID, rows)
		}
	}
}

// Covers: `AT-NO-048`「原判断已被后续版本接续之后，同一来源重投——返回已有结果，带原判断种类与当前有效版本」的真库面与 HTTP
// 面（ADR-0199 决定二）：委托带客户包裹件号接受之后，两件作业实物先后贴着它经生产装配送站，ADR-0196 给先到那件追加冲突版；
// 冲突版在库里指名先到那件的原判断为前版、带后到那次送站的来源文档。经真 HTTP 适配器重投先到那件：200 EXISTING_RESULT，
// `recordedOutcome` 是 INTAKE_FORMED，`currentVersion` 是冲突版；册上不多一行。两件都带着那个客户账户的送站预报（ADR-0212 决定四：
// 客户包裹件号的唯一识别要有可验证交付范围，一码两物只从唯一命中那一支来）。夹具全为合成（S 级）。
func TestTheWiredReplayAfterAnIdentityConflictAnswersTheCurrentVersion(t *testing.T) {
	pool, db, reception, _ := wiredReceptionWithStore(t)
	acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-NRS02"))
	basis := preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-CUSTOMER-PARCEL-NRS02")

	deliver := func(sourceID, unit string) nodeopsapp.ReceiveDeliveredUnitCommand {
		command := receptionCommand(t, sourceID, nodeopsapp.ExplicitReception)
		command.Unit = mustValue(t, nodomain.NewHandlingUnitID, unit)
		command.Mark = noports.ExternalMarkObservation{Mark: "SYN-CUSTOMER-PARCEL-NRS02"}
		command.ScopeBasis = basis
		return command
	}
	first := deliver("SYN-DELIVERY-NRS02-A", "SYN-UNIT-NRS02-A")
	formed, err := reception.Handle(t.Context(), first)
	if err != nil || formed.Outcome() != nodeopsapp.NodeIntakeFormed {
		t.Fatalf("先到那件 outcome = %v err = %v, want INTAKE_FORMED", formed.Outcome(), err)
	}
	original, _ := formed.Record()
	second := deliver("SYN-DELIVERY-NRS02-B", "SYN-UNIT-NRS02-B")
	if conflicted, err := reception.Handle(t.Context(), second); err != nil || conflicted.Outcome() != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("后到那件 outcome = %v err = %v, want PENDING_IDENTIFICATION", conflicted.Outcome(), err)
	}

	var appendedVersion, appendedSourceUnit string
	if err := pool.QueryRow(t.Context(),
		`SELECT judgment_version, source_document ->> 'unit' FROM node_operations.reception
		  WHERE tenant_id = $1 AND predecessor_version = $2`,
		first.TenantID.String(), original.JudgmentVersion.String()).Scan(&appendedVersion, &appendedSourceUnit); err != nil {
		t.Fatalf("找指名原判断 %q 为前版的冲突版：%v", original.JudgmentVersion, err)
	}
	if appendedSourceUnit != "SYN-UNIT-NRS02-B" {
		t.Fatalf("冲突版来源文档里的作业实物 = %q, want 后到那件 SYN-UNIT-NRS02-B", appendedSourceUnit)
	}

	before := countRows(t, pool, `SELECT count(*) FROM node_operations.reception`)
	answer := postWiredReception(t, reception, first)
	var cells struct {
		RecordedOutcome string `json:"recordedOutcome"`
		CurrentVersion  string `json:"currentVersion"`
	}
	if err := json.Unmarshal([]byte(answer.raw), &cells); err != nil {
		t.Fatalf("解码响应 %q：%v", answer.raw, err)
	}
	if answer.status != 200 || answer.Outcome != "EXISTING_RESULT" {
		t.Fatalf("重投先到那件 %d %q, want 200 EXISTING_RESULT（%s）", answer.status, answer.Outcome, answer.raw)
	}
	if cells.RecordedOutcome != "INTAKE_FORMED" || cells.CurrentVersion != appendedVersion {
		t.Fatalf("recordedOutcome %q currentVersion %q, want INTAKE_FORMED %q", cells.RecordedOutcome, cells.CurrentVersion, appendedVersion)
	}
	if after := countRows(t, pool, `SELECT count(*) FROM node_operations.reception`); after != before {
		t.Fatalf("重投落了 %d 行", after-before)
	}
}

// Covers: 观察号码带 NUL 的送站在入口答坏报文（票 no-reception-source/02 评审阻断）的生产装配面：作业事实收寄口与生产
// 装配之间，三种观察都答 400 MALFORMED_REQUEST、册上不落。来源文档原样保全这个号（ADR-0199 决定一），jsonb 存不下 NUL，
// 放进编排就是 5xx、设备永远重投；明确接收那一支在来源保全之前就已在身份核对按这个号查 PS 时撞错，同由入口挡住。扫描附带的
// Tab 与回车换行照收：带着它们的拒收照常落册，原样读回。夹具全为合成（S 级）。
func TestTheWiredReceptionRefusesAMarkCarryingNUL(t *testing.T) {
	pool, _, reception, receptions := wiredReceptionWithStore(t)
	endpoint := nodeopshttp.NewReceiveDeliveredUnitEndpoint(workFactReceptionForTest(t), reception)
	post := func(sourceID, mark, claim string) *httptest.ResponseRecorder {
		t.Helper()
		encoded, err := json.Marshal(mark)
		if err != nil {
			t.Fatalf("编码 mark：%v", err)
		}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader(
			`{"sourceId":"`+sourceID+`","deliveredBy":"SYN-COURIER-NRS02","handlingUnit":"SYN-HU-NRS02-NUL",`+
				`"mark":`+string(encoded)+`,`+claim+`,"occurredAt":"2026-09-24T09:00:00+08:00"}`))
		stampWorkFact(request, "NODE", "SYN-NODE-SHA-HUB", "SYN-DEVICE-NODE")
		endpoint.ServeHTTP(recorder, request)
		return recorder
	}

	for name, delivery := range map[string]struct{ sourceID, claim string }{
		"明确接收": {"SYN-DEVICE-NRS02/NUL-RECEIVED", `"claim":"RECEIVED","evidenceRef":"SYN-EVIDENCE/tally-NRS02"`},
		"明确拒收": {"SYN-DEVICE-NRS02/NUL-REFUSED", `"claim":"REFUSED","refusalReason":"SYN 外包装破损拒收"`},
		"仅扫描":  {"SYN-DEVICE-NRS02/NUL-SCANNED", `"claim":"SCAN_ONLY"`},
	} {
		t.Run(name, func(t *testing.T) {
			answer := post(delivery.sourceID, "SYN-MARK-\x00-NRS02", delivery.claim)
			if answer.Code != http.StatusBadRequest || problemCode(t, answer) != "MALFORMED_REQUEST" {
				t.Fatalf("带 NUL 的号答 %d %s, want 400 MALFORMED_REQUEST", answer.Code, answer.Body.String())
			}
			if rows := receptionRows(t, pool, delivery.sourceID); rows != 0 {
				t.Fatalf("%s 在册 %d 行, want 0", delivery.sourceID, rows)
			}
		})
	}

	const kept = "SYN-MARK-NRS02\t\r\n"
	answer := post("SYN-DEVICE-NRS02/TAB-REFUSED", kept, `"claim":"REFUSED","refusalReason":"SYN 外包装破损拒收"`)
	if answer.Code != http.StatusOK || registrationOutcome(t, answer) != "INTAKE_NOT_FORMED" {
		t.Fatalf("带 Tab 与回车换行的号答 %d %s, want 200 INTAKE_NOT_FORMED", answer.Code, answer.Body.String())
	}
	tenant := mustValue(t, nodomain.NewTenantID, "SYN-TENANT-01")
	found, exists, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: tenant, SourceID: "SYN-DEVICE-NRS02/TAB-REFUSED"})
	if err != nil || !exists || found.Source.Mark != kept {
		t.Fatalf("读回的外部标识观察 %q exists=%v err=%v, want 原样 %q", found.Source.Mark, exists, err, kept)
	}
}
