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

// 本文件的操作者、租户、仓与库位类型都是隔离合成值，只记 `S`。认证与装配沿用 assemble_warehouse_fulfillment_test.go：令牌 token-a 是
// 租户 SYN-TENANT-01 的操作者，token-b 是租户 SYN-TENANT-02 的操作者，其余一概走生产的铸造器、认证适配器、Intake、编排与真库。

func locationTypeDocument(code string, revision int, name string, deactivated bool) string {
	return fmt.Sprintf(`{"locationTypeCode":%q,"revision":%d,"name":%q,"deactivated":%t}`, code, revision, name, deactivated)
}

type locationTypeRegistrationAnswer struct {
	Outcome        string `json:"outcome"`
	Refusal        string `json:"refusal"`
	Field          string `json:"field"`
	LatestRevision *int   `json:"latestRevision"`
	LocationType   struct {
		LocationTypeCode string `json:"locationTypeCode"`
		Revision         int    `json:"revision"`
		Name             string `json:"name"`
		Deactivated      bool   `json:"deactivated"`
		Registrant       string `json:"registrant"`
		RegisteredAt     string `json:"registeredAt"`
	} `json:"locationType"`
}

func decodeLocationTypeAnswer(t *testing.T, response *httptest.ResponseRecorder) locationTypeRegistrationAnswer {
	t.Helper()
	var answer locationTypeRegistrationAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复体 %q：%v", response.Body.String(), err)
	}
	return answer
}

type locationTypeListedAnswer struct {
	Outcome       string `json:"outcome"`
	LocationTypes []struct {
		LocationTypeCode string `json:"locationTypeCode"`
		Revision         int    `json:"revision"`
		Name             string `json:"name"`
		Deactivated      bool   `json:"deactivated"`
	} `json:"locationTypes"`
	Page struct {
		Total *int64 `json:"total"`
	} `json:"page"`
}

func decodeLocationTypeList(t *testing.T, response *httptest.ResponseRecorder) locationTypeListedAnswer {
	t.Helper()
	var answer locationTypeListedAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || response.Code != http.StatusOK {
		t.Fatalf("列表：%d %s（%v）", response.Code, response.Body.String(), err)
	}
	return answer
}

type locationTypeRevisionsListedAnswer struct {
	Outcome          string `json:"outcome"`
	LocationTypeCode string `json:"locationTypeCode"`
	Revisions        []struct {
		Revision    int    `json:"revision"`
		Name        string `json:"name"`
		Deactivated bool   `json:"deactivated"`
	} `json:"revisions"`
}

