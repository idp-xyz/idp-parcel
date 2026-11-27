package warehousehttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// waveQueriesStub 只认得一个波次；记下列表收到的状态筛选。
type waveQueriesStub struct {
	row      ports.WaveRow
	statuses []string
	asked    []string
}

func (stub *waveQueriesStub) ListWaves(_ context.Context, _ domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.WaveRow], error) {
	stub.statuses = query.Filters["status"]
	return cataloguepage.Finish(query, []ports.WaveRow{stub.row}, limit, 1, func(item ports.WaveRow, _ string) cataloguepage.Position {
		return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.ReleasedAt), Identity: []string{item.WaveID}}
	})
}

func (stub *waveQueriesStub) ListPickTasks(_ context.Context, _ domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.PickTaskListRow], error) {
	stub.statuses = query.Filters["status"]
	row := ports.PickTaskListRow{
		TaskID: "SYN-TASK-02", WaveID: "SYN-WAVE-01", RequestID: "SYN-REQ-01", LocationCode: "SYN-LOC-01",
		GoodsCode: "SYN-GOODS-01", Unit: "件", StockState: "GOOD", Quantity: 2, Status: ports.PickTaskOpen,
		WarehouseCode: "SYN-WH-01", ReleasedAt: time.Date(2026, 11, 20, 6, 0, 0, 0, time.UTC),
	}
	return cataloguepage.Finish(query, []ports.PickTaskListRow{row}, limit, 1, func(item ports.PickTaskListRow, _ string) cataloguepage.Position {
		return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.ReleasedAt), Identity: []string{item.TaskID}}
	})
}

func (stub *waveQueriesStub) ListPickDifferences(_ context.Context, _ domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.PickDifferenceRow], error) {
	stub.statuses = query.Filters["status"]
	recorded := time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC)
	row := ports.PickDifferenceRow{
		DifferenceID: "SYN-DIFF-01", Status: "PENDING", Quantity: 2, TaskID: "SYN-TASK-01", WaveID: "SYN-WAVE-01", RequestID: "SYN-REQ-01",
		OwnerAccount: "SYN-OWNER-01", WarehouseCode: "SYN-WH-01", LocationCode: "SYN-LOC-01", GoodsCode: "SYN-GOODS-01",
		Unit: "件", StockState: "GOOD", TaskQuantity: 5, Picked: 3, RecordedAt: &recorded,
	}
	return cataloguepage.Finish(query, []ports.PickDifferenceRow{row}, limit, 1, func(item ports.PickDifferenceRow, _ string) cataloguepage.Position {
		return cataloguepage.Position{Value: item.DifferenceID, Identity: []string{item.DifferenceID}}
	})
}

func (stub *waveQueriesStub) LoadWaveRow(_ context.Context, _ domain.TenantID, waveID string) (ports.WaveRow, bool, error) {
	stub.asked = append(stub.asked, waveID)
	if waveID != stub.row.WaveID {
		return ports.WaveRow{}, false, nil
	}
	return stub.row, true, nil
}

type waveBody struct {
	WaveID   string   `json:"waveId"`
	Status   string   `json:"status"`
	Requests []string `json:"requests"`
	Tasks    []struct {
		TaskID string `json:"taskId"`
		Status string `json:"status"`
	} `json:"tasks"`
}

type waveReadAnswer struct {
	Outcome string     `json:"outcome"`
	Wave    *waveBody  `json:"wave"`
	Waves   []waveBody `json:"waves"`
	Error   *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func waveRead(t *testing.T, handler http.Handler, path string) (*httptest.ResponseRecorder, waveReadAnswer) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer presented.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var answer waveReadAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("%s 答复解不开：%v，正文 %s", path, err, response.Body.String())
	}
	return response, answer
}

func pickingWave() ports.WaveRow {
	return ports.WaveRow{
		WaveID: "SYN-WAVE-01", WarehouseCode: "SYN-WH-01", Operator: "operator-1", Status: ports.WavePicking,
		ReleasedAt: time.Date(2026, 11, 20, 6, 0, 0, 0, time.UTC),
		Requests:   []string{"SYN-REQ-01"},
		Tasks: []ports.WaveTaskRow{
			{TaskID: "SYN-TASK-01", RequestID: "SYN-REQ-01", Quantity: 3, Picked: 3, Status: "DONE"},
			{TaskID: "SYN-TASK-02", RequestID: "SYN-REQ-01", Quantity: 2, Status: "OPEN"},
		},
	}
}

// 按波次号读一张：答这个波次连同状态、出库请求与拣货任务；没有这个波次是一种答案，不带 wave；不给波次号不受理。
func TestWaveReadAnswersOneWaveByItsID(t *testing.T) {
	queries := &waveQueriesStub{row: pickingWave()}
	endpoint := warehousehttp.NewQueryWaveEndpoint(stockReadIntakeStub{}, queries)

	response, found := waveRead(t, endpoint, "/warehouse-fulfillment-wave?waveId=SYN-WAVE-01")
	if response.Code != http.StatusOK || found.Outcome != "WAVE_FOUND" || found.Wave == nil ||
		found.Wave.Status != ports.WavePicking || len(found.Wave.Requests) != 1 || len(found.Wave.Tasks) != 2 {
		t.Fatalf("读到的波次 = %d %s", response.Code, response.Body.String())
	}
	// 没有撤出的也答空列表，不省掉这一格：调用方不必分「没撤过」与「旧答复没有这一格」。
	if !strings.Contains(response.Body.String(), `"withdrawnRequests":[]`) {
		t.Fatalf("没带撤出的那一格：%s", response.Body.String())
	}

	response, missing := waveRead(t, endpoint, "/warehouse-fulfillment-wave?waveId=SYN-WAVE-99")
	if response.Code != http.StatusOK || missing.Outcome != "WAVE_NOT_FOUND" || missing.Wave != nil {
		t.Fatalf("没有的波次 = %d %s", response.Code, response.Body.String())
	}

	response, refused := waveRead(t, endpoint, "/warehouse-fulfillment-wave?waveId=%20")
	if response.Code != http.StatusBadRequest || refused.Error == nil || len(queries.asked) != 2 {
		t.Fatalf("不给波次号 = %d %s，问过 %v", response.Code, response.Body.String(), queries.asked)
	}
}

