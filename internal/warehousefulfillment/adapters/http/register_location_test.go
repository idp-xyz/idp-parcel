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

const locationRegistrationsPath = "/warehouse-fulfillment-location-registrations"

type locationEcho struct {
	WarehouseCode    string  `json:"warehouseCode"`
	LocationCode     string  `json:"locationCode"`
	Revision         int     `json:"revision"`
	ZoneCode         string  `json:"zoneCode"`
	LocationTypeCode *string `json:"locationTypeCode"`
	Priority         *int32  `json:"priority"`
	Deactivated      bool    `json:"deactivated"`
	Registrant       string  `json:"registrant"`
	RegisteredAt     string  `json:"registeredAt"`
}

// locationEchoOf 取答复体里回显的那一笔；cells 是回显对象上的键，供「没有库位类型与优先级时那两个键整键缺席」的断言用。
func locationEchoOf(t *testing.T, object map[string]json.RawMessage) (locationEcho, map[string]json.RawMessage) {
	t.Helper()
	raw, present := object["location"]
	if !present {
		t.Fatalf("答复体没有 location：%v", keysOf(object))
	}
	var echo locationEcho
	if err := json.Unmarshal(raw, &echo); err != nil {
		t.Fatalf("location 不是预期形状：%s", raw)
	}
	var cells map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cells); err != nil {
		t.Fatalf("location 不是对象：%s", raw)
	}
	return echo, cells
}

// Covers: AT-WF-001（库位的部分）——首次登记答 201 REGISTERED，回显在册那一笔（键名 location，含登记责任方与登记时刻）；原样重投答 200
// ALREADY_REGISTERED，回显的是当初落册的那一份，不是这次重投的操作者与时刻；同一修订号换内容答 200 CONTENT_CONFLICT，答复体只有
// outcome；首次登记的修订号给 2 答 200 NOT_ACCEPTED，refusal 是 REVISION_OUT_OF_SEQUENCE，latestRevision 是 0。
func TestLocationRegistrationAnswersEachOutcomeWithItsStatusAndBody(t *testing.T) {
	fixture := newLocationRegisterFixture(t)
	original := syntheticLocation("SYN-LOC-01", 1).typed("SYN-LT-01").prioritised(7)

	registered := fixture.post(original.String())
	if registered.Code != http.StatusCreated {
		t.Fatalf("首次登记：状态 %d %s，want 201", registered.Code, registered.Body.String())
	}
	if contentType := registered.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("业务答复的 Content-Type = %q，want application/json", contentType)
	}
	object := decodeObject(t, registered)
	assertKeys(t, "REGISTERED", object, "outcome", "location")
	if outcome := stringMember(t, object, "outcome"); outcome != "REGISTERED" {
		t.Fatalf("outcome = %s", outcome)
	}
	echo, cells := locationEchoOf(t, object)
	assertKeys(t, "回显", cells, "warehouseCode", "locationCode", "revision", "zoneCode", "locationTypeCode", "priority", "deactivated", "registrant", "registeredAt")
	if echo.WarehouseCode != "SYN-WH-01" || echo.LocationCode != "SYN-LOC-01" || echo.Revision != 1 || echo.ZoneCode != "SYN-ZONE-01" ||
		echo.LocationTypeCode == nil || *echo.LocationTypeCode != "SYN-LT-01" || echo.Priority == nil || *echo.Priority != 7 ||
		echo.Deactivated || echo.Registrant != operatorOne || echo.RegisteredAt != "2026-11-18T09:00:00Z" {
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
	assertKeys(t, "ALREADY_REGISTERED", object, "outcome", "location")
	if stringMember(t, object, "outcome") != "ALREADY_REGISTERED" {
		t.Fatalf("重放的 outcome：%s", replayed.Body.String())
	}
	if replay, _ := locationEchoOf(t, object); replay.Registrant != operatorOne || replay.RegisteredAt != "2026-11-18T09:00:00Z" {
		t.Fatalf("重放回显的登记责任方与时刻 = %s、%s，want 当初落册的那一份", replay.Registrant, replay.RegisteredAt)
	}

	conflict := fixture.post(original.prioritised(8).String())
	if conflict.Code != http.StatusOK {
		t.Fatalf("冲突：状态 %d %s，want 200", conflict.Code, conflict.Body.String())
	}
	object = decodeObject(t, conflict)
	assertKeys(t, "CONTENT_CONFLICT", object, "outcome")
	if stringMember(t, object, "outcome") != "CONTENT_CONFLICT" {
		t.Fatalf("冲突的 outcome：%s", conflict.Body.String())
	}

	skipped := fixture.post(syntheticLocation("SYN-LOC-NEW", 2).String())
	if skipped.Code != http.StatusOK {
		t.Fatalf("修订错位：状态 %d %s，want 200", skipped.Code, skipped.Body.String())
	}
	object = decodeObject(t, skipped)
	assertKeys(t, "REVISION_OUT_OF_SEQUENCE", object, "outcome", "refusal", "latestRevision")
	if stringMember(t, object, "outcome") != "NOT_ACCEPTED" || stringMember(t, object, "refusal") != "REVISION_OUT_OF_SEQUENCE" ||
		string(object["latestRevision"]) != "0" {
		t.Fatalf("册上没有这个库位时的错位答复 = %s，want latestRevision 0", skipped.Body.String())
	}
}

