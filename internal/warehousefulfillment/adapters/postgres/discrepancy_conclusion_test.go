package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// Covers: AT-WF-050——没有结论时货主确认；再交一次是已有结果；改口是 CONCLUSION_HELD 且现行不变；
// 同一请求键对另一份差异是来源冲突；别的货主与查不到同答；库存行数不变。
func TestDiscrepancyConclusionOwnerConfirmReplayHoldAndSourceConflict(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice, id := world.overageDiscrepancy(t, "REQ-CONC-A", "SYN-FACT-CONC-A")
	_, other := world.overageDiscrepancy(t, "REQ-CONC-B", "SYN-FACT-CONC-B")
	movements := world.movementCount(t)
	lines := world.stockLineCount(t)

	ownerBody := receiptReadJSON(t, world, advice.ID().String(), false)
	ownerReceipt := firstObject(t, objectField(t, ownerBody, "preAdvice"), "receipts")
	if !jsonBool(t, ownerReceipt, "awaitingOwnerConfirmation") || hasKey(ownerReceipt, "conclusion") {
		t.Fatalf("还没有结论：%s", ownerReceipt)
	}

	recorded := world.conclude(t, ownerConclusion(t, id, domain.ConclusionConfirm, "SYN-KEY-A", "确认这一份"))
	if recorded.Outcome() != application.ConcludeDiscrepancyRecorded || verdictOf(t, recorded) != "CONFIRM" {
		t.Fatalf("确认 %s %s", recorded.Outcome(), verdictOf(t, recorded))
	}
	again := world.conclude(t, ownerConclusion(t, id, domain.ConclusionConfirm, "SYN-KEY-A", "再说一遍"))
	if again.Outcome() != application.ConcludeDiscrepancyExistingResult || verdictOf(t, again) != "CONFIRM" || world.conclusionRows(t, id) != 1 {
		t.Fatalf("再交 %s 行 %d", again.Outcome(), world.conclusionRows(t, id))
	}
	held := world.conclude(t, ownerConclusion(t, id, domain.ConclusionObject, "SYN-KEY-OTHER", ""))
	if held.Outcome() != application.ConcludeDiscrepancyHeld || verdictOf(t, held) != "CONFIRM" || world.conclusionRows(t, id) != 1 {
		t.Fatalf("改口 %s 现行 %s 行 %d", held.Outcome(), verdictOf(t, held), world.conclusionRows(t, id))
	}
	conflict := world.conclude(t, ownerConclusion(t, other, domain.ConclusionConfirm, "SYN-KEY-A", ""))
	if conflict.Outcome() != application.ConcludeDiscrepancySourceConflict || world.conclusionRows(t, other) != 0 {
		t.Fatalf("来源冲突 %s 另一份行 %d", conflict.Outcome(), world.conclusionRows(t, other))
	}
	missing := world.conclude(t, ownerConclusion(t, valueOf(t, domain.NewReceiptDiscrepancyID, "RDP-MISSING"), domain.ConclusionConfirm, "SYN-KEY-MISSING", ""))
	if missing.Outcome() != application.ConcludeDiscrepancyNotFound {
		t.Fatalf("查无 %s", missing.Outcome())
	}
	foreign := ownerConclusion(t, id, domain.ConclusionObject, "SYN-KEY-FOREIGN", "")
	foreign.Owner = valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02")
	if world.conclude(t, foreign).Outcome() != application.ConcludeDiscrepancyNotFound || world.conclusionRows(t, id) != 1 {
		t.Fatal("别的货主不该改得了这份差异")
	}
	if world.movementCount(t) != movements || world.stockLineCount(t) != lines {
		t.Fatalf("库存行变了：流水 %d 行 %d", world.movementCount(t), world.stockLineCount(t))
	}

	read := firstObject(t, objectField(t, receiptReadJSON(t, world, advice.ID().String(), false), "preAdvice"), "receipts")
	conclusion := objectField(t, read, "conclusion")
	if jsonString(t, conclusion, "conclusion") != "CONFIRM" || jsonString(t, conclusion, "submissionPath") != "OWNER" ||
		jsonString(t, conclusion, "note") != "确认这一份" || hasKey(conclusion, "operator") || hasKey(conclusion, "evidenceReference") ||
		hasKey(read, "supersededConclusions") {
		t.Fatalf("货主读口 %s", read)
	}
}