// 波次列表的每一行带状态，状态是声明过的筛选维；词表之外的状态不受理。
func TestWaveListCarriesStatusAndFiltersByIt(t *testing.T) {
	queries := &waveQueriesStub{row: pickingWave()}
	endpoint := warehousehttp.NewQueryWavesEndpoint(stockReadIntakeStub{}, queries)

	response, listed := waveRead(t, endpoint, "/warehouse-fulfillment-wave-list?status=RELEASED&status=PICKING")
	if response.Code != http.StatusOK || len(listed.Waves) != 1 || listed.Waves[0].Status != ports.WavePicking {
		t.Fatalf("波次列表 = %d %s", response.Code, response.Body.String())
	}
	if len(queries.statuses) != 2 || queries.statuses[0] != ports.WavePicking || queries.statuses[1] != ports.WaveReleased {
		t.Fatalf("交给读面的状态筛选 = %v", queries.statuses)
	}

	response, _ = waveRead(t, endpoint, "/warehouse-fulfillment-wave-list?status=CLOSED")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("词表之外的状态 = %d %s", response.Code, response.Body.String())
	}
}

// 拣货任务列表：拣货工位按仓与待执行取队列；状态是声明过的筛选维，词表之外不受理。
func TestPickTaskListFiltersByStatus(t *testing.T) {
	queries := &waveQueriesStub{row: pickingWave()}
	endpoint := warehousehttp.NewQueryPickTasksEndpoint(stockReadIntakeStub{}, queries)
	response, _ := waveRead(t, endpoint, "/warehouse-fulfillment-pick-tasks?warehouseCode=SYN-WH-01&status=OPEN")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"outcome":"PICK_TASKS_LISTED"`) ||
		!strings.Contains(response.Body.String(), `"taskId":"SYN-TASK-02"`) {
		t.Fatalf("拣货任务列表 = %d %s", response.Code, response.Body.String())
	}
	if len(queries.statuses) != 1 || queries.statuses[0] != ports.PickTaskOpen {
		t.Fatalf("交给读面的状态筛选 = %v", queries.statuses)
	}
	refused := httptest.NewRecorder()
	endpoint.ServeHTTP(refused, httptest.NewRequest(http.MethodGet, "/warehouse-fulfillment-pick-tasks?status=CLOSED", nil))
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("词表之外的状态 = %d %s", refused.Code, refused.Body.String())
	}
}

// 拣货差异列表：每行带它在哪个库位少了几件、了结了没有；没了结的不带了结时刻；状态是声明过的筛选维。
func TestPickDifferenceListCarriesWhereItFellShort(t *testing.T) {
	queries := &waveQueriesStub{row: pickingWave()}
	endpoint := warehousehttp.NewQueryPickDifferencesEndpoint(stockReadIntakeStub{}, queries)
	request := httptest.NewRequest(http.MethodGet, "/warehouse-fulfillment-pick-differences?status=PENDING", nil)
	request.Header.Set("Authorization", "Bearer presented.token")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var answer struct {
		Outcome     string `json:"outcome"`
		Differences []struct {
			DifferenceID string  `json:"differenceId"`
			Status       string  `json:"status"`
			Quantity     int64   `json:"quantity"`
			LocationCode string  `json:"locationCode"`
			TaskQuantity int64   `json:"taskQuantity"`
			Picked       int64   `json:"picked"`
			RecordedAt   string  `json:"recordedAt"`
			ClosedAt     *string `json:"closedAt"`
		} `json:"differences"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复解不开：%v，正文 %s", err, response.Body.String())
	}
	if response.Code != http.StatusOK || answer.Outcome != "PICK_DIFFERENCES_LISTED" || len(answer.Differences) != 1 {
		t.Fatalf("拣货差异列表 = %d %s", response.Code, response.Body.String())
	}
	got := answer.Differences[0]
	if got.DifferenceID != "SYN-DIFF-01" || got.Status != "PENDING" || got.Quantity != 2 || got.LocationCode != "SYN-LOC-01" ||
		got.TaskQuantity != 5 || got.Picked != 3 || got.RecordedAt != "2026-11-20T07:00:00Z" || got.ClosedAt != nil {
		t.Fatalf("这一行 = %s", response.Body.String())
	}
	if len(queries.statuses) != 1 || queries.statuses[0] != "PENDING" {
		t.Fatalf("交给读面的状态筛选 = %v", queries.statuses)
	}
	refused := httptest.NewRecorder()
	endpoint.ServeHTTP(refused, httptest.NewRequest(http.MethodGet, "/warehouse-fulfillment-pick-differences?status=OPEN", nil))
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("词表之外的状态 = %d %s", refused.Code, refused.Body.String())
	}
}
