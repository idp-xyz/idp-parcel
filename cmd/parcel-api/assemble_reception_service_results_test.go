package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件是收寄编排服务结果读口的真库半边：生产装配的读口经 NO 的防腐适配器读 PS 的取消决定册与终局册（票
// operator-channel/25），读不动时收寄照常落册、记读不动（ADR-0186）。读口跟着收寄走调用方那一笔事务，并不是为了
// 一致性——PS 读不加锁，READ COMMITTED 下同事务读既无收益也无代价；它在事务里，所以要经 savepoint 隔离，免得一次
// 读失败作废收寄的写。夹具全为合成登记（S 级）。

var serviceResultsAt = time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)

// seedParcelShipmentServiceResults 在 PS 两册上各登一份：parcel 上一份已成立的取消决定与一份已定案的终局，走 PS 的生产写口。
func seedParcelShipmentServiceResults(t *testing.T, db *bentopg.DB, tenant, parcel string) {
	t.Helper()
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		t.Fatal(err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		t.Fatal(err)
	}
	psTenant := mustValue(t, psdomain.NewTenantID, tenant)
	psParcel := mustValue(t, psdomain.NewDeclaredParcelID, parcel)
	cancellation, err := psdomain.DecideParcelCancellation(psdomain.ParcelCancellationSpec{
		ID:          mustValue(t, psdomain.NewParcelCancellationID, "SYN-CANCEL-OC25"),
		Parcel:      psParcel,
		Requester:   mustValue(t, psdomain.NewCancellationRequesterReference, "SYN-CUSTOMER-OC25"),
		Authority:   mustValue(t, psdomain.NewCancellationAuthorityReference, "SYN-CANCEL-RULE-OC25"),
		Reason:      mustValue(t, psdomain.NewCancellationReasonReference, "SYN-CHANGED-MIND"),
		RequestedAt: serviceResultsAt,
	}, psdomain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("取消决定：%v", err)
	}
	finalKey := psports.FinalAdoptionKey{
		TenantID: psTenant,
		Parcel:   psParcel,
		Kind:     psdomain.EffectiveDeliveryOutcome,
		Version:  mustValue(t, psdomain.NewResponsibilityOutcomeVersion, "SYN-DELIVERY-OC25/v1"),
	}
	source, err := psdomain.NewResponsibilityOutcome(psdomain.ResponsibilityOutcomeSpec{
		Kind:       finalKey.Kind,
		Parcel:     psParcel,
		Decision:   mustValue(t, psdomain.NewResponsibilityDecisionReference, "SYN-DELIVERY-JUDGMENT-OC25"),
		Execution:  mustValue(t, psdomain.NewExecutionEvidenceReference, "SYN-POD-OC25"),
		Version:    finalKey.Version,
		OccurredAt: serviceResultsAt,
	})
	if err != nil {
		t.Fatalf("责任结果：%v", err)
	}
	final, err := psdomain.FormParcelFinalOutcome(psdomain.ParcelFinalOutcomeSpec{
		Version:     mustValue(t, psdomain.NewFinalOutcomeVersionID, "SYN-FINAL-OC25/v1"),
		Parcel:      psParcel,
		Kind:        mustValue(t, psdomain.NewFinalKindReference, "NETWORK_SERVICE_DELIVERED"),
		Source:      source,
		RuleVersion: mustValue(t, psdomain.NewFinalRuleVersionReference, "SYN-FINAL-RULES-OC25/v1"),
	})
	if err != nil {
		t.Fatalf("终局：%v", err)
	}
	// 记录在回调外构造：回调里的 t.Fatal 走 runtime.Goexit，bento 只在 recover 到 panic 时回滚，事务就既不提交也不回滚。
	cancellationRecord := psports.CancellationRecord{
		Key: psports.CancellationRequestKey{
			TenantID:   psTenant,
			RequestKey: mustValue(t, psdomain.NewSourceRequestKey, "SYN-CANCEL-REQUEST-OC25"),
			Parcel:     psParcel,
		},
		ContentDigest: "sha256:syn-cancel-oc25",
		Kind:          psports.RecordParcelCancelled,
		Cancellation:  cancellation,
		DecidedAt:     serviceResultsAt,
	}
	finalRecord := psports.FinalOutcomeRecord{
		Key:           finalKey,
		ContentDigest: "sha256:syn-final-oc25",
		Finalized:     true,
		Final:         final,
		AdoptedAt:     serviceResultsAt,
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := cancellations.Save(txCtx, cancellationRecord); err != nil {
			return err
		}
		_, err := finals.Save(txCtx, finalRecord)
		return err
	}); err != nil {
		t.Fatalf("登 PS 服务结果：%v", err)
	}
}

