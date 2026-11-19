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

const zoneRegistrationsPath = "/warehouse-fulfillment-zone-registrations"

type zoneEcho struct {
	WarehouseCode string `json:"warehouseCode"`
	ZoneCode      string `json:"zoneCode"`
	Revision      int    `json:"revision"`
	Name          string `json:"name"`
	Usage         string `json:"usage"`
	Deactivated   bool   `json:"deactivated"`
	Registrant    string `json:"registrant"`
	RegisteredAt  string `json:"registeredAt"`
}

func zoneEchoOf(t *testing.T, object map[string]json.RawMessage) zoneEcho {
	t.Helper()
	raw, present := object["zone"]
	if !present {
		t.Fatalf("答复体没有 zone：%v", keysOf(object))
	}
	var echo zoneEcho
	if err := json.Unmarshal(raw, &echo); err != nil {
		t.Fatalf("zone 不是预期形状：%s", raw)
	}
	return echo
}

// Covers: AT-WF-001（库区的部分）——首次登记答 201 REGISTERED，回显在册那一笔（键名 zone，含登记责任方与登记时刻）；原样重投答 200
// ALREADY_REGISTERED，回显的是当初落册的那一份，不是这次重投的操作者与时刻；同一修订号换内容答 200 CONTENT_CONFLICT，答复体只有
// outcome；首次登记的修订号给 2 答 200 NOT_ACCEPTED，refusal 是 REVISION_OUT_OF_SEQUENCE，latestRevision 是 0。
func TestZoneRegistrationAnswersEachOutcomeWithItsStatusAndBody(t *testing.T) {
	fixture := newZoneRegisterFixture(t)
	original := syntheticZone("SYN-ZONE-01", 1)

	registered := fixture.post(original.String())
	if registered.Code != http.StatusCreated {
		t.Fatalf("首次登记：状态 %d %s，want 201", registered.Code, registered.Body.String())
	}
	if contentType := registered.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("业务答复的 Content-Type = %q，want application/json", contentType)
	}
	object := decodeObject(t, registered)
	assertKeys(t, "REGISTERED", object, "outcome", "zone")
	if outcome := stringMember(t, object, "outcome"); outcome != "REGISTERED" {
		t.Fatalf("outcome = %s", outcome)
	}
	echo := zoneEchoOf(t, object)
	if echo.WarehouseCode != "SYN-WH-01" || echo.ZoneCode != "SYN-ZONE-01" || echo.Revision != 1 || echo.Name != "合成存储区" ||
		echo.Usage != "STORAGE" || echo.Deactivated || echo.Registrant != operatorOne || echo.RegisteredAt != "2026-11-18T09:00:00Z" {
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
	assertKeys(t, "ALREADY_REGISTERED", object, "outcome", "zone")
	if stringMember(t, object, "outcome") != "ALREADY_REGISTERED" {
		t.Fatalf("重放的 outcome：%s", replayed.Body.String())
	}
	if replay := zoneEchoOf(t, object); replay.Registrant != operatorOne || replay.RegisteredAt != "2026-11-18T09:00:00Z" {
		t.Fatalf("重放回显的登记责任方与时刻 = %s、%s，want 当初落册的那一份", replay.Registrant, replay.RegisteredAt)
	}

	renamed := original
	renamed.name = "合成存储区（改名）"
	conflict := fixture.post(renamed.String())
	if conflict.Code != http.StatusOK {
		t.Fatalf("冲突：状态 %d %s，want 200", conflict.Code, conflict.Body.String())
	}
	object = decodeObject(t, conflict)
	assertKeys(t, "CONTENT_CONFLICT", object, "outcome")
	if stringMember(t, object, "outcome") != "CONTENT_CONFLICT" {
		t.Fatalf("冲突的 outcome：%s", conflict.Body.String())
	}

	skipped := fixture.post(syntheticZone("SYN-ZONE-NEW", 2).String())
	if skipped.Code != http.StatusOK {
		t.Fatalf("修订错位：状态 %d %s，want 200", skipped.Code, skipped.Body.String())
	}
	object = decodeObject(t, skipped)
	assertKeys(t, "REVISION_OUT_OF_SEQUENCE", object, "outcome", "refusal", "latestRevision")
	if stringMember(t, object, "outcome") != "NOT_ACCEPTED" || stringMember(t, object, "refusal") != "REVISION_OUT_OF_SEQUENCE" ||
		string(object["latestRevision"]) != "0" {
		t.Fatalf("册上没有这个库区时的错位答复 = %s，want latestRevision 0", skipped.Body.String())
	}
}