// Covers: AT-WF-084——代录确认后货主异议取代它，被取代的还在；代录不取代货主已有的结论；库存行数不变。
func TestDiscrepancyConclusionOnBehalfYieldsToTheOwnerAndDoesNotReplaceAnOwnerConclusion(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice, id := world.overageDiscrepancy(t, "REQ-BEHALF", "SYN-FACT-BEHALF")
	_, owned := world.overageDiscrepancy(t, "REQ-OWNED", "SYN-FACT-OWNED")
	_, sameID := world.overageDiscrepancy(t, "REQ-SAME", "SYN-FACT-SAME")
	movements := world.movementCount(t)
	lines := world.stockLineCount(t)

	recorded := world.conclude(t, onBehalfConclusion(t, id, domain.ConclusionConfirm, "SYN-EVIDENCE-BEHALF"))
	if recorded.Outcome() != application.ConcludeDiscrepancyRecorded || verdictOf(t, recorded) != "CONFIRM" {
		t.Fatalf("代录 %s", recorded.Outcome())
	}
	same := world.conclude(t, onBehalfConclusion(t, id, domain.ConclusionConfirm, "SYN-EVIDENCE-AGAIN"))
	if same.Outcome() != application.ConcludeDiscrepancyExistingResult || world.conclusionRows(t, id) != 1 {
		t.Fatalf("再代录同一结论 %s 行 %d", same.Outcome(), world.conclusionRows(t, id))
	}
	different := world.conclude(t, onBehalfConclusion(t, id, domain.ConclusionObject, "SYN-EVIDENCE-OBJECT"))
	if different.Outcome() != application.ConcludeDiscrepancyHeld || verdictOf(t, different) != "CONFIRM" {
		t.Fatalf("代录改口 %s 现行 %s", different.Outcome(), verdictOf(t, different))
	}
	replaced := world.conclude(t, ownerConclusion(t, id, domain.ConclusionObject, "SYN-KEY-REPLACE", ""))
	if replaced.Outcome() != application.ConcludeDiscrepancyRecorded || verdictOf(t, replaced) != "OBJECT" ||
		world.conclusionRows(t, id) != 2 || world.supersededRows(t, id) != 1 {
		t.Fatalf("取代 %s 行 %d 被取代 %d", replaced.Outcome(), world.conclusionRows(t, id), world.supersededRows(t, id))
	}
	if world.conclude(t, onBehalfConclusion(t, sameID, domain.ConclusionConfirm, "SYN-EVIDENCE-SAME")).Outcome() != application.ConcludeDiscrepancyRecorded {
		t.Fatal("另一份代录没有记下")
	}
	sameAgain := world.conclude(t, ownerConclusion(t, sameID, domain.ConclusionConfirm, "SYN-KEY-SAME", ""))
	if sameAgain.Outcome() != application.ConcludeDiscrepancyRecorded || verdictOf(t, sameAgain) != "CONFIRM" || world.supersededRows(t, sameID) != 1 {
		t.Fatalf("同样的确认也要取代 %s %s", sameAgain.Outcome(), verdictOf(t, sameAgain))
	}

	ownerFirst := world.conclude(t, ownerConclusion(t, owned, domain.ConclusionConfirm, "SYN-KEY-OWNED", ""))
	if ownerFirst.Outcome() != application.ConcludeDiscrepancyRecorded {
		t.Fatal(ownerFirst.Outcome())
	}
	behalfDifferent := world.conclude(t, onBehalfConclusion(t, owned, domain.ConclusionObject, "SYN-EVIDENCE-OWNED"))
	behalfSame := world.conclude(t, onBehalfConclusion(t, owned, domain.ConclusionConfirm, "SYN-EVIDENCE-OWNED-SAME"))
	if behalfDifferent.Outcome() != application.ConcludeDiscrepancyHeld || behalfSame.Outcome() != application.ConcludeDiscrepancyExistingResult ||
		verdictOf(t, behalfSame) != "CONFIRM" || world.conclusionRows(t, owned) != 1 {
		t.Fatalf("代录不取代货主 %s %s 行 %d", behalfDifferent.Outcome(), behalfSame.Outcome(), world.conclusionRows(t, owned))
	}
	if world.movementCount(t) != movements || world.stockLineCount(t) != lines {
		t.Fatalf("库存行变了：流水 %d 行 %d", world.movementCount(t), world.stockLineCount(t))
	}

	ownerRead := firstObject(t, objectField(t, receiptReadJSON(t, world, advice.ID().String(), false), "preAdvice"), "receipts")
	operatorRead := firstObject(t, objectField(t, receiptReadJSON(t, world, advice.ID().String(), true), "preAdvice"), "receipts")
	ownerCurrent := objectField(t, ownerRead, "conclusion")
	if jsonString(t, ownerCurrent, "conclusion") != "OBJECT" || jsonString(t, ownerCurrent, "submissionPath") != "OWNER" || hasKey(ownerCurrent, "operator") {
		t.Fatalf("现行不是货主本人的：%s", ownerCurrent)
	}
	ownerPrior := firstConclusion(t, ownerRead, "supersededConclusions")
	operatorPrior := firstConclusion(t, operatorRead, "supersededConclusions")
	if jsonString(t, ownerPrior, "submissionPath") != "ON_BEHALF" || jsonString(t, ownerPrior, "evidenceReference") != "SYN-EVIDENCE-BEHALF" ||
		hasKey(ownerPrior, "operator") {
		t.Fatalf("货主读口的被取代结论 %s", ownerPrior)
	}
	if jsonString(t, operatorPrior, "operator") == "" || jsonString(t, operatorPrior, "evidenceReference") != "SYN-EVIDENCE-BEHALF" {
		t.Fatalf("运营查阅没有操作者或依据：%s", operatorPrior)
	}
}

