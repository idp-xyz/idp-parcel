package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件的操作者、租户、仓与包材都是隔离合成值，只记 `S`。认证与装配沿用 assemble_warehouse_fulfillment_test.go：令牌 token-a 是租户 SYN-TENANT-01 的
// 操作者，token-b 是租户 SYN-TENANT-02 的操作者，其余一概走生产的铸造器、认证适配器、Intake、编排与真库。

// packagingMaterialSpec 是一份包材报文的各格；数值与单位都以 JSON 字符串给出。
type packagingMaterialSpec struct {
	code, name                    string
	revision                      int
	length, width, height, weight string
	dimensionsUnit, weightUnit    string
	deactivated                   bool
}

func standardPackagingMaterial(code string, revision int) packagingMaterialSpec {
	return packagingMaterialSpec{
		code: code, revision: revision, name: "合成一号纸箱",
		length: "30", width: "20", height: "15.5", dimensionsUnit: "CM", weight: "0.25", weightUnit: "KG",
	}
}

func (spec packagingMaterialSpec) document() string {
	return fmt.Sprintf(`{"packagingMaterialCode":%q,"revision":%d,"name":%q,`+
		`"dimensions":{"length":%q,"width":%q,"height":%q,"unit":%q},"weight":{"value":%q,"unit":%q},"deactivated":%t}`,
		spec.code, spec.revision, spec.name, spec.length, spec.width, spec.height, spec.dimensionsUnit, spec.weight, spec.weightUnit, spec.deactivated)
}

type packagingMaterialRegistrationAnswer struct {
	Outcome           string `json:"outcome"`
	Refusal           string `json:"refusal"`
	Field             string `json:"field"`
	LatestRevision    *int   `json:"latestRevision"`
	PackagingMaterial struct {
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
	} `json:"packagingMaterial"`
}

func decodePackagingMaterialAnswer(t *testing.T, response *httptest.ResponseRecorder) packagingMaterialRegistrationAnswer {
	t.Helper()
	var answer packagingMaterialRegistrationAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复体 %q：%v", response.Body.String(), err)
	}
	return answer
}

type packagingMaterialListedAnswer struct {
	Outcome            string `json:"outcome"`
	PackagingMaterials []struct {
		PackagingMaterialCode string `json:"packagingMaterialCode"`
		Revision              int    `json:"revision"`
		Name                  string `json:"name"`
		Dimensions            struct {
			Length string `json:"length"`
			Unit   string `json:"unit"`
		} `json:"dimensions"`
		Weight struct {
			Value string `json:"value"`
			Unit  string `json:"unit"`
		} `json:"weight"`
		Deactivated bool `json:"deactivated"`
	} `json:"packagingMaterials"`
	Page struct {
		Total *int64 `json:"total"`
	} `json:"page"`
}

func decodePackagingMaterialList(t *testing.T, response *httptest.ResponseRecorder) packagingMaterialListedAnswer {
	t.Helper()
	var answer packagingMaterialListedAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || response.Code != http.StatusOK {
		t.Fatalf("列表：%d %s（%v）", response.Code, response.Body.String(), err)
	}
	return answer
}

type packagingMaterialRevisionsListedAnswer struct {
	Outcome               string `json:"outcome"`
	PackagingMaterialCode string `json:"packagingMaterialCode"`
	Revisions             []struct {
		Revision   int    `json:"revision"`
		Name       string `json:"name"`
		Dimensions struct {
			Length string `json:"length"`
			Unit   string `json:"unit"`
		} `json:"dimensions"`
		Deactivated bool `json:"deactivated"`
	} `json:"revisions"`
}

