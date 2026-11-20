package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// Covers: AT-WF-049（完结）——收货中、还有没到的：少收是折成基本单位后的差额，已经多收的商品不再记少收，库存不变。
func TestClosePreAdviceWhileShortRecordsTheDifferenceAndLeavesStock(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-02", 1, "REQ-GOODS-2", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = nil
	}))
	advice := world.adviseLines(t, "REQ-SHORT",
		goodsInboundLine(t, "SYN-GOODS-01", "箱", 10),
		goodsInboundLine(t, "SYN-GOODS-02", "件", 10),
	)
	confirmed := world.confirm(t, receiptCommand(t, "SYN-FACT-SHORT", advice.ID().String(),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 100}),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-02", quantity: 15}),
	))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.adviceStatus(t, advice.ID()) != "RECEIVING" {
		t.Fatalf("收货 %s 状态 %s", confirmed.Outcome(), world.adviceStatus(t, advice.ID()))
	}
	movements := world.movementCount(t)
	closed := world.closeAdvice(t, advice.ID(), "留下没到的")
	closure, ok := closed.Closure()
	if closed.Outcome() != application.ClosePreAdviceClosed || !ok || world.adviceStatus(t, advice.ID()) != "CLOSED" ||
		world.stockOf(t, "SYN-GOODS-01") != 100 || world.stockOf(t, "SYN-GOODS-02") != 15 || world.movementCount(t) != movements ||
		world.closureCount(t) != 1 || world.shortageCount(t) != 1 || world.receiptShortageCount(t) != 0 {
		t.Fatalf("完结 %s 库存 %d/%d 流水 %d 完结 %d 少收 %d 收货差异里的少收 %d",
			closed.Outcome(), world.stockOf(t, "SYN-GOODS-01"), world.stockOf(t, "SYN-GOODS-02"), world.movementCount(t),
			world.closureCount(t), world.shortageCount(t), world.receiptShortageCount(t))
	}
	lines := closure.Shortages()
	if len(lines) != 1 || lines[0].Goods().String() != "SYN-GOODS-01" || lines[0].Quantity() != 140 || lines[0].Kind() != "SHORTAGE" {
		t.Fatalf("少收 = %+v，want SYN-GOODS-01 140 SHORTAGE", lines)
	}
	if closure.Note().String() != "留下没到的" || closure.Operator().String() != "operator-close" {
		t.Fatalf("完结说明 %q 操作者 %s", closure.Note().String(), closure.Operator().String())
	}

	ownerAdvice := objectField(t, receiptReadJSON(t, world, advice.ID().String(), false), "preAdvice")
	operatorAdvice := objectField(t, receiptReadJSON(t, world, advice.ID().String(), true), "preAdvice")
	ownerClosure := objectField(t, ownerAdvice, "closure")
	operatorClosure := objectField(t, operatorAdvice, "closure")
	if hasKey(ownerClosure, "operator") {
		t.Fatalf("货主口带了完结操作者：%s", ownerAdvice["closure"])
	}
	if string(operatorClosure["operator"]) == "" || string(operatorClosure["operator"]) == `""` {
		t.Fatalf("运营查阅没有完结操作者：%s", operatorAdvice["closure"])
	}
	ownerLines := shortageBodies(t, ownerClosure)
	operatorLines := shortageBodies(t, operatorClosure)
	if len(ownerLines) != 1 || ownerLines[0].GoodsCode != "SYN-GOODS-01" || ownerLines[0].Quantity != 140 || ownerLines[0].Kind != "SHORTAGE" ||
		len(operatorLines) != 1 || operatorLines[0].Quantity != 140 {
		t.Fatalf("读口少收 货主 %s 运营 %s", ownerAdvice["closure"], operatorAdvice["closure"])
	}
}

// Covers: AT-WF-049（完结）——已经到齐，或已经超出：完结，没有少收行。
func TestClosePreAdviceWhenReceivedInFullOrBeyondRecordsNoShortage(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	full := world.adviseGoods(t, "REQ-FULL", "SYN-GOODS-01", "件", 100)
	over := world.adviseGoods(t, "REQ-OVER", "SYN-GOODS-01", "件", 100)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-FULL", full.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 100}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("到齐的那笔没收成")
	}
	if world.confirm(t, receiptCommand(t, "SYN-FACT-OVER", over.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 120}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("超出的那笔没收成")
	}
	stock := world.stockOf(t, "SYN-GOODS-01")
	movements := world.movementCount(t)
	for _, advice := range []domain.PreAdvice{full, over} {
		closed := world.closeAdvice(t, advice.ID(), "")
		closure, ok := closed.Closure()
		if closed.Outcome() != application.ClosePreAdviceClosed || !ok || len(closure.Shortages()) != 0 || world.adviceStatus(t, advice.ID()) != "CLOSED" {
			t.Fatalf("%s 完结 %s 少收 %d 状态 %s", advice.ID(), closed.Outcome(), len(closure.Shortages()), world.adviceStatus(t, advice.ID()))
		}
		body := objectField(t, objectField(t, receiptReadJSON(t, world, advice.ID().String(), true), "preAdvice"), "closure")
		if hasKey(body, "shortages") {
			t.Fatalf("%s 编了少收行：%s", advice.ID(), body["shortages"])
		}
	}
	if world.stockOf(t, "SYN-GOODS-01") != stock || world.movementCount(t) != movements || world.shortageCount(t) != 0 || world.receiptShortageCount(t) != 0 {
		t.Fatalf("库存 %d 流水 %d 少收 %d 收货差异里的少收 %d", world.stockOf(t, "SYN-GOODS-01"), world.movementCount(t), world.shortageCount(t), world.receiptShortageCount(t))
	}
}

