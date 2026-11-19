package warehousehttp_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

const locationTypeRegistrationsPath = "/warehouse-fulfillment-location-type-registrations"

type locationTypeEcho struct {
	LocationTypeCode string `json:"locationTypeCode"`
	Revision         int    `json:"revision"`
	Name             string `json:"name"`
	Deactivated      bool   `json:"deactivated"`
	Registrant       string `json:"registrant"`
	RegisteredAt     string `json:"registeredAt"`
}

func locationTypeEchoOf(t *testing.T, object map[string]json.RawMessage) locationTypeEcho {
	t.Helper()
	raw, present := object["locationType"]
	if !present {
		t.Fatalf("答复体没有 locationType：%v", keysOf(object))
	}
	var echo locationTypeEcho
	if err := json.Unmarshal(raw, &echo); err != nil {
		t.Fatalf("locationType 不是预期形状：%s", raw)
	}
	return echo
}

// Covers: AT-WF-001（库位类型的部分）——首次登记答 201 REGISTERED，回显在册那一笔（键名 locationType，含登记责任方与登记时刻）；原样重投答
// 200 ALREADY_REGISTERED，回显的是当初落册的那一份，不是这次重投的操作者与时刻；同一修订号换内容答 200 CONTENT_CONFLICT，答复体只有
// outcome；首次登记的修订号给 2 答 200 NOT_ACCEPTED，refusal 是 REVISION_OUT_OF_SEQUENCE，latestRevision 是 0。
func TestLocationTypeRegistrationAnswersEachOutcomeWithItsStatusAndBody(t *testing.T) {
	fixture := newLocationTypeRegisterFixture(t)
	original := syntheticLocationType("SYN-LT-01", 1)

	registered := fixture.post(original.String())
	if registered.Code != http.StatusCreated {
		t.Fatalf("首次登记：状态 %d %s，want 201", registered.Code, registered.Body.String())
	}
	if contentType := registered.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("业务答复的 Content-Type = %q，want application/json", contentType)
	}
	object := decodeObject(t, registered)
	assertKeys(t, "REGISTERED", object, "outcome", "locationType")
	if outcome := stringMember(t, object, "outcome"); outcome != "REGISTERED" {
		t.Fatalf("outcome = %s", outcome)
	}
	echo := locationTypeEchoOf(t, object)
	if echo.LocationTypeCode != "SYN-LT-01" || echo.Revision != 1 || echo.Name != "合成货架位" || echo.Deactivated ||
		echo.Registrant != operatorOne || echo.RegisteredAt != "2026-11-18T09:00:00Z" {
		t.Fatalf("在册那一笔的回显 = %+v", echo)
	}

	// 换一个操作者、晚三个小时原样重投：重放只比内容，回显的是在册那一份。
	fixture.authenticator.identity = operatorIdentity(t, tenantA, operatorTwo)
	*fixture.now = laterInstant
	replayed := fixture.post(original.String())
	if replayed.Code != http.StatusOK {
		t.Fatalf("重放：状态 %d %s，want 200", replayed.Code, replayed.Body.String())
	}
	object = decodeObject(t, replayed)
	assertKeys(t, "ALREADY_REGISTERED", object, "outcome", "locationType")
	if stringMember(t, object, "outcome") != "ALREADY_REGISTERED" {
		t.Fatalf("重放的 outcome：%s", replayed.Body.String())
	}
	if replay := locationTypeEchoOf(t, object); replay.Registrant != operatorOne || replay.RegisteredAt != "2026-11-18T09:00:00Z" {
		t.Fatalf("重放回显的登记责任方与时刻 = %s、%s，want 当初落册的那一份", replay.Registrant, replay.RegisteredAt)
	}

	renamed := original
	renamed.name = "合成货架位（改名）"
	conflict := fixture.post(renamed.String())
	if conflict.Code != http.StatusOK {
		t.Fatalf("冲突：状态 %d %s，want 200", conflict.Code, conflict.Body.String())
	}
	object = decodeObject(t, conflict)
	assertKeys(t, "CONTENT_CONFLICT", object, "outcome")
	if stringMember(t, object, "outcome") != "CONTENT_CONFLICT" {
		t.Fatalf("冲突的 outcome：%s", conflict.Body.String())
	}

	skipped := fixture.post(syntheticLocationType("SYN-LT-NEW", 2).String())
	if skipped.Code != http.StatusOK {
		t.Fatalf("修订错位：状态 %d %s，want 200", skipped.Code, skipped.Body.String())
	}
	object = decodeObject(t, skipped)
	assertKeys(t, "REVISION_OUT_OF_SEQUENCE", object, "outcome", "refusal", "latestRevision")
	if stringMember(t, object, "outcome") != "NOT_ACCEPTED" || stringMember(t, object, "refusal") != "REVISION_OUT_OF_SEQUENCE" ||
		string(object["latestRevision"]) != "0" {
		t.Fatalf("册上没有这个库位类型时的错位答复 = %s，want latestRevision 0", skipped.Body.String())
	}
}

