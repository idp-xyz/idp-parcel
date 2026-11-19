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

const packagingMaterialRegistrationsPath = "/warehouse-fulfillment-packaging-material-registrations"

type packagingMaterialEcho struct {
	PackagingMaterialCode string `json:"packagingMaterialCode"`
	Revision              int    `json:"revision"`
	Name                  string `json:"name"`
	Dimensions            struct {
		Length string `json:"length"`
		Width  string `json:"width"`
		Height string `json:"height"`
		Unit   string `json:"unit"`
	} `json:"dimensions"`
	Weight struct {
		Value string `json:"value"`
		Unit  string `json:"unit"`
	} `json:"weight"`
	Deactivated  bool   `json:"deactivated"`
	Registrant   string `json:"registrant"`
	RegisteredAt string `json:"registeredAt"`
}

func packagingMaterialEchoOf(t *testing.T, object map[string]json.RawMessage) packagingMaterialEcho {
	t.Helper()
	raw, present := object["packagingMaterial"]
	if !present {
		t.Fatalf("答复体没有 packagingMaterial：%v", keysOf(object))
	}
	var echo packagingMaterialEcho
	if err := json.Unmarshal(raw, &echo); err != nil {
		t.Fatalf("packagingMaterial 不是预期形状：%s", raw)
	}
	return echo
}

// Covers: AT-WF-001（包材的部分）——首次登记答 201 REGISTERED，回显在册那一笔（键名 packagingMaterial，形状与请求体的内容部分一致，另带登记责任方与登记时刻）；
// 原样重投答 200 ALREADY_REGISTERED，回显的是当初落册的那一份，不是这次重投的操作者与时刻；同一修订号换内容答 200 CONTENT_CONFLICT，答复体只有 outcome；
// 首次登记的修订号给 2 答 200 NOT_ACCEPTED，refusal 是 REVISION_OUT_OF_SEQUENCE，latestRevision 是 0。
func TestPackagingMaterialRegistrationAnswersEachOutcomeWithItsStatusAndBody(t *testing.T) {
	fixture := newPackagingMaterialRegisterFixture(t)
	original := syntheticPackagingMaterial("SYN-BOX-01", 1)

	registered := fixture.post(original.String())
	if registered.Code != http.StatusCreated {
		t.Fatalf("首次登记：状态 %d %s，want 201", registered.Code, registered.Body.String())
	}
	if contentType := registered.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("业务答复的 Content-Type = %q，want application/json", contentType)
	}
	object := decodeObject(t, registered)
	assertKeys(t, "REGISTERED", object, "outcome", "packagingMaterial")
	if outcome := stringMember(t, object, "outcome"); outcome != "REGISTERED" {
		t.Fatalf("outcome = %s", outcome)
	}
	echo := packagingMaterialEchoOf(t, object)
	if echo.PackagingMaterialCode != "SYN-BOX-01" || echo.Revision != 1 || echo.Name != "合成一号纸箱" || echo.Deactivated ||
		echo.Dimensions.Length != "30" || echo.Dimensions.Width != "20" || echo.Dimensions.Height != "15.5" || echo.Dimensions.Unit != "CM" ||
		echo.Weight.Value != "0.25" || echo.Weight.Unit != "KG" ||
		echo.Registrant != operatorOne || echo.RegisteredAt != "2026-11-18T09:00:00Z" {
		t.Fatalf("在册那一笔的回显 = %+v", echo)
	}
	// 数值是 JSON 字符串，不是 JSON 数字：回显也一样。
	var rawEcho map[string]json.RawMessage
	if err := json.Unmarshal(object["packagingMaterial"], &rawEcho); err != nil {
		t.Fatal(err)
	}
	var rawDimensions map[string]json.RawMessage
	if err := json.Unmarshal(rawEcho["dimensions"], &rawDimensions); err != nil || string(rawDimensions["height"]) != `"15.5"` {
		t.Fatalf("回显的 dimensions.height = %s，want JSON 字符串 \"15.5\"", rawDimensions["height"])
	}

	// 换一个操作者、晚三个小时原样重投：重放只比内容，回显的是在册那一份。
	fixture.authenticator.identity = operatorIdentity(t, tenantA, operatorTwo)
	*fixture.now = laterInstant
	replayed := fixture.post(original.String())
	if replayed.Code != http.StatusOK {
		t.Fatalf("重放：状态 %d %s，want 200", replayed.Code, replayed.Body.String())
	}
	object = decodeObject(t, replayed)
	assertKeys(t, "ALREADY_REGISTERED", object, "outcome", "packagingMaterial")
	if stringMember(t, object, "outcome") != "ALREADY_REGISTERED" {
		t.Fatalf("重放的 outcome：%s", replayed.Body.String())
	}
	if replay := packagingMaterialEchoOf(t, object); replay.Registrant != operatorOne || replay.RegisteredAt != "2026-11-18T09:00:00Z" {
		t.Fatalf("重放回显的登记责任方与时刻 = %s、%s，want 当初落册的那一份", replay.Registrant, replay.RegisteredAt)
	}

	renamed := original
	renamed.name = "合成一号纸箱（改名）"
	conflict := fixture.post(renamed.String())
	if conflict.Code != http.StatusOK {
		t.Fatalf("冲突：状态 %d %s，want 200", conflict.Code, conflict.Body.String())
	}
	object = decodeObject(t, conflict)
	assertKeys(t, "CONTENT_CONFLICT", object, "outcome")
	if stringMember(t, object, "outcome") != "CONTENT_CONFLICT" {
		t.Fatalf("冲突的 outcome：%s", conflict.Body.String())
	}

	skipped := fixture.post(syntheticPackagingMaterial("SYN-BOX-NEW", 2).String())
	if skipped.Code != http.StatusOK {
		t.Fatalf("修订错位：状态 %d %s，want 200", skipped.Code, skipped.Body.String())
	}
	object = decodeObject(t, skipped)
	assertKeys(t, "REVISION_OUT_OF_SEQUENCE", object, "outcome", "refusal", "latestRevision")
	if stringMember(t, object, "outcome") != "NOT_ACCEPTED" || stringMember(t, object, "refusal") != "REVISION_OUT_OF_SEQUENCE" ||
		string(object["latestRevision"]) != "0" {
		t.Fatalf("册上没有这个包材时的错位答复 = %s，want latestRevision 0", skipped.Body.String())
	}
}