// Covers: 票 operator-channel/25 完成判据「生产装配接上 PS 真库读口」的真库面——生产装配的服务结果读口在一笔事务里读出
// PS 的取消决定与已定案终局，另一件什么都没有的包裹答空。
func TestTheWiredServiceResultViewReadsParcelShipmentCancellationAndFinal(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seedParcelShipmentServiceResults(t, db, "SYN-TENANT-01", "SYN-PARCEL-OC25")
	view, err := buildParcelServiceResultView(db)
	if err != nil {
		t.Fatalf("装配服务结果读口：%v", err)
	}
	tenant := mustValue(t, nodomain.NewTenantID, "SYN-TENANT-01")
	read := func(parcel string) noports.ParcelServiceResults {
		t.Helper()
		association := mustValue(t, nodomain.NewParcelAssociationReference, parcel)
		var results noports.ParcelServiceResults
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var findErr error
			results, findErr = view.FindParcelServiceResults(txCtx, tenant, association)
			return findErr
		}); err != nil {
			t.Fatalf("读 %s：%v", parcel, err)
		}
		return results
	}
	if got := read("SYN-PARCEL-OC25"); got != (noports.ParcelServiceResults{
		Cancellation: "SYN-CANCEL-OC25", FinalKind: "NETWORK_SERVICE_DELIVERED", FinalVersion: "SYN-FINAL-OC25/v1",
	}) {
		t.Fatalf("登过的包裹读回 %+v", got)
	}
	if got := read("SYN-PARCEL-OC25-NONE"); got != (noports.ParcelServiceResults{}) {
		t.Fatalf("什么都没登的包裹读回 %+v，want 空", got)
	}
}

// fixedReceptionIntake 是接入层的替身，交回一条备好的命令：认证与载荷解析不是这里要证的，状态码与响应体出自真 HTTP
// 适配器、答案出自生产装配的收寄编排。
type fixedReceptionIntake struct {
	command nodeopsapp.ReceiveDeliveredUnitCommand
}

func (intake fixedReceptionIntake) IntakeReception(context.Context, *http.Request) (nodeopsapp.ReceiveDeliveredUnitCommand, error) {
	return intake.command, nil
}

type receptionAnswer struct {
	status             int
	raw                string
	Outcome            string   `json:"outcome"`
	Association        string   `json:"association"`
	ServiceMarkers     []string `json:"serviceMarkers"`
	ServiceResultCheck string   `json:"serviceResultCheck"`
}

func postWiredReception(t *testing.T, reception nodeopshttp.ReceptionHandler, command nodeopsapp.ReceiveDeliveredUnitCommand) receptionAnswer {
	t.Helper()
	endpoint := nodeopshttp.NewReceiveDeliveredUnitEndpoint(fixedReceptionIntake{command: command}, reception)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader("{}")))
	answer := receptionAnswer{status: recorder.Code, raw: recorder.Body.String()}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("解码响应 %q：%v", answer.raw, err)
	}
	return answer
}

// countRows 数一条 count(*) 查询的结果。
func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("计数 %q：%v", query, err)
	}
	return count
}

func receptionRows(t *testing.T, pool *pgxpool.Pool, sourceID string) int {
	t.Helper()
	return countRows(t, pool, `SELECT count(*) FROM node_operations.reception WHERE source_id = $1`, sourceID)
}