// Covers: AT-WF-001（库区的部分）——修订号接在在册最新修订之后才落册；错位答复回显在册最新修订号，登记方照它重读册面。
func TestZoneOutOfSequenceEchoesTheLatestRegisteredRevision(t *testing.T) {
	fixture := newZoneRegisterFixture(t)
	for revision := 1; revision <= 2; revision++ {
		if response := fixture.post(syntheticZone("SYN-ZONE-01", revision).String()); response.Code != http.StatusCreated {
			t.Fatalf("登记修订 %d：%d %s", revision, response.Code, response.Body.String())
		}
	}

	response := fixture.post(syntheticZone("SYN-ZONE-01", 4).String())
	object := decodeObject(t, response)
	assertKeys(t, "out of sequence", object, "outcome", "refusal", "latestRevision")
	if response.Code != http.StatusOK || string(object["latestRevision"]) != "2" {
		t.Fatalf("答复 = %d %s，want 200 且 latestRevision 2", response.Code, response.Body.String())
	}
}

// Covers: AT-WF-002（库区的部分）——以新修订改库区用途答 200 NOT_ACCEPTED，refusal 是 IMMUTABLE_FIELD，field 指出 usage，不带别的格；
// 只改名称的新修订答 201。
func TestAChangeOfZoneUsageIsRefusedAsAnImmutableFieldAndNamesTheCell(t *testing.T) {
	fixture := newZoneRegisterFixture(t)
	if response := fixture.post(syntheticZone("SYN-ZONE-01", 1).String()); response.Code != http.StatusCreated {
		t.Fatalf("首次登记：%d %s", response.Code, response.Body.String())
	}

	picking := syntheticZone("SYN-ZONE-01", 2)
	picking.usage = "PICKING"
	response := fixture.post(picking.String())
	object := decodeObject(t, response)
	assertKeys(t, "IMMUTABLE_FIELD", object, "outcome", "refusal", "field")
	if response.Code != http.StatusOK || stringMember(t, object, "outcome") != "NOT_ACCEPTED" ||
		stringMember(t, object, "refusal") != "IMMUTABLE_FIELD" || stringMember(t, object, "field") != "usage" {
		t.Fatalf("答复 = %d %s", response.Code, response.Body.String())
	}

	renamed := syntheticZone("SYN-ZONE-01", 2)
	renamed.name = "合成存储区（新名）"
	if response := fixture.post(renamed.String()); response.Code != http.StatusCreated {
		t.Fatalf("只改名称：%d %s，want 201", response.Code, response.Body.String())
	}
}

// Covers: AT-WF-003 与 AT-WF-009（库区的部分）——指名没登记的仓答 200 NOT_ACCEPTED，refusal 是 REFERENCE_NOT_FOUND，field 指出
// warehouseCode，不带别的格，不落册；指名别的租户的仓编码同答，答复体与指名一个从没登记过的仓逐字节相同（只差回显的仓编码不存在，
// 答复里本来就没有仓编码），不泄露别的租户有没有这个仓。
func TestAZoneInAWarehouseThatIsNotRegisteredIsRefusedAsReferenceNotFound(t *testing.T) {
	fixture := newZoneRegisterFixture(t)
	// SYN-WH-OTHER 只在别的租户名下登记过。
	fixture.warehouses.registered[tenantB+"\x00SYN-WH-OTHER"] = true

	never := syntheticZone("SYN-ZONE-01", 1)
	never.warehouse = "SYN-WH-NEVER"
	otherTenants := syntheticZone("SYN-ZONE-01", 1)
	otherTenants.warehouse = "SYN-WH-OTHER"

	absent, elsewhere := fixture.post(never.String()), fixture.post(otherTenants.String())
	for name, response := range map[string]*httptest.ResponseRecorder{"never registered": absent, "another tenant's": elsewhere} {
		object := decodeObject(t, response)
		assertKeys(t, name, object, "outcome", "refusal", "field")
		if response.Code != http.StatusOK || stringMember(t, object, "outcome") != "NOT_ACCEPTED" ||
			stringMember(t, object, "refusal") != "REFERENCE_NOT_FOUND" || stringMember(t, object, "field") != "warehouseCode" {
			t.Fatalf("%s：答复 = %d %s", name, response.Code, response.Body.String())
		}
	}
	if absent.Body.String() != elsewhere.Body.String() {
		t.Fatalf("别的租户的仓答 %s，与没登记过的仓答 %s 不同：泄露了是否存在", elsewhere.Body.String(), absent.Body.String())
	}
	if fixture.zones.writes != 0 {
		t.Fatalf("查无所指写了 %d 次册", fixture.zones.writes)
	}

	// 仓登记之后，同一份报文原样重交落册。
	fixture.warehouses.registered[tenantA+"\x00SYN-WH-NEVER"] = true
	if response := fixture.post(never.String()); response.Code != http.StatusCreated {
		t.Fatalf("登记仓之后原样重交：%d %s，want 201", response.Code, response.Body.String())
	}
}