// Covers: AT-WF-001（库位的部分）——没有库位类型或优先级时，回显里那个键整键缺席（不是 null、空串或 0）；优先级 0 要出现：「没给」与
// 「给了 0」是两份内容，回显分得开；负优先级与 32 位两端原样回显。
func TestLocationEchoOmitsAbsentCellsAndKeepsAZeroPriority(t *testing.T) {
	fixture := newLocationRegisterFixture(t)

	bare := fixture.post(syntheticLocation("SYN-LOC-BARE", 1).String())
	if bare.Code != http.StatusCreated {
		t.Fatalf("只带所属库区：%d %s", bare.Code, bare.Body.String())
	}
	_, cells := locationEchoOf(t, decodeObject(t, bare))
	assertKeys(t, "没有类型与优先级的回显", cells, "warehouseCode", "locationCode", "revision", "zoneCode", "deactivated", "registrant", "registeredAt")

	onlyType := fixture.post(syntheticLocation("SYN-LOC-TYPE", 1).typed("SYN-LT-01").String())
	_, cells = locationEchoOf(t, decodeObject(t, onlyType))
	assertKeys(t, "只带类型的回显", cells, "warehouseCode", "locationCode", "revision", "zoneCode", "locationTypeCode", "deactivated", "registrant", "registeredAt")

	for name, value := range map[string]int32{"zero": 0, "negative": -9, "max": 2147483647, "min": -2147483648} {
		response := fixture.post(syntheticLocation("SYN-LOC-"+strings.ToUpper(name), 1).prioritised(value).String())
		if response.Code != http.StatusCreated {
			t.Fatalf("优先级 %s：%d %s", name, response.Code, response.Body.String())
		}
		echo, cells := locationEchoOf(t, decodeObject(t, response))
		assertKeys(t, "只带优先级的回显", cells, "warehouseCode", "locationCode", "revision", "zoneCode", "priority", "deactivated", "registrant", "registeredAt")
		if echo.Priority == nil || *echo.Priority != value {
			t.Fatalf("优先级 %s 的回显 = %v，want %d", name, echo.Priority, value)
		}
	}

	// 同一修订号一笔带优先级 0、一笔不带，是内容冲突。
	conflict := fixture.post(syntheticLocation("SYN-LOC-BARE", 1).prioritised(0).String())
	if conflict.Code != http.StatusOK || stringMember(t, decodeObject(t, conflict), "outcome") != "CONTENT_CONFLICT" {
		t.Fatalf("带优先级 0 对不带：%d %s，want 200 CONTENT_CONFLICT", conflict.Code, conflict.Body.String())
	}
	// 上一笔带类型、这一笔不带，是合法的新修订。
	removed := fixture.post(syntheticLocation("SYN-LOC-TYPE", 2).String())
	if removed.Code != http.StatusCreated {
		t.Fatalf("去掉库位类型：%d %s，want 201", removed.Code, removed.Body.String())
	}
	if _, cells := locationEchoOf(t, decodeObject(t, removed)); cells["locationTypeCode"] != nil {
		t.Fatalf("去掉类型之后回显仍带 locationTypeCode：%s", removed.Body.String())
	}
}