// Covers: 收货形成的差异与完结形成的少收各是一个标识，两份各自作结论。
func TestDiscrepancyConclusionReceiptAndShortageEachTakeTheirOwn(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	_, receiptID := world.overageDiscrepancy(t, "REQ-RECEIPT", "SYN-FACT-RECEIPT")
	_, shortageID := world.shortageDiscrepancy(t, "REQ-SHORTAGE", "SYN-FACT-SHORTAGE")
	if receiptID == shortageID {
		t.Fatal("两份差异用了同一个标识")
	}
	movements := world.movementCount(t)
	if world.conclude(t, ownerConclusion(t, receiptID, domain.ConclusionConfirm, "SYN-KEY-RECEIPT", "")).Outcome() != application.ConcludeDiscrepancyRecorded {
		t.Fatal("收货差异没有记下")
	}
	if world.conclude(t, ownerConclusion(t, shortageID, domain.ConclusionObject, "SYN-KEY-SHORTAGE", "")).Outcome() != application.ConcludeDiscrepancyRecorded {
		t.Fatal("少收没有记下")
	}
	if verdictOf(t, world.conclude(t, ownerConclusion(t, receiptID, domain.ConclusionConfirm, "SYN-KEY-RECEIPT", ""))) != "CONFIRM" ||
		verdictOf(t, world.conclude(t, ownerConclusion(t, shortageID, domain.ConclusionObject, "SYN-KEY-SHORTAGE", ""))) != "OBJECT" ||
		world.movementCount(t) != movements {
		t.Fatal("两份结论串了，或库存变了")
	}
}

// Covers: AT-WF-052（结论）——一个仓都没有：未配置，不查差异在不在。
func TestDiscrepancyConclusionWithoutAWarehouseIsNotConfigured(t *testing.T) {
	world := newReceiptWorld(t)
	result := world.conclude(t, ownerConclusion(t, valueOf(t, domain.NewReceiptDiscrepancyID, "RDP-ANY"), domain.ConclusionConfirm, "SYN-KEY-NONE", ""))
	if result.Outcome() != application.ConcludeDiscrepancyNotConfigured || world.conclusionRows(t, valueOf(t, domain.NewReceiptDiscrepancyID, "RDP-ANY")) != 0 {
		t.Fatalf("没登记仓 %s", result.Outcome())
	}
}

