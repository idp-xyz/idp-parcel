package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// previewEndpoint 装一只真问询编排：读数那三口与 record_movement_fact_test.go 的出发用同一组替身（段 segment-1 里有 parcel-1，
// 班次 schedule-1 自 NODE-SHA 到 NODE-SZX），Intake 是作业事实那一面。
func previewEndpoint(t *testing.T, customs *movementCustomsDouble, intake tfhttp.DepartureGatePreviewIntake) http.Handler {
	t.Helper()
	handler, err := application.NewPreviewDepartureGateHandler(movementGateReader(t, customs))
	if err != nil {
		t.Fatalf("装问询编排：%v", err)
	}
	return tfhttp.NewPreviewDepartureGateEndpoint(intake, handler)
}

func servePreview(endpoint http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// rawGateReading 取答复里 `gateReading` 那一段原样字节：两口比的是线上那一份，不是各自解出来的结构。
func rawGateReading(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s：%v", response.Body.String(), err)
	}
	return string(body["gateReading"])
}

// Covers: AT-TF-227 线格式半边——问询答 200 `DEPARTURE_GATE_PREVIEWED`，读数阻断也是 200（挡不挡归设备与现场）；`gateReading`
// 与同参出发答复里的那一段逐字节相同，同样不带关务判断标识与依据来源（ADR-0205 决定一、`AT-TF-063`）。
func TestAPreviewAnswersOKWithTheSameGateReadingBytesAsTheDeparture(t *testing.T) {
	customs := &movementCustomsDouble{answer: domain.DepartureGateBlocked}
	response := servePreview(previewEndpoint(t, customs, workFactIntakeForTest(t)),
		commandRequest(`{"schedule":"schedule-1","segment":"segment-1","departsAt":"2026-09-05T10:00:00Z"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200（读数阻断也只是答复）; body = %s", response.Code, response.Body.String())
	}
	var view struct {
		Outcome     string `json:"outcome"`
		GateReading struct {
			Answer  string           `json:"answer"`
			From    string           `json:"from"`
			To      string           `json:"to"`
			Objects []map[string]any `json:"objects"`
		} `json:"gateReading"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode：%v", err)
	}
	if view.Outcome != "DEPARTURE_GATE_PREVIEWED" || view.GateReading.Answer != "BLOCKED" || view.GateReading.From != "NODE-SHA" ||
		view.GateReading.To != "NODE-SZX" || len(view.GateReading.Objects) != 1 || view.GateReading.Objects[0]["object"] != "parcel-1" ||
		len(view.GateReading.Objects[0]) != 2 {
		t.Fatalf("答复 = %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "AADJ-1") || strings.Contains(response.Body.String(), "SRC-1") {
		t.Fatalf("答复带出了关务判断标识或依据来源：%s", response.Body.String())
	}

	fixture := newMovementFactFixture(t)
	fixture.customs.answer = domain.DepartureGateBlocked
	departure := defaultMovementFactBody()
	departure.Kind, departure.Fact, departure.Segment = "DEPARTURE", "movement-fact-dep-preview", "segment-1"
	departed := postTo(t, fixture.record, "/transport-fulfillment/movement-facts", departure)
	if departed.Code != http.StatusCreated {
		t.Fatalf("同参出发 status = %d; body = %s", departed.Code, departed.Body.String())
	}
	if previewed, recorded := rawGateReading(t, response), rawGateReading(t, departed); previewed != recorded {
		t.Fatalf("两口的 gateReading 不是同一份：\n问询 %s\n出发 %s", previewed, recorded)
	}
}

// Covers: AT-TF-228 线格式半边——拟出发时点或班次空着答 200 `INPUT_NOT_ACCEPTED`、不带读数；时点给了却解不出、载荷带三格以外
// 的键（例如照抄出发载荷带上 `fact`）是坏报文 400；方法不对 405。
func TestAPreviewRefusesWhatItCannotAccept(t *testing.T) {
	endpoint := previewEndpoint(t, &movementCustomsDouble{answer: domain.DepartureGateNotGuarded}, workFactIntakeForTest(t))

	for name, body := range map[string]string{
		"缺时点": `{"schedule":"schedule-1","segment":"segment-1"}`,
		"缺班次": `{"segment":"segment-1","departsAt":"2026-09-05T10:00:00Z"}`,
	} {
		response := servePreview(endpoint, commandRequest(body))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"outcome":"INPUT_NOT_ACCEPTED"`) ||
			strings.Contains(response.Body.String(), "gateReading") {
			t.Fatalf("%s：%d %s, want 200 INPUT_NOT_ACCEPTED 不带读数", name, response.Code, response.Body.String())
		}
	}
	for name, body := range map[string]string{
		"时点解不出":  `{"schedule":"schedule-1","departsAt":"tomorrow"}`,
		"带出发事实键": `{"fact":"f-1","schedule":"schedule-1","departsAt":"2026-09-05T10:00:00Z"}`,
	} {
		response := servePreview(endpoint, commandRequest(body))
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s：%d %s, want 400 MALFORMED_REQUEST", name, response.Code, response.Body.String())
		}
	}
	get := commandRequest("")
	get.Method = http.MethodGet
	if response := servePreview(endpoint, get); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET：%d, want 405", response.Code)
	}
}

// Covers: AT-TF-230 线格式半边——没有作业事实登记授予答 403 `OPERATOR_NOT_GRANTED`，设备不在用答 403 `DEVICE_NOT_IN_USE`：与移动
// 事实口同一道门。渠道未配置答 403 且不读体。
func TestAPreviewIsRefusedWithoutTheMovementFactGrant(t *testing.T) {
	body := `{"schedule":"schedule-1","segment":"segment-1","departsAt":"2026-09-05T10:00:00Z"}`
	for code, refusal := range map[string]error{
		"OPERATOR_NOT_GRANTED": tfhttp.ErrOperatorNotGranted,
		"DEVICE_NOT_IN_USE":    tfhttp.ErrDeviceNotInUse,
	} {
		customs := &movementCustomsDouble{answer: domain.DepartureGateNotGuarded}
		response := servePreview(previewEndpoint(t, customs, workFactIntakeWith(t, workFactAuthenticatorFake{err: refusal})), commandRequest(body))
		if response.Code != http.StatusForbidden || problemCode(t, response) != code {
			t.Fatalf("%s：%d %s", code, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	}

	probe := &readProbe{}
	response := servePreview(tfhttp.NewPreviewDepartureGateEndpoint(tfhttp.UnconfiguredIntake{}, unreachablePreviewer{t: t}),
		httptest.NewRequest(http.MethodPost, "/transport-fulfillment/departure-gate-previews", probe))
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("未配置：%d %s", response.Code, response.Body.String())
	}
	if probe.read {
		t.Fatal("未配置的 Intake 读了业务内容")
	}
}

// Covers: ADR-0022——编排交回 error 是没形成答案，500 且不带 outcome。
func TestAPreviewWhoseReadingCannotBeFormedFormsNoAnswer(t *testing.T) {
	endpoint := tfhttp.NewPreviewDepartureGateEndpoint(workFactIntakeForTest(t), failingPreviewer{})
	response := servePreview(endpoint, commandRequest(`{"schedule":"schedule-1","departsAt":"2026-09-05T10:00:00Z"}`))
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

type failingPreviewer struct{}

func (failingPreviewer) Preview(context.Context, application.PreviewDepartureGateCommand) (application.PreviewDepartureGateResult, error) {
	return application.PreviewDepartureGateResult{}, errors.New("reading could not be formed")
}

type unreachablePreviewer struct{ t *testing.T }

func (previewer unreachablePreviewer) Preview(context.Context, application.PreviewDepartureGateCommand) (application.PreviewDepartureGateResult, error) {
	previewer.t.Fatal("a request passed the unconfigured intake and reached the preview")
	return application.PreviewDepartureGateResult{}, nil
}
