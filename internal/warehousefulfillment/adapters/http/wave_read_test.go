package warehousehttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