// Covers: 代录与货主同时到达同一份差异时串行。后一笔等在差异标识的咨询锁上，先到的代录被后到的货主取代。
func TestDiscrepancyConclusionOnBehalfAndOwnerWaitOnTheDiscrepancyLock(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	_, id := world.overageDiscrepancy(t, "REQ-RACE", "SYN-FACT-RACE")
	movements := world.movementCount(t)
	observer := newBlockedSessionObserver(t, world.pool)
	handler := world.concluder()
	ctx := t.Context()
	inserted := make(chan struct{})
	release := make(chan struct{})
	var first, second application.ConcludeDiscrepancyResult
	var firstErr, secondErr error
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		firstErr = world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			first, err = handler.Conclude(txCtx, onBehalfConclusion(t, id, domain.ConclusionConfirm, "SYN-EVIDENCE-RACE"))
			close(inserted)
			if err == nil {
				<-release
			}
			return err
		})
	}()
	<-inserted
	wait.Add(1)
	go func() {
		defer wait.Done()
		secondErr = world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			second, err = handler.Conclude(txCtx, ownerConclusion(t, id, domain.ConclusionObject, "SYN-KEY-RACE", ""))
			return err
		})
	}()
	if !observer.waitForBlockedSession(ctx, 10*time.Second) {
		close(release)
		wait.Wait()
		t.Fatal("后一笔没有等在锁上")
	}
	close(release)
	wait.Wait()
	if firstErr != nil || secondErr != nil {
		t.Fatalf("并发 %v %v", firstErr, secondErr)
	}
	if first.Outcome() != application.ConcludeDiscrepancyRecorded || second.Outcome() != application.ConcludeDiscrepancyRecorded ||
		verdictOf(t, second) != "OBJECT" || world.supersededRows(t, id) != 1 || world.movementCount(t) != movements {
		t.Fatalf("先代录 %s 后货主 %s 被取代 %d", first.Outcome(), second.Outcome(), world.supersededRows(t, id))
	}
}

func TestDiscrepancyConclusionsRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	ctx := context.Background()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	id := valueOf(t, domain.NewReceiptDiscrepancyID, "RDP-OUTSIDE")
	key := valueOf(t, domain.NewRequestKey, "receipt-discrepancy:SYN-KEY-OUTSIDE")
	if err := world.conclusions.Hold(ctx, tenant, id); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Hold：%v", err)
	}
	if err := world.conclusions.HoldRequestKey(ctx, tenant, key); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldRequestKey：%v", err)
	}
	if err := world.conclusions.Record(ctx, domain.DiscrepancyConclusion{}, false); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Record：%v", err)
	}
}

// Covers: 记下结论答 201，其余业务结果答 200。代录没带依据、自带货主都是坏载荷，依据不出现在问题正文里。
func TestDiscrepancyConclusionStatusesAndBadPayloads(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	_, id := world.overageDiscrepancy(t, "REQ-HTTP", "SYN-FACT-HTTP")
	customer := world.customerConclusionEndpoint()
	operator := world.operatorConclusionEndpoint()

	created := postConclusion(customer, "/v1/warehouse/receipt-discrepancy-conclusions",
		`{"accessRequestId":" SYN-HTTP-1 ","discrepancyId":"`+id.String()+`","conclusion":"CONFIRM"}`)
	if created.Code != http.StatusCreated || outcomeOf(t, created) != "CONCLUSION_RECORDED" {
		t.Fatalf("记下 %d %s", created.Code, created.Body.String())
	}
	replayed := postConclusion(customer, "/v1/warehouse/receipt-discrepancy-conclusions",
		`{"accessRequestId":"SYN-HTTP-2","discrepancyId":"`+id.String()+`","conclusion":"CONFIRM"}`)
	if replayed.Code != http.StatusOK || outcomeOf(t, replayed) != "EXISTING_RESULT" {
		t.Fatalf("再交 %d %s", replayed.Code, replayed.Body.String())
	}
	held := postConclusion(customer, "/v1/warehouse/receipt-discrepancy-conclusions",
		`{"accessRequestId":"SYN-HTTP-3","discrepancyId":"`+id.String()+`","conclusion":"OBJECT"}`)
	if held.Code != http.StatusOK || outcomeOf(t, held) != "CONCLUSION_HELD" {
		t.Fatalf("改口 %d %s", held.Code, held.Body.String())
	}

	missingEvidence := postConclusion(operator, "/warehouse-fulfillment-receipt-discrepancy-conclusion-on-behalf",
		`{"discrepancyId":"`+id.String()+`","conclusion":"CONFIRM"}`)
	assertOperationProblem(t, missingEvidence, http.StatusBadRequest, "MALFORMED_REQUEST")
	carriedOwner := postConclusion(operator, "/warehouse-fulfillment-receipt-discrepancy-conclusion-on-behalf",
		`{"discrepancyId":"`+id.String()+`","conclusion":"CONFIRM","evidenceReference":"SYN-SECRET-BASIS","ownerAccount":"SYN-OWNER-01"}`)
	assertOperationProblem(t, carriedOwner, http.StatusBadRequest, "MALFORMED_REQUEST")
	if strings.Contains(carriedOwner.Body.String(), "SYN-SECRET-BASIS") {
		t.Fatalf("依据进了问题正文：%s", carriedOwner.Body.String())
	}
	customerEvidence := postConclusion(customer, "/v1/warehouse/receipt-discrepancy-conclusions",
		`{"accessRequestId":"SYN-HTTP-4","discrepancyId":"`+id.String()+`","conclusion":"OBJECT","evidenceReference":"SYN-SECRET-BASIS"}`)
	if customerEvidence.Code != http.StatusBadRequest || customerEvidence.Header().Get("Content-Type") != "application/problem+json" ||
		strings.Contains(customerEvidence.Body.String(), "SYN-SECRET-BASIS") {
		t.Fatalf("货主口坏载荷 %d %s", customerEvidence.Code, customerEvidence.Body.String())
	}
}