func nodeIntakeIntents(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	return countRows(t, pool,
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'node-operations.node-intake.formed'`)
}

// moveTable 把一张 PS 表挪到别的名字下，PS 读口于是报 SQL 级错误（42P01）——真库上能造出的最接近「读不动」的形状，
// 不换读口、不改装配。restore 挪回。
func moveTable(t *testing.T, pool *pgxpool.Pool, table string) (restore func()) {
	t.Helper()
	moved := table + "_moved_oc29"
	if _, err := pool.Exec(t.Context(), `ALTER TABLE parcel_shipment.`+table+` RENAME TO `+moved); err != nil {
		t.Fatalf("挪走 %s：%v", table, err)
	}
	return func() {
		t.Helper()
		if _, err := pool.Exec(t.Context(), `ALTER TABLE parcel_shipment.`+moved+` RENAME TO `+table); err != nil {
			t.Fatalf("挪回 %s：%v", table, err)
		}
	}
}

// wiredIdentifiedReception 在真库上备一件已接受、带客户包裹件号的包裹与那个客户账户列着它的送站预报，交回生产装配的收寄编排与
// 一条按那个号码、带着那份预报明确接收的命令——身份核对对它答唯一命中（ADR-0212 决定四：客户包裹件号的唯一识别要有可验证交付范围）。
func wiredIdentifiedReception(t *testing.T, sourceID string) (*pgxpool.Pool, nodeopshttp.ReceptionHandler, nodeopsapp.ReceiveDeliveredUnitCommand) {
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
	acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-0001"))
	command := receptionCommand(t, sourceID, nodeopsapp.ExplicitReception)
	command.Mark = noports.ExternalMarkObservation{Mark: "SYN-CUSTOMER-PARCEL-0001"}
	command.ScopeBasis = preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-CUSTOMER-PARCEL-0001")
	return pool, reception, command
}

// Covers: ADR-0186「身份唯一，但服务结果读不动：照常`节点收寄已形成`」的生产装配面——PS 取消决定册被挪走，一次身份唯一
// 的收寄照样答 201 INTAKE_FORMED、核对态读不动、不带标记、意图一封；重投答 200 EXISTING_RESULT，仍是一行、核对态照读回。
func TestTheWiredReceptionFormsTheIntakeWhenServiceResultsAreUnreadable(t *testing.T) {
	pool, reception, command := wiredIdentifiedReception(t, "SYN-DELIVERY-OC29-UNREADABLE")
	moveTable(t, pool, "parcel_cancellation")

	first := postWiredReception(t, reception, command)
	if first.status != http.StatusCreated || first.Outcome != "INTAKE_FORMED" {
		t.Fatalf("首答 %d %s, want 201 INTAKE_FORMED——服务结果读不动不挡物理事实", first.status, first.raw)
	}
	if first.ServiceResultCheck != "UNREADABLE" || len(first.ServiceMarkers) != 0 {
		t.Fatalf("首答 核对态 %q 标记 %v, want UNREADABLE 且无标记", first.ServiceResultCheck, first.ServiceMarkers)
	}
	if got := nodeIntakeIntents(t, pool); got != 1 {
		t.Fatalf("意图 %d 封, want 1", got)
	}

	replay := postWiredReception(t, reception, command)
	if replay.status != http.StatusOK || replay.Outcome != "EXISTING_RESULT" || replay.ServiceResultCheck != "UNREADABLE" {
		t.Fatalf("重投 %d %s, want 200 EXISTING_RESULT 且核对态读不动", replay.status, replay.raw)
	}
	if got := receptionRows(t, pool, command.SourceID); got != 1 {
		t.Fatalf("收寄 %d 行, want 1", got)
	}
	if got := nodeIntakeIntents(t, pool); got != 1 {
		t.Fatalf("重投后意图 %d 封, want 仍是 1", got)
	}
}