// Covers: AT-WF-001、AT-WF-009、AT-WF-010（库位类型的部分）、UC-WF-001「权限、隔离与审计」——开关开着，经生产装配、真认证适配器、真编排与真库走通：
// 一个仓都没登记时登记库位类型答 200 NOT_CONFIGURED（答复体只有 outcome）、册里没有它，列表与修订史答空；别的租户登记了仓不算；登记仓之后同一笔
// 原样重交答 201 REGISTERED、原样重投答 200 ALREADY_REGISTERED（回显当初那一笔）、同一修订号换内容答 CONTENT_CONFLICT、跳号答修订错位并回显
// 在册最新修订号、改名为修订 2、停用与再启用各是一笔新修订；列表与修订史读到它，按是否停用筛得出；另一个租户读不到它，也可以在自己登记仓之后登记同编码的
// 库位类型，互不相干。
func TestLocationTypesAreRegisteredAndReadBackThroughTheProductionAssembly(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, true)
	const code = "SYN-LT-01"
	registerLocationType := func(token, body string) *httptest.ResponseRecorder {
		return warehouseRequest(router, http.MethodPost, locationTypeRegistrationsLine, token, body)
	}
	registerWarehouse := func(token, body string) *httptest.ResponseRecorder {
		return warehouseRequest(router, http.MethodPost, warehouseRegistrationsLine, token, body)
	}

	// AT-WF-009：一个仓都没登记。
	notConfigured := registerLocationType("token-a", locationTypeDocument(code, 1, "合成货架位", false))
	if notConfigured.Code != http.StatusOK || strings.TrimSpace(notConfigured.Body.String()) != `{"outcome":"NOT_CONFIGURED"}` {
		t.Fatalf("一个仓都没登记时登记库位类型：%d %s，want 200 且答复体恰是 {\"outcome\":\"NOT_CONFIGURED\"}", notConfigured.Code, notConfigured.Body.String())
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.location_type_revision`); got != 0 {
		t.Fatalf("未配置之后册里有 %d 笔库位类型修订，want 不落册", got)
	}
	emptyList := warehouseRequest(router, http.MethodGet, locationTypeListLine, "token-a", "")
	emptyHistory := warehouseRequest(router, http.MethodGet, locationTypeRevisionsLine+"?locationTypeCode="+code, "token-a", "")
	if decodeLocationTypeList(t, emptyList).Outcome != "LOCATION_TYPES_LISTED" || !strings.Contains(emptyList.Body.String(), `"locationTypes":[]`) ||
		!strings.Contains(emptyHistory.Body.String(), `"revisions":[]`) {
		t.Fatalf("空册的列表与修订史该是空数组：%s、%s", emptyList.Body.String(), emptyHistory.Body.String())
	}

	// 别的租户登记了仓，不算本租户的仓。
	if response := registerWarehouse("token-b", warehouseDocument("SYN-WH-01", 1, "另一租户的仓", "AGENT")); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记仓：%d %s", response.Code, response.Body.String())
	}
	if stillNotConfigured := registerLocationType("token-a", locationTypeDocument(code, 1, "合成货架位", false)); decodeLocationTypeAnswer(t, stillNotConfigured).Outcome != "NOT_CONFIGURED" {
		t.Fatalf("只有别的租户登记了仓：%s，want NOT_CONFIGURED", stillNotConfigured.Body.String())
	}

	// 登记仓，再把同一笔原样重交。
	if response := registerWarehouse("token-a", warehouseDocument("SYN-WH-01", 1, "合成一号仓", "SELF_OPERATED")); response.Code != http.StatusCreated {
		t.Fatalf("登记仓：%d %s", response.Code, response.Body.String())
	}
	first := registerLocationType("token-a", locationTypeDocument(code, 1, "合成货架位", false))
	if first.Code != http.StatusCreated {
		t.Fatalf("登记仓之后原样重交：%d %s，want 201", first.Code, first.Body.String())
	}
	registered := decodeLocationTypeAnswer(t, first)
	if registered.Outcome != "REGISTERED" || registered.LocationType.LocationTypeCode != code || registered.LocationType.Revision != 1 ||
		registered.LocationType.Registrant != "https://id.syn.example/dex#SYN-OPERATOR-01" || registered.LocationType.RegisteredAt == "" {
		t.Fatalf("首次登记的答复 = %s", first.Body.String())
	}

	replayed := registerLocationType("token-a", locationTypeDocument(code, 1, "合成货架位", false))
	if replayed.Code != http.StatusOK {
		t.Fatalf("原样重投：%d %s，want 200", replayed.Code, replayed.Body.String())
	}
	if answer := decodeLocationTypeAnswer(t, replayed); answer.Outcome != "ALREADY_REGISTERED" || answer.LocationType.RegisteredAt != registered.LocationType.RegisteredAt {
		t.Fatalf("原样重投的答复 = %s，want ALREADY_REGISTERED 并回显当初那一笔", replayed.Body.String())
	}
	if conflict := registerLocationType("token-a", locationTypeDocument(code, 1, "合成货架位（改名）", false)); conflict.Code != http.StatusOK ||
		decodeLocationTypeAnswer(t, conflict).Outcome != "CONTENT_CONFLICT" {
		t.Fatalf("同一修订号换内容：%d %s，want 200 CONTENT_CONFLICT", conflict.Code, conflict.Body.String())
	}
	if skipped := registerLocationType("token-a", locationTypeDocument(code, 3, "合成货架位", false)); skipped.Code != http.StatusOK {
		t.Fatalf("跳号：%d %s", skipped.Code, skipped.Body.String())
	} else if answer := decodeLocationTypeAnswer(t, skipped); answer.Refusal != "REVISION_OUT_OF_SEQUENCE" || answer.LatestRevision == nil || *answer.LatestRevision != 1 {
		t.Fatalf("跳号的答复 = %s，want 回显在册最新修订号 1", skipped.Body.String())
	}
	for _, step := range []struct {
		label    string
		document string
	}{
		{"改名（修订 2）", locationTypeDocument(code, 2, "合成货架位（新名）", false)},
		{"停用（修订 3）", locationTypeDocument(code, 3, "合成货架位（新名）", true)},
		{"再启用（修订 4）", locationTypeDocument(code, 4, "合成货架位（新名）", false)},
	} {
		if response := registerLocationType("token-a", step.document); response.Code != http.StatusCreated {
			t.Fatalf("%s：%d %s，want 201", step.label, response.Code, response.Body.String())
		}
	}
	// 同一个库位类型编码逐字保存：带首尾空白与大小写混写的编码是另一个库位类型，且停用的库位类型照样登记。
	const verbatim = " Syn-LT 09 "
	if response := registerLocationType("token-a", locationTypeDocument(verbatim, 1, "逐字编码的库位类型", true)); response.Code != http.StatusCreated {
		t.Fatalf("逐字编码：%d %s", response.Code, response.Body.String())
	}

	list := decodeLocationTypeList(t, warehouseRequest(router, http.MethodGet, locationTypeListLine, "token-a", ""))
	if list.Outcome != "LOCATION_TYPES_LISTED" || len(list.LocationTypes) != 2 || list.Page.Total == nil || *list.Page.Total != 2 {
		t.Fatalf("列表 = %+v，want 两个库位类型（各自的当前修订）", list)
	}
	byCode := map[string]int{}
	names := map[string]string{}
	for _, row := range list.LocationTypes {
		byCode[row.LocationTypeCode] = row.Revision
		names[row.LocationTypeCode] = row.Name
	}
	if byCode[code] != 4 || names[code] != "合成货架位（新名）" || byCode[verbatim] != 1 {
		t.Fatalf("列表读到的当前修订 = %v %v，want %s 为修订 4、%q 逐字为修订 1", byCode, names, code, verbatim)
	}
	deactivated := decodeLocationTypeList(t, warehouseRequest(router, http.MethodGet, locationTypeListLine+"?deactivated=true", "token-a", ""))
	if len(deactivated.LocationTypes) != 1 || deactivated.LocationTypes[0].LocationTypeCode != verbatim {
		t.Fatalf("按已停用筛 = %+v，want 只有逐字编码的那一个（%s 再启用之后回到未停用）", deactivated, code)
	}

	history := warehouseRequest(router, http.MethodGet, locationTypeRevisionsLine+"?locationTypeCode="+code, "token-a", "")
	var revisions locationTypeRevisionsListedAnswer
	if err := json.Unmarshal(history.Body.Bytes(), &revisions); err != nil || history.Code != http.StatusOK {
		t.Fatalf("修订史：%d %s（%v）", history.Code, history.Body.String(), err)
	}
	if revisions.Outcome != "LOCATION_TYPE_REVISIONS_LISTED" || revisions.LocationTypeCode != code || len(revisions.Revisions) != 4 ||
		revisions.Revisions[0].Revision != 1 || revisions.Revisions[0].Name != "合成货架位" ||
		revisions.Revisions[2].Revision != 3 || !revisions.Revisions[2].Deactivated || revisions.Revisions[3].Deactivated {
		t.Fatalf("修订史 = %s，want 修订 1 至 4 按序，修订 3 停用、修订 4 再启用", history.Body.String())
	}
	verbatimHistory := warehouseRequest(router, http.MethodGet, locationTypeRevisionsLine+"?locationTypeCode="+url.QueryEscape(verbatim), "token-a", "")
	var oddRevisions locationTypeRevisionsListedAnswer
	if err := json.Unmarshal(verbatimHistory.Body.Bytes(), &oddRevisions); err != nil || oddRevisions.LocationTypeCode != verbatim || len(oddRevisions.Revisions) != 1 {
		t.Fatalf("逐字库位类型编码的修订史 = %s", verbatimHistory.Body.String())
	}

	// 另一个租户读不到它：列表空，修订史是空数组，与从没登记过的库位类型同答。
	otherList := warehouseRequest(router, http.MethodGet, locationTypeListLine, "token-b", "")
	if otherList.Code != http.StatusOK || !strings.Contains(otherList.Body.String(), `"locationTypes":[]`) {
		t.Fatalf("另一个租户的列表：%d %s，want 200 与空数组", otherList.Code, otherList.Body.String())
	}
	otherHistory := warehouseRequest(router, http.MethodGet, locationTypeRevisionsLine+"?locationTypeCode="+code, "token-b", "")
	neverRegistered := warehouseRequest(router, http.MethodGet, locationTypeRevisionsLine+"?locationTypeCode=SYN-LT-NEVER", "token-b", "")
	if want := strings.ReplaceAll(neverRegistered.Body.String(), "SYN-LT-NEVER", code); otherHistory.Code != http.StatusOK || otherHistory.Body.String() != want {
		t.Fatalf("别的租户的同编码库位类型答 %d %s，与从没登记过的库位类型答 %d %s 不同：泄露了是否存在",
			otherHistory.Code, otherHistory.Body.String(), neverRegistered.Code, neverRegistered.Body.String())
	}
	// 另一个租户已登记过仓：登记同编码的库位类型从修订 1 起，互不相干；A 的当前修订不变。
	if response := registerLocationType("token-b", locationTypeDocument(code, 1, "另一租户的货架位", false)); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记同编码的库位类型：%d %s，want 201", response.Code, response.Body.String())
	}
	again := decodeLocationTypeList(t, warehouseRequest(router, http.MethodGet, locationTypeListLine, "token-a", ""))
	for _, row := range again.LocationTypes {
		if row.LocationTypeCode == code && (row.Revision != 4 || row.Name != "合成货架位（新名）") {
			t.Fatalf("另一个租户登记后，A 的库位类型 = %+v，want 不变", row)
		}
	}

	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.location_type_revision WHERE tenant_id = $1`, "SYN-TENANT-01"); got != 5 {
		t.Fatalf("SYN-TENANT-01 在册 %d 笔库位类型修订，want 5（%s 四笔、逐字编码一笔；未配置、冲突与跳号都不落册）", got, code)
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.location_type_revision WHERE tenant_id = $1`, "SYN-TENANT-02"); got != 1 {
		t.Fatalf("SYN-TENANT-02 在册 %d 笔库位类型修订，want 1", got)
	}
}

// Covers: AT-WF-012（端点那一半）——开关关着时，同样的真库与真渠道（授予齐备、令牌有效）打库位类型的各口，答 403
// WAREHOUSE_FULFILLMENT_NOT_ENABLED，库位类型册里一行也没落：登记口没有走到任何编排，库表（空表）原样。
func TestSwitchedOffLocationTypesNeverReachTheRegisterOnARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, false)

	registration := warehouseRequest(router, http.MethodPost, locationTypeRegistrationsLine, "token-a", locationTypeDocument("SYN-LT-01", 1, "合成货架位", false))
	list := warehouseRequest(router, http.MethodGet, locationTypeListLine, "token-a", "")
	history := warehouseRequest(router, http.MethodGet, locationTypeRevisionsLine+"?locationTypeCode=SYN-LT-01", "token-a", "")
	for name, response := range map[string]*httptest.ResponseRecorder{"registration": registration, "list": list, "history": history} {
		if response.Code != http.StatusForbidden || problemCode(t, response) != "WAREHOUSE_FULFILLMENT_NOT_ENABLED" {
			t.Fatalf("%s：答 %d %s，want 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED", name, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response, name)
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.location_type_revision`); got != 0 {
		t.Fatalf("开关关着时库位类型册里有 %d 笔修订，want 0", got)
	}
}