func (world receiptWorld) concluder() *application.ConcludeDiscrepancyHandler {
	return application.NewConcludeDiscrepancyHandler(world.conclusions, world.warehouses, world.clock)
}

func (world receiptWorld) conclude(t *testing.T, command application.ConcludeDiscrepancyCommand) application.ConcludeDiscrepancyResult {
	t.Helper()
	var result application.ConcludeDiscrepancyResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var concludeErr error
		result, concludeErr = world.concluder().Conclude(ctx, command)
		return concludeErr
	})
	if err != nil {
		t.Fatalf("结论：%v", err)
	}
	return result
}

func (world receiptWorld) overageDiscrepancy(t *testing.T, key, fact string) (domain.PreAdvice, domain.ReceiptDiscrepancyID) {
	t.Helper()
	advice := world.adviseGoods(t, key, "SYN-GOODS-01", "件", 10)
	result := world.confirm(t, receiptCommand(t, fact, advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 20})))
	if result.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatalf("收货 %s", result.Outcome())
	}
	id, ok := requireReceipt(t, result).Discrepancy()
	if !ok {
		t.Fatal("收货没有形成差异")
	}
	return advice, id
}

func (world receiptWorld) shortageDiscrepancy(t *testing.T, key, fact string) (domain.PreAdvice, domain.ReceiptDiscrepancyID) {
	t.Helper()
	advice := world.adviseGoods(t, key, "SYN-GOODS-01", "件", 10)
	confirmed := world.confirm(t, receiptCommand(t, fact, advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 4})))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatalf("收货 %s", confirmed.Outcome())
	}
	if _, has := requireReceipt(t, confirmed).Discrepancy(); has {
		t.Fatal("没收多的收货不该形成差异")
	}
	closed := world.closeAdvice(t, advice.ID(), "")
	closure, ok := closed.Closure()
	if closed.Outcome() != application.ClosePreAdviceClosed || !ok {
		t.Fatalf("完结 %s", closed.Outcome())
	}
	id, has := closure.Discrepancy()
	if !has {
		t.Fatal("少收没有差异标识")
	}
	return advice, id
}

func ownerConclusion(t *testing.T, id domain.ReceiptDiscrepancyID, verdict domain.ConclusionVerdict, key, note string) application.ConcludeDiscrepancyCommand {
	t.Helper()
	command := application.ConcludeDiscrepancyCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Discrepancy: id, Verdict: verdict,
		Path: domain.SubmissionByOwner, Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		RequestKey: valueOf(t, domain.NewRequestKey, "receipt-discrepancy:"+key),
	}
	if note != "" {
		command.Note = valueOf(t, domain.NewConclusionNote, note)
	}
	return command
}

func onBehalfConclusion(t *testing.T, id domain.ReceiptDiscrepancyID, verdict domain.ConclusionVerdict, evidence string) application.ConcludeDiscrepancyCommand {
	t.Helper()
	return application.ConcludeDiscrepancyCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Discrepancy: id, Verdict: verdict,
		Path: domain.SubmissionOnBehalf, Operator: valueOf(t, domain.NewRegistrant, "https://id.syn.example/dex#operator-behalf"),
		Evidence: valueOf(t, domain.NewEvidenceReference, evidence),
	}
}

