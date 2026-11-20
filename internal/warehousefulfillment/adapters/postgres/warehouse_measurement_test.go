package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	whaccess "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type measurementWorld struct {
	receiptWorld
	measurements *adapter.WarehouseMeasurements
}

func newMeasurementWorld(t *testing.T) measurementWorld {
	t.Helper()
	rw := newReceiptWorld(t)
	measurements, err := adapter.NewWarehouseMeasurements(rw.db)
	if err != nil {
		t.Fatal(err)
	}
	return measurementWorld{receiptWorld: rw, measurements: measurements}
}

func (world measurementWorld) recorder(clock time.Time) *application.RecordWarehouseMeasurementHandler {
	return application.NewRecordWarehouseMeasurementHandler(
		world.measurements, world.warehouses, world.goods, world.cartons, fixedClock{at: clock},
	)
}

func (world measurementWorld) record(
	t *testing.T,
	clock time.Time,
	command application.RecordWarehouseMeasurementCommand,
) application.RecordWarehouseMeasurementResult {
	t.Helper()
	var result application.RecordWarehouseMeasurementResult
	err := world.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var recordErr error
		result, recordErr = world.recorder(clock).Record(txCtx, command)
		return recordErr
	})
	if err != nil {
		t.Fatalf("仓测登记：%v", err)
	}
	return result
}

func (world measurementWorld) goodsQueries() *application.QueryGoodsHandler {
	return application.NewQueryGoodsHandler(
		world.catalogue, world.catalogue, world.catalogue, world.measurements,
		fixedClock{at: preAdviceNumberingInstant},
	)
}

func (world measurementWorld) measurementQueries() *application.QueryWarehouseMeasurementsHandler {
	return application.NewQueryWarehouseMeasurementsHandler(world.catalogue, world.measurements)
}

func goodsLevelMeasureCommand(
	t *testing.T,
	factID, device string,
	occurredAt time.Time,
	seq int64,
	warehouse, owner, goods, level string,
	l, w, h, dimUnit, wt, wtUnit string,
) application.RecordWarehouseMeasurementCommand {
	t.Helper()
	dims, err := domain.NewDeclaredDimensions(
		measureValue(t, l), measureValue(t, w), measureValue(t, h), valueOf(t, domain.NewUnitCode, dimUnit),
	)
	if err != nil {
		t.Fatal(err)
	}
	weight, err := domain.NewDeclaredWeight(measureValue(t, wt), valueOf(t, domain.NewUnitCode, wtUnit))
	if err != nil {
		t.Fatal(err)
	}
	target, err := domain.NewGoodsLevelMeasurementTarget(
		valueOf(t, domain.NewOwnerAccountID, owner),
		valueOf(t, domain.NewGoodsCode, goods),
		valueOf(t, domain.NewGoodsUnitName, level),
	)
	if err != nil {
		t.Fatal(err)
	}
	return application.RecordWarehouseMeasurementCommand{
		Tenant:         valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		FactID:         valueOf(t, domain.NewMeasurementFactID, factID),
		Device:         valueOf(t, domain.NewMeasurementDeviceID, device),
		Operator:       valueOf(t, domain.NewMeasurementOperatorID, "https://id.syn.example/dex#SYN-OP-1"),
		OccurredAt:     occurredAt,
		DeviceSequence: seq,
		Warehouse:      valueOf(t, domain.NewWarehouseCode, warehouse),
		Target:         target,
		Dimensions:     dims,
		Weight:         weight,
	}
}

func cartonMeasureCommand(
	t *testing.T,
	factID, device string,
	occurredAt time.Time,
	seq int64,
	warehouse, owner, carton string,
) application.RecordWarehouseMeasurementCommand {
	t.Helper()
	dims, err := domain.NewDeclaredDimensions(
		measureValue(t, "50"), measureValue(t, "40"), measureValue(t, "30"), valueOf(t, domain.NewUnitCode, "CM"),
	)
	if err != nil {
		t.Fatal(err)
	}
	weight, err := domain.NewDeclaredWeight(measureValue(t, "12.5"), valueOf(t, domain.NewUnitCode, "KG"))
	if err != nil {
		t.Fatal(err)
	}
	target, err := domain.NewCartonMeasurementTarget(
		valueOf(t, domain.NewOwnerAccountID, owner),
		valueOf(t, domain.NewCartonNumber, carton),
	)
	if err != nil {
		t.Fatal(err)
	}
	return application.RecordWarehouseMeasurementCommand{
		Tenant:         valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		FactID:         valueOf(t, domain.NewMeasurementFactID, factID),
		Device:         valueOf(t, domain.NewMeasurementDeviceID, device),
		Operator:       valueOf(t, domain.NewMeasurementOperatorID, "https://id.syn.example/dex#SYN-OP-1"),
		OccurredAt:     occurredAt,
		DeviceSequence: seq,
		Warehouse:      valueOf(t, domain.NewWarehouseCode, warehouse),
		Target:         target,
		Dimensions:     dims,
		Weight:         weight,
	}
}