// Covers: AT-WF-001（包材的部分）——修订号接在在册最新修订之后才落册；错位答复回显在册最新修订号，登记方照它重读册面。
func TestPackagingMaterialOutOfSequenceEchoesTheLatestRegisteredRevision(t *testing.T) {
	fixture := newPackagingMaterialRegisterFixture(t)
	for revision := 1; revision <= 2; revision++ {
		if response := fixture.post(syntheticPackagingMaterial("SYN-BOX-01", revision).String()); response.Code != http.StatusCreated {
			t.Fatalf("登记修订 %d：%d %s", revision, response.Code, response.Body.String())
		}
	}

	response := fixture.post(syntheticPackagingMaterial("SYN-BOX-01", 4).String())
	object := decodeObject(t, response)
	assertKeys(t, "out of sequence", object, "outcome", "refusal", "latestRevision")
	if response.Code != http.StatusOK || string(object["latestRevision"]) != "2" {
		t.Fatalf("答复 = %d %s，want 200 且 latestRevision 2", response.Code, response.Body.String())
	}
}

// Covers: AT-WF-004（包材的部分）「数值按数值比，不按写法比」「回显用去掉多余前导零与尾零的写法」——登记时给 30.00、020、15.50、0.250，回显 30、20、15.5、0.25；
// 原样重投换一种写法（30、20.000000、015.5、00.25）答 200 ALREADY_REGISTERED，不是冲突；登记时不折算单位：把 30 CM 换成 0.3 M 是另一份内容，同一修订号答
// CONTENT_CONFLICT，新修订号答 201。
func TestPackagingMaterialNumbersAreComparedByValueEchoedTrimmedAndNeverConverted(t *testing.T) {
	fixture := newPackagingMaterialRegisterFixture(t)
	first := syntheticPackagingMaterial("SYN-BOX-01", 1)
	first.length, first.width, first.height, first.weight = "30.00", "020", "15.50", "0.250"

	registered := fixture.post(first.String())
	if registered.Code != http.StatusCreated {
		t.Fatalf("首次登记：%d %s", registered.Code, registered.Body.String())
	}
	echo := packagingMaterialEchoOf(t, decodeObject(t, registered))
	if echo.Dimensions.Length != "30" || echo.Dimensions.Width != "20" || echo.Dimensions.Height != "15.5" || echo.Weight.Value != "0.25" {
		t.Fatalf("回显的数值 = %s、%s、%s、%s，want 去掉多余零的 30、20、15.5、0.25",
			echo.Dimensions.Length, echo.Dimensions.Width, echo.Dimensions.Height, echo.Weight.Value)
	}

	rewritten := syntheticPackagingMaterial("SYN-BOX-01", 1)
	rewritten.length, rewritten.width, rewritten.height, rewritten.weight = "30", "20.000000", "015.5", "00.25"
	replay := fixture.post(rewritten.String())
	if replay.Code != http.StatusOK || stringMember(t, decodeObject(t, replay), "outcome") != "ALREADY_REGISTERED" {
		t.Fatalf("换一种数值写法的原样重投：%d %s，want 200 ALREADY_REGISTERED", replay.Code, replay.Body.String())
	}
	if fixture.packagingMaterials.writes != 1 {
		t.Fatalf("换写法的重放写了 %d 次册，want 只有首笔那 1 次", fixture.packagingMaterials.writes)
	}

	meters := syntheticPackagingMaterial("SYN-BOX-01", 1)
	meters.length, meters.width, meters.height, meters.dimensionsUnit = "0.3", "0.2", "0.155", "M"
	if conflict := fixture.post(meters.String()); conflict.Code != http.StatusOK || stringMember(t, decodeObject(t, conflict), "outcome") != "CONTENT_CONFLICT" {
		t.Fatalf("30 CM 换成 0.3 M 的同一修订号：%d %s，want 200 CONTENT_CONFLICT（不折算单位）", conflict.Code, conflict.Body.String())
	}
	meters.revision = 2
	landed := fixture.post(meters.String())
	if landed.Code != http.StatusCreated {
		t.Fatalf("0.3 M 作修订 2：%d %s，want 201", landed.Code, landed.Body.String())
	}
	if echo := packagingMaterialEchoOf(t, decodeObject(t, landed)); echo.Dimensions.Length != "0.3" || echo.Dimensions.Unit != "M" {
		t.Fatalf("回显 = %+v，want 原样 0.3 M，不折算", echo.Dimensions)
	}
}

