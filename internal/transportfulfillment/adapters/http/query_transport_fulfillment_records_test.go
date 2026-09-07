package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var catalogueBaseAt = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)

// grantedCatalogueIntake 装出「认证已就位」的接入面：作用域与页大小都来自它，不读
// 请求内容。真通道未登记（PAR-INT-01），生产装配点不会有这样的实现——它只在测试里
// 存在，为的是隔离验证端点的转写。
type grantedCatalogueIntake struct {
	tenant string
	limit  int
}

func (intake grantedCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (tfhttp.CatalogueQuery, error) {
	reference, err := domain.NewOperationsScopeReference("OPS-SCOPE-1")
	if err != nil {
		return tfhttp.CatalogueQuery{}, err
	}
	tenant, err := domain.NewTenantID(intake.tenant)
	if err != nil {
		return tfhttp.CatalogueQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		return tfhttp.CatalogueQuery{}, err
	}
	return tfhttp.CatalogueQuery{Scope: scope, Limit: intake.limit}, nil
}

func grantedIntake() grantedCatalogueIntake {
	return grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}
}

type failingCatalogueIntake struct{ err error }

func (intake failingCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (tfhttp.CatalogueQuery, error) {
	return tfhttp.CatalogueQuery{}, intake.err
}

// unreachableReviewCatalogue 断言读口未被触到：传输形状的拒绝与未配置格都发生在
// 读库之前。一个类型实现四个读口——同一道分界，替身分设只会让它看起来能只守一半。
type unreachableReviewCatalogue struct{ t *testing.T }

func (stub unreachableReviewCatalogue) refuse(register string) {
	stub.t.Helper()
	stub.t.Fatalf("a refused request reached the %s register", register)
}

func (stub unreachableReviewCatalogue) ListTransportSchedules(
	context.Context, domain.TenantID, int,
) ([]ports.TransportScheduleCatalogueRow, error) {
	stub.refuse("transport schedule")
	return nil, nil
}

func (stub unreachableReviewCatalogue) ListCapacityPools(
	context.Context, domain.TenantID, int,
) ([]ports.CapacityPoolCatalogueRow, error) {
	stub.refuse("capacity pool")
	return nil, nil
}

func (stub unreachableReviewCatalogue) ListTransportHandovers(
	context.Context, domain.TenantID, int,
) ([]ports.TransportHandoverCatalogueRow, error) {
	stub.refuse("transport handover")
	return nil, nil
}

func (stub unreachableReviewCatalogue) ListEffectiveDeliveries(
	context.Context, domain.TenantID, int,
) ([]ports.EffectiveDeliveryCatalogueRow, error) {
	stub.refuse("effective delivery")
	return nil, nil
}

func (stub unreachableReviewCatalogue) ListCarrierMasterDocuments(
	context.Context, domain.TenantID, int,
) ([]ports.CarrierMasterDocumentCatalogueRow, error) {
	stub.refuse("carrier master document")
	return nil, nil
}

// stubReviewCatalogue 是五本册子的读口替身，记下读口收到的键。
type stubReviewCatalogue struct {
	schedules       []ports.TransportScheduleCatalogueRow
	pools           []ports.CapacityPoolCatalogueRow
	handovers       []ports.TransportHandoverCatalogueRow
	deliveries      []ports.EffectiveDeliveryCatalogueRow
	masterDocuments []ports.CarrierMasterDocumentCatalogueRow
	err             error

	gotTenant string
	gotLimit  int
}

func (stub *stubReviewCatalogue) ListTransportSchedules(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.TransportScheduleCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.schedules, stub.err
}

func (stub *stubReviewCatalogue) ListCapacityPools(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.CapacityPoolCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.pools, stub.err
}

func (stub *stubReviewCatalogue) ListTransportHandovers(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.TransportHandoverCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.handovers, stub.err
}

func (stub *stubReviewCatalogue) ListEffectiveDeliveries(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.EffectiveDeliveryCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.deliveries, stub.err
}

func (stub *stubReviewCatalogue) ListCarrierMasterDocuments(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.CarrierMasterDocumentCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.masterDocuments, stub.err
}

func recordsEndpoint(register *stubReviewCatalogue) http.Handler {
	return tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(grantedIntake(), register)
}

// serveGet 打一份 GET 进端点，交回记录器。problemCode 与 assertNoOutcome 两个助手
// 已在 unconfigured_intake_test.go（同包），此处复用不另立。
func serveGet(handler http.Handler, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

// arrayAt 取出答复里某个数组键的原文，用来分辨空数组与 null。
func arrayAt(t *testing.T, response *httptest.ResponseRecorder, key string) string {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	raw, present := fields[key]
	if !present {
		t.Fatalf("答复缺 %q 键：%s", key, response.Body.String())
	}
	return string(raw)
}

var fulfillmentRegistries = []string{
	"transport-schedule", "capacity-pool", "transport-handover", "effective-delivery", "carrier-master-document",
}

const fulfillmentTarget = "/transport-fulfillment-records?registry=transport-schedule"

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，读口不被触到。
func TestTransportFulfillmentRecordsQueryRefusesNonGetMethods(t *testing.T) {
	endpoint := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(
		grantedIntake(), unreachableReviewCatalogue{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, fulfillmentTarget, nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 一律 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED，四本册子一致，读口不被触到，不带业务结果格。
func TestUnconfiguredIntakeRefusesEveryTransportFulfillmentRecordsRequest(t *testing.T) {
	endpoint := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(
		tfhttp.UnconfiguredIntake{}, unreachableReviewCatalogue{t: t},
	)
	for _, registry := range fulfillmentRegistries {
		t.Run(registry, func(t *testing.T) {
			response := serveGet(endpoint, "/transport-fulfillment-records?registry="+registry)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
			if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", got)
			}
			assertNoOutcome(t, response)
		})
	}
}

// Covers: 分派参数的封闭集 — 未知册名、空册名都是坏请求（400），读口不被触到。运输
// 舱单在存储上没有登记册，它的名字在封闭集之外：没有表就没有读法；总单立册之前用过
// 的泛名 `transport-document` 也不是册名——册名指一本表，不指一个页面分区。
func TestTransportFulfillmentRecordsQueryRejectsUnknownRegistries(t *testing.T) {
	endpoint := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(
		grantedIntake(), unreachableReviewCatalogue{t: t},
	)
	for _, registry := range []string{"", "?registry=", "?registry=unknown", "?registry=transport-schedules", "?registry=transport-document", "?registry=transport-manifest"} {
		t.Run(registry, func(t *testing.T) {
			response := serveGet(endpoint, "/transport-fulfillment-records"+registry)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d（%s）", response.Code, http.StatusBadRequest, response.Body.String())
			}
			if got := problemCode(t, response); got != "MALFORMED_REQUEST" {
				t.Fatalf("code = %q, want MALFORMED_REQUEST", got)
			}
			assertNoOutcome(t, response)
		})
	}
}

// Covers: 门的次序 — 请求形状立不起来先于渠道未配置作答。
func TestAnUnknownRegistryIsRefusedBeforeTheChannelCheck(t *testing.T) {
	endpoint := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(
		tfhttp.UnconfiguredIntake{}, unreachableReviewCatalogue{t: t},
	)
	response := serveGet(endpoint, "/transport-fulfillment-records?registry=unknown")

	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0022 — Intake 的三格各自映射：坏请求 400、未配置 403（上面单独用例）、
// 其余是 500 INTAKE_FAILED。
func TestTransportFulfillmentRecordsIntakeFailuresKeepTheirGrades(t *testing.T) {
	malformed := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(
		failingCatalogueIntake{err: fmt.Errorf("%w: no scope", tfhttp.ErrMalformedRequest)},
		unreachableReviewCatalogue{t: t},
	)
	response := serveGet(malformed, fulfillmentTarget)
	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("malformed: %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)

	failing := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(
		failingCatalogueIntake{err: errors.New("identity capability is down")},
		unreachableReviewCatalogue{t: t},
	)
	response = serveGet(failing, fulfillmentTarget)
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "INTAKE_FAILED" {
		t.Fatalf("failing: %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

// Covers: ADR-0022 — 读库失败没有形成答案：500 + NO_ANSWER_FORMED，不带业务结果格。
func TestTransportFulfillmentRecordsQueryAnswers500WhenTheRegisterFails(t *testing.T) {
	for _, registry := range fulfillmentRegistries {
		t.Run(registry, func(t *testing.T) {
			register := &stubReviewCatalogue{err: errors.New("database is down")}
			response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry="+registry)

			if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			assertNoOutcome(t, response)
		})
	}
}

// Covers: 逐格转写 — 班次册：身份、方向、出发时刻照转写；没有执行准备与实际执行键
// （无处可登不代填）；读口收到的是接入面裁决的键。
func TestTransportScheduleRegistryTranscribesRows(t *testing.T) {
	register := &stubReviewCatalogue{
		schedules: []ports.TransportScheduleCatalogueRow{
			{
				ScheduleID: "sched-1",
				Direction:  "CN-US",
				DepartsAt:  catalogueBaseAt,
				RecordedAt: catalogueBaseAt.Add(time.Minute),
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=transport-schedule&tenant=TENANT-9")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome   string                       `json:"outcome"`
		Schedules []map[string]json.RawMessage `json:"schedules"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "TRANSPORT_SCHEDULES_LISTED" || len(body.Schedules) != 1 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	schedule := body.Schedules[0]
	if string(schedule["scheduleId"]) != `"sched-1"` ||
		string(schedule["direction"]) != `"CN-US"` ||
		string(schedule["departsAt"]) != `"`+catalogueBaseAt.Format(time.RFC3339Nano)+`"` {
		t.Fatalf("班次行转写走样：%s", response.Body.String())
	}
	for _, key := range []string{"preparation", "actualExecution"} {
		if _, present := schedule[key]; present {
			t.Fatalf("班次行不该带 %q 键（无处可登不代填）：%s", key, response.Body.String())
		}
	}
}

// Covers: 逐格转写 — 容量池册四量是十进制计数串（2^53 之上 JSON number 会被 JS 读者
// 悄悄取整），逐维独立不互相抵扣。
func TestCapacityPoolRegistryTranscribesQuantitiesAsStrings(t *testing.T) {
	register := &stubReviewCatalogue{
		pools: []ports.CapacityPoolCatalogueRow{
			{
				PoolID:     "pool-1",
				Schedule:   "sched-1",
				Unit:       "kg",
				Capacity:   9007199254740993, // 2^53+1：直投 number 必失真
				Reserved:   15,
				Released:   2,
				Consumed:   3,
				RecordedAt: catalogueBaseAt,
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=capacity-pool")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string                       `json:"outcome"`
		Pools   []map[string]json.RawMessage `json:"pools"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "CAPACITY_POOLS_LISTED" || len(body.Pools) != 1 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	pool := body.Pools[0]
	if string(pool["poolId"]) != `"pool-1"` ||
		string(pool["schedule"]) != `"sched-1"` ||
		string(pool["unit"]) != `"kg"` ||
		string(pool["capacity"]) != `"9007199254740993"` ||
		string(pool["reserved"]) != `"15"` ||
		string(pool["released"]) != `"2"` ||
		string(pool["consumed"]) != `"3"` {
		t.Fatalf("容量池行转写走样：%s", response.Body.String())
	}
}

// Covers: 逐格转写 — 权威交接册：三值裁决词原样透出；已交接行不带依据键；更正版本
// 带回指两键；两方参与方引用各占一键，不折成一个「边界」串。
func TestTransportHandoverRegistryTranscribesVersionChain(t *testing.T) {
	correctedAt := catalogueBaseAt.Add(2 * time.Hour)
	register := &stubReviewCatalogue{
		handovers: []ports.TransportHandoverCatalogueRow{
			{
				Object: "parcel-1", Scope: "scope-1", Version: "handover-v2",
				ReleasedBy: "node-1", ReceivedBy: "carrier-1",
				Verdict: "REFUSED", Basis: "seal-broken",
				CorrectsVersion: "handover-v1", CorrectedAt: &correctedAt,
				JudgedAt:   catalogueBaseAt.Add(time.Hour),
				RecordedAt: catalogueBaseAt.Add(time.Hour),
			},
			{
				Object: "parcel-1", Scope: "scope-1", Version: "handover-v1",
				ReleasedBy: "node-1", ReceivedBy: "carrier-1",
				Verdict:    "HANDED_OVER",
				JudgedAt:   catalogueBaseAt,
				RecordedAt: catalogueBaseAt,
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=transport-handover")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome   string                       `json:"outcome"`
		Handovers []map[string]json.RawMessage `json:"handovers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "TRANSPORT_HANDOVERS_LISTED" || len(body.Handovers) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	refused := body.Handovers[0]
	if string(refused["object"]) != `"parcel-1"` ||
		string(refused["scope"]) != `"scope-1"` ||
		string(refused["version"]) != `"handover-v2"` ||
		string(refused["releasedBy"]) != `"node-1"` ||
		string(refused["receivedBy"]) != `"carrier-1"` ||
		string(refused["verdict"]) != `"REFUSED"` ||
		string(refused["basis"]) != `"seal-broken"` ||
		string(refused["correctsVersion"]) != `"handover-v1"` ||
		string(refused["correctedAt"]) != `"`+correctedAt.Format(time.RFC3339Nano)+`"` {
		t.Fatalf("更正版转写走样：%s", response.Body.String())
	}
	handed := body.Handovers[1]
	if string(handed["verdict"]) != `"HANDED_OVER"` {
		t.Fatalf("首登版转写走样：%s", response.Body.String())
	}
	for _, key := range []string{"basis", "correctsVersion", "correctedAt"} {
		if _, present := handed[key]; present {
			t.Fatalf("已交接首登行不该带 %q 键：%s", key, response.Body.String())
		}
	}
}

// Covers: 逐格转写 — 有效交付册：证明是引用键不是证据内容；首登行不带更正两键；
// 更正版本带回指。
func TestEffectiveDeliveryRegistryTranscribesRows(t *testing.T) {
	correctedAt := catalogueBaseAt.Add(3 * time.Hour)
	register := &stubReviewCatalogue{
		deliveries: []ports.EffectiveDeliveryCatalogueRow{
			{
				Object: "parcel-1", Attempt: "attempt-1", Version: "delivery-v2",
				Place: "door-1", Method: "signature", Recipient: "recipient-1",
				Proof:           "pod-2",
				CorrectsVersion: "delivery-v1", CorrectedAt: &correctedAt,
				OccurredAt: catalogueBaseAt,
				RecordedAt: catalogueBaseAt.Add(time.Minute),
			},
			{
				Object: "parcel-2", Attempt: "attempt-2", Version: "delivery-v3",
				Place: "locker-1", Method: "locker", Recipient: "recipient-2",
				Proof:      "pod-3",
				OccurredAt: catalogueBaseAt.Add(-time.Hour),
				RecordedAt: catalogueBaseAt,
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=effective-delivery")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome    string                       `json:"outcome"`
		Deliveries []map[string]json.RawMessage `json:"deliveries"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "EFFECTIVE_DELIVERIES_LISTED" || len(body.Deliveries) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	corrected := body.Deliveries[0]
	if string(corrected["object"]) != `"parcel-1"` ||
		string(corrected["attempt"]) != `"attempt-1"` ||
		string(corrected["version"]) != `"delivery-v2"` ||
		string(corrected["place"]) != `"door-1"` ||
		string(corrected["method"]) != `"signature"` ||
		string(corrected["recipient"]) != `"recipient-1"` ||
		string(corrected["proof"]) != `"pod-2"` ||
		string(corrected["correctsVersion"]) != `"delivery-v1"` ||
		string(corrected["correctedAt"]) != `"`+correctedAt.Format(time.RFC3339Nano)+`"` ||
		string(corrected["occurredAt"]) != `"`+catalogueBaseAt.Format(time.RFC3339Nano)+`"` {
		t.Fatalf("更正版转写走样：%s", response.Body.String())
	}
	first := body.Deliveries[1]
	for _, key := range []string{"correctsVersion", "correctedAt"} {
		if _, present := first[key]; present {
			t.Fatalf("首登行不该带 %q 键：%s", key, response.Body.String())
		}
	}
}

// Covers: 逐格转写 — 总单册（ADR-0113 决定六）：一行一版本；首版不带回指两键与替代者；
// 关联重述版仍 IN_FORCE 且回指前版；已替代版带 replacedBy；可缺引用缺席即不带键；关联数
// 是十进制计数串。
func TestCarrierMasterDocumentRegistryTranscribesVersionChain(t *testing.T) {
	changedAt := catalogueBaseAt.Add(2 * time.Hour)
	register := &stubReviewCatalogue{
		masterDocuments: []ports.CarrierMasterDocumentCatalogueRow{
			{
				Document: "SYN-MAWB-1", Version: "MDV-3", Issuer: "party/carrier-x", Scope: "SYN-LANE-1",
				Standing: "SUPERSEDED", Supersedes: "MDV-2", ReplacedBy: "SYN-MAWB-1B",
				AssociationCount: 2, ChangedAt: &changedAt,
				RecordedAt: catalogueBaseAt.Add(3 * time.Hour),
			},
			{
				Document: "SYN-MAWB-1", Version: "MDV-2", Issuer: "party/carrier-x", Scope: "SYN-LANE-1",
				Commission: "COMM-1", Booking: "BOOK-1",
				Standing: "IN_FORCE", Supersedes: "MDV-1",
				AssociationCount: 2, ChangedAt: &changedAt,
				RecordedAt: catalogueBaseAt.Add(2 * time.Hour),
			},
			{
				Document: "SYN-MAWB-1", Version: "MDV-1", Issuer: "party/carrier-x", Scope: "SYN-LANE-1",
				Standing: "IN_FORCE", AssociationCount: 0,
				RecordedAt: catalogueBaseAt,
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=carrier-master-document")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome         string                       `json:"outcome"`
		MasterDocuments []map[string]json.RawMessage `json:"masterDocuments"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "CARRIER_MASTER_DOCUMENTS_LISTED" || len(body.MasterDocuments) != 3 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	superseded := body.MasterDocuments[0]
	if string(superseded["document"]) != `"SYN-MAWB-1"` ||
		string(superseded["version"]) != `"MDV-3"` ||
		string(superseded["issuer"]) != `"party/carrier-x"` ||
		string(superseded["scope"]) != `"SYN-LANE-1"` ||
		string(superseded["standing"]) != `"SUPERSEDED"` ||
		string(superseded["supersedes"]) != `"MDV-2"` ||
		string(superseded["replacedBy"]) != `"SYN-MAWB-1B"` ||
		string(superseded["associationCount"]) != `"2"` ||
		string(superseded["changedAt"]) != `"`+changedAt.Format(time.RFC3339Nano)+`"` {
		t.Fatalf("已替代版转写走样：%s", response.Body.String())
	}
	for _, key := range []string{"commission", "booking"} {
		if _, present := superseded[key]; present {
			t.Fatalf("没登的可缺引用不该带 %q 键：%s", key, response.Body.String())
		}
	}
	restated := body.MasterDocuments[1]
	if string(restated["standing"]) != `"IN_FORCE"` ||
		string(restated["supersedes"]) != `"MDV-1"` ||
		string(restated["commission"]) != `"COMM-1"` ||
		string(restated["booking"]) != `"BOOK-1"` {
		t.Fatalf("关联重述版转写走样：%s", response.Body.String())
	}
	if _, present := restated["replacedBy"]; present {
		t.Fatalf("仍有效的版本不该带 replacedBy 键：%s", response.Body.String())
	}
	first := body.MasterDocuments[2]
	if string(first["associationCount"]) != `"0"` {
		t.Fatalf("零关联应转写成 \"0\"：%s", response.Body.String())
	}
	for _, key := range []string{"supersedes", "changedAt", "replacedBy"} {
		if _, present := first[key]; present {
			t.Fatalf("首版不该带 %q 键：%s", key, response.Body.String())
		}
	}
}

// Covers: ADR-0077 Decision 四 — 空登记册是内容不是错误：200 + 空数组（不是 null），
// 五本册子一致。写入方是各命令端点的真渠道，未配置时册子就是空的，读面不代答续办。
// 总单一格自 ADR-0113 立册起属这一族：空册说的是「登记册为空」，不再是「无处可登」。
func TestEmptyFulfillmentRegistersAnswerEmptyArrays(t *testing.T) {
	for registry, key := range map[string]string{
		"transport-schedule":      "schedules",
		"capacity-pool":           "pools",
		"transport-handover":      "handovers",
		"effective-delivery":      "deliveries",
		"carrier-master-document": "masterDocuments",
	} {
		t.Run(registry, func(t *testing.T) {
			register := &stubReviewCatalogue{}
			response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry="+registry)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if raw := arrayAt(t, response, key); raw != "[]" {
				t.Fatalf("%s = %s, want []", key, raw)
			}
		})
	}
}