// Covers: AT-WF-021、AT-WF-023（仓测真库）——登记仓测、原样重投答 EXISTING_RESULT、换数值答 CONTENT_CONFLICT 且在册不变；
// 第二台设备再测追加一版；迟到的更早事实照留为一版但不顶掉当前那一版；
// 读未仓测有声明的商品如实答未测量并附货主声明来源、无声明的既未测量也无声明、已仓测的答当前那一版与发生时间；别的货主读不到。
func TestWarehouseMeasurementVersionsOrderingReplayConflictAndReads(t *testing.T) {
	world := newMeasurementWorld(t)
	world.seedReceiving(t)

	// 另存一个没有声明尺寸重量的商品，与 SYN-GOODS-01（有「件」声明重量）对照 AT-WF-023。
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-BARE-01", 1, "REQ-BARE-1", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = nil
		spec.DeclaredMeasurements = nil
	}))

	// 还没仓测前：SYN-GOODS-01 的「件」答未测量且附带 OWNER_DECLARED 参照；SYN-BARE-01 答未测量且无参照。
	declaredBefore, found, err := world.goodsQueries().CurrentGoods(t.Context(),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
	)
	if err != nil || !found || len(declaredBefore.WarehouseMeasurements) != 2 {
		t.Fatalf("未仓测有声明 = %+v (%v, %v)", declaredBefore.WarehouseMeasurements, found, err)
	}
	if declaredBefore.WarehouseMeasurements[0].Measured || declaredBefore.WarehouseMeasurements[0].Current != nil ||
		declaredBefore.WarehouseMeasurements[0].Declared == nil || declaredBefore.WarehouseMeasurements[0].Declared.Source != application.OwnerDeclaredMeasurementSource ||
		declaredBefore.WarehouseMeasurements[0].Declared.Weight == nil || declaredBefore.WarehouseMeasurements[0].Declared.Weight.Value != "0.5" {
		t.Fatalf("有声明未仓测 = %+v", declaredBefore.WarehouseMeasurements[0])
	}
	if declaredBefore.WarehouseMeasurements[1].Measured || declaredBefore.WarehouseMeasurements[1].Declared != nil {
		t.Fatalf("箱层级既没声明也没仓测 = %+v", declaredBefore.WarehouseMeasurements[1])
	}

	bareBefore, err := world.measurementQueries().GoodsLevelMeasurements(t.Context(),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewGoodsCode, "SYN-BARE-01"),
		valueOf(t, domain.NewGoodsUnitName, "件"),
	)
	if err != nil || !bareBefore.Found || bareBefore.Measured || bareBefore.Current != nil || bareBefore.Declared != nil || len(bareBefore.Versions) != 0 {
		t.Fatalf("两样都没有 = %+v (%v)", bareBefore, err)
	}

	t1 := baseInstant.Add(10 * time.Minute)
	t2 := baseInstant.Add(20 * time.Minute)
	t0 := baseInstant.Add(5 * time.Minute)

	firstCmd := goodsLevelMeasureCommand(t, "SYN-MF-1", "SYN-DEV-1", t1, 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件", "30", "20", "10", "CM", "1.2", "KG")
	first := world.record(t, t1.Add(time.Second), firstCmd)
	if first.Outcome() != application.RecordMeasurementRecorded {
		t.Fatalf("首次仓测 = %s", first.Outcome())
	}

	// 原样重投（数值 30.00 归一后同 30）答 EXISTING_RESULT，不追加第二版。
	replayCmd := goodsLevelMeasureCommand(t, "SYN-MF-1", "SYN-DEV-1", t1, 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件", "30.00", "20", "10", "CM", "1.20", "KG")
	replay := world.record(t, t1.Add(time.Minute), replayCmd)
	if replay.Outcome() != application.RecordMeasurementExistingResult {
		t.Fatalf("原样重投 = %s", replay.Outcome())
	}

	// 同事实身份换数值答 CONTENT_CONFLICT，在册那一版不变。
	conflictCmd := goodsLevelMeasureCommand(t, "SYN-MF-1", "SYN-DEV-1", t1, 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件", "31", "20", "10", "CM", "1.2", "KG")
	conflict := world.record(t, t1.Add(2*time.Minute), conflictCmd)
	if conflict.Outcome() != application.RecordMeasurementContentConflict {
		t.Fatalf("换数值 = %s", conflict.Outcome())
	}

	// 另一台设备更晚测一版；再有一条发生时刻更早的迟到事实到达。
	secondCmd := goodsLevelMeasureCommand(t, "SYN-MF-2", "SYN-DEV-2", t2, 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件", "32", "21", "11", "CM", "1.4", "KG")
	if r := world.record(t, t2.Add(time.Second), secondCmd); r.Outcome() != application.RecordMeasurementRecorded {
		t.Fatalf("再测 = %s", r.Outcome())
	}
	lateOlderCmd := goodsLevelMeasureCommand(t, "SYN-MF-LATE", "SYN-DEV-3", t0, 9, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件", "29", "19", "9", "CM", "1.0", "KG")
	if r := world.record(t, t2.Add(5*time.Minute), lateOlderCmd); r.Outcome() != application.RecordMeasurementRecorded {
		t.Fatalf("迟到旧事实 = %s", r.Outcome())
	}

	// 读口：三版都在，按发生次序排（t0, t1, t2），当前那一版是 t2（SYN-MF-2），长度 32。
	reading, err := world.measurementQueries().GoodsLevelMeasurements(t.Context(),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
		valueOf(t, domain.NewGoodsUnitName, "件"),
	)
	if err != nil || !reading.Found || !reading.Measured || reading.Current == nil || reading.Current.FactID != "SYN-MF-2" ||
		reading.Current.Dimensions.Length != "32" || !reading.Current.OccurredAt.Equal(t2) || len(reading.Versions) != 3 ||
		reading.Versions[0].FactID != "SYN-MF-LATE" || reading.Versions[1].FactID != "SYN-MF-1" || reading.Versions[2].FactID != "SYN-MF-2" {
		t.Fatalf("仓测读回 = %+v (%v)", reading, err)
	}

	// 别的货主读不到 SYN-OWNER-01 的商品与仓测。
	otherOwner, err := world.measurementQueries().GoodsLevelMeasurements(t.Context(),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02"),
		valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
		valueOf(t, domain.NewGoodsUnitName, "件"),
	)
	if err != nil || otherOwner.Found || otherOwner.Measured || len(otherOwner.Versions) != 0 {
		t.Fatalf("别的货主读到了 = %+v (%v)", otherOwner, err)
	}
}

// Covers: AT-WF-022、AT-WF-024——没登记仓答 WAREHOUSE_NOT_FOUND、代理仓答 WAREHOUSE_NOT_ACCEPTING/AGENT、停用自营仓照记；
// 没登记商品、没声明包装层级、箱不在库、数值为零、单位不在内置目录或类别不对均答 NOT_ACCEPTED 并指出格，不占事实身份；
// 箱入库之后再测答 RECORDED。
func TestWarehouseMeasurementRejectionsCartonOnHandAndDeactivatedSelfOperated(t *testing.T) {
	world := newMeasurementWorld(t)
	now := baseInstant.Add(15 * time.Minute)

	// 一个仓都没登记时答 WAREHOUSE_NOT_FOUND，不占事实身份（AT-WF-024）。
	noWh := world.record(t, now, goodsLevelMeasureCommand(t, "SYN-MF-REUSE", "SYN-DEV-1", now, 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件", "10", "10", "10", "CM", "1", "KG"))
	if noWh.Outcome() != application.RecordMeasurementWarehouseNotFound {
		t.Fatalf("无仓 = %s", noWh.Outcome())
	}

	world.seedReceiving(t)
	// 登记一个代理仓：答 WAREHOUSE_NOT_ACCEPTING / AGENT。
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-AGENT", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Kind = domain.WarehouseKindAgent
	}))
	agent := world.record(t, now, goodsLevelMeasureCommand(t, "SYN-MF-REUSE", "SYN-DEV-1", now, 1, "SYN-WH-AGENT", "SYN-OWNER-01", "SYN-GOODS-01", "件", "10", "10", "10", "CM", "1", "KG"))
	if reason, ok := agent.NotAcceptingReason(); agent.Outcome() != application.RecordMeasurementWarehouseNotAccepting || !ok || reason != application.WarehouseNotAcceptingAgent {
		t.Fatalf("代理仓 = %s %s (%v)", agent.Outcome(), reason, ok)
	}

	// 没登记商品 + 零长度 + 错误单位类别，一次收齐全部不合的格，不占事实身份。
	badGoods := world.record(t, now, goodsLevelMeasureCommand(t, "SYN-MF-REUSE", "SYN-DEV-1", now, 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-MISSING", "件", "0", "10", "10", "KG", "0", "CM"))
	if badGoods.Outcome() != application.RecordMeasurementNotAccepted {
		t.Fatalf("不合 = %s", badGoods.Outcome())
	}
	wantFields := []domain.MeasurementField{
		domain.MeasurementFieldGoodsCode,
		domain.MeasurementFieldDimensionsLength,
		domain.MeasurementFieldDimensionsUnit,
		domain.MeasurementFieldWeightValue,
		domain.MeasurementFieldWeightUnit,
	}
	if got := badGoods.InvalidFields(); len(got) != len(wantFields) {
		t.Fatalf("不合的格 = %v，want %v", got, wantFields)
	}

	// 包装层级未声明。
	badLevel := world.record(t, now, goodsLevelMeasureCommand(t, "SYN-MF-REUSE", "SYN-DEV-1", now, 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "托", "10", "10", "10", "CM", "1", "KG"))
	if badLevel.Outcome() != application.RecordMeasurementNotAccepted || len(badLevel.InvalidFields()) != 1 || badLevel.InvalidFields()[0] != domain.MeasurementFieldPackagingLevel {
		t.Fatalf("未声明层级 = %s %v", badLevel.Outcome(), badLevel.InvalidFields())
	}

	// 按箱测：还没收货入账，箱不在库，答 NOT_ACCEPTED 并指 cartonNumber，不占事实身份。
	cartonCmd := cartonMeasureCommand(t, "SYN-MF-CARTON", "SYN-DEV-1", now, 2, "SYN-WH-01", "SYN-OWNER-01", "SYN-CTN-MEAS-1")
	offStock := world.record(t, now, cartonCmd)
	if offStock.Outcome() != application.RecordMeasurementNotAccepted || len(offStock.InvalidFields()) != 1 || offStock.InvalidFields()[0] != domain.MeasurementFieldCartonNumber {
		t.Fatalf("箱不在库 = %s %v", offStock.Outcome(), offStock.InvalidFields())
	}

	// 整箱预报并收货确认入整箱账后，用同一个事实身份再投按箱仓测：成功落册。
	advice := world.adviseCartons(t, "REQ-CTN-MEAS", "SYN-CTN-MEAS-1")
	if confirmed := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-CTN-MEAS", advice.ID().String(), mustCartonLine(t, "SYN-CTN-MEAS-1", "", false))); confirmed.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatalf("整箱收货 = %s", confirmed.Outcome())
	}
	if onStock := world.record(t, now.Add(time.Minute), cartonCmd); onStock.Outcome() != application.RecordMeasurementRecorded {
		t.Fatalf("箱在库后再测 = %s", onStock.Outcome())
	}
	cartonRead, err := world.measurementQueries().CartonMeasurements(t.Context(),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		valueOf(t, domain.NewCartonNumber, "SYN-CTN-MEAS-1"),
	)
	if err != nil || !cartonRead.Measured || cartonRead.Current == nil || cartonRead.Current.FactID != "SYN-MF-CARTON" {
		t.Fatalf("按箱读回 = %+v (%v)", cartonRead, err)
	}

	// 停用的自营仓照记，且之前被拒的事实身份 SYN-MF-REUSE 未被占用，仍可成功落册。
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 2, func(spec *domain.WarehouseContentSpec) {
		spec.Deactivated = true
	}))
	if reused := world.record(t, now.Add(2*time.Minute), goodsLevelMeasureCommand(t, "SYN-MF-REUSE", "SYN-DEV-1", now, 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件", "10", "10", "10", "CM", "1", "KG")); reused.Outcome() != application.RecordMeasurementRecorded {
		t.Fatalf("停用自营仓复投 = %s", reused.Outcome())
	}

	// 无环境事务拒绝 HoldFact 与 Save。
	if err := world.measurements.HoldFact(context.Background(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewMeasurementFactID, "SYN-NO-TX")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 HoldFact = %v", err)
	}
	if _, err := world.measurements.Save(context.Background(), domain.WarehouseMeasurement{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 Save = %v", err)
	}
}

// Covers: AT-WF-014、AT-WF-015、AT-WF-020、AT-WF-024——货主经 /v1/warehouse/goods 首次登记、重放、冲突；
// 货主与运营代录写同一条修订序列，货主在读口看得到代录修订的依据引用且看不到登记责任方。
func TestCustomerGoodsSharesRevisionSequenceWithOnBehalfAndHidesRegistrant(t *testing.T) {
	world := newMeasurementWorld(t)

	// 货主与运营用同一套真实库表与编排。
	at := preAdviceNumberingInstant
	subject, err := accessidentity.NewCustomerClientSubject("https://auth.syn.example/customers", "SYN-CUSTOMER-GOODS-01")
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := accessidentity.NewCredentialReference("SYN-CRED/goods")
	binding, _ := accessidentity.NewCustomerClientBinding(subject, "SYN-TENANT-01", "SYN-OWNER-01", ref, false, "SYN-BASIS")
	interval, _ := accessidentity.NewEffectiveInterval(at.Add(-time.Hour), time.Time{})
	var grants []accessidentity.RecordedCustomerClientGrant
	for _, capWord := range []accessidentity.CustomerCapability{accessidentity.CustomerCapabilityGoodsMaintenance, accessidentity.CustomerCapabilityWarehouseRead} {
		g, _ := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-G-"+capWord.String(), subject, capWord, interval, "SYN-BASIS")
		rec, _ := accessidentity.NewRecordedCustomerClientGrant(g, nil)
		grants = append(grants, rec)
	}
	standing, _ := accessidentity.NewCustomerClientStanding(binding, grants)
	minter, err := accessidentity.NewCustomerClientMinter(
		goodsCustomerVerifier{subject: subject},
		goodsCustomerRegistry{standing: standing},
		goodsAdmitAll{},
		func() time.Time { return at },
	)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := whaccess.NewCustomerGoodsAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := warehousehttp.NewCustomerGoodsIntake(auth)
	if err != nil {
		t.Fatal(err)
	}
	registrar := txGoodsRegistrar{
		transactor: world.transactor,
		inner:      application.NewRegisterGoodsHandler(world.goods, world.warehouses, world.ledger, presentOwnerAccounts{}, fixedClock{at: at}),
	}
	writeEndpoint := warehousehttp.NewMaintainCustomerGoodsEndpoint(intake, registrar)
	revisionsEndpoint := warehousehttp.NewListCustomerGoodsRevisionsEndpoint(intake, world.goodsQueries())

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/warehouse/goods", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer presented")
		res := httptest.NewRecorder()
		writeEndpoint.ServeHTTP(res, req)
		return res
	}

	rev1Body := `{"accessRequestId":"SYN-OWNER-REQ-1","goodsCode":"SYN-SHARED-01","name":"货主首登","baseUnit":"件","packagingLevels":[{"name":"件","factor":1}]}`
	// 一个仓都没登记时答 NOT_CONFIGURED（AT-WF-024）。
	unconfigured := post(rev1Body)
	if unconfigured.Code != http.StatusOK || outcomeOf(t, unconfigured) != "NOT_CONFIGURED" {
		t.Fatalf("无仓 = %d %s", unconfigured.Code, unconfigured.Body.String())
	}

	world.seedReceiving(t)
	// 登记仓后原样重交：修订 1 落册，答 201 REGISTERED；回显不带 registrant。
	created := post(rev1Body)
	if created.Code != http.StatusCreated || outcomeOf(t, created) != "REGISTERED" || strings.Contains(created.Body.String(), "registrant") {
		t.Fatalf("货主首登 = %d %s", created.Code, created.Body.String())
	}
	// 原样重投答 EXISTING_RESULT；换内容答 SOURCE_CONFLICT。
	replayed := post(rev1Body)
	if replayed.Code != http.StatusOK || outcomeOf(t, replayed) != "EXISTING_RESULT" {
		t.Fatalf("重投 = %d %s", replayed.Code, replayed.Body.String())
	}
	conflict := post(strings.Replace(rev1Body, "货主首登", "换名字", 1))
	if conflict.Code != http.StatusOK || outcomeOf(t, conflict) != "SOURCE_CONFLICT" {
		t.Fatalf("冲突 = %d %s", conflict.Code, conflict.Body.String())
	}

	// 运营代录基于修订 1 追加修订 2。
	onBehalfSub, err := domain.NewOnBehalfSubmission(valueOf(t, domain.NewRequestKey, "DEC-ON-BEHALF-2"), valueOf(t, domain.NewEvidenceReference, "SYN-OWNER-AUTH-DOC-2"))
	if err != nil {
		t.Fatal(err)
	}
	rev2Content := goodsContentFixture(t, func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "代录修订二")
		spec.Aliases = nil
	})
	rev2Res, err := registrar.Register(t.Context(), application.RegisterGoodsCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Owner:      valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Code:       valueOf(t, domain.NewGoodsCode, "SYN-SHARED-01"),
		Basis:      domain.NewBasisRevision(1),
		Content:    rev2Content,
		Submission: onBehalfSub,
		Registrant: valueOf(t, domain.NewRegistrant, "https://id.syn.example/dex#SYN-OP-HIDDEN"),
	})
	if err != nil || rev2Res.Outcome() != application.GoodsOutcomeRegistered {
		t.Fatalf("代录修订 2 = %s (%v)", rev2Res.Outcome(), err)
	}

	// 货主查修订史：看到修订 1（OWNER）与修订 2（ON_BEHALF，带依据引用），不带登记责任方。
	req := httptest.NewRequest(http.MethodGet, "/v1/warehouse/reads/goods-revisions?goodsCode=SYN-SHARED-01", nil)
	req.Header.Set("Authorization", "Bearer presented")
	res := httptest.NewRecorder()
	revisionsEndpoint.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("修订史 = %d %s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "registrant") || strings.Contains(res.Body.String(), "SYN-OP-HIDDEN") {
		t.Fatalf("货主修订史泄露了操作者：%s", res.Body.String())
	}
	var payload struct {
		Outcome   string `json:"outcome"`
		Revisions []struct {
			Revision          int    `json:"revision"`
			SubmissionPath    string `json:"submissionPath"`
			EvidenceReference string `json:"evidenceReference"`
		} `json:"revisions"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Outcome != "GOODS_REVISIONS_LISTED" || len(payload.Revisions) != 2 ||
		payload.Revisions[0].Revision != 1 || payload.Revisions[0].SubmissionPath != "OWNER" ||
		payload.Revisions[1].Revision != 2 || payload.Revisions[1].SubmissionPath != "ON_BEHALF" ||
		payload.Revisions[1].EvidenceReference != "SYN-OWNER-AUTH-DOC-2" {
		t.Fatalf("修订史 = %+v", payload)
	}
}

type goodsCustomerVerifier struct {
	subject accessidentity.CustomerClientSubject
}

func (v goodsCustomerVerifier) VerifyCustomerClientCredential(context.Context, accessidentity.IntegrationClientCredential) (accessidentity.VerifiedCustomerClientToken, error) {
	return accessidentity.NewVerifiedCustomerClientToken(v.subject, false), nil
}

type goodsCustomerRegistry struct {
	standing accessidentity.CustomerClientStanding
}

func (r goodsCustomerRegistry) FindCustomerClient(context.Context, accessidentity.CustomerClientSubject) (accessidentity.CustomerClientStanding, bool, error) {
	return r.standing, true, nil
}

type goodsAdmitAll struct{}

func (goodsAdmitAll) Admits(context.Context, string, accessidentity.AdmissionRequirement, time.Time) (bool, error) {
	return true, nil
}

type txGoodsRegistrar struct {
	transactor bentoapp.Transactor
	inner      *application.RegisterGoodsHandler
}

func (r txGoodsRegistrar) Register(ctx context.Context, command application.RegisterGoodsCommand) (application.RegisterGoodsResult, error) {
	var result application.RegisterGoodsResult
	err := r.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var regErr error
		result, regErr = r.inner.Register(txCtx, command)
		return regErr
	})
	return result, err
}
