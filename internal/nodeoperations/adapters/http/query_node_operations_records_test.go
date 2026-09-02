package nodeopshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
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
) (nodeopshttp.CatalogueQuery, error) {
	reference, err := domain.NewOperationsScopeReference("OPS-SCOPE-1")
	if err != nil {
		return nodeopshttp.CatalogueQuery{}, err
	}
	tenant, err := domain.NewTenantID(intake.tenant)
	if err != nil {
		return nodeopshttp.CatalogueQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		return nodeopshttp.CatalogueQuery{}, err
	}
	return nodeopshttp.CatalogueQuery{Scope: scope, Limit: intake.limit}, nil
}

func grantedIntake() grantedCatalogueIntake {
	return grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}
}

type failingCatalogueIntake struct{ err error }

func (intake failingCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (nodeopshttp.CatalogueQuery, error) {
	return nodeopshttp.CatalogueQuery{}, intake.err
}

// unreachableReviewCatalogue 断言读口未被触到：传输形状的拒绝与未配置格都发生在
// 读库之前。一个类型实现三个读口——同一道分界，替身分设只会让它看起来能只守一半。
type unreachableReviewCatalogue struct{ t *testing.T }

func (stub unreachableReviewCatalogue) refuse(register string) {
	stub.t.Helper()
	stub.t.Fatalf("a refused request reached the %s register", register)
}

func (stub unreachableReviewCatalogue) ListReceptions(
	context.Context, domain.TenantID, int,
) ([]ports.ReceptionCatalogueRow, error) {
	stub.refuse("reception")
	return nil, nil
}

func (stub unreachableReviewCatalogue) ListUnidentifiedItems(
	context.Context, domain.TenantID, int,
) ([]ports.UnidentifiedItemCatalogueRow, error) {
	stub.refuse("unidentified item")
	return nil, nil
}

func (stub unreachableReviewCatalogue) ListConsolidationUnits(
	context.Context, domain.TenantID, int,
) ([]ports.ConsolidationUnitCatalogueRow, error) {
	stub.refuse("consolidation unit")
	return nil, nil
}

// stubReviewCatalogue 是三本册子的读口替身，记下读口收到的键。
type stubReviewCatalogue struct {
	receptions []ports.ReceptionCatalogueRow
	items      []ports.UnidentifiedItemCatalogueRow
	units      []ports.ConsolidationUnitCatalogueRow
	err        error

	gotTenant string
	gotLimit  int
}

func (stub *stubReviewCatalogue) ListReceptions(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.ReceptionCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.receptions, stub.err
}

func (stub *stubReviewCatalogue) ListUnidentifiedItems(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.UnidentifiedItemCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.items, stub.err
}

func (stub *stubReviewCatalogue) ListConsolidationUnits(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.ConsolidationUnitCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.units, stub.err
}

func recordsEndpoint(register *stubReviewCatalogue) http.Handler {
	return nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(grantedIntake(), register)
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

const recordsTarget = "/node-operations-records?registry=reception"

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，读口不被触到。
func TestNodeOperationsRecordsQueryRefusesNonGetMethods(t *testing.T) {
	endpoint := nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(
		grantedIntake(), unreachableReviewCatalogue{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, recordsTarget, nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 一律 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED，三本册子一致，读口不被触到，不带业务结果格。
func TestUnconfiguredIntakeRefusesEveryNodeOperationsRecordsRequest(t *testing.T) {
	endpoint := nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(
		nodeopshttp.UnconfiguredIntake{}, unreachableReviewCatalogue{t: t},
	)
	for _, registry := range []string{"reception", "unidentified-item", "consolidation-unit"} {
		t.Run(registry, func(t *testing.T) {
			response := serveGet(endpoint, "/node-operations-records?registry="+registry)

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

// Covers: 分派参数的封闭集 — 未知册名、空册名都是坏请求（400），读口不被触到。实际
// 测量与交接证据两区在存储上没有登记册，它们的名字也在封闭集之外：没有表就没有读法，
// 答一份恒空册子会把「无处可登」演成「登记册为空」。
func TestNodeOperationsRecordsQueryRejectsUnknownRegistries(t *testing.T) {
	endpoint := nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(
		grantedIntake(), unreachableReviewCatalogue{t: t},
	)
	for _, registry := range []string{"", "?registry=", "?registry=unknown", "?registry=receptions", "?registry=measurement", "?registry=handover-evidence"} {
		t.Run(registry, func(t *testing.T) {
			response := serveGet(endpoint, "/node-operations-records"+registry)
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

// Covers: 门的次序 — 请求形状立不起来先于渠道未配置作答。册名认不出是请求自身的事，
// 判它不需要先知道调用方是谁。
func TestAnUnknownRegistryIsRefusedBeforeTheChannelCheck(t *testing.T) {
	endpoint := nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(
		nodeopshttp.UnconfiguredIntake{}, unreachableReviewCatalogue{t: t},
	)
	response := serveGet(endpoint, "/node-operations-records?registry=unknown")

	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0022 — Intake 的三格各自映射：坏请求 400、未配置 403（上面单独用例）、
// 其余是 500 INTAKE_FAILED。
func TestNodeOperationsRecordsIntakeFailuresKeepTheirGrades(t *testing.T) {
	malformed := nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(
		failingCatalogueIntake{err: fmt.Errorf("%w: no scope", nodeopshttp.ErrMalformedRequest)},
		unreachableReviewCatalogue{t: t},
	)
	response := serveGet(malformed, recordsTarget)
	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("malformed: %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)

	failing := nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(
		failingCatalogueIntake{err: errors.New("identity capability is down")},
		unreachableReviewCatalogue{t: t},
	)
	response = serveGet(failing, recordsTarget)
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "INTAKE_FAILED" {
		t.Fatalf("failing: %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

// Covers: ADR-0022 — 读库失败没有形成答案：500 + NO_ANSWER_FORMED，不带业务结果格。
func TestNodeOperationsRecordsQueryAnswers500WhenTheRegisterFails(t *testing.T) {
	for _, registry := range []string{"reception", "unidentified-item", "consolidation-unit"} {
		t.Run(registry, func(t *testing.T) {
			register := &stubReviewCatalogue{err: errors.New("database is down")}
			response := serveGet(recordsEndpoint(register), "/node-operations-records?registry="+registry)

			if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			assertNoOutcome(t, response)
		})
	}
}

// Covers: 逐格转写 — 收寄册两格各自的形状：形成格带控制转出两键，待识别格不带；
// 服务标记是空数组不是 null；时刻按 RFC 3339 转写；读口收到的是接入面裁决的键。
func TestReceptionRegistryTranscribesRows(t *testing.T) {
	releasedAt := catalogueBaseAt.Add(6 * time.Hour)
	register := &stubReviewCatalogue{
		receptions: []ports.ReceptionCatalogueRow{
			{
				SourceID:             "scan-1",
				Kind:                 "INTAKE_FORMED",
				Unit:                 "unit-1",
				Node:                 "node-1",
				DeliveredBy:          "courier-1",
				ReceivedAt:           catalogueBaseAt,
				ControlKind:          "NODE_INTAKE",
				ControlEstablishedAt: catalogueBaseAt,
				ControlReleasedBy:    "HANDOVER/TF-9",
				ControlReleasedAt:    &releasedAt,
				ServiceMarkers:       []string{"CANCELLED_BEFORE_ARRIVAL"},
				RecordedAt:           catalogueBaseAt.Add(time.Minute),
			},
			{
				SourceID:             "scan-2",
				Kind:                 "PENDING_IDENTIFICATION",
				Unit:                 "unit-2",
				Node:                 "node-1",
				DeliveredBy:          "courier-1",
				ReceivedAt:           catalogueBaseAt,
				ControlKind:          "NODE_INTAKE",
				ControlEstablishedAt: catalogueBaseAt,
				RecordedAt:           catalogueBaseAt.Add(2 * time.Minute),
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/node-operations-records?registry=reception&tenant=TENANT-9")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome    string                       `json:"outcome"`
		Receptions []map[string]json.RawMessage `json:"receptions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "RECEPTIONS_LISTED" || len(body.Receptions) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	formed := body.Receptions[0]
	if string(formed["sourceId"]) != `"scan-1"` ||
		string(formed["kind"]) != `"INTAKE_FORMED"` ||
		string(formed["unit"]) != `"unit-1"` ||
		string(formed["node"]) != `"node-1"` ||
		string(formed["deliveredBy"]) != `"courier-1"` ||
		string(formed["receivedAt"]) != `"`+catalogueBaseAt.Format(time.RFC3339Nano)+`"` ||
		string(formed["controlKind"]) != `"NODE_INTAKE"` ||
		string(formed["controlReleasedBy"]) != `"HANDOVER/TF-9"` ||
		string(formed["controlReleasedAt"]) != `"`+releasedAt.Format(time.RFC3339Nano)+`"` ||
		string(formed["serviceMarkers"]) != `["CANCELLED_BEFORE_ARRIVAL"]` {
		t.Fatalf("形成格转写走样：%s", response.Body.String())
	}
	pending := body.Receptions[1]
	if _, present := pending["controlReleasedBy"]; present {
		t.Fatalf("未转出的行不该带转出键：%s", response.Body.String())
	}
	if _, present := pending["controlReleasedAt"]; present {
		t.Fatalf("未转出的行不该带转出时刻：%s", response.Body.String())
	}
	if string(pending["kind"]) != `"PENDING_IDENTIFICATION"` ||
		string(pending["serviceMarkers"]) != `[]` {
		t.Fatalf("待识别格转写走样：%s", response.Body.String())
	}
}

// Covers: 逐格转写 — 待识别册的候选与冲突标照登记转写；登记时没有正式关联就整键
// 不出现，不代填。
func TestUnidentifiedItemRegistryTranscribesRows(t *testing.T) {
	register := &stubReviewCatalogue{
		items: []ports.UnidentifiedItemCatalogueRow{
			{
				SourceID:         "scan-2",
				Unit:             "unit-2",
				Node:             "node-1",
				Candidates:       []string{"parcel-7/link-v1", "parcel-8/link-v1"},
				IdentityConflict: true,
				ReceivedAt:       catalogueBaseAt,
				RecordedAt:       catalogueBaseAt.Add(time.Minute),
			},
			{
				SourceID:    "scan-4",
				Unit:        "unit-4",
				Node:        "node-1",
				Candidates:  []string{},
				ReceivedAt:  catalogueBaseAt,
				Association: "parcel-9/link-v1",
				RecordedAt:  catalogueBaseAt.Add(2 * time.Minute),
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/node-operations-records?registry=unidentified-item")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string                       `json:"outcome"`
		Items   []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "UNIDENTIFIED_ITEMS_LISTED" || len(body.Items) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	conflicted := body.Items[0]
	if string(conflicted["unit"]) != `"unit-2"` ||
		string(conflicted["candidates"]) != `["parcel-7/link-v1","parcel-8/link-v1"]` ||
		string(conflicted["identityConflict"]) != `true` {
		t.Fatalf("冲突行转写走样：%s", response.Body.String())
	}
	if _, present := conflicted["association"]; present {
		t.Fatalf("登记时无关联的行不该带关联键：%s", response.Body.String())
	}
	associated := body.Items[1]
	if string(associated["association"]) != `"parcel-9/link-v1"` ||
		string(associated["candidates"]) != `[]` ||
		string(associated["identityConflict"]) != `false` {
		t.Fatalf("带关联行转写走样：%s", response.Body.String())
	}
}

// Covers: 逐格转写 — 集运单元册三相各自的形状：开放行不带封签与关闭键，封装行带最近
// 封签两键，关闭行带关闭时刻；成员数与封装次数是数不是清单。
func TestConsolidationUnitRegistryTranscribesRows(t *testing.T) {
	sealedAt := catalogueBaseAt.Add(time.Hour)
	closedAt := catalogueBaseAt.Add(2 * time.Hour)
	register := &stubReviewCatalogue{
		units: []ports.ConsolidationUnitCatalogueRow{
			{
				UnitID: "bag-1", Asset: "asset-1", Phase: "OPEN",
				OpenedSourceID: "scan-open-1", OpenedBy: "packer-1",
			},
			{
				UnitID: "bag-2", Asset: "asset-2", Phase: "SEALED",
				MemberCount: 2, SealCount: 1,
				OpenedSourceID: "scan-open-2", OpenedBy: "packer-1",
				LatestSeal: "seal-1", LatestSealedAt: &sealedAt,
				LatestSealSourceID: "scan-seal-2", LatestSealPerformedBy: "packer-2",
			},
			{
				UnitID: "bag-3", Asset: "asset-3", Phase: "CLOSED",
				MemberCount: 1, SealCount: 2,
				OpenedSourceID: "scan-open-3", OpenedBy: "packer-1",
				LatestSeal: "seal-9", LatestSealedAt: &sealedAt,
				LatestSealSourceID: "scan-seal-3", LatestSealPerformedBy: "packer-3",
				ClosedAt: &closedAt,
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/node-operations-records?registry=consolidation-unit")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string                       `json:"outcome"`
		Units   []map[string]json.RawMessage `json:"units"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "CONSOLIDATION_UNITS_LISTED" || len(body.Units) != 3 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	open := body.Units[0]
	if string(open["phase"]) != `"OPEN"` || string(open["memberCount"]) != `0` {
		t.Fatalf("开放行转写走样：%s", response.Body.String())
	}
	// 开启来源两件在三相上都在场：单元不可能没有开启那一次作业，库面该列也是 NOT NULL。
	// 它们与封签那两件的分别正在这里——后者随「有没有封装过」成对进出。
	if string(open["openedSourceId"]) != `"scan-open-1"` || string(open["openedBy"]) != `"packer-1"` {
		t.Fatalf("开放行丢了开启来源两件：%s", response.Body.String())
	}
	for _, key := range []string{
		"latestSeal", "latestSealSourceId", "latestSealPerformedBy", "latestSealedAt", "closedAt",
	} {
		if _, present := open[key]; present {
			t.Fatalf("开放行不该带 %q 键：%s", key, response.Body.String())
		}
	}
	sealed := body.Units[1]
	if string(sealed["phase"]) != `"SEALED"` ||
		string(sealed["memberCount"]) != `2` ||
		string(sealed["sealCount"]) != `1` ||
		string(sealed["latestSeal"]) != `"seal-1"` ||
		string(sealed["latestSealedAt"]) != `"`+sealedAt.Format(time.RFC3339Nano)+`"` {
		t.Fatalf("封装行转写走样：%s", response.Body.String())
	}
	// 封签的来源身份与执行方各自成键，且与开启那一组不同值——两组若被同一个值填满，
	// 「这次封装是谁报的」就说不清了，而分辨导入与扫描正靠 latestSealSourceId。
	if string(sealed["latestSealSourceId"]) != `"scan-seal-2"` ||
		string(sealed["latestSealPerformedBy"]) != `"packer-2"` ||
		string(sealed["openedBy"]) != `"packer-1"` {
		t.Fatalf("封装行的来源两件走样：%s", response.Body.String())
	}
	closed := body.Units[2]
	if string(closed["phase"]) != `"CLOSED"` ||
		string(closed["closedAt"]) != `"`+closedAt.Format(time.RFC3339Nano)+`"` ||
		string(closed["sealCount"]) != `2` ||
		string(closed["latestSealSourceId"]) != `"scan-seal-3"` {
		t.Fatalf("关闭行转写走样：%s", response.Body.String())
	}
}

// Covers: ADR-0077 Decision 四 — 空登记册是内容不是错误：200 + 空数组（不是 null），
// 三本册子一致。写入方是收寄命令端点的真渠道，未配置时册子就是空的，读面不代答续办。
func TestEmptyRegistersAnswerEmptyArrays(t *testing.T) {
	for registry, key := range map[string]string{
		"reception":          "receptions",
		"unidentified-item":  "items",
		"consolidation-unit": "units",
	} {
		t.Run(registry, func(t *testing.T) {
			register := &stubReviewCatalogue{}
			response := serveGet(recordsEndpoint(register), "/node-operations-records?registry="+registry)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if raw := arrayAt(t, response, key); raw != "[]" {
				t.Fatalf("%s = %s, want []", key, raw)
			}
		})
	}
}