func (world receiptWorld) conclusionRows(t *testing.T, id domain.ReceiptDiscrepancyID) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.receipt_discrepancy_conclusion
		  WHERE tenant_id = $1 AND discrepancy_id = $2`, "SYN-TENANT-01", id.String()).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) supersededRows(t *testing.T, id domain.ReceiptDiscrepancyID) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.receipt_discrepancy_conclusion
		  WHERE tenant_id = $1 AND discrepancy_id = $2 AND NOT current`, "SYN-TENANT-01", id.String()).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) stockLineCount(t *testing.T) int {
	t.Helper()
	return world.countRows(t, `SELECT count(*) FROM warehouse_fulfillment.stock_movement_line WHERE tenant_id = $1`)
}

func verdictOf(t *testing.T, result application.ConcludeDiscrepancyResult) string {
	t.Helper()
	current, ok := result.Current()
	if !ok {
		t.Fatal("没有回显现行结论")
	}
	return current.Verdict().String()
}

func jsonString(t *testing.T, object map[string]json.RawMessage, key string) string {
	t.Helper()
	var value string
	if err := json.Unmarshal(object[key], &value); err != nil {
		t.Fatalf("%s：%v %s", key, err, object[key])
	}
	return value
}

func jsonBool(t *testing.T, object map[string]json.RawMessage, key string) bool {
	t.Helper()
	var value bool
	if err := json.Unmarshal(object[key], &value); err != nil {
		t.Fatalf("%s：%v %s", key, err, object[key])
	}
	return value
}

func firstConclusion(t *testing.T, object map[string]json.RawMessage, key string) map[string]json.RawMessage {
	t.Helper()
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(object[key], &rows); err != nil || len(rows) == 0 {
		t.Fatalf("%s：%v %s", key, err, object[key])
	}
	return rows[0]
}

type conclusionCustomerAuth struct{}

func (conclusionCustomerAuth) AuthenticateReceiptDiscrepancyConclusion(
	_ context.Context,
	_ warehousehttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (warehousehttp.CustomerCommandIdentity, error) {
	raw, err := accessRequestID()
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, err
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, err
	}
	owner, err := domain.NewOwnerAccountID("SYN-OWNER-01")
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, err
	}
	source, err := domain.NewSubmissionSource("https://auth.syn.example/customers#SYN-CUSTOMER-CLIENT-01")
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, err
	}
	key, err := domain.NewRequestKey("receipt-discrepancy:" + strings.TrimSpace(raw))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, err
	}
	registrant, err := domain.NewRegistrant(source.String())
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, err
	}
	return warehousehttp.CustomerCommandIdentity{
		Tenant: tenant, Owner: owner, Source: source, RequestKey: key, Registrant: registrant,
	}, nil
}

type conclusionOperatorAuth struct{}

func (conclusionOperatorAuth) AuthenticateDiscrepancyConclusion(context.Context, string) (warehousehttp.OperatorIdentity, error) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		return warehousehttp.OperatorIdentity{}, err
	}
	operator, err := domain.NewRegistrant("https://id.syn.example/dex#operator-behalf")
	if err != nil {
		return warehousehttp.OperatorIdentity{}, err
	}
	return warehousehttp.OperatorIdentity{Tenant: tenant, Registrant: operator}, nil
}

type transactionalConclusion struct {
	world receiptWorld
}

func (wrapper transactionalConclusion) Conclude(
	ctx context.Context,
	command application.ConcludeDiscrepancyCommand,
) (application.ConcludeDiscrepancyResult, error) {
	var result application.ConcludeDiscrepancyResult
	err := wrapper.world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var concludeErr error
		result, concludeErr = wrapper.world.concluder().Conclude(txCtx, command)
		return concludeErr
	})
	return result, err
}

func (world receiptWorld) customerConclusionEndpoint() http.Handler {
	intake, err := warehousehttp.NewCustomerDiscrepancyConclusionIntake(conclusionCustomerAuth{})
	if err != nil {
		panic(err)
	}
	return warehousehttp.NewConcludeDiscrepancyEndpoint(intake, transactionalConclusion{world: world})
}

func (world receiptWorld) operatorConclusionEndpoint() http.Handler {
	intake, err := warehousehttp.NewOperatorDiscrepancyConclusionIntake(conclusionOperatorAuth{})
	if err != nil {
		panic(err)
	}
	return warehousehttp.NewConcludeDiscrepancyOnBehalfEndpoint(intake, transactionalConclusion{world: world})
}

func postConclusion(handler http.Handler, target, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func outcomeOf(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("答复 %s：%v", response.Body.String(), err)
	}
	return body.Outcome
}