// Covers: AT-WF-049（完结）——已预报、还没收过货：状态不符，预报仍是已预报。
func TestClosePreAdviceOfAnAdvisedPreAdviceIsAStatusMismatch(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-ADVISED", "SYN-GOODS-01", "件", 10)
	refused := world.closeAdvice(t, advice.ID(), "不该收成已完结")
	status, ok := refused.Status()
	if refused.Outcome() != application.ClosePreAdviceStatusMismatch || !ok || status != domain.PreAdviceAdvised ||
		world.adviceStatus(t, advice.ID()) != "ADVISED" || world.closureCount(t) != 0 || world.stockOf(t, "SYN-GOODS-01") != 0 {
		t.Fatalf("已预报 %s 回显 %s 状态 %s 完结 %d", refused.Outcome(), status, world.adviceStatus(t, advice.ID()), world.closureCount(t))
	}
	body := objectField(t, receiptReadJSON(t, world, advice.ID().String(), true), "preAdvice")
	if hasKey(body, "closure") {
		t.Fatalf("没完结却带了完结：%s", body["closure"])
	}
}

// Covers: AT-WF-049（完结）——已取消同样是状态不符，不把预报收成已完结。
func TestClosePreAdviceOfACancelledPreAdviceIsAStatusMismatch(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-CANCEL", "SYN-GOODS-01", "件", 10)
	var withdrawal application.AcceptPreAdviceResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var withdrawErr error
		withdrawal, withdrawErr = world.handler().Withdraw(ctx, preAdviceWithdrawCommand(t, advice.ID(), "REQ-W"))
		return withdrawErr
	})
	if err != nil || withdrawal.Outcome() != application.PreAdviceOutcomeCancelled {
		t.Fatalf("撤回 %s %v", withdrawal.Outcome(), err)
	}
	refused := world.closeAdvice(t, advice.ID(), "")
	status, ok := refused.Status()
	if refused.Outcome() != application.ClosePreAdviceStatusMismatch || !ok || status != domain.PreAdviceCancelled ||
		world.adviceStatus(t, advice.ID()) != "CANCELLED" || world.closureCount(t) != 0 {
		t.Fatalf("已取消 %s 回显 %s 状态 %s", refused.Outcome(), status, world.adviceStatus(t, advice.ID()))
	}
}