// Covers: AT-WF-001（库位的部分）——修订号接在在册最新修订之后才落册；错位答复回显在册最新修订号，登记方照它重读册面。
func TestLocationOutOfSequenceEchoesTheLatestRegisteredRevision(t *testing.T) {
	fixture := newLocationRegisterFixture(t)
	for revision := 1; revision <= 2; revision++ {
		if response := fixture.post(syntheticLocation("SYN-LOC-01", revision).String()); response.Code != http.StatusCreated {
			t.Fatalf("登记修订 %d：%d %s", revision, response.Code, response.Body.String())
		}
	}

	response := fixture.post(syntheticLocation("SYN-LOC-01", 4).String())
	object := decodeObject(t, response)
	assertKeys(t, "out of sequence", object, "outcome", "refusal", "latestRevision")
	if response.Code != http.StatusOK || string(object["latestRevision"]) != "2" {
		t.Fatalf("答复 = %d %s，want 200 且 latestRevision 2", response.Code, response.Body.String())
	}
}

// Covers: AT-WF-002（库位的部分）——以新修订把库位改到另一个库区答 200 NOT_ACCEPTED，refusal 是 IMMUTABLE_FIELD，field 指出 zoneCode，
// 不带别的格；只改优先级的新修订答 201，修订号加一。
func TestAChangeOfLocationZoneIsRefusedAsAnImmutableFieldAndNamesTheCell(t *testing.T) {
	fixture := newLocationRegisterFixture(t)
	fixture.zones.registered[zoneLookupKey(tenantA, "SYN-WH-01", "SYN-ZONE-02")] = true
	if response := fixture.post(syntheticLocation("SYN-LOC-01", 1).String()); response.Code != http.StatusCreated {
		t.Fatalf("首次登记：%d %s", response.Code, response.Body.String())
	}

	moved := syntheticLocation("SYN-LOC-01", 2)
	moved.zone = "SYN-ZONE-02"
	response := fixture.post(moved.String())
	object := decodeObject(t, response)
	assertKeys(t, "IMMUTABLE_FIELD", object, "outcome", "refusal", "field")
	if response.Code != http.StatusOK || stringMember(t, object, "outcome") != "NOT_ACCEPTED" ||
		stringMember(t, object, "refusal") != "IMMUTABLE_FIELD" || stringMember(t, object, "field") != "zoneCode" {
		t.Fatalf("答复 = %d %s", response.Code, response.Body.String())
	}

	reprioritised := fixture.post(syntheticLocation("SYN-LOC-01", 2).prioritised(3).String())
	if reprioritised.Code != http.StatusCreated {
		t.Fatalf("只改优先级：%d %s，want 201", reprioritised.Code, reprioritised.Body.String())
	}
	if echo, _ := locationEchoOf(t, decodeObject(t, reprioritised)); echo.Revision != 2 || echo.Priority == nil || *echo.Priority != 3 {
		t.Fatalf("改优先级的回显 = %+v", echo)
	}
}