// Covers: AT-WF-001（库位类型的部分）——修订号接在在册最新修订之后才落册；错位答复回显在册最新修订号，登记方照它重读册面。
func TestLocationTypeOutOfSequenceEchoesTheLatestRegisteredRevision(t *testing.T) {
	fixture := newLocationTypeRegisterFixture(t)
	for revision := 1; revision <= 2; revision++ {
		if response := fixture.post(syntheticLocationType("SYN-LT-01", revision).String()); response.Code != http.StatusCreated {
			t.Fatalf("登记修订 %d：%d %s", revision, response.Code, response.Body.String())
		}
	}

	response := fixture.post(syntheticLocationType("SYN-LT-01", 4).String())
	object := decodeObject(t, response)
	assertKeys(t, "out of sequence", object, "outcome", "refusal", "latestRevision")
	if response.Code != http.StatusOK || string(object["latestRevision"]) != "2" {
		t.Fatalf("答复 = %d %s，want 200 且 latestRevision 2", response.Code, response.Body.String())
	}
}

// Covers: AT-WF-009（库位类型的部分）——本租户一个仓都没登记时登记库位类型答 200 NOT_CONFIGURED，答复体恰是 {"outcome":"NOT_CONFIGURED"}：
// 没有 refusal、field、latestRevision，也没有回显的库位类型；不落册。登记仓之后同一份报文原样重交答 201 REGISTERED，再原样重投答 200
// ALREADY_REGISTERED。别的租户登记了仓不算：本租户仍答 NOT_CONFIGURED。
func TestLocationTypeRegistrationBeforeAnyWarehouseAnswersNotConfiguredWithOnlyTheOutcome(t *testing.T) {
	fixture := newLocationTypeRegisterFixture(t)
	fixture.warehouses.registered = map[string]bool{tenantB: true}

	document := syntheticLocationType("SYN-LT-01", 1).String()
	response := fixture.post(document)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("未配置：%d %s（%s），want 200 application/json", response.Code, response.Body.String(), response.Header().Get("Content-Type"))
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"outcome":"NOT_CONFIGURED"}` {
		t.Fatalf("未配置的答复体 = %s，want 恰是 {\"outcome\":\"NOT_CONFIGURED\"}", body)
	}
	assertKeys(t, "NOT_CONFIGURED", decodeObject(t, response), "outcome")
	if fixture.locationTypes.writes != 0 {
		t.Fatalf("未配置写了 %d 次册，want 不落册", fixture.locationTypes.writes)
	}

	fixture.warehouses.registered[tenantA] = true
	if landed := fixture.post(document); landed.Code != http.StatusCreated {
		t.Fatalf("登记仓之后原样重交：%d %s，want 201", landed.Code, landed.Body.String())
	}
	if replay := fixture.post(document); replay.Code != http.StatusOK || stringMember(t, decodeObject(t, replay), "outcome") != "ALREADY_REGISTERED" {
		t.Fatalf("再原样重投：%d %s，want 200 ALREADY_REGISTERED", replay.Code, replay.Body.String())
	}
}

// Covers: AT-WF-009、AT-WF-001（库位类型的部分）判定次序——重放先于 NOT_CONFIGURED：册上早就有的修订，哪怕此刻本租户一个仓都问不到，原样
// 重投仍答 ALREADY_REGISTERED、换内容仍答 CONTENT_CONFLICT；未落册的新修订才答 NOT_CONFIGURED。
func TestLocationTypeReplayAndConflictComeBeforeNotConfigured(t *testing.T) {
	fixture := newLocationTypeRegisterFixture(t)
	original := syntheticLocationType("SYN-LT-01", 1)
	if response := fixture.post(original.String()); response.Code != http.StatusCreated {
		t.Fatalf("首次登记：%d %s", response.Code, response.Body.String())
	}
	fixture.warehouses.registered = map[string]bool{}

	if replay := fixture.post(original.String()); replay.Code != http.StatusOK || stringMember(t, decodeObject(t, replay), "outcome") != "ALREADY_REGISTERED" {
		t.Fatalf("一个仓都问不到时重放：%d %s，want 200 ALREADY_REGISTERED", replay.Code, replay.Body.String())
	}
	renamed := original
	renamed.name = "另一个名字"
	if conflict := fixture.post(renamed.String()); conflict.Code != http.StatusOK || stringMember(t, decodeObject(t, conflict), "outcome") != "CONTENT_CONFLICT" {
		t.Fatalf("一个仓都问不到时换内容：%d %s，want 200 CONTENT_CONFLICT", conflict.Code, conflict.Body.String())
	}
	if fresh := fixture.post(syntheticLocationType("SYN-LT-02", 1).String()); stringMember(t, decodeObject(t, fresh), "outcome") != "NOT_CONFIGURED" {
		t.Fatalf("一个仓都问不到时登记新库位类型：%s，want NOT_CONFIGURED", fresh.Body.String())
	}
}

// Covers: AT-WF-004——坏载荷答 400 MALFORMED_REQUEST（问题详情）：JSON 不合法、认不得的键（含自带租户或登记责任方）、必填格缺失或类型不对、
// 必填文本格是空白。不读册、不写册、不问仓；理由只点名格、不回显取值。
func TestMalformedLocationTypePayloadsAreBadRequestsThatNeverTouchTheRegisters(t *testing.T) {
	valid := syntheticLocationType("SYN-LT-01", 1)
	valid.name = secretAddress
	validJSON := valid.String()
	cases := map[string]string{
		"not JSON":                   `{`,
		"empty body":                 ``,
		"unknown key":                strings.Replace(validJSON, `{`, `{"floor":3,`, 1),
		"tenant carried":             strings.Replace(validJSON, `{`, `{"tenantId":"`+tenantB+`",`, 1),
		"tenant equal to real":       strings.Replace(validJSON, `{`, `{"tenantId":"`+tenantA+`",`, 1),
		"registrant carried":         strings.Replace(validJSON, `{`, `{"registrant":"`+operatorTwo+`",`, 1),
		"locationTypeCode missing":   strings.Replace(validJSON, `"locationTypeCode":"SYN-LT-01",`, ``, 1),
		"name missing":               strings.Replace(validJSON, `"name":"`+secretAddress+`",`, ``, 1),
		"revision is string":         strings.Replace(validJSON, `"revision":1`, `"revision":"1"`, 1),
		"name blank":                 strings.Replace(validJSON, secretAddress, ` `, 1),
		"locationTypeCode blank":     strings.Replace(validJSON, `SYN-LT-01`, `  `, 1),
		"deactivated is string":      strings.Replace(validJSON, `"deactivated":false`, `"deactivated":"no"`, 1),
		"a zone cell":                strings.Replace(validJSON, `{`, `{"usage":"STORAGE",`, 1),
		"locationTypeCode is number": strings.Replace(validJSON, `"SYN-LT-01"`, `7`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newLocationTypeRegisterFixture(t)
			response := fixture.post(body)

			object := assertProblem(t, name, response, http.StatusBadRequest, "MALFORMED_REQUEST")
			if detail := stringMember(t, object, "detail"); detail == "" || strings.Contains(response.Body.String(), secretAddress) {
				t.Fatalf("坏载荷要带一句理由、且不回显名称：%s", response.Body.String())
			}
			if fixture.touched() != 0 {
				t.Fatalf("坏载荷碰了册：读写共 %d 次", fixture.touched())
			}
		})
	}
}

// Covers: AT-WF-010（库位类型的部分）——发行方未配置、令牌无效、没有登记册配置写授予，以及身份依赖读不动，各答其格（状态码与码照仓那一册与登记写面
// 的先例），不读册、不写册、不问仓；403 与 5xx 的问题详情不带 detail。
func TestLocationTypeEnvelopeGradesAnswerTheirOwnCellAndNeverTouchTheRegisters(t *testing.T) {
	for _, testCase := range envelopeCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newLocationTypeRegisterFixture(t)
			fixture.authenticator.err = testCase.err

			response := fixture.post(syntheticLocationType("SYN-LT-01", 1).String())

			object := assertProblem(t, testCase.name, response, testCase.status, testCase.code)
			if _, present := object["detail"]; present && (testCase.status == http.StatusForbidden || testCase.status >= 500) {
				t.Fatalf("%d 的问题详情带了 detail：%s", testCase.status, response.Body.String())
			}
			if fixture.touched() != 0 {
				t.Fatalf("信封一层的拒绝碰了册：读写共 %d 次", fixture.touched())
			}
		})
	}
}

// Covers: AT-WF-010（库位类型的部分）——先认证、后读报文：令牌不过的请求，请求体一个字节都不读；带着坏载荷的调用方看到的是 401，
// 看不到报文校验的结果。持授予的调用方才走到译装。
func TestLocationTypeAuthenticationComesBeforeTheBodyIsRead(t *testing.T) {
	fixture := newLocationTypeRegisterFixture(t)
	fixture.authenticator.err = warehousehttp.ErrOperatorCredentialRejected
	body := newCountingBody(`{not even JSON`)

	request := httptest.NewRequest(http.MethodPost, locationTypeRegistrationsPath, body)
	request.Header.Set("Authorization", "Bearer rejected.token")
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)

	assertProblem(t, "令牌无效", response, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED")
	if body.reads != 0 {
		t.Fatalf("令牌不过，请求体仍被读了 %d 次", body.reads)
	}
	if fixture.authenticator.token != "rejected.token" {
		t.Fatalf("认证收到的令牌 = %q，want 请求头里的 Bearer 令牌", fixture.authenticator.token)
	}

	fixture.authenticator.err = nil
	granted := newCountingBody(`{not even JSON`)
	request = httptest.NewRequest(http.MethodPost, locationTypeRegistrationsPath, granted)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response = httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	assertProblem(t, "持授予的坏载荷", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if granted.reads == 0 {
		t.Fatal("持授予的请求没有读请求体")
	}
}

// Covers: AT-WF-010（库位类型的部分）——持授予照常登记，全程不判准入范围；租户与登记责任方是认证交回的那两个，不取自载荷。库位类型册与「有没有仓」都
// 按租户隔离：换一个没登记过仓的租户，同一份报文答 NOT_CONFIGURED。
func TestAGrantedOperatorRegistersALocationTypeWithTheTenantAndRegistrantFromTheEnvelope(t *testing.T) {
	fixture := newLocationTypeRegisterFixture(t)
	fixture.authenticator.identity = operatorIdentity(t, tenantB, operatorTwo)

	// 租户 B 没登记过仓。
	if response := fixture.post(syntheticLocationType("SYN-LT-01", 1).String()); stringMember(t, decodeObject(t, response), "outcome") != "NOT_CONFIGURED" {
		t.Fatalf("没登记过仓的租户登记：%s，want NOT_CONFIGURED", response.Body.String())
	}
	fixture.warehouses.registered[tenantB] = true
	response := fixture.post(syntheticLocationType("SYN-LT-01", 1).String())
	if response.Code != http.StatusCreated {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	if echo := locationTypeEchoOf(t, decodeObject(t, response)); echo.Registrant != operatorTwo {
		t.Fatalf("登记责任方 = %q，want 认证交回的 %q", echo.Registrant, operatorTwo)
	}
	// 同一份报文换租户 A 登记：库位类型册按租户隔离，从修订 1 起。
	fixture.authenticator.identity = operatorIdentity(t, tenantA, operatorOne)
	if response := fixture.post(syntheticLocationType("SYN-LT-01", 1).String()); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记同编码的库位类型：%d %s，want 201", response.Code, response.Body.String())
	}
}

// Covers: ADR-0055 「未配置不改变方法约束」——方法不对由处理器自己答 405 并带 Allow，不认证、不读册。
func TestLocationTypeRegistrationRefusesOtherMethodsBeforeAuthentication(t *testing.T) {
	fixture := newLocationTypeRegisterFixture(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		request := httptest.NewRequest(method, locationTypeRegistrationsPath, strings.NewReader(syntheticLocationType("SYN-LT-01", 1).String()))
		response := httptest.NewRecorder()
		fixture.handler.ServeHTTP(response, request)

		assertProblem(t, method, response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		if allow := response.Header().Get("Allow"); allow != http.MethodPost {
			t.Fatalf("%s：Allow = %q，want POST", method, allow)
		}
	}
	if fixture.authenticator.asked != 0 || fixture.touched() != 0 {
		t.Fatalf("方法不对仍认证了 %d 次、碰了册 %d 次", fixture.authenticator.asked, fixture.touched())
	}
}

// Covers: UC-WF-001「权限、隔离与审计」——库位类型册或仓册读写不动不形成答案：答 500 NO_ANSWER_FORMED，不折成未配置或拒绝（折成「未配置」会把
// 登记方打发去登记一个本来就在册的仓）；答复体与日志都不带名称，日志只带库位类型编码与修订号。
func TestLocationTypeRegistersThatCannotBeReachedFormNoAnswerAndLeakNoName(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	for name, arrange := range map[string]func(*locationTypeRegisterFixture){
		"location type register down": func(fixture *locationTypeRegisterFixture) {
			fixture.locationTypes.loadErr = errors.New("register unreachable, failing row contains " + secretAddress)
		},
		"warehouse register down": func(fixture *locationTypeRegisterFixture) {
			fixture.warehouses.err = errors.New("register unreachable, failing row contains " + secretAddress)
		},
	} {
		t.Run(name, func(t *testing.T) {
			logged.Reset()
			fixture := newLocationTypeRegisterFixture(t)
			arrange(fixture)
			locationType := syntheticLocationType("SYN-LT-01", 1)
			locationType.name = secretAddress

			response := fixture.post(locationType.String())

			object := assertProblem(t, name, response, http.StatusInternalServerError, "NO_ANSWER_FORMED")
			if _, present := object["detail"]; present {
				t.Fatalf("5xx 的问题详情带了 detail：%s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), secretAddress) || strings.Contains(logged.String(), secretAddress) {
				t.Fatalf("名称外泄：答复 %s，日志 %s", response.Body.String(), logged.String())
			}
			if !strings.Contains(logged.String(), "SYN-LT-01") {
				t.Fatalf("日志没有带库位类型编码：%q", logged.String())
			}
		})
	}
}

// Covers: ADR-0055 决定三——渠道未配置时 UnconfiguredIntake 对库位类型登记同答 403 ACCESS_CHANNEL_NOT_CONFIGURED：不读请求体、不碰册。
func TestTheUnconfiguredIntakeAnswersALocationTypeRegistrationWithoutReadingAnything(t *testing.T) {
	locationTypes := newMemoryLocationTypeRegistry()
	register := warehousehttp.NewRegisterLocationTypeEndpoint(warehousehttp.UnconfiguredIntake{}, nil)

	body := newCountingBody(syntheticLocationType("SYN-LT-01", 1).String())
	request := httptest.NewRequest(http.MethodPost, locationTypeRegistrationsPath, body)
	request.Header.Set("Authorization", "Bearer anything")
	response := httptest.NewRecorder()
	register.ServeHTTP(response, request)

	assertProblem(t, "登记", response, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	if body.reads != 0 || locationTypes.touched() != 0 {
		t.Fatalf("未配置仍读了请求体 %d 次、碰了册 %d 次", body.reads, locationTypes.touched())
	}
}