// Covers: AT-WF-004（包材的部分）——尺寸单位不在目录；重量给了长度类的单位；尺寸给了重量类的单位；重量为零；某一边尺寸为零：各答 200 NOT_ACCEPTED，
// refusal 是 CONTENT_INVALID，field 指出 dimensions.unit、weight.unit、dimensions.unit、weight.value、dimensions.length / width / height，答复体恰有
// outcome、refusal、field 三格（没有回显、没有最新修订号），不落册、不取默认值、不折算单位。
func TestAPackagingMaterialThatFailsTheContentGatesIsRefusedAndNamesTheCell(t *testing.T) {
	for name, test := range map[string]struct {
		edit func(*packagingMaterialJSON)
		want string
	}{
		"尺寸单位不在目录里":  {func(m *packagingMaterialJSON) { m.dimensionsUnit = "CUBIT" }, "dimensions.unit"},
		"尺寸单位的大小写不对": {func(m *packagingMaterialJSON) { m.dimensionsUnit = "cm" }, "dimensions.unit"},
		"重量给了长度类的单位": {func(m *packagingMaterialJSON) { m.weightUnit = "CM" }, "weight.unit"},
		"尺寸给了重量类的单位": {func(m *packagingMaterialJSON) { m.dimensionsUnit = "KG" }, "dimensions.unit"},
		"重量单位不在目录里":  {func(m *packagingMaterialJSON) { m.weightUnit = "STONE" }, "weight.unit"},
		"重量为零":       {func(m *packagingMaterialJSON) { m.weight = "0" }, "weight.value"},
		"重量为 0.0":    {func(m *packagingMaterialJSON) { m.weight = "0.0" }, "weight.value"},
		"长为零":        {func(m *packagingMaterialJSON) { m.length = "0" }, "dimensions.length"},
		"宽为零":        {func(m *packagingMaterialJSON) { m.width = "0.000000" }, "dimensions.width"},
		"高为零":        {func(m *packagingMaterialJSON) { m.height = "000" }, "dimensions.height"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newPackagingMaterialRegisterFixture(t)
			document := syntheticPackagingMaterial("SYN-BOX-01", 1)
			test.edit(&document)

			response := fixture.post(document.String())

			if response.Code != http.StatusOK {
				t.Fatalf("状态 %d %s，want 200（业务拒绝不是失败）", response.Code, response.Body.String())
			}
			object := decodeObject(t, response)
			assertKeys(t, name, object, "outcome", "refusal", "field")
			if stringMember(t, object, "outcome") != "NOT_ACCEPTED" || stringMember(t, object, "refusal") != "CONTENT_INVALID" ||
				stringMember(t, object, "field") != test.want {
				t.Fatalf("答复 = %s，want NOT_ACCEPTED/CONTENT_INVALID 指出 %s", response.Body.String(), test.want)
			}
			if fixture.packagingMaterials.writes != 0 {
				t.Fatalf("内容不合写了 %d 次册", fixture.packagingMaterials.writes)
			}
		})
	}
}