// Covers: AT-WF-003（库位的部分）——指名没登记的仓、没登记的库区（含另一个仓下的库区）、没登记的库位类型，各答 200 NOT_ACCEPTED，
// refusal 是 REFERENCE_NOT_FOUND，field 各指出 warehouseCode、zoneCode、locationTypeCode，不带别的格，不落册；指名别的租户的仓、库区、
// 库位类型同答，答复体与指名一个从没登记过的逐字节相同，不泄露别的租户有没有它们。
func TestALocationNamingWhatItCannotFindIsRefusedAsReferenceNotFound(t *testing.T) {
	fixture := newLocationRegisterFixture(t)
	// 这几样只在别的租户名下、或只在另一个仓下登记过。
	fixture.warehouses.registered[tenantB+"\x00SYN-WH-OTHER"] = true
	fixture.zones.registered[zoneLookupKey(tenantB, "SYN-WH-01", "SYN-ZONE-OTHER")] = true
	fixture.zones.registered[zoneLookupKey(tenantA, "SYN-WH-02", "SYN-ZONE-ELSEWHERE")] = true
	fixture.types.deactivated[tenantB+"\x00SYN-LT-OTHER"] = false

	for name, test := range map[string]struct {
		missing, present locationJSON
		field            string
	}{
		"warehouse": {
			missing: func() locationJSON { l := syntheticLocation("SYN-LOC-01", 1); l.warehouse = "SYN-WH-NEVER"; return l }(),
			present: func() locationJSON { l := syntheticLocation("SYN-LOC-01", 1); l.warehouse = "SYN-WH-OTHER"; return l }(),
			field:   "warehouseCode",
		},
		"zone": {
			missing: func() locationJSON { l := syntheticLocation("SYN-LOC-01", 1); l.zone = "SYN-ZONE-NEVER"; return l }(),
			present: func() locationJSON { l := syntheticLocation("SYN-LOC-01", 1); l.zone = "SYN-ZONE-OTHER"; return l }(),
			field:   "zoneCode",
		},
		"zone of another warehouse": {
			missing: func() locationJSON { l := syntheticLocation("SYN-LOC-01", 1); l.zone = "SYN-ZONE-NEVER"; return l }(),
			present: func() locationJSON { l := syntheticLocation("SYN-LOC-01", 1); l.zone = "SYN-ZONE-ELSEWHERE"; return l }(),
			field:   "zoneCode",
		},
		"location type": {
			missing: syntheticLocation("SYN-LOC-01", 1).typed("SYN-LT-NEVER"),
			present: syntheticLocation("SYN-LOC-01", 1).typed("SYN-LT-OTHER"),
			field:   "locationTypeCode",
		},
	} {
		absent, elsewhere := fixture.post(test.missing.String()), fixture.post(test.present.String())
		for label, response := range map[string]*httptest.ResponseRecorder{"never registered": absent, "registered elsewhere": elsewhere} {
			object := decodeObject(t, response)
			assertKeys(t, name+"/"+label, object, "outcome", "refusal", "field")
			if response.Code != http.StatusOK || stringMember(t, object, "outcome") != "NOT_ACCEPTED" ||
				stringMember(t, object, "refusal") != "REFERENCE_NOT_FOUND" || stringMember(t, object, "field") != test.field {
				t.Fatalf("%s/%s：答复 = %d %s，want REFERENCE_NOT_FOUND 指出 %s", name, label, response.Code, response.Body.String(), test.field)
			}
		}
		if absent.Body.String() != elsewhere.Body.String() {
			t.Fatalf("%s：别的租户或别的仓的对象答 %s，与没登记过的答 %s 不同：泄露了是否存在", name, elsewhere.Body.String(), absent.Body.String())
		}
	}
	if fixture.locations.writes != 0 {
		t.Fatalf("查无所指写了 %d 次册", fixture.locations.writes)
	}
}

// Covers: AT-WF-003（库位的部分）——新指名一个已停用的库位类型答 200 NOT_ACCEPTED，refusal 是 REFERENCE_DEACTIVATED，field 指出
// locationTypeCode，不带别的格，不落册；沿用原有的、后来被停用的库位类型只改优先级答 201。
func TestALocationNewlyNamingADeactivatedTypeIsRefusedAndKeepingAnExistingOneIsNot(t *testing.T) {
	fixture := newLocationRegisterFixture(t)
	fixture.types.deactivated[tenantA+"\x00SYN-LT-GONE"] = true

	response := fixture.post(syntheticLocation("SYN-LOC-NEW", 1).typed("SYN-LT-GONE").String())
	object := decodeObject(t, response)
	assertKeys(t, "REFERENCE_DEACTIVATED", object, "outcome", "refusal", "field")
	if response.Code != http.StatusOK || stringMember(t, object, "outcome") != "NOT_ACCEPTED" ||
		stringMember(t, object, "refusal") != "REFERENCE_DEACTIVATED" || stringMember(t, object, "field") != "locationTypeCode" {
		t.Fatalf("答复 = %d %s", response.Code, response.Body.String())
	}
	if fixture.locations.writes != 0 {
		t.Fatalf("所指已停用写了 %d 次册", fixture.locations.writes)
	}

	if first := fixture.post(syntheticLocation("SYN-LOC-01", 1).typed("SYN-LT-01").prioritised(1).String()); first.Code != http.StatusCreated {
		t.Fatalf("指名在用的类型：%d %s", first.Code, first.Body.String())
	}
	fixture.types.deactivated[tenantA+"\x00SYN-LT-01"] = true
	kept := fixture.post(syntheticLocation("SYN-LOC-01", 2).typed("SYN-LT-01").prioritised(2).String())
	if kept.Code != http.StatusCreated {
		t.Fatalf("沿用后来被停用的类型、只改优先级：%d %s，want 201", kept.Code, kept.Body.String())
	}
}