// Covers: AT-WF-049（完结）——再完结一次答已有结果，回显当初的完结与少收。新建答 201，重放答 200。
// 问题体是运营决定口的旧形状。
func TestClosePreAdviceAgainReturnsTheExistingResult(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-AGAIN", "SYN-GOODS-01", "件", 100)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-AGAIN", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 40}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("没收成")
	}
	handler := world.closeEndpoint()
	carried := []string{
		`{"preAdviceId":"` + advice.ID().String() + `","ownerAccount":"SYN-OWNER-01"}`,
		`{"preAdviceId":"` + advice.ID().String() + `","decisionId":"SYN-DECISION-01"}`,
		`{"preAdviceId":"` + advice.ID().String() + `","tenantId":"SYN-TENANT-02"}`,
	}
	for _, body := range carried {
		response := postClose(handler, body)
		assertOperationProblem(t, response, http.StatusBadRequest, "MALFORMED_REQUEST")
	}
	if world.adviceStatus(t, advice.ID()) != "RECEIVING" || world.closureCount(t) != 0 {
		t.Fatalf("坏载荷改了册：状态 %s 完结 %d", world.adviceStatus(t, advice.ID()), world.closureCount(t))
	}

	created := postClose(handler, `{"preAdviceId":"`+advice.ID().String()+`","note":"第一次"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("新建 %d %s", created.Code, created.Body.String())
	}
	first := decodeClose(t, created)
	if first.Outcome != "CLOSED" || len(first.Closure.Shortages) != 1 || first.Closure.Shortages[0].Quantity != 60 || first.Closure.Note != "第一次" {
		t.Fatalf("新建答复 %s", created.Body.String())
	}
	stock := world.stockOf(t, "SYN-GOODS-01")
	again := postClose(handler, `{"preAdviceId":"`+advice.ID().String()+`","note":"第二次"}`)
	if again.Code != http.StatusOK {
		t.Fatalf("重放 %d %s", again.Code, again.Body.String())
	}
	second := decodeClose(t, again)
	if second.Outcome != "EXISTING_RESULT" || second.Closure.Note != "第一次" || len(second.Closure.Shortages) != 1 ||
		second.Closure.Shortages[0].Quantity != 60 || world.closureCount(t) != 1 || world.shortageCount(t) != 1 ||
		world.stockOf(t, "SYN-GOODS-01") != stock || world.adviceStatus(t, advice.ID()) != "CLOSED" {
		t.Fatalf("重放 %s 完结 %d 少收 %d 库存 %d", again.Body.String(), world.closureCount(t), world.shortageCount(t), world.stockOf(t, "SYN-GOODS-01"))
	}
}

// Covers: AT-WF-049（完结）——完结之后再来收货，答 DIVERTED_TO_UNCLAIMED，不入账，保全一件待认领到货。
func TestClosePreAdviceThenReceiptIsNotReceivable(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-LATE", "SYN-GOODS-01", "件", 100)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-EARLY", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 40}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("没收成")
	}
	if world.closeAdvice(t, advice.ID(), "").Outcome() != application.ClosePreAdviceClosed {
		t.Fatal("没完结")
	}
	stock := world.stockOf(t, "SYN-GOODS-01")
	movements := world.movementCount(t)
	goodsLines := world.goodsMovementRows(t)
	cartonLines := world.cartonLineCount(t)
	late := world.confirm(t, receiptCommand(t, "SYN-FACT-LATE", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 10})))
	if late.Outcome() != application.ConfirmReceiptDivertedToUnclaimed || world.stockOf(t, "SYN-GOODS-01") != stock ||
		world.movementCount(t) != movements || world.goodsMovementRows(t) != goodsLines || world.cartonLineCount(t) != cartonLines ||
		world.unclaimedCount(t) != 1 || world.adviceStatus(t, advice.ID()) != "CLOSED" || world.shortageCount(t) != 1 {
		t.Fatalf("迟到的收货 %s 库存 %d 流水 %d 行 %d %d 到货 %d 状态 %s",
			late.Outcome(), world.stockOf(t, "SYN-GOODS-01"), world.movementCount(t),
			world.goodsMovementRows(t), world.cartonLineCount(t), world.unclaimedCount(t), world.adviceStatus(t, advice.ID()))
	}
}

// Covers: AT-WF-052（完结）——一个仓都没有：未配置，不查这份预报在不在。
func TestClosePreAdviceWithNoWarehouseIsNotConfigured(t *testing.T) {
	world := newReceiptWorld(t)
	refused := world.closeAdvice(t, valueOf(t, domain.NewPreAdviceID, "SYN-IPA-MISSING"), "")
	if refused.Outcome() != application.ClosePreAdviceNotConfigured || world.closureCount(t) != 0 {
		t.Fatalf("没登记仓 %s 完结 %d", refused.Outcome(), world.closureCount(t))
	}
	_, found := refused.PreAdvice()
	if found {
		t.Fatal("未配置却回显了预报")
	}
}

func TestPreAdviceClosuresRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	if _, err := world.closures.Save(context.Background(), domain.PreAdviceClosure{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Save：%v", err)
	}
}

func (world receiptWorld) closer() *application.ClosePreAdviceHandler {
	return application.NewClosePreAdviceHandler(
		world.closures, world.preAdvices, world.handler(), world.receipts, world.goods, world.warehouses, world.discrepancy, world.clock,
	)
}

func (world receiptWorld) closeAdvice(t *testing.T, id domain.PreAdviceID, note string) application.ClosePreAdviceResult {
	t.Helper()
	var written domain.ClosureNote
	if note != "" {
		written = valueOf(t, domain.NewClosureNote, note)
	}
	command := application.ClosePreAdviceCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: id,
		Operator: valueOf(t, domain.NewRegistrant, "operator-close"), Note: written,
	}
	var result application.ClosePreAdviceResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var closeErr error
		result, closeErr = world.closer().ClosePreAdvice(ctx, command)
		return closeErr
	})
	if err != nil {
		t.Fatalf("完结：%v", err)
	}
	return result
}

func (world receiptWorld) adviseLines(t *testing.T, key string, lines ...domain.GoodsInboundLine) domain.PreAdvice {
	t.Helper()
	result := world.accept(t, preAdviceAcceptCommand(t, key, func(command *application.AcceptPreAdviceCommand) {
		command.GoodsLines = lines
	}))
	if result.Outcome() != application.PreAdviceOutcomeAdvised {
		t.Fatalf("受理 %s：%s", key, result.Outcome())
	}
	advice, ok := result.PreAdvice()
	if !ok {
		t.Fatal("受理没有回显预报")
	}
	return advice
}

func goodsInboundLine(t *testing.T, goods, unit string, quantity int64) domain.GoodsInboundLine {
	t.Helper()
	line, err := domain.NewGoodsInboundLine(
		valueOf(t, domain.NewGoodsCode, goods), valueOf(t, domain.NewGoodsUnitName, unit),
		quantity, domain.BatchNumber{}, domain.ExpiryDate{}, domain.CartonCount{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func (world receiptWorld) closureCount(t *testing.T) int {
	t.Helper()
	return world.countRows(t, `SELECT count(*) FROM warehouse_fulfillment.inbound_pre_advice_closure WHERE tenant_id = $1`)
}

func (world receiptWorld) shortageCount(t *testing.T) int {
	t.Helper()
	return world.countRows(t, `SELECT count(*) FROM warehouse_fulfillment.inbound_pre_advice_shortage WHERE tenant_id = $1`)
}

func (world receiptWorld) receiptShortageCount(t *testing.T) int {
	t.Helper()
	return world.countRows(t, `SELECT count(*) FROM warehouse_fulfillment.receipt_discrepancy WHERE tenant_id = $1 AND kind = 'SHORTAGE'`)
}

func (world receiptWorld) countRows(t *testing.T, query string) int {
	t.Helper()
	var count int
	if err := world.pool.QueryRow(t.Context(), query, "SYN-TENANT-01").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

type closeEndpointAuth struct{}

func (closeEndpointAuth) AuthenticatePreAdviceClose(context.Context, string) (warehousehttp.OperatorIdentity, error) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		return warehousehttp.OperatorIdentity{}, err
	}
	operator, err := domain.NewRegistrant("https://id.syn.example/dex#SYN-OPERATOR-CLOSE")
	if err != nil {
		return warehousehttp.OperatorIdentity{}, err
	}
	return warehousehttp.OperatorIdentity{Tenant: tenant, Registrant: operator}, nil
}

type transactionalClose struct {
	world receiptWorld
}

func (closer transactionalClose) ClosePreAdvice(
	ctx context.Context,
	command application.ClosePreAdviceCommand,
) (application.ClosePreAdviceResult, error) {
	var result application.ClosePreAdviceResult
	err := closer.world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var closeErr error
		result, closeErr = closer.world.closer().ClosePreAdvice(txCtx, command)
		return closeErr
	})
	return result, err
}

func (world receiptWorld) closeEndpoint() http.Handler {
	intake, err := warehousehttp.NewOperatorPreAdviceCloseIntake(closeEndpointAuth{})
	if err != nil {
		panic(err)
	}
	return warehousehttp.NewClosePreAdviceEndpoint(intake, transactionalClose{world: world})
}

func postClose(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/warehouse-fulfillment-inbound-pre-advice-closures", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertOperationProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body struct {
		Code    string `json:"code"`
		Outcome string `json:"outcome"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("问题体 %s：%v", response.Body.String(), err)
	}
	if response.Code != status || body.Error.Code != code || body.Code != "" || body.Outcome != "" {
		t.Fatalf("问题体 %d %s，want %d error.code=%s", response.Code, response.Body.String(), status, code)
	}
}

type closeHTTPBody struct {
	Outcome string `json:"outcome"`
	Closure struct {
		Note      string `json:"note"`
		Shortages []struct {
			GoodsCode string `json:"goodsCode"`
			Kind      string `json:"kind"`
			Quantity  int64  `json:"quantity"`
		} `json:"shortages"`
	} `json:"closure"`
}

func decodeClose(t *testing.T, response *httptest.ResponseRecorder) closeHTTPBody {
	t.Helper()
	var body closeHTTPBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("完结答复 %s：%v", response.Body.String(), err)
	}
	return body
}

type shortageJSON struct {
	GoodsCode    string `json:"goodsCode"`
	CartonNumber string `json:"cartonNumber"`
	Kind         string `json:"kind"`
	Quantity     int64  `json:"quantity"`
}

func shortageBodies(t *testing.T, closure map[string]json.RawMessage) []shortageJSON {
	t.Helper()
	var lines []shortageJSON
	if err := json.Unmarshal(closure["shortages"], &lines); err != nil {
		t.Fatalf("少收 %s：%v", closure["shortages"], err)
	}
	return lines
}
