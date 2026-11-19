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

const registrationsPath = "/warehouse-fulfillment-warehouse-registrations"

type warehouseEcho struct {
	WarehouseCode    string `json:"warehouseCode"`
	Revision         int    `json:"revision"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	BusinessTimeZone string `json:"businessTimeZone"`
	ColocatedNode    string `json:"colocatedNode"`
	Deactivated      bool   `json:"deactivated"`
	Registrant       string `json:"registrant"`
	RegisteredAt     string `json:"registeredAt"`
	Address          struct {
		Country string `json:"country"`
		Text    string `json:"text"`
	} `json:"address"`
}

func echoOf(t *testing.T, object map[string]json.RawMessage) warehouseEcho {
	t.Helper()
	raw, present := object["warehouse"]
	if !present {
		t.Fatalf("答复体没有 warehouse：%v", keysOf(object))
	}
	var echo warehouseEcho
	if err := json.Unmarshal(raw, &echo); err != nil {
		t.Fatalf("warehouse 不是预期形状：%s", raw)
	}
	return echo
}

// Covers: AT-WF-001（仓的部分）——首次登记答 201 REGISTERED，回显在册那一笔（含登记责任方与登记时刻）；原样重投答 200
// ALREADY_REGISTERED，回显的是当初落册的那一份，不是这次重投的操作者与时刻；同一修订号换内容答 200 CONTENT_CONFLICT，答复体
// 只有 outcome；首次登记的修订号给 2 答 200 NOT_ACCEPTED，refusal 是 REVISION_OUT_OF_SEQUENCE，latestRevision 是 0。
func TestRegistrationAnswersEachOutcomeWithItsStatusAndBody(t *testing.T) {
	fixture := newRegisterFixture(t)
	original := syntheticWarehouse("SYN-WH-01", 1)

	registered := fixture.post(original.String())
	if registered.Code != http.StatusCreated {
		t.Fatalf("首次登记：状态 %d %s，want 201", registered.Code, registered.Body.String())
	}
	if contentType := registered.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("业务答复的 Content-Type = %q，want application/json", contentType)
	}
	object := decodeObject(t, registered)
	assertKeys(t, "REGISTERED", object, "outcome", "warehouse")
	if outcome := stringMember(t, object, "outcome"); outcome != "REGISTERED" {
		t.Fatalf("outcome = %s", outcome)
	}
	echo := echoOf(t, object)
	if echo.WarehouseCode != "SYN-WH-01" || echo.Revision != 1 || echo.Name != "合成一号仓" || echo.Kind != "SELF_OPERATED" ||
		echo.Address.Country != "CN" || echo.Address.Text != "合成路 1 号" || echo.BusinessTimeZone != "Asia/Shanghai" ||
		echo.ColocatedNode != "SYN-NODE-01" || echo.Deactivated || echo.Registrant != operatorOne ||
		echo.RegisteredAt != "2026-11-18T09:00:00Z" {
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
	assertKeys(t, "ALREADY_REGISTERED", object, "outcome", "warehouse")
	if stringMember(t, object, "outcome") != "ALREADY_REGISTERED" {
		t.Fatalf("重放的 outcome：%s", replayed.Body.String())
	}
	if replay := echoOf(t, object); replay.Registrant != operatorOne || replay.RegisteredAt != "2026-11-18T09:00:00Z" {
		t.Fatalf("重放回显的登记责任方与时刻 = %s、%s，want 当初落册的那一份", replay.Registrant, replay.RegisteredAt)
	}

	renamed := original
	renamed.name = "合成一号仓（改名）"
	conflict := fixture.post(renamed.String())
	if conflict.Code != http.StatusOK {
		t.Fatalf("冲突：状态 %d %s，want 200", conflict.Code, conflict.Body.String())
	}
	object = decodeObject(t, conflict)
	assertKeys(t, "CONTENT_CONFLICT", object, "outcome")
	if stringMember(t, object, "outcome") != "CONTENT_CONFLICT" {
		t.Fatalf("冲突的 outcome：%s", conflict.Body.String())
	}

	skipped := fixture.post(syntheticWarehouse("SYN-WH-NEW", 2).String())
	if skipped.Code != http.StatusOK {
		t.Fatalf("修订错位：状态 %d %s，want 200", skipped.Code, skipped.Body.String())
	}
	object = decodeObject(t, skipped)
	assertKeys(t, "REVISION_OUT_OF_SEQUENCE", object, "outcome", "refusal", "latestRevision")
	if stringMember(t, object, "outcome") != "NOT_ACCEPTED" || stringMember(t, object, "refusal") != "REVISION_OUT_OF_SEQUENCE" ||
		string(object["latestRevision"]) != "0" {
		t.Fatalf("册上没有这个仓时的错位答复 = %s，want latestRevision 0", skipped.Body.String())
	}
}

// Covers: AT-WF-001（仓的部分）——修订号接在在册最新修订之后才落册；错位答复回显在册最新修订号，登记方照它重读册面。
func TestOutOfSequenceEchoesTheLatestRegisteredRevision(t *testing.T) {
	fixture := newRegisterFixture(t)
	for revision := 1; revision <= 2; revision++ {
		if response := fixture.post(syntheticWarehouse("SYN-WH-01", revision).String()); response.Code != http.StatusCreated {
			t.Fatalf("登记修订 %d：%d %s", revision, response.Code, response.Body.String())
		}
	}

	response := fixture.post(syntheticWarehouse("SYN-WH-01", 4).String())
	object := decodeObject(t, response)
	assertKeys(t, "out of sequence", object, "outcome", "refusal", "latestRevision")
	if response.Code != http.StatusOK || string(object["latestRevision"]) != "2" {
		t.Fatalf("答复 = %d %s，want 200 且 latestRevision 2", response.Code, response.Body.String())
	}
}

// Covers: AT-WF-002——以新修订把自营仓改成代理仓答 200 NOT_ACCEPTED，refusal 是 IMMUTABLE_FIELD，field 指出 kind，不带别的格；
// 只改名称与地址的新修订答 201。
func TestAChangeOfKindIsRefusedAsAnImmutableFieldAndNamesTheCell(t *testing.T) {
	fixture := newRegisterFixture(t)
	if response := fixture.post(syntheticWarehouse("SYN-WH-01", 1).String()); response.Code != http.StatusCreated {
		t.Fatalf("首次登记：%d %s", response.Code, response.Body.String())
	}

	agent := syntheticWarehouse("SYN-WH-01", 2)
	agent.kind = "AGENT"
	response := fixture.post(agent.String())
	object := decodeObject(t, response)
	assertKeys(t, "IMMUTABLE_FIELD", object, "outcome", "refusal", "field")
	if response.Code != http.StatusOK || stringMember(t, object, "outcome") != "NOT_ACCEPTED" ||
		stringMember(t, object, "refusal") != "IMMUTABLE_FIELD" || stringMember(t, object, "field") != "kind" {
		t.Fatalf("答复 = %d %s", response.Code, response.Body.String())
	}

	renamed := syntheticWarehouse("SYN-WH-01", 2)
	renamed.name, renamed.text = "合成一号仓（新名）", "合成路 9 号"
	if response := fixture.post(renamed.String()); response.Code != http.StatusCreated {
		t.Fatalf("只改名称与地址：%d %s，want 201", response.Code, response.Body.String())
	}
}

// Covers: AT-WF-004（国家）、AT-WF-006——国家码不在内置目录、业务时区认不出，答 200 NOT_ACCEPTED，refusal 是 CONTENT_INVALID，
// field 点名那一格（address.country、businessTimeZone）；它们不是坏载荷，译装没有提前拒。不落册。
func TestContentTheApplicationRefusesNamesTheCellAndIsNotABadPayload(t *testing.T) {
	cases := map[string]struct {
		edit  func(*warehouseJSON)
		field string
	}{
		"country outside the catalogue": {func(warehouse *warehouseJSON) { warehouse.country = "ZZ" }, "address.country"},
		"time zone not recognised":      {func(warehouse *warehouseJSON) { warehouse.zone = "Mars/Olympus" }, "businessTimeZone"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newRegisterFixture(t)
			warehouse := syntheticWarehouse("SYN-WH-01", 1)
			testCase.edit(&warehouse)

			response := fixture.post(warehouse.String())
			object := decodeObject(t, response)
			assertKeys(t, name, object, "outcome", "refusal", "field")
			if response.Code != http.StatusOK || stringMember(t, object, "outcome") != "NOT_ACCEPTED" ||
				stringMember(t, object, "refusal") != "CONTENT_INVALID" || stringMember(t, object, "field") != testCase.field {
				t.Fatalf("答复 = %d %s，want 200 CONTENT_INVALID 并点名 %s", response.Code, response.Body.String(), testCase.field)
			}
			if fixture.registry.writes != 0 {
				t.Fatalf("未受理写了 %d 次册", fixture.registry.writes)
			}
		})
	}
}

// Covers: AT-WF-004——坏载荷答 400 MALFORMED_REQUEST（问题详情）：JSON 不合法、认不得的键（含自带租户或登记责任方）、必填格缺失或
// 类型不对、种类不在封闭集里、必填文本格是空白。不读册、不写册；理由只点名格、不回显取值。
func TestMalformedPayloadsAreBadRequestsThatNeverTouchTheRegister(t *testing.T) {
	valid := syntheticWarehouse("SYN-WH-01", 1)
	valid.text = secretAddress
	validJSON := valid.String()
	cases := map[string]string{
		"not JSON":              `{`,
		"empty body":            ``,
		"unknown key":           strings.Replace(validJSON, `{`, `{"floor":3,`, 1),
		"tenant carried":        strings.Replace(validJSON, `{`, `{"tenantId":"`+tenantB+`",`, 1),
		"tenant equal to real":  strings.Replace(validJSON, `{`, `{"tenantId":"`+tenantA+`",`, 1),
		"registrant carried":    strings.Replace(validJSON, `{`, `{"registrant":"`+operatorTwo+`",`, 1),
		"warehouseCode missing": strings.Replace(validJSON, `,"warehouseCode":"SYN-WH-01"`, ``, 1),
		"revision is string":    strings.Replace(validJSON, `"revision":1`, `"revision":"1"`, 1),
		"kind outside the set":  strings.Replace(validJSON, `SELF_OPERATED`, `PARTNER`, 1),
		"name blank":            strings.Replace(validJSON, `合成一号仓`, `  `, 1),
		"address text blank":    strings.Replace(validJSON, secretAddress, ` `, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newRegisterFixture(t)
			response := fixture.post(body)

			object := assertProblem(t, name, response, http.StatusBadRequest, "MALFORMED_REQUEST")
			if detail := stringMember(t, object, "detail"); detail == "" || strings.Contains(response.Body.String(), secretAddress) {
				t.Fatalf("坏载荷要带一句理由、且不回显地址正文：%s", response.Body.String())
			}
			if fixture.registry.touched() != 0 {
				t.Fatalf("坏载荷碰了册：读写共 %d 次", fixture.registry.touched())
			}
		})
	}
}

// Covers: AT-WF-010（仓的部分）——发行方未配置、令牌无效、没有登记册配置写授予，以及身份依赖读不动，各答其格（状态码与码照
// 登记写面的先例），不读册、不写册；403 与 5xx 的问题详情不带 detail。
func TestEnvelopeGradesAnswerTheirOwnCellAndNeverTouchTheRegister(t *testing.T) {
	for _, testCase := range envelopeCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRegisterFixture(t)
			fixture.authenticator.err = testCase.err

			response := fixture.post(syntheticWarehouse("SYN-WH-01", 1).String())

			object := assertProblem(t, testCase.name, response, testCase.status, testCase.code)
			if _, present := object["detail"]; present && (testCase.status == http.StatusForbidden || testCase.status >= 500) {
				t.Fatalf("%d 的问题详情带了 detail：%s", testCase.status, response.Body.String())
			}
			if fixture.registry.touched() != 0 {
				t.Fatalf("信封一层的拒绝碰了册：读写共 %d 次", fixture.registry.touched())
			}
		})
	}
}

// Covers: AT-WF-010（仓的部分）——先认证、后读报文：令牌不过的请求，请求体一个字节都不读；带着坏载荷的调用方看到的是 401，
// 看不到报文校验的结果。持授予的调用方才走到译装。
func TestAuthenticationComesBeforeTheBodyIsRead(t *testing.T) {
	fixture := newRegisterFixture(t)
	fixture.authenticator.err = warehousehttp.ErrOperatorCredentialRejected
	body := newCountingBody(`{not even JSON`)

	request := httptest.NewRequest(http.MethodPost, registrationsPath, body)
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
	request = httptest.NewRequest(http.MethodPost, registrationsPath, granted)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response = httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	assertProblem(t, "持授予的坏载荷", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if granted.reads == 0 {
		t.Fatal("持授予的请求没有读请求体")
	}
}

// Covers: AT-WF-010（仓的部分）——持授予照常登记，全程不判准入范围：登记口的 Intake 只认证、不碰任何准入对照，接口上就没有
// 准入这一格。租户与登记责任方是认证交回的那两个，不取自载荷。
func TestAGrantedOperatorRegistersWithTheTenantAndRegistrantFromTheEnvelope(t *testing.T) {
	fixture := newRegisterFixture(t)
	fixture.authenticator.identity = operatorIdentity(t, tenantB, operatorTwo)

	response := fixture.post(syntheticWarehouse("SYN-WH-01", 1).String())
	if response.Code != http.StatusCreated {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	if echo := echoOf(t, decodeObject(t, response)); echo.Registrant != operatorTwo {
		t.Fatalf("登记责任方 = %q，want 认证交回的 %q", echo.Registrant, operatorTwo)
	}
	// 同一份报文换一个租户登记：册按租户隔离，仍是首次登记。
	fixture.authenticator.identity = operatorIdentity(t, tenantA, operatorOne)
	if response := fixture.post(syntheticWarehouse("SYN-WH-01", 1).String()); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记同编码的仓：%d %s，want 201", response.Code, response.Body.String())
	}
}

// Covers: ADR-0055 「未配置不改变方法约束」——方法不对由处理器自己答 405 并带 Allow，不认证、不读册。
func TestAWrongMethodIsRefusedBeforeAuthentication(t *testing.T) {
	fixture := newRegisterFixture(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		request := httptest.NewRequest(method, registrationsPath, strings.NewReader(syntheticWarehouse("SYN-WH-01", 1).String()))
		response := httptest.NewRecorder()
		fixture.handler.ServeHTTP(response, request)

		assertProblem(t, method, response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		if allow := response.Header().Get("Allow"); allow != http.MethodPost {
			t.Fatalf("%s：Allow = %q，want POST", method, allow)
		}
	}
	if fixture.authenticator.asked != 0 || fixture.registry.touched() != 0 {
		t.Fatalf("方法不对仍认证了 %d 次、碰了册 %d 次", fixture.authenticator.asked, fixture.registry.touched())
	}
}

// Covers: UC-WF-001「权限、隔离与审计」——册读写不动不形成答案：答 500 NO_ANSWER_FORMED，不折成拒绝；答复体与日志都不带地址正文，
// 日志只带仓编码与修订号。
func TestARegisterThatCannotBeReachedFormsNoAnswerAndLeaksNoAddress(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	fixture := newRegisterFixture(t)
	fixture.registry.loadErr = errors.New("register unreachable, failing row contains " + secretAddress)
	warehouse := syntheticWarehouse("SYN-WH-01", 1)
	warehouse.text = secretAddress

	response := fixture.post(warehouse.String())

	object := assertProblem(t, "册读不动", response, http.StatusInternalServerError, "NO_ANSWER_FORMED")
	if _, present := object["detail"]; present {
		t.Fatalf("5xx 的问题详情带了 detail：%s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), secretAddress) || strings.Contains(logged.String(), secretAddress) {
		t.Fatalf("地址正文外泄：答复 %s，日志 %s", response.Body.String(), logged.String())
	}
	if !strings.Contains(logged.String(), "SYN-WH-01") {
		t.Fatalf("日志没有带仓编码：%q", logged.String())
	}
}

// Covers: ADR-0055 决定三——渠道未配置时 UnconfiguredIntake 对一切请求同答 403 ACCESS_CHANNEL_NOT_CONFIGURED：不读请求体、不碰册，
// 登记、列表与修订史各口都一样。
func TestTheUnconfiguredIntakeAnswersEveryRequestTheSameWithoutReadingAnything(t *testing.T) {
	registry := newMemoryRegistry()
	catalogue := &fakeCatalogue{}
	register := warehousehttp.NewRegisterWarehouseEndpoint(warehousehttp.UnconfiguredIntake{}, nil)
	list := warehousehttp.NewQueryWarehousesEndpoint(warehousehttp.UnconfiguredIntake{}, catalogue)
	revisions := warehousehttp.NewQueryWarehouseRevisionsEndpoint(warehousehttp.UnconfiguredIntake{}, catalogue)

	body := newCountingBody(syntheticWarehouse("SYN-WH-01", 1).String())
	request := httptest.NewRequest(http.MethodPost, registrationsPath, body)
	request.Header.Set("Authorization", "Bearer anything")
	response := httptest.NewRecorder()
	register.ServeHTTP(response, request)
	assertProblem(t, "登记", response, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	if body.reads != 0 {
		t.Fatalf("未配置仍读了请求体 %d 次", body.reads)
	}

	assertProblem(t, "列表", get(list, "/warehouse-fulfillment-warehouses?deactivated=bogus"), http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	assertProblem(t, "修订史", get(revisions, "/warehouse-fulfillment-warehouse-revisions"), http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	if registry.touched() != 0 || catalogue.listed != 0 || catalogue.revisions != 0 {
		t.Fatal("未配置仍碰了读写面")
	}
}