// Covers: AT-WF-009（包材的部分）——本租户一个仓都没登记时登记包材答 200 NOT_CONFIGURED，答复体恰是 {"outcome":"NOT_CONFIGURED"}：没有 refusal、field、
// latestRevision，也没有回显的包材；不落册；内容不合的报文在这时也只答未配置（内容门排在后面）。登记仓之后同一份报文原样重交答 201 REGISTERED，再原样重投
// 答 200 ALREADY_REGISTERED。别的租户登记了仓不算：本租户仍答 NOT_CONFIGURED。
func TestPackagingMaterialRegistrationBeforeAnyWarehouseAnswersNotConfiguredWithOnlyTheOutcome(t *testing.T) {
	fixture := newPackagingMaterialRegisterFixture(t)
	fixture.warehouses.registered = map[string]bool{tenantB: true}

	document := syntheticPackagingMaterial("SYN-BOX-01", 1).String()
	response := fixture.post(document)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("未配置：%d %s（%s），want 200 application/json", response.Code, response.Body.String(), response.Header().Get("Content-Type"))
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"outcome":"NOT_CONFIGURED"}` {
		t.Fatalf("未配置的答复体 = %s，want 恰是 {\"outcome\":\"NOT_CONFIGURED\"}", body)
	}
	assertKeys(t, "NOT_CONFIGURED", decodeObject(t, response), "outcome")

	invalid := syntheticPackagingMaterial("SYN-BOX-01", 1)
	invalid.weight = "0"
	if body := strings.TrimSpace(fixture.post(invalid.String()).Body.String()); body != `{"outcome":"NOT_CONFIGURED"}` {
		t.Fatalf("一个仓都没登记且内容不合：%s，want 仍是 NOT_CONFIGURED", body)
	}
	if fixture.packagingMaterials.writes != 0 {
		t.Fatalf("未配置写了 %d 次册，want 不落册", fixture.packagingMaterials.writes)
	}

	fixture.warehouses.registered[tenantA] = true
	if landed := fixture.post(document); landed.Code != http.StatusCreated {
		t.Fatalf("登记仓之后原样重交：%d %s，want 201", landed.Code, landed.Body.String())
	}
	if replay := fixture.post(document); replay.Code != http.StatusOK || stringMember(t, decodeObject(t, replay), "outcome") != "ALREADY_REGISTERED" {
		t.Fatalf("再原样重投：%d %s，want 200 ALREADY_REGISTERED", replay.Code, replay.Body.String())
	}
}

// Covers: AT-WF-009、AT-WF-001（包材的部分）判定次序——重放先于 NOT_CONFIGURED：册上早就有的修订，哪怕此刻本租户一个仓都问不到，原样重投仍答
// ALREADY_REGISTERED、换内容仍答 CONTENT_CONFLICT（换成数值为零的内容也是冲突，不是内容不合）；未落册的新修订才答 NOT_CONFIGURED。
func TestPackagingMaterialReplayAndConflictComeBeforeNotConfigured(t *testing.T) {
	fixture := newPackagingMaterialRegisterFixture(t)
	original := syntheticPackagingMaterial("SYN-BOX-01", 1)
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
	zeroWeight := original
	zeroWeight.weight = "0"
	if conflict := fixture.post(zeroWeight.String()); stringMember(t, decodeObject(t, conflict), "outcome") != "CONTENT_CONFLICT" {
		t.Fatalf("对在册的修订换成数值为零的内容：%s，want CONTENT_CONFLICT", conflict.Body.String())
	}
	if fresh := fixture.post(syntheticPackagingMaterial("SYN-BOX-02", 1).String()); stringMember(t, decodeObject(t, fresh), "outcome") != "NOT_CONFIGURED" {
		t.Fatalf("一个仓都问不到时登记新包材：%s，want NOT_CONFIGURED", fresh.Body.String())
	}
}

// Covers: AT-WF-004（包材的部分）——坏载荷答 400 MALFORMED_REQUEST（问题详情）：JSON 不合法、认不得的键（含自带租户或登记责任方）、必填格缺失或类型不对、
// 必填文本格是空白，以及数值写法的各种坏载荷（负号、正号、指数、千分位、只有小数点、小数点一边没有数字、超位数、空白、JSON 数字）与空白的单位。不读册、不写册、
// 不问仓；理由只点名格、不回显取值。
func TestMalformedPackagingMaterialPayloadsAreBadRequestsThatNeverTouchTheRegisters(t *testing.T) {
	valid := syntheticPackagingMaterial("SYN-BOX-01", 1)
	valid.name = secretAddress
	validJSON := valid.String()
	cases := map[string]string{
		"not JSON":                   `{`,
		"empty body":                 ``,
		"unknown key":                strings.Replace(validJSON, `{`, `{"colour":"brown",`, 1),
		"tenant carried":             strings.Replace(validJSON, `{`, `{"tenantId":"`+tenantB+`",`, 1),
		"tenant equal to real":       strings.Replace(validJSON, `{`, `{"tenantId":"`+tenantA+`",`, 1),
		"registrant carried":         strings.Replace(validJSON, `{`, `{"registrant":"`+operatorTwo+`",`, 1),
		"packagingMaterialCode gone": strings.Replace(validJSON, `"packagingMaterialCode":"SYN-BOX-01",`, ``, 1),
		"revision is string":         strings.Replace(validJSON, `"revision":1`, `"revision":"1"`, 1),
		"name blank":                 strings.Replace(validJSON, secretAddress, ` `, 1),
		"code blank":                 strings.Replace(validJSON, `SYN-BOX-01`, `  `, 1),
		"dimensions is string":       strings.Replace(validJSON, `"dimensions":{"height":"15.5","length":"30","unit":"CM","width":"20"}`, `"dimensions":"30x20x15.5"`, 1),
		"weight missing":             strings.Replace(validJSON, `,"weight":{"unit":"KG","value":"0.25"}`, ``, 1),
		"length missing":             strings.Replace(validJSON, `"length":"30",`, ``, 1),
		"dimension unit blank":       strings.Replace(validJSON, `"unit":"CM"`, `"unit":" "`, 1),
		"weight unit blank":          strings.Replace(validJSON, `"unit":"KG"`, `"unit":""`, 1),
		"length is a JSON number":    strings.Replace(validJSON, `"length":"30"`, `"length":30`, 1),
		"weight is a JSON number":    strings.Replace(validJSON, `"value":"0.25"`, `"value":0.25`, 1),
		"length with a minus":        strings.Replace(validJSON, `"length":"30"`, `"length":"-30"`, 1),
		"length with a plus":         strings.Replace(validJSON, `"length":"30"`, `"length":"+30"`, 1),
		"width with an exponent":     strings.Replace(validJSON, `"width":"20"`, `"width":"2e1"`, 1),
		"height with a thousands":    strings.Replace(validJSON, `"height":"15.5"`, `"height":"1,000"`, 1),
		"weight only a point":        strings.Replace(validJSON, `"value":"0.25"`, `"value":"."`, 1),
		"weight nothing after":       strings.Replace(validJSON, `"value":"0.25"`, `"value":"1."`, 1),
		"weight nothing before":      strings.Replace(validJSON, `"value":"0.25"`, `"value":".5"`, 1),
		"length too many integers":   strings.Replace(validJSON, `"length":"30"`, `"length":"1234567890"`, 1),
		"width too many fractions":   strings.Replace(validJSON, `"width":"20"`, `"width":"1.1234567"`, 1),
		"height with a space":        strings.Replace(validJSON, `"height":"15.5"`, `"height":" 15.5"`, 1),
		"deactivated is string":      strings.Replace(validJSON, `"deactivated":false`, `"deactivated":"no"`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if body == validJSON {
				t.Fatalf("用例 %q 没有改动报文", name)
			}
			fixture := newPackagingMaterialRegisterFixture(t)
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

// Covers: AT-WF-004（包材的部分）「detail 只点名格，不回显取值」——数值写法不对时，detail 点名那一格的键路径，不带取值本身。
func TestAMalformedPackagingMaterialNumberNamesTheCellAndNeverEchoesTheValue(t *testing.T) {
	for cell, build := range map[string]func(value string) packagingMaterialJSON{
		"dimensions.length": func(value string) packagingMaterialJSON {
			document := syntheticPackagingMaterial("SYN-BOX-01", 1)
			document.length = value
			return document
		},
		"dimensions.width": func(value string) packagingMaterialJSON {
			document := syntheticPackagingMaterial("SYN-BOX-01", 1)
			document.width = value
			return document
		},
		"dimensions.height": func(value string) packagingMaterialJSON {
			document := syntheticPackagingMaterial("SYN-BOX-01", 1)
			document.height = value
			return document
		},
		"weight.value": func(value string) packagingMaterialJSON {
			document := syntheticPackagingMaterial("SYN-BOX-01", 1)
			document.weight = value
			return document
		},
	} {
		for _, bad := range []string{"-9.87", "1e3", "9.87,5", "98765432101", "1.9876543"} {
			fixture := newPackagingMaterialRegisterFixture(t)
			response := fixture.post(build(bad).String())
			object := assertProblem(t, cell+" "+bad, response, http.StatusBadRequest, "MALFORMED_REQUEST")
			detail := stringMember(t, object, "detail")
			if !strings.Contains(detail, cell) || strings.Contains(response.Body.String(), bad) {
				t.Fatalf("%s 给 %q：detail = %q，want 点名 %s 且不回显取值", cell, bad, detail, cell)
			}
		}
	}
}

// Covers: AT-WF-010（包材的部分）——发行方未配置、令牌无效、没有登记册配置写授予，以及身份依赖读不动，各答其格（状态码与码照仓那一册与登记写面的先例），
// 不读册、不写册、不问仓；403 与 5xx 的问题详情不带 detail。
func TestPackagingMaterialEnvelopeGradesAnswerTheirOwnCellAndNeverTouchTheRegisters(t *testing.T) {
	for _, testCase := range envelopeCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newPackagingMaterialRegisterFixture(t)
			fixture.authenticator.err = testCase.err

			response := fixture.post(syntheticPackagingMaterial("SYN-BOX-01", 1).String())

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

// Covers: AT-WF-010（包材的部分）——先认证、后读报文：令牌不过的请求，请求体一个字节都不读；带着坏载荷的调用方看到的是 401，看不到报文校验的结果。
// 持授予的调用方才走到译装。
func TestPackagingMaterialAuthenticationComesBeforeTheBodyIsRead(t *testing.T) {
	fixture := newPackagingMaterialRegisterFixture(t)
	fixture.authenticator.err = warehousehttp.ErrOperatorCredentialRejected
	body := newCountingBody(`{not even JSON`)

	request := httptest.NewRequest(http.MethodPost, packagingMaterialRegistrationsPath, body)
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
	request = httptest.NewRequest(http.MethodPost, packagingMaterialRegistrationsPath, granted)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response = httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	assertProblem(t, "持授予的坏载荷", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if granted.reads == 0 {
		t.Fatal("持授予的请求没有读请求体")
	}
}

// Covers: AT-WF-010（包材的部分）——持授予照常登记，全程不判准入范围；租户与登记责任方是认证交回的那两个，不取自载荷。包材册与「有没有仓」都按租户隔离：
// 换一个没登记过仓的租户，同一份报文答 NOT_CONFIGURED。
func TestAGrantedOperatorRegistersAPackagingMaterialWithTheTenantAndRegistrantFromTheEnvelope(t *testing.T) {
	fixture := newPackagingMaterialRegisterFixture(t)
	fixture.authenticator.identity = operatorIdentity(t, tenantB, operatorTwo)

	// 租户 B 没登记过仓。
	if response := fixture.post(syntheticPackagingMaterial("SYN-BOX-01", 1).String()); stringMember(t, decodeObject(t, response), "outcome") != "NOT_CONFIGURED" {
		t.Fatalf("没登记过仓的租户登记：%s，want NOT_CONFIGURED", response.Body.String())
	}
	fixture.warehouses.registered[tenantB] = true
	response := fixture.post(syntheticPackagingMaterial("SYN-BOX-01", 1).String())
	if response.Code != http.StatusCreated {
		t.Fatalf("答复 %d %s", response.Code, response.Body.String())
	}
	if echo := packagingMaterialEchoOf(t, decodeObject(t, response)); echo.Registrant != operatorTwo {
		t.Fatalf("登记责任方 = %q，want 认证交回的 %q", echo.Registrant, operatorTwo)
	}
	// 同一份报文换租户 A 登记：包材册按租户隔离，从修订 1 起。
	fixture.authenticator.identity = operatorIdentity(t, tenantA, operatorOne)
	if response := fixture.post(syntheticPackagingMaterial("SYN-BOX-01", 1).String()); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记同编码的包材：%d %s，want 201", response.Code, response.Body.String())
	}
}

// Covers: ADR-0055 「未配置不改变方法约束」——方法不对由处理器自己答 405 并带 Allow，不认证、不读册。
func TestPackagingMaterialRegistrationRefusesOtherMethodsBeforeAuthentication(t *testing.T) {
	fixture := newPackagingMaterialRegisterFixture(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		request := httptest.NewRequest(method, packagingMaterialRegistrationsPath, strings.NewReader(syntheticPackagingMaterial("SYN-BOX-01", 1).String()))
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

// Covers: UC-WF-001「权限、隔离与审计」——包材册或仓册读写不动不形成答案：答 500 NO_ANSWER_FORMED，不折成未配置或拒绝（折成「未配置」会把登记方打发去登记一个
// 本来就在册的仓）；答复体与日志都不带名称，日志只带包材编码与修订号。
func TestPackagingMaterialRegistersThatCannotBeReachedFormNoAnswerAndLeakNoName(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	for name, arrange := range map[string]func(*packagingMaterialRegisterFixture){
		"packaging material register down": func(fixture *packagingMaterialRegisterFixture) {
			fixture.packagingMaterials.loadErr = errors.New("register unreachable, failing row contains " + secretAddress)
		},
		"warehouse register down": func(fixture *packagingMaterialRegisterFixture) {
			fixture.warehouses.err = errors.New("register unreachable, failing row contains " + secretAddress)
		},
	} {
		t.Run(name, func(t *testing.T) {
			logged.Reset()
			fixture := newPackagingMaterialRegisterFixture(t)
			arrange(fixture)
			material := syntheticPackagingMaterial("SYN-BOX-01", 1)
			material.name = secretAddress

			response := fixture.post(material.String())

			object := assertProblem(t, name, response, http.StatusInternalServerError, "NO_ANSWER_FORMED")
			if _, present := object["detail"]; present {
				t.Fatalf("5xx 的问题详情带了 detail：%s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), secretAddress) || strings.Contains(logged.String(), secretAddress) {
				t.Fatalf("名称外泄：答复 %s，日志 %s", response.Body.String(), logged.String())
			}
			if !strings.Contains(logged.String(), "SYN-BOX-01") {
				t.Fatalf("日志没有带包材编码：%q", logged.String())
			}
		})
	}
}

// Covers: ADR-0055 决定三——渠道未配置时 UnconfiguredIntake 对包材登记同答 403 ACCESS_CHANNEL_NOT_CONFIGURED：不读请求体、不碰册。
func TestTheUnconfiguredIntakeAnswersAPackagingMaterialRegistrationWithoutReadingAnything(t *testing.T) {
	packagingMaterials := newMemoryPackagingMaterialRegistry()
	register := warehousehttp.NewRegisterPackagingMaterialEndpoint(warehousehttp.UnconfiguredIntake{}, nil)

	body := newCountingBody(syntheticPackagingMaterial("SYN-BOX-01", 1).String())
	request := httptest.NewRequest(http.MethodPost, packagingMaterialRegistrationsPath, body)
	request.Header.Set("Authorization", "Bearer anything")
	response := httptest.NewRecorder()
	register.ServeHTTP(response, request)

	assertProblem(t, "登记", response, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	if body.reads != 0 || packagingMaterials.touched() != 0 {
		t.Fatalf("未配置仍读了请求体 %d 次、碰了册 %d 次", body.reads, packagingMaterials.touched())
	}
}