// Covers: AT-WF-009（库位的部分）——一个仓都没登记的租户登记库位：答 200 NOT_ACCEPTED，refusal 是 REFERENCE_NOT_FOUND，field 指出
// warehouseCode，不落册；之后登记了仓，同一份报文原样重交落册。
func TestALocationBeforeAnyWarehouseIsRegisteredIsRefusedAndLandsAfterwards(t *testing.T) {
	fixture := newLocationRegisterFixture(t)
	// 仓册里一个仓都没有：把夹具默认登记的仓拿掉。
	delete(fixture.warehouses.registered, tenantA+"\x00SYN-WH-01")
	post := fixture.post

	response := post(syntheticLocation("SYN-LOC-01", 1).String())
	object := decodeObject(t, response)
	assertKeys(t, "no warehouse", object, "outcome", "refusal", "field")
	if response.Code != http.StatusOK || stringMember(t, object, "refusal") != "REFERENCE_NOT_FOUND" || stringMember(t, object, "field") != "warehouseCode" {
		t.Fatalf("答复 = %d %s", response.Code, response.Body.String())
	}
	if fixture.locations.writes != 0 {
		t.Fatalf("一个仓都没登记写了 %d 次册", fixture.locations.writes)
	}

	fixture.warehouses.registered[tenantA+"\x00SYN-WH-01"] = true
	if response := post(syntheticLocation("SYN-LOC-01", 1).String()); response.Code != http.StatusCreated {
		t.Fatalf("登记仓之后原样重交：%d %s，want 201", response.Code, response.Body.String())
	}
}

// Covers: AT-WF-004（库位的部分）——坏载荷答 400 MALFORMED_REQUEST（问题详情）：JSON 不合法、认不得的键（含自带租户或登记责任方）、
// 必填格缺失或类型不对、必填文本格是空白、给了 locationTypeCode 却是空白、优先级带小数、是字符串、超出 32 位范围。不读册、不写册、不问
// 别的册；理由只点名格、不回显取值。
func TestMalformedLocationPayloadsAreBadRequestsThatNeverTouchTheRegisters(t *testing.T) {
	valid := syntheticLocation("SYN-LOC-01", 1).typed("SYN-LT-01").prioritised(5)
	validJSON := valid.String()
	cases := map[string]string{
		"not JSON":                   `{`,
		"empty body":                 ``,
		"unknown key":                strings.Replace(validJSON, `{`, `{"floor":3,`, 1),
		"tenant carried":             strings.Replace(validJSON, `{`, `{"tenantId":"`+tenantB+`",`, 1),
		"tenant equal to real":       strings.Replace(validJSON, `{`, `{"tenantId":"`+tenantA+`",`, 1),
		"registrant carried":         strings.Replace(validJSON, `{`, `{"registrant":"`+operatorTwo+`",`, 1),
		"warehouseCode missing":      strings.Replace(validJSON, `"warehouseCode":"SYN-WH-01",`, ``, 1),
		"locationCode missing":       strings.Replace(validJSON, `"locationCode":"SYN-LOC-01",`, ``, 1),
		"zoneCode missing":           strings.Replace(validJSON, `,"zoneCode":"SYN-ZONE-01"`, ``, 1),
		"revision is string":         strings.Replace(validJSON, `"revision":1`, `"revision":"1"`, 1),
		"locationCode blank":         strings.Replace(validJSON, `SYN-LOC-01`, `  `, 1),
		"zoneCode blank":             strings.Replace(validJSON, `SYN-ZONE-01`, ` `, 1),
		"warehouseCode blank":        strings.Replace(validJSON, `SYN-WH-01`, ` `, 1),
		"locationTypeCode blank":     strings.Replace(validJSON, `SYN-LT-01`, ` `, 1),
		"locationTypeCode is number": strings.Replace(validJSON, `"SYN-LT-01"`, `7`, 1),
		"priority fractional":        strings.Replace(validJSON, `"priority":5`, `"priority":5.5`, 1),
		"priority is string":         strings.Replace(validJSON, `"priority":5`, `"priority":"5"`, 1),
		"priority too large":         strings.Replace(validJSON, `"priority":5`, `"priority":2147483648`, 1),
		"priority too small":         strings.Replace(validJSON, `"priority":5`, `"priority":-2147483649`, 1),
		"deactivated is string":      strings.Replace(validJSON, `"deactivated":false`, `"deactivated":"no"`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newLocationRegisterFixture(t)
			response := fixture.post(body)

			object := assertProblem(t, name, response, http.StatusBadRequest, "MALFORMED_REQUEST")
			if detail := stringMember(t, object, "detail"); detail == "" || strings.Contains(response.Body.String(), "SYN-LT-01") {
				t.Fatalf("坏载荷要带一句理由、且不回显取值：%s", response.Body.String())
			}
			if fixture.touched() != 0 {
				t.Fatalf("坏载荷碰了册：读写共 %d 次", fixture.touched())
			}
		})
	}
}

