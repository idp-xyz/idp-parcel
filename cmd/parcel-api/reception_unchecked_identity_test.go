package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件对真实 PostgreSQL 证身份核对读不动翻成`待识别实物已建立`加「未核对」（ADR-0199 决定五，票 no-reception-source/05）的生产
// 装配面：编排、收寄库、身份读口的 savepoint、意图交付与自投门铃都是生产装配的；PS 读不动的形状是把外部标识关联变更册挪走，
// 身份读口于是报 SQL 级错误（42P01）。夹具全为合成（S 级）。门铃投递之后怎样续上见 parcel-dispatch 的门铃真库用例。

// uncheckedAnswer 是收寄答复里本文件要看的几格。
type uncheckedAnswer struct {
	status          int
	raw             string
	Outcome         string   `json:"outcome"`
	IdentityCheck   string   `json:"identityCheck"`
	RecordedOutcome string   `json:"recordedOutcome"`
	CurrentVersion  string   `json:"currentVersion"`
	IntakeVersion   string   `json:"intakeVersion"`
	Association     string   `json:"association"`
	Candidates      []string `json:"candidates"`
	ControlBasis    string   `json:"controlBasis"`
}

// controlStartsOfUnit 数这件实物在册上的控制起点：带控制的各行里控制依据不同的有几个。
func controlStartsOfUnit(t *testing.T, pool *pgxpool.Pool, command nodeopsapp.ReceiveDeliveredUnitCommand) int {
	t.Helper()
	return countRows(t, pool, `SELECT count(DISTINCT control ->> 'basis') FROM node_operations.reception
	  WHERE tenant_id = $1 AND control IS NOT NULL AND intake ->> 'unit' = $2`, command.TenantID.String(), command.Unit.String())
}

// uncheckedDoorbells 交回各封自投门铃载荷里的判断版本。
func uncheckedDoorbells(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1 ORDER BY event_id`,
		nopostgres.UncheckedPendingIdentityRecordedEventType)
	if err != nil {
		t.Fatalf("读门铃：%v", err)
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("扫门铃：%v", err)
		}
		var payload struct {
			JudgmentVersion string `json:"judgmentVersion"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("门铃载荷：%v", err)
		}
		versions = append(versions, payload.JudgmentVersion)
	}
	return versions
}

func decodeUnchecked(t *testing.T, answer receptionAnswer) uncheckedAnswer {
	t.Helper()
	decoded := uncheckedAnswer{status: answer.status, raw: answer.raw}
	if err := json.Unmarshal([]byte(answer.raw), &decoded); err != nil {
		t.Fatalf("解码响应 %q：%v", answer.raw, err)
	}
	return decoded
}

// Covers: 票 no-reception-source/05 真库用例一（票 01 点名）与 `AT-NO-051` 的生产装配面——PS 身份读口读不动期间，一次明确
// 接收答 200 PENDING_IDENTIFICATION、identityCheck UNCHECKED、没有候选与关联、控制照建；同一来源重投（PS 仍读不动、PS 恢复后
// 各一次）收寄不多出一条：答 200 EXISTING_RESULT，recordedOutcome 是 PENDING_IDENTIFICATION、identityCheck 仍是 UNCHECKED、
// currentVersion 是那一版。册上一行、一个控制起点、意图为零；同一笔自投的门铃恰一封、指名那一版。重投不重判身份：续上靠门铃
// 重核，不靠重投——翻转之后设备收到 2xx 就出队。
func TestTheWiredReceptionFormsAnUncheckedPendingUnitWhileIdentityIsUnreadable(t *testing.T) {
	pool, reception, command := wiredIdentifiedReception(t, "SYN-DELIVERY-NRS05-UNCHECKED")
	restore := moveTable(t, pool, "external_identifier_relation_change")

	first := decodeUnchecked(t, postWiredReception(t, reception, command))
	if first.status != http.StatusOK || first.Outcome != "PENDING_IDENTIFICATION" || first.IdentityCheck != "UNCHECKED" {
		t.Fatalf("身份读不动首答 %d %s, want 200 PENDING_IDENTIFICATION identityCheck UNCHECKED", first.status, first.raw)
	}
	if len(first.Candidates) != 0 || first.Association != "" || first.ControlBasis == "" {
		t.Fatalf("首答 %s, want 没有候选与关联、控制照建", first.raw)
	}

	stillDown := decodeUnchecked(t, postWiredReception(t, reception, command))
	restore()
	recovered := decodeUnchecked(t, postWiredReception(t, reception, command))
	for name, replay := range map[string]uncheckedAnswer{"PS 仍读不动": stillDown, "PS 恢复后": recovered} {
		if replay.status != http.StatusOK || replay.Outcome != "EXISTING_RESULT" || replay.RecordedOutcome != "PENDING_IDENTIFICATION" ||
			replay.IdentityCheck != "UNCHECKED" || replay.CurrentVersion != first.IntakeVersion {
			t.Fatalf("%s重投 %d %s, want 200 EXISTING_RESULT、PENDING_IDENTIFICATION、UNCHECKED、currentVersion %s",
				name, replay.status, replay.raw, first.IntakeVersion)
		}
	}
	if rows := receptionRows(t, pool, command.SourceID); rows != 1 {
		t.Fatalf("收寄 %d 行, want 1", rows)
	}
	if starts := controlStartsOfUnit(t, pool, command); starts != 1 {
		t.Fatalf("控制起点 %d 个, want 1", starts)
	}
	if intents := nodeIntakeIntents(t, pool); intents != 0 {
		t.Fatalf("意图 %d 封, want 0——未核对的待识别没有可采用的关联", intents)
	}
	if doorbells := uncheckedDoorbells(t, pool); len(doorbells) != 1 || doorbells[0] != first.IntakeVersion {
		t.Fatalf("门铃 %v, want 恰一封、指名 %s", doorbells, first.IntakeVersion)
	}
}