// Covers: AT-WF-004（库区的部分）——坏载荷答 400 MALFORMED_REQUEST（问题详情）：JSON 不合法、认不得的键（含自带租户或登记责任方）、
// 必填格缺失或类型不对、用途不在封闭六种里、必填文本格是空白。不读册、不写册、不问仓册；理由只点名格、不回显取值。
func TestMalformedZonePayloadsAreBadRequestsThatNeverTouchTheRegisters(t *testing.T) {
	valid := syntheticZone("SYN-ZONE-01", 1)
	valid.name = secretAddress
	validJSON := valid.String()
	cases := map[string]string{
		"not JSON":              `{`,
		"empty body":            ``,
		"unknown key":           strings.Replace(validJSON, `{`, `{"floor":3,`, 1),
		"tenant carried":        strings.Replace(validJSON, `{`, `{"tenantId":"`+tenantB+`",`, 1),
		"tenant equal to real":  strings.Replace(validJSON, `{`, `{"tenantId":"`+tenantA+`",`, 1),
		"registrant carried":    strings.Replace(validJSON, `{`, `{"registrant":"`+operatorTwo+`",`, 1),
		"warehouseCode missing": strings.Replace(validJSON, `"warehouseCode":"SYN-WH-01",`, ``, 1),
		"zoneCode missing":      strings.Replace(validJSON, `,"zoneCode":"SYN-ZONE-01"`, ``, 1),
		"revision is string":    strings.Replace(validJSON, `"revision":1`, `"revision":"1"`, 1),
		"usage outside the set": strings.Replace(validJSON, `STORAGE`, `COLD_STORAGE`, 1),
		"usage in lower case":   strings.Replace(validJSON, `STORAGE`, `storage`, 1),
		"name blank":            strings.Replace(validJSON, secretAddress, ` `, 1),
		"zoneCode blank":        strings.Replace(validJSON, `SYN-ZONE-01`, `  `, 1),
		"warehouseCode blank":   strings.Replace(validJSON, `SYN-WH-01`, ` `, 1),
		"deactivated is string": strings.Replace(validJSON, `"deactivated":false`, `"deactivated":"no"`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newZoneRegisterFixture(t)
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

// Covers: AT-WF-010（库区的部分）——发行方未配置、令牌无效、没有登记册配置写授予，以及身份依赖读不动，各答其格（状态码与码照仓那一册
// 与登记写面的先例），不读册、不写册；403 与 5xx 的问题详情不带 detail。
func TestZoneEnvelopeGradesAnswerTheirOwnCellAndNeverTouchTheRegisters(t *testing.T) {
	for _, testCase := range envelopeCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newZoneRegisterFixture(t)
			fixture.authenticator.err = testCase.err

			response := fixture.post(syntheticZone("SYN-ZONE-01", 1).String())

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

// Covers: AT-WF-010（库区的部分）——先认证、后读报文：令牌不过的请求，请求体一个字节都不读；带着坏载荷的调用方看到的是 401，
// 看不到报文校验的结果。持授予的调用方才走到译装。
func TestZoneAuthenticationComesBeforeTheBodyIsRead(t *testing.T) {
	fixture := newZoneRegisterFixture(t)
	fixture.authenticator.err = warehousehttp.ErrOperatorCredentialRejected
	body := newCountingBody(`{not even JSON`)

	request := httptest.NewRequest(http.MethodPost, zoneRegistrationsPath, body)
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
	request = httptest.NewRequest(http.MethodPost, zoneRegistrationsPath, granted)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response = httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	assertProblem(t, "持授予的坏载荷", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if granted.reads == 0 {
		t.Fatal("持授予的请求没有读请求体")
	}
}

// Covers: AT-WF-010（库区的部分）——持授予照常登记，全程不判准入范围；租户与登记责任方是认证交回的那两个，不取自载荷。库区册与仓册
// 都按租户隔离：换一个租户，同一份报文所指的仓在那个租户名下没登记，答查无所指。
func TestAGrantedOperatorRegistersAZoneWithTheTenantAndRegistrantFromTheEnvelope(t *testing.T) {
	fixture := newZoneRegisterFixture(t)
	fixture.warehouses.registered[tenantB+"\x00SYN-WH-01"] = true
	fixture.authenticator.identity = operatorIdentity(t, tenantB, operatorTwo)

	response := fixture.post(syntheticZone("SYN-ZONE-01", 1).String())
	if response.Code != http.StatusCreated {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	if echo := zoneEchoOf(t, decodeObject(t, response)); echo.Registrant != operatorTwo {
		t.Fatalf("登记责任方 = %q，want 认证交回的 %q", echo.Registrant, operatorTwo)
	}
	// 同一份报文换一个租户登记：库区册按租户隔离，仓也要在那个租户名下已登记。
	fixture.authenticator.identity = operatorIdentity(t, tenantA, operatorOne)
	if response := fixture.post(syntheticZone("SYN-ZONE-01", 1).String()); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记同编码的库区：%d %s，want 201", response.Code, response.Body.String())
	}
}

// Covers: ADR-0055 「未配置不改变方法约束」——方法不对由处理器自己答 405 并带 Allow，不认证、不读册。
func TestZoneRegistrationRefusesOtherMethodsBeforeAuthentication(t *testing.T) {
	fixture := newZoneRegisterFixture(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		request := httptest.NewRequest(method, zoneRegistrationsPath, strings.NewReader(syntheticZone("SYN-ZONE-01", 1).String()))
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

// Covers: UC-WF-001「权限、隔离与审计」——库区册或仓册读写不动不形成答案：答 500 NO_ANSWER_FORMED，不折成拒绝（折成「查无所指」会把
// 登记方打发去登记一个本来就在册的仓）；答复体与日志都不带名称，日志只带仓编码、库区编码与修订号。
func TestZoneRegistersThatCannotBeReachedFormNoAnswerAndLeakNoName(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	for name, arrange := range map[string]func(*zoneRegisterFixture){
		"zone register down": func(fixture *zoneRegisterFixture) {
			fixture.zones.loadErr = errors.New("register unreachable, failing row contains " + secretAddress)
		},
		"warehouse register down": func(fixture *zoneRegisterFixture) {
			fixture.warehouses.err = errors.New("register unreachable, failing row contains " + secretAddress)
		},
	} {
		t.Run(name, func(t *testing.T) {
			logged.Reset()
			fixture := newZoneRegisterFixture(t)
			arrange(fixture)
			zone := syntheticZone("SYN-ZONE-01", 1)
			zone.name = secretAddress

			response := fixture.post(zone.String())

			object := assertProblem(t, name, response, http.StatusInternalServerError, "NO_ANSWER_FORMED")
			if _, present := object["detail"]; present {
				t.Fatalf("5xx 的问题详情带了 detail：%s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), secretAddress) || strings.Contains(logged.String(), secretAddress) {
				t.Fatalf("名称外泄：答复 %s，日志 %s", response.Body.String(), logged.String())
			}
			if !strings.Contains(logged.String(), "SYN-WH-01") || !strings.Contains(logged.String(), "SYN-ZONE-01") {
				t.Fatalf("日志没有带仓编码与库区编码：%q", logged.String())
			}
		})
	}
}

// Covers: ADR-0055 决定三——渠道未配置时 UnconfiguredIntake 对库区登记同答 403 ACCESS_CHANNEL_NOT_CONFIGURED：不读请求体、不碰册。
func TestTheUnconfiguredIntakeAnswersAZoneRegistrationWithoutReadingAnything(t *testing.T) {
	zones := newMemoryZoneRegistry()
	register := warehousehttp.NewRegisterZoneEndpoint(warehousehttp.UnconfiguredIntake{}, nil)

	body := newCountingBody(syntheticZone("SYN-ZONE-01", 1).String())
	request := httptest.NewRequest(http.MethodPost, zoneRegistrationsPath, body)
	request.Header.Set("Authorization", "Bearer anything")
	response := httptest.NewRecorder()
	register.ServeHTTP(response, request)

	assertProblem(t, "登记", response, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	if body.reads != 0 || zones.touched() != 0 {
		t.Fatalf("未配置仍读了请求体 %d 次、碰了册 %d 次", body.reads, zones.touched())
	}
}