// Covers: AT-WF-010（库位的部分）——发行方未配置、令牌无效、没有登记册配置写授予，以及身份依赖读不动，各答其格（状态码与码照仓那一册
// 与登记写面的先例），不读册、不写册；403 与 5xx 的问题详情不带 detail。
func TestLocationEnvelopeGradesAnswerTheirOwnCellAndNeverTouchTheRegisters(t *testing.T) {
	for _, testCase := range envelopeCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newLocationRegisterFixture(t)
			fixture.authenticator.err = testCase.err

			response := fixture.post(syntheticLocation("SYN-LOC-01", 1).String())

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

// Covers: AT-WF-010（库位的部分）——先认证、后读报文：令牌不过的请求，请求体一个字节都不读；带着坏载荷的调用方看到的是 401，
// 看不到报文校验的结果。持授予的调用方才走到译装。
func TestLocationAuthenticationComesBeforeTheBodyIsRead(t *testing.T) {
	fixture := newLocationRegisterFixture(t)
	fixture.authenticator.err = warehousehttp.ErrOperatorCredentialRejected
	body := newCountingBody(`{not even JSON`)

	request := httptest.NewRequest(http.MethodPost, locationRegistrationsPath, body)
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
	request = httptest.NewRequest(http.MethodPost, locationRegistrationsPath, granted)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response = httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	assertProblem(t, "持授予的坏载荷", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if granted.reads == 0 {
		t.Fatal("持授予的请求没有读请求体")
	}
}

// Covers: AT-WF-010（库位的部分）——持授予照常登记，全程不判准入范围；租户与登记责任方是认证交回的那两个，不取自载荷。库位册与别的册
// 都按租户隔离：换一个租户，同一份报文所指的仓、库区在那个租户名下没登记，答查无所指。
func TestAGrantedOperatorRegistersALocationWithTheTenantAndRegistrantFromTheEnvelope(t *testing.T) {
	fixture := newLocationRegisterFixture(t)
	fixture.warehouses.registered[tenantB+"\x00SYN-WH-01"] = true
	fixture.zones.registered[zoneLookupKey(tenantB, "SYN-WH-01", "SYN-ZONE-01")] = true
	fixture.authenticator.identity = operatorIdentity(t, tenantB, operatorTwo)

	response := fixture.post(syntheticLocation("SYN-LOC-01", 1).String())
	if response.Code != http.StatusCreated {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	if echo, _ := locationEchoOf(t, decodeObject(t, response)); echo.Registrant != operatorTwo {
		t.Fatalf("登记责任方 = %q，want 认证交回的 %q", echo.Registrant, operatorTwo)
	}
	// 同一份报文换一个租户登记：库位册按租户隔离，仓与库区也要在那个租户名下已登记。
	fixture.authenticator.identity = operatorIdentity(t, tenantA, operatorOne)
	if response := fixture.post(syntheticLocation("SYN-LOC-01", 1).String()); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记同编码的库位：%d %s，want 201", response.Code, response.Body.String())
	}
}

// Covers: ADR-0055 「未配置不改变方法约束」——方法不对由处理器自己答 405 并带 Allow，不认证、不读册。
func TestLocationRegistrationRefusesOtherMethodsBeforeAuthentication(t *testing.T) {
	fixture := newLocationRegisterFixture(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		request := httptest.NewRequest(method, locationRegistrationsPath, strings.NewReader(syntheticLocation("SYN-LOC-01", 1).String()))
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

// Covers: UC-WF-001「权限、隔离与审计」——库位册、仓册、库区册或库位类型册读写不动不形成答案：答 500 NO_ANSWER_FORMED，不折成拒绝
// （折成「查无所指」会把登记方打发去登记一个本来就在册的对象）；答复体与日志都不带库位类型编码，日志只带仓编码、库位编码与修订号。
func TestLocationRegistersThatCannotBeReachedFormNoAnswerAndLeakNoName(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	for name, arrange := range map[string]func(*locationRegisterFixture){
		"location register down": func(fixture *locationRegisterFixture) {
			fixture.locations.loadErr = errors.New("register unreachable, failing row contains " + secretAddress)
		},
		"warehouse register down": func(fixture *locationRegisterFixture) {
			fixture.warehouses.err = errors.New("register unreachable, failing row contains " + secretAddress)
		},
		"zone register down": func(fixture *locationRegisterFixture) {
			fixture.zones.err = errors.New("register unreachable, failing row contains " + secretAddress)
		},
		"location type register down": func(fixture *locationRegisterFixture) {
			fixture.types.err = errors.New("register unreachable, failing row contains " + secretAddress)
		},
	} {
		t.Run(name, func(t *testing.T) {
			logged.Reset()
			fixture := newLocationRegisterFixture(t)
			arrange(fixture)

			response := fixture.post(syntheticLocation("SYN-LOC-01", 1).typed("SYN-LT-01").String())

			object := assertProblem(t, name, response, http.StatusInternalServerError, "NO_ANSWER_FORMED")
			if _, present := object["detail"]; present {
				t.Fatalf("5xx 的问题详情带了 detail：%s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), secretAddress) || strings.Contains(logged.String(), secretAddress) {
				t.Fatalf("内容外泄：答复 %s，日志 %s", response.Body.String(), logged.String())
			}
			if strings.Contains(logged.String(), "SYN-LT-01") {
				t.Fatalf("日志带了库位类型编码：%q", logged.String())
			}
			if !strings.Contains(logged.String(), "SYN-WH-01") || !strings.Contains(logged.String(), "SYN-LOC-01") {
				t.Fatalf("日志没有带仓编码与库位编码：%q", logged.String())
			}
		})
	}
}

// Covers: ADR-0055 决定三——渠道未配置时 UnconfiguredIntake 对库位登记同答 403 ACCESS_CHANNEL_NOT_CONFIGURED：不读请求体、不碰册。
func TestTheUnconfiguredIntakeAnswersALocationRegistrationWithoutReadingAnything(t *testing.T) {
	locations := newMemoryLocationRegistry()
	register := warehousehttp.NewRegisterLocationEndpoint(warehousehttp.UnconfiguredIntake{}, nil)

	body := newCountingBody(syntheticLocation("SYN-LOC-01", 1).String())
	request := httptest.NewRequest(http.MethodPost, locationRegistrationsPath, body)
	request.Header.Set("Authorization", "Bearer anything")
	response := httptest.NewRecorder()
	register.ServeHTTP(response, request)

	assertProblem(t, "登记", response, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	if body.reads != 0 || locations.touched() != 0 {
		t.Fatalf("未配置仍读了请求体 %d 次、碰了册 %d 次", body.reads, locations.touched())
	}
}