// Covers: 票 no-reception-source/05 真库用例三（票 01 点名）——换来源重扫同一作业实物，不再形成第二个控制起点：PS 读不动时落下
// 未核对的待识别之后，同一件实物换来源再送到同一节点（PS 仍读不动一次、PS 恢复后一次），都并入那一条：答 200 EXISTING_RESULT，
// recordedOutcome 是 PENDING_IDENTIFICATION、identityCheck 是 UNCHECKED、currentVersion 是那一版；册上各落一行并入、指名那一版；
// 控制起点一个、意图为零、门铃仍是一封。PS 恢复后也不因身份读得动了绕过并入。
func TestTheWiredReceptionMergesTheSameUnitIntoItsUncheckedPendingUnit(t *testing.T) {
	pool, reception, command := wiredIdentifiedReception(t, "SYN-DELIVERY-NRS05-FIRST")
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	restore := moveTable(t, pool, "external_identifier_relation_change")
	first := decodeUnchecked(t, postWiredReception(t, reception, command))
	if first.Outcome != "PENDING_IDENTIFICATION" || first.IdentityCheck != "UNCHECKED" {
		t.Fatalf("首答 %s, want 未核对的待识别", first.raw)
	}

	for index, name := range []string{"PS-DOWN", "PS-UP"} {
		if name == "PS-UP" {
			restore()
		}
		again := sameUnitAgain(t, command, "SYN-DELIVERY-NRS05-AGAIN-"+name, nodeopsapp.ExplicitReception, time.Duration(index+1)*time.Hour)
		merged := decodeUnchecked(t, postWiredReception(t, reception, again))
		if merged.status != http.StatusOK || merged.Outcome != "EXISTING_RESULT" || merged.RecordedOutcome != "PENDING_IDENTIFICATION" ||
			merged.IdentityCheck != "UNCHECKED" || merged.CurrentVersion != first.IntakeVersion {
			t.Fatalf("%s 换来源再来 %d %s, want 200 EXISTING_RESULT 并入那条未核对的待识别", name, merged.status, merged.raw)
		}
		row, found, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: again.TenantID, SourceID: again.SourceID})
		if err != nil || !found || row.Kind != noports.RecordMergedIntoExistingIntake || row.MergedInto.String() != first.IntakeVersion {
			t.Fatalf("%s 的并入行 = %+v（found %v err %v）, want 指名 %s", name, row, found, err, first.IntakeVersion)
		}
	}
	if starts := controlStartsOfUnit(t, pool, command); starts != 1 {
		t.Fatalf("控制起点 %d 个, want 1", starts)
	}
	if intents, doorbells := nodeIntakeIntents(t, pool), uncheckedDoorbells(t, pool); intents != 0 || len(doorbells) != 1 {
		t.Fatalf("意图 %d 封、门铃 %d 封, want 0、1", intents, len(doorbells))
	}
}