// Covers: AT-WF-001、AT-WF-004、AT-WF-009、AT-WF-010（包材的部分）、UC-WF-001「权限、隔离与审计」——开关开着，经生产装配、真认证适配器、真编排与真库走通：
// 一个仓都没登记时登记包材答 200 NOT_CONFIGURED（答复体只有 outcome）、册里没有它，列表与修订史答空；登记仓之后同一笔原样重交答 201 REGISTERED（回显数值去掉多余的零，
// 字符串写法）、原样重投答 200 ALREADY_REGISTERED（回显当初那一笔）、换一种数值写法重投仍答 ALREADY_REGISTERED、同一修订号换内容答 CONTENT_CONFLICT、
// 不折算单位（30 CM 换成 0.3 M 的同一修订号答冲突，新修订号答 REGISTERED）、跳号答修订错位并回显在册最新修订号、停用与再启用各是一笔新修订；尺寸单位不在目录、重量给了
// 长度类的单位、尺寸给了重量类的单位、重量为零、一边为零各答 CONTENT_INVALID 并指出那一格，不落册；坏的数值写法答 400；列表与修订史读到它，按是否停用筛得出；
// 另一个租户读不到它，也可以在自己登记仓之后登记同编码的包材，互不相干。
func TestPackagingMaterialsAreRegisteredAndReadBackThroughTheProductionAssembly(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, true)
	const code = "SYN-BOX-01"
	registerPackagingMaterial := func(token, body string) *httptest.ResponseRecorder {
		return warehouseRequest(router, http.MethodPost, packagingMaterialRegistrationsLine, token, body)
	}
	registerWarehouse := func(token, body string) *httptest.ResponseRecorder {
		return warehouseRequest(router, http.MethodPost, warehouseRegistrationsLine, token, body)
	}
	original := standardPackagingMaterial(code, 1)

	// AT-WF-009：一个仓都没登记。
	notConfigured := registerPackagingMaterial("token-a", original.document())
	if notConfigured.Code != http.StatusOK || strings.TrimSpace(notConfigured.Body.String()) != `{"outcome":"NOT_CONFIGURED"}` {
		t.Fatalf("一个仓都没登记时登记包材：%d %s，want 200 且答复体恰是 {\"outcome\":\"NOT_CONFIGURED\"}", notConfigured.Code, notConfigured.Body.String())
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.packaging_material_revision`); got != 0 {
		t.Fatalf("未配置之后册里有 %d 笔包材修订，want 不落册", got)
	}
	emptyList := warehouseRequest(router, http.MethodGet, packagingMaterialListLine, "token-a", "")
	emptyHistory := warehouseRequest(router, http.MethodGet, packagingMaterialRevisionsLine+"?packagingMaterialCode="+code, "token-a", "")
	if decodePackagingMaterialList(t, emptyList).Outcome != "PACKAGING_MATERIALS_LISTED" || !strings.Contains(emptyList.Body.String(), `"packagingMaterials":[]`) ||
		!strings.Contains(emptyHistory.Body.String(), `"revisions":[]`) {
		t.Fatalf("空册的列表与修订史该是空数组：%s、%s", emptyList.Body.String(), emptyHistory.Body.String())
	}

	// 登记仓，再把同一笔原样重交；报文里数值的写法带多余的零，回显去掉它们。
	if response := registerWarehouse("token-a", warehouseDocument("SYN-WH-01", 1, "合成一号仓", "SELF_OPERATED")); response.Code != http.StatusCreated {
		t.Fatalf("登记仓：%d %s", response.Code, response.Body.String())
	}
	padded := original
	padded.length, padded.width, padded.height, padded.weight = "30.00", "020", "15.50", "0.250"
	first := registerPackagingMaterial("token-a", padded.document())
	if first.Code != http.StatusCreated {
		t.Fatalf("登记仓之后原样重交：%d %s，want 201", first.Code, first.Body.String())
	}
	registered := decodePackagingMaterialAnswer(t, first)
	echo := registered.PackagingMaterial
	if registered.Outcome != "REGISTERED" || echo.PackagingMaterialCode != code || echo.Revision != 1 || echo.Name != "合成一号纸箱" ||
		echo.Dimensions.Length != "30" || echo.Dimensions.Width != "20" || echo.Dimensions.Height != "15.5" || echo.Dimensions.Unit != "CM" ||
		echo.Weight.Value != "0.25" || echo.Weight.Unit != "KG" ||
		echo.Registrant != "https://id.syn.example/dex#SYN-OPERATOR-01" || echo.RegisteredAt == "" {
		t.Fatalf("首次登记的答复 = %s", first.Body.String())
	}

	replayed := registerPackagingMaterial("token-a", original.document())
	if replayed.Code != http.StatusOK {
		t.Fatalf("换一种数值写法重投：%d %s，want 200", replayed.Code, replayed.Body.String())
	}
	if answer := decodePackagingMaterialAnswer(t, replayed); answer.Outcome != "ALREADY_REGISTERED" || answer.PackagingMaterial.RegisteredAt != echo.RegisteredAt {
		t.Fatalf("换一种数值写法重投的答复 = %s，want ALREADY_REGISTERED 并回显当初那一笔", replayed.Body.String())
	}
	renamed := original
	renamed.name = "合成一号纸箱（改名）"
	if conflict := registerPackagingMaterial("token-a", renamed.document()); conflict.Code != http.StatusOK ||
		decodePackagingMaterialAnswer(t, conflict).Outcome != "CONTENT_CONFLICT" {
		t.Fatalf("同一修订号换内容：%d %s，want 200 CONTENT_CONFLICT", conflict.Code, conflict.Body.String())
	}
	meters := original
	meters.length, meters.width, meters.height, meters.dimensionsUnit = "0.3", "0.2", "0.155", "M"
	if conflict := registerPackagingMaterial("token-a", meters.document()); conflict.Code != http.StatusOK ||
		decodePackagingMaterialAnswer(t, conflict).Outcome != "CONTENT_CONFLICT" {
		t.Fatalf("30 CM 换成 0.3 M 的同一修订号：%d %s，want 200 CONTENT_CONFLICT（不折算单位）", conflict.Code, conflict.Body.String())
	}
	if skipped := registerPackagingMaterial("token-a", standardPackagingMaterial(code, 3).document()); skipped.Code != http.StatusOK {
		t.Fatalf("跳号：%d %s", skipped.Code, skipped.Body.String())
	} else if answer := decodePackagingMaterialAnswer(t, skipped); answer.Refusal != "REVISION_OUT_OF_SEQUENCE" || answer.LatestRevision == nil || *answer.LatestRevision != 1 {
		t.Fatalf("跳号的答复 = %s，want 回显在册最新修订号 1", skipped.Body.String())
	}

	// AT-WF-004：内容不合各指出那一格，不落册。
	for name, test := range map[string]struct {
		edit func(*packagingMaterialSpec)
		want string
	}{
		"尺寸单位不在目录里":  {func(spec *packagingMaterialSpec) { spec.dimensionsUnit = "CUBIT" }, "dimensions.unit"},
		"重量给了长度类的单位": {func(spec *packagingMaterialSpec) { spec.weightUnit = "CM" }, "weight.unit"},
		"尺寸给了重量类的单位": {func(spec *packagingMaterialSpec) { spec.dimensionsUnit = "KG" }, "dimensions.unit"},
		"重量为零":       {func(spec *packagingMaterialSpec) { spec.weight = "0.0" }, "weight.value"},
		"长为零":        {func(spec *packagingMaterialSpec) { spec.length = "0" }, "dimensions.length"},
		"宽为零":        {func(spec *packagingMaterialSpec) { spec.width = "000" }, "dimensions.width"},
		"高为零":        {func(spec *packagingMaterialSpec) { spec.height = "0.000000" }, "dimensions.height"},
	} {
		invalid := standardPackagingMaterial(code, 2)
		test.edit(&invalid)
		response := registerPackagingMaterial("token-a", invalid.document())
		answer := decodePackagingMaterialAnswer(t, response)
		if response.Code != http.StatusOK || answer.Outcome != "NOT_ACCEPTED" || answer.Refusal != "CONTENT_INVALID" || answer.Field != test.want {
			t.Fatalf("%s：%d %s，want 200 NOT_ACCEPTED/CONTENT_INVALID 指出 %s", name, response.Code, response.Body.String(), test.want)
		}
	}
	// 数值写法不对是坏载荷。
	for name, body := range map[string]string{
		"负号":           strings.Replace(standardPackagingMaterial(code, 2).document(), `"length":"30"`, `"length":"-30"`, 1),
		"指数":           strings.Replace(standardPackagingMaterial(code, 2).document(), `"width":"20"`, `"width":"2e1"`, 1),
		"千分位":          strings.Replace(standardPackagingMaterial(code, 2).document(), `"height":"15.5"`, `"height":"1,000"`, 1),
		"超位数":          strings.Replace(standardPackagingMaterial(code, 2).document(), `"value":"0.25"`, `"value":"1.1234567"`, 1),
		"JSON 数字":      strings.Replace(standardPackagingMaterial(code, 2).document(), `"length":"30"`, `"length":30`, 1),
		"自带租户":         strings.Replace(standardPackagingMaterial(code, 2).document(), `{`, `{"tenantId":"SYN-TENANT-02",`, 1),
		"自带登记责任方":      strings.Replace(standardPackagingMaterial(code, 2).document(), `{`, `{"registrant":"someone",`, 1),
		"单位是空白":        strings.Replace(standardPackagingMaterial(code, 2).document(), `"unit":"KG"`, `"unit":" "`, 1),
		"尺寸给成 JSON 数字": strings.Replace(standardPackagingMaterial(code, 2).document(), `"width":"20"`, `"width":20.5`, 1),
	} {
		if response := registerPackagingMaterial("token-a", body); response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s：%d %s，want 400 MALFORMED_REQUEST", name, response.Code, response.Body.String())
		}
	}

	// 改尺寸与重量（修订 2，单位换成米与克，各自原样不折算）、停用（修订 3）、再启用（修订 4）。
	changed := original
	changed.revision, changed.length, changed.width, changed.height, changed.dimensionsUnit = 2, "0.3", "0.2", "0.155", "M"
	changed.weight, changed.weightUnit = "250", "G"
	changed.name = "合成一号纸箱（新名）"
	secondRevision := registerPackagingMaterial("token-a", changed.document())
	if secondRevision.Code != http.StatusCreated {
		t.Fatalf("修订 2：%d %s，want 201", secondRevision.Code, secondRevision.Body.String())
	}
	if answer := decodePackagingMaterialAnswer(t, secondRevision); answer.PackagingMaterial.Dimensions.Length != "0.3" || answer.PackagingMaterial.Dimensions.Unit != "M" ||
		answer.PackagingMaterial.Weight.Value != "250" || answer.PackagingMaterial.Weight.Unit != "G" {
		t.Fatalf("修订 2 的回显 = %s，want 原样 0.3 M 与 250 G，不折算", secondRevision.Body.String())
	}
	changed.revision, changed.deactivated = 3, true
	if response := registerPackagingMaterial("token-a", changed.document()); response.Code != http.StatusCreated {
		t.Fatalf("停用（修订 3）：%d %s，want 201", response.Code, response.Body.String())
	}
	changed.revision, changed.deactivated = 4, false
	if response := registerPackagingMaterial("token-a", changed.document()); response.Code != http.StatusCreated {
		t.Fatalf("再启用（修订 4）：%d %s，want 201", response.Code, response.Body.String())
	}
	// 包材编码逐字保存：带首尾空白与大小写混写的编码是另一个包材，且停用的包材照样登记。
	const verbatim = " Syn-Box 09 "
	deactivatedBag := standardPackagingMaterial(verbatim, 1)
	deactivatedBag.deactivated = true
	if response := registerPackagingMaterial("token-a", deactivatedBag.document()); response.Code != http.StatusCreated {
		t.Fatalf("逐字编码：%d %s", response.Code, response.Body.String())
	}

	list := decodePackagingMaterialList(t, warehouseRequest(router, http.MethodGet, packagingMaterialListLine, "token-a", ""))
	if list.Outcome != "PACKAGING_MATERIALS_LISTED" || len(list.PackagingMaterials) != 2 || list.Page.Total == nil || *list.Page.Total != 2 {
		t.Fatalf("列表 = %+v，want 两个包材（各自的当前修订）", list)
	}
	byCode := map[string]int{}
	for _, row := range list.PackagingMaterials {
		byCode[row.PackagingMaterialCode] = row.Revision
		if row.PackagingMaterialCode == code && (row.Name != "合成一号纸箱（新名）" || row.Dimensions.Length != "0.3" || row.Dimensions.Unit != "M" || row.Weight.Value != "250" || row.Weight.Unit != "G" || row.Deactivated) {
			t.Fatalf("列表里 %s 的当前修订 = %+v，want 修订 4 的各格（再启用之后未停用）", code, row)
		}
	}
	if byCode[code] != 4 || byCode[verbatim] != 1 {
		t.Fatalf("列表读到的当前修订 = %v，want %s 为修订 4、%q 逐字为修订 1", byCode, code, verbatim)
	}
	deactivated := decodePackagingMaterialList(t, warehouseRequest(router, http.MethodGet, packagingMaterialListLine+"?deactivated=true", "token-a", ""))
	if len(deactivated.PackagingMaterials) != 1 || deactivated.PackagingMaterials[0].PackagingMaterialCode != verbatim {
		t.Fatalf("按已停用筛 = %+v，want 只有逐字编码的那一个（%s 再启用之后回到未停用）", deactivated, code)
	}

	history := warehouseRequest(router, http.MethodGet, packagingMaterialRevisionsLine+"?packagingMaterialCode="+code, "token-a", "")
	var revisions packagingMaterialRevisionsListedAnswer
	if err := json.Unmarshal(history.Body.Bytes(), &revisions); err != nil || history.Code != http.StatusOK {
		t.Fatalf("修订史：%d %s（%v）", history.Code, history.Body.String(), err)
	}
	if revisions.Outcome != "PACKAGING_MATERIAL_REVISIONS_LISTED" || revisions.PackagingMaterialCode != code || len(revisions.Revisions) != 4 ||
		revisions.Revisions[0].Revision != 1 || revisions.Revisions[0].Dimensions.Length != "30" || revisions.Revisions[0].Dimensions.Unit != "CM" ||
		revisions.Revisions[1].Dimensions.Unit != "M" || !revisions.Revisions[2].Deactivated || revisions.Revisions[3].Deactivated {
		t.Fatalf("修订史 = %s，want 修订 1 至 4 按序，旧修订照留、修订 3 停用、修订 4 再启用", history.Body.String())
	}
	verbatimHistory := warehouseRequest(router, http.MethodGet, packagingMaterialRevisionsLine+"?packagingMaterialCode="+url.QueryEscape(verbatim), "token-a", "")
	var oddRevisions packagingMaterialRevisionsListedAnswer
	if err := json.Unmarshal(verbatimHistory.Body.Bytes(), &oddRevisions); err != nil || oddRevisions.PackagingMaterialCode != verbatim || len(oddRevisions.Revisions) != 1 {
		t.Fatalf("逐字包材编码的修订史 = %s", verbatimHistory.Body.String())
	}

	// 另一个租户读不到它：列表空，修订史是空数组，与从没登记过的包材同答；它没登记仓，登记包材答未配置。
	otherList := warehouseRequest(router, http.MethodGet, packagingMaterialListLine, "token-b", "")
	if otherList.Code != http.StatusOK || !strings.Contains(otherList.Body.String(), `"packagingMaterials":[]`) {
		t.Fatalf("另一个租户的列表：%d %s，want 200 与空数组", otherList.Code, otherList.Body.String())
	}
	otherHistory := warehouseRequest(router, http.MethodGet, packagingMaterialRevisionsLine+"?packagingMaterialCode="+code, "token-b", "")
	neverRegistered := warehouseRequest(router, http.MethodGet, packagingMaterialRevisionsLine+"?packagingMaterialCode=SYN-BOX-NEVER", "token-b", "")
	if want := strings.ReplaceAll(neverRegistered.Body.String(), "SYN-BOX-NEVER", code); otherHistory.Code != http.StatusOK || otherHistory.Body.String() != want {
		t.Fatalf("别的租户的同编码包材答 %d %s，与从没登记过的包材答 %d %s 不同：泄露了是否存在",
			otherHistory.Code, otherHistory.Body.String(), neverRegistered.Code, neverRegistered.Body.String())
	}
	if foreign := registerPackagingMaterial("token-b", original.document()); decodePackagingMaterialAnswer(t, foreign).Outcome != "NOT_CONFIGURED" {
		t.Fatalf("另一个租户没登记仓时登记包材：%s，want NOT_CONFIGURED（本租户登记了仓不算它的）", foreign.Body.String())
	}
	// 另一个租户登记了自己的仓之后，登记同编码的包材从修订 1 起，互不相干；A 的当前修订不变。
	if response := registerWarehouse("token-b", warehouseDocument("SYN-WH-01", 1, "另一租户的仓", "AGENT")); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记仓：%d %s", response.Code, response.Body.String())
	}
	otherMaterial := original
	otherMaterial.name = "另一租户的纸箱"
	if response := registerPackagingMaterial("token-b", otherMaterial.document()); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记同编码的包材：%d %s，want 201", response.Code, response.Body.String())
	}
	again := decodePackagingMaterialList(t, warehouseRequest(router, http.MethodGet, packagingMaterialListLine, "token-a", ""))
	for _, row := range again.PackagingMaterials {
		if row.PackagingMaterialCode == code && (row.Revision != 4 || row.Name != "合成一号纸箱（新名）") {
			t.Fatalf("另一个租户登记后，A 的包材 = %+v，want 不变", row)
		}
	}

	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.packaging_material_revision WHERE tenant_id = $1`, "SYN-TENANT-01"); got != 5 {
		t.Fatalf("SYN-TENANT-01 在册 %d 笔包材修订，want 5（%s 四笔、逐字编码一笔；未配置、重放、冲突、跳号、内容不合与坏载荷都不落册）", got, code)
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.packaging_material_revision WHERE tenant_id = $1`, "SYN-TENANT-02"); got != 1 {
		t.Fatalf("SYN-TENANT-02 在册 %d 笔包材修订，want 1", got)
	}
}

// Covers: AT-WF-009（库位类型与包材的部分）、AT-WF-001、AT-WF-004——开关开着，经生产装配、真认证适配器、真编排与真库走通同一个租户的「从没登记到登记」：
// 一个仓都没登记时，登记库位类型与登记包材都答 200 NOT_CONFIGURED、册里都没有它们，两种查阅都答空；登记一个仓；把同一笔库位类型与同一笔包材原样重交都答 201 REGISTERED；
// 再原样重投都答 200 ALREADY_REGISTERED；包材换一种数值写法重投仍答 ALREADY_REGISTERED；列表与修订史读到它们；另一个租户读不到。
func TestLocationTypesAndPackagingMaterialsWaitForTheFirstWarehouseThenLandOnResubmission(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, true)
	locationType := locationTypeDocument("SYN-LT-01", 1, "合成货架位", false)
	material := standardPackagingMaterial("SYN-BOX-01", 1)
	rewritten := material
	rewritten.length, rewritten.width, rewritten.height, rewritten.weight = "030.000", "20.0", "015.50", "0.2500"

	postLocationType := func(token, body string) *httptest.ResponseRecorder {
		return warehouseRequest(router, http.MethodPost, locationTypeRegistrationsLine, token, body)
	}
	postMaterial := func(token, body string) *httptest.ResponseRecorder {
		return warehouseRequest(router, http.MethodPost, packagingMaterialRegistrationsLine, token, body)
	}
	expectOutcome := func(label string, response *httptest.ResponseRecorder, status int, outcome string) {
		t.Helper()
		var answer struct {
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || response.Code != status || answer.Outcome != outcome {
			t.Fatalf("%s：%d %s，want %d %s", label, response.Code, response.Body.String(), status, outcome)
		}
	}

	// 一个仓都没登记。
	expectOutcome("没有仓时登记库位类型", postLocationType("token-a", locationType), http.StatusOK, "NOT_CONFIGURED")
	expectOutcome("没有仓时登记包材", postMaterial("token-a", material.document()), http.StatusOK, "NOT_CONFIGURED")
	if got := countOf(t, pool, `SELECT (SELECT count(*) FROM warehouse_fulfillment.location_type_revision)
		+ (SELECT count(*) FROM warehouse_fulfillment.packaging_material_revision)`); got != 0 {
		t.Fatalf("未配置之后两册里共有 %d 笔修订，want 不落册", got)
	}
	for name, target := range map[string]string{"库位类型": locationTypeListLine, "包材": packagingMaterialListLine} {
		if response := warehouseRequest(router, http.MethodGet, target, "token-a", ""); response.Code != http.StatusOK ||
			!strings.Contains(response.Body.String(), `:[]`) || strings.Contains(response.Body.String(), "NOT_CONFIGURED") {
			t.Fatalf("没有仓时查阅%s：%d %s，want 200 与空数组、不答未配置", name, response.Code, response.Body.String())
		}
	}

	// 登记一个仓；把同一笔原样重交。
	if response := warehouseRequest(router, http.MethodPost, warehouseRegistrationsLine, "token-a", warehouseDocument("SYN-WH-01", 1, "合成一号仓", "SELF_OPERATED")); response.Code != http.StatusCreated {
		t.Fatalf("登记仓：%d %s", response.Code, response.Body.String())
	}
	expectOutcome("登记仓之后重交库位类型", postLocationType("token-a", locationType), http.StatusCreated, "REGISTERED")
	expectOutcome("登记仓之后重交包材", postMaterial("token-a", material.document()), http.StatusCreated, "REGISTERED")
	expectOutcome("再原样重投库位类型", postLocationType("token-a", locationType), http.StatusOK, "ALREADY_REGISTERED")
	expectOutcome("再原样重投包材", postMaterial("token-a", material.document()), http.StatusOK, "ALREADY_REGISTERED")
	expectOutcome("包材换一种数值写法重投", postMaterial("token-a", rewritten.document()), http.StatusOK, "ALREADY_REGISTERED")

	locationTypes := decodeLocationTypeList(t, warehouseRequest(router, http.MethodGet, locationTypeListLine, "token-a", ""))
	materials := decodePackagingMaterialList(t, warehouseRequest(router, http.MethodGet, packagingMaterialListLine, "token-a", ""))
	if len(locationTypes.LocationTypes) != 1 || locationTypes.LocationTypes[0].LocationTypeCode != "SYN-LT-01" ||
		len(materials.PackagingMaterials) != 1 || materials.PackagingMaterials[0].PackagingMaterialCode != "SYN-BOX-01" ||
		materials.PackagingMaterials[0].Dimensions.Length != "30" || materials.PackagingMaterials[0].Revision != 1 {
		t.Fatalf("列表 = %+v、%+v，want 各一行", locationTypes, materials)
	}
	for name, target := range map[string]string{
		"库位类型修订史": locationTypeRevisionsLine + "?locationTypeCode=SYN-LT-01",
		"包材修订史":   packagingMaterialRevisionsLine + "?packagingMaterialCode=SYN-BOX-01",
	} {
		response := warehouseRequest(router, http.MethodGet, target, "token-a", "")
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"revisions":[]`) || !strings.Contains(response.Body.String(), `"revision":1`) {
			t.Fatalf("%s：%d %s，want 读到修订 1", name, response.Code, response.Body.String())
		}
	}
	if got := countOf(t, pool, `SELECT (SELECT count(*) FROM warehouse_fulfillment.location_type_revision)
		+ (SELECT count(*) FROM warehouse_fulfillment.packaging_material_revision)`); got != 2 {
		t.Fatalf("两册里共有 %d 笔修订，want 2（各一笔；重放与换写法的重放都不落册）", got)
	}

	// 另一个租户读不到它们。
	for name, read := range map[string]struct{ target, empty string }{
		"库位类型":    {locationTypeListLine, `"locationTypes":[]`},
		"包材":      {packagingMaterialListLine, `"packagingMaterials":[]`},
		"库位类型修订史": {locationTypeRevisionsLine + "?locationTypeCode=SYN-LT-01", `"revisions":[]`},
		"包材修订史":   {packagingMaterialRevisionsLine + "?packagingMaterialCode=SYN-BOX-01", `"revisions":[]`},
	} {
		response := warehouseRequest(router, http.MethodGet, read.target, "token-b", "")
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), read.empty) {
			t.Fatalf("另一个租户读%s：%d %s，want 200 与空数组（读不到）", name, response.Code, response.Body.String())
		}
	}
}

// Covers: AT-WF-012（端点那一半）——开关关着时，同样的真库与真渠道（授予齐备、令牌有效）打包材的各口，答 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED，包材册里一行也没落：
// 登记口没有走到任何编排，库表（空表）原样。
func TestSwitchedOffPackagingMaterialsNeverReachTheRegisterOnARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, false)

	registration := warehouseRequest(router, http.MethodPost, packagingMaterialRegistrationsLine, "token-a", standardPackagingMaterial("SYN-BOX-01", 1).document())
	list := warehouseRequest(router, http.MethodGet, packagingMaterialListLine, "token-a", "")
	history := warehouseRequest(router, http.MethodGet, packagingMaterialRevisionsLine+"?packagingMaterialCode=SYN-BOX-01", "token-a", "")
	for name, response := range map[string]*httptest.ResponseRecorder{"registration": registration, "list": list, "history": history} {
		if response.Code != http.StatusForbidden || problemCode(t, response) != "WAREHOUSE_FULFILLMENT_NOT_ENABLED" {
			t.Fatalf("%s：答 %d %s，want 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED", name, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response, name)
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.packaging_material_revision`); got != 0 {
		t.Fatalf("开关关着时包材册里有 %d 笔修订，want 0", got)
	}
}
