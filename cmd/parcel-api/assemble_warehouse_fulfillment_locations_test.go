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

// 本文件的操作者、租户、仓、库区、库位类型与库位都是隔离合成值，只记 `S`。认证与装配沿用 assemble_warehouse_fulfillment_test.go：令牌 token-a 是
// 租户 SYN-TENANT-01 的操作者，token-b 是租户 SYN-TENANT-02 的操作者，其余一概走生产的铸造器、认证适配器、Intake、编排与真库。

// warehouseLocationDocument 拼一份库位登记报文：库位类型与优先级可缺，缺就不写那个键。
func warehouseLocationDocument(warehouse, code string, revision int, zone, locationType string, priority *int, deactivated bool) string {
	parts := []string{
		fmt.Sprintf(`"warehouseCode":%q`, warehouse), fmt.Sprintf(`"locationCode":%q`, code),
		fmt.Sprintf(`"revision":%d`, revision), fmt.Sprintf(`"zoneCode":%q`, zone), fmt.Sprintf(`"deactivated":%t`, deactivated),
	}
	if locationType != "" {
		parts = append(parts, fmt.Sprintf(`"locationTypeCode":%q`, locationType))
	}
	if priority != nil {
		parts = append(parts, fmt.Sprintf(`"priority":%d`, *priority))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func intPtr(value int) *int { return &value }

type locationRegistrationAnswer struct {
	Outcome        string `json:"outcome"`
	Refusal        string `json:"refusal"`
	Field          string `json:"field"`
	LatestRevision *int   `json:"latestRevision"`
	Location       struct {
		WarehouseCode    string  `json:"warehouseCode"`
		LocationCode     string  `json:"locationCode"`
		Revision         int     `json:"revision"`
		ZoneCode         string  `json:"zoneCode"`
		LocationTypeCode *string `json:"locationTypeCode"`
		Priority         *int    `json:"priority"`
		Deactivated      bool    `json:"deactivated"`
		Registrant       string  `json:"registrant"`
		RegisteredAt     string  `json:"registeredAt"`
	} `json:"location"`
}

func decodeLocationAnswer(t *testing.T, response *httptest.ResponseRecorder) locationRegistrationAnswer {
	t.Helper()
	var answer locationRegistrationAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复体 %q：%v", response.Body.String(), err)
	}
	return answer
}

type locationListedAnswer struct {
	Outcome   string `json:"outcome"`
	Locations []struct {
		WarehouseCode    string  `json:"warehouseCode"`
		LocationCode     string  `json:"locationCode"`
		Revision         int     `json:"revision"`
		ZoneCode         string  `json:"zoneCode"`
		LocationTypeCode *string `json:"locationTypeCode"`
		Priority         *int    `json:"priority"`
		Deactivated      bool    `json:"deactivated"`
	} `json:"locations"`
	Page struct {
		Total *int64 `json:"total"`
	} `json:"page"`
}

func decodeLocationList(t *testing.T, response *httptest.ResponseRecorder) locationListedAnswer {
	t.Helper()
	var answer locationListedAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || response.Code != http.StatusOK {
		t.Fatalf("列表：%d %s（%v）", response.Code, response.Body.String(), err)
	}
	return answer
}

type locationRevisionsListedAnswer struct {
	Outcome       string `json:"outcome"`
	WarehouseCode string `json:"warehouseCode"`
	LocationCode  string `json:"locationCode"`
	Revisions     []struct {
		Revision         int     `json:"revision"`
		Priority         *int    `json:"priority"`
		LocationTypeCode *string `json:"locationTypeCode"`
		Deactivated      bool    `json:"deactivated"`
	} `json:"revisions"`
}

// Covers: AT-WF-001、AT-WF-002、AT-WF-003、AT-WF-009、AT-WF-010（库位的部分）、UC-WF-001「权限、隔离与审计」——开关开着，经生产装配、真认证
// 适配器、真编排与真库走通：一个仓都没登记时登记库位答 REFERENCE_NOT_FOUND 并指出仓那一格、不落册，列表与修订史答空；登记仓、库区与库位类型
// 之后，没登记的仓、没登记的库区、另一个仓下的库区、没登记的库位类型各答 REFERENCE_NOT_FOUND 并指出对应的格；登记库位（带类型与优先级，201
// REGISTERED）、原样重投（200 ALREADY_REGISTERED，回显当初那一笔）、同一修订号换内容（CONTENT_CONFLICT）、改库区（IMMUTABLE_FIELD 指出
// zoneCode）、跳号（修订错位并回显在册最新修订号）、只改优先级（修订 2）；列表（含按库区筛）与修订史读到它，没有库位类型与优先级的库位回显与读面
// 里那两个键整键缺席；另一个租户读不到它，也登记不了它的仓、库区、库位类型（与没登记过的同答）；把库位类型停用之后，新库位指名它答
// REFERENCE_DEACTIVATED，原来的库位沿用它只改优先级答 REGISTERED；停用的仓与库区下照常登记库位。
func TestLocationsAreRegisteredAndReadBackThroughTheProductionAssembly(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, true)
	const warehouse, zone, location, locationType = "SYN-WH-01", "SYN-ZONE-01", "SYN-LOC-01", "SYN-LT-01"
	post := func(token, line, body string) *httptest.ResponseRecorder {
		return warehouseRequest(router, http.MethodPost, line, token, body)
	}
	registerLocation := func(token, body string) *httptest.ResponseRecorder {
		return post(token, locationRegistrationsLine, body)
	}
	expectCreated := func(label string, response *httptest.ResponseRecorder) {
		t.Helper()
		if response.Code != http.StatusCreated {
			t.Fatalf("%s：%d %s，want 201", label, response.Code, response.Body.String())
		}
	}
	expectRefusal := func(label string, response *httptest.ResponseRecorder, refusal, field string) {
		t.Helper()
		answer := decodeLocationAnswer(t, response)
		if response.Code != http.StatusOK || answer.Outcome != "NOT_ACCEPTED" || answer.Refusal != refusal || answer.Field != field {
			t.Fatalf("%s：%d %s，want NOT_ACCEPTED/%s 指出 %s", label, response.Code, response.Body.String(), refusal, field)
		}
	}
	rowsOf := func(tenant string) int {
		return countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.location_revision WHERE tenant_id = $1`, tenant)
	}

	// AT-WF-009：一个仓都没登记。
	refused := registerLocation("token-a", warehouseLocationDocument(warehouse, location, 1, zone, "", nil, false))
	expectRefusal("一个仓都没登记时登记库位", refused, "REFERENCE_NOT_FOUND", "warehouseCode")
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.location_revision`); got != 0 {
		t.Fatalf("被拒之后册里有 %d 笔库位修订，want 不落册", got)
	}
	emptyList := warehouseRequest(router, http.MethodGet, locationListLine, "token-a", "")
	emptyHistory := warehouseRequest(router, http.MethodGet, locationRevisionsLine+"?warehouseCode="+warehouse+"&locationCode="+location, "token-a", "")
	if decodeLocationList(t, emptyList).Outcome != "LOCATIONS_LISTED" || !strings.Contains(emptyList.Body.String(), `"locations":[]`) ||
		!strings.Contains(emptyHistory.Body.String(), `"revisions":[]`) {
		t.Fatalf("空册的列表与修订史该是空数组：%s、%s", emptyList.Body.String(), emptyHistory.Body.String())
	}

	// 登记仓、库区与库位类型。
	expectCreated("登记一号仓", post("token-a", warehouseRegistrationsLine, warehouseDocument(warehouse, 1, "合成一号仓", "SELF_OPERATED")))
	expectCreated("登记二号仓", post("token-a", warehouseRegistrationsLine, warehouseDocument("SYN-WH-02", 1, "合成二号仓", "AGENT")))
	expectCreated("登记一号仓的库区", post("token-a", zoneRegistrationsLine, zoneDocument(warehouse, zone, 1, "合成存储区", "STORAGE", false)))
	expectCreated("登记一号仓的另一个库区", post("token-a", zoneRegistrationsLine, zoneDocument(warehouse, "SYN-ZONE-02", 1, "合成拣选区", "PICKING", false)))
	expectCreated("登记二号仓的库区", post("token-a", zoneRegistrationsLine, zoneDocument("SYN-WH-02", "SYN-ZONE-W2", 1, "二号仓的存储区", "STORAGE", false)))
	expectCreated("登记库位类型", post("token-a", locationTypeRegistrationsLine, locationTypeDocument(locationType, 1, "合成货架位", false)))

	// AT-WF-003：各指名对象查不到。
	expectRefusal("指名没登记的仓", registerLocation("token-a", warehouseLocationDocument("SYN-WH-NEVER", location, 1, zone, "", nil, false)),
		"REFERENCE_NOT_FOUND", "warehouseCode")
	expectRefusal("指名没登记的库区", registerLocation("token-a", warehouseLocationDocument(warehouse, location, 1, "SYN-ZONE-NEVER", "", nil, false)),
		"REFERENCE_NOT_FOUND", "zoneCode")
	expectRefusal("指名另一个仓下的库区", registerLocation("token-a", warehouseLocationDocument(warehouse, location, 1, "SYN-ZONE-W2", "", nil, false)),
		"REFERENCE_NOT_FOUND", "zoneCode")
	expectRefusal("指名没登记的库位类型", registerLocation("token-a", warehouseLocationDocument(warehouse, location, 1, zone, "SYN-LT-NEVER", nil, false)),
		"REFERENCE_NOT_FOUND", "locationTypeCode")
	if got := rowsOf("SYN-TENANT-01"); got != 0 {
		t.Fatalf("各种查无所指之后册里有 %d 笔库位修订，want 不落册", got)
	}

	// 首次登记（带类型与优先级）、重放、冲突、改库区、跳号、改优先级。
	first := registerLocation("token-a", warehouseLocationDocument(warehouse, location, 1, zone, locationType, intPtr(5), false))
	expectCreated("首次登记库位", first)
	registered := decodeLocationAnswer(t, first)
	if registered.Outcome != "REGISTERED" || registered.Location.WarehouseCode != warehouse || registered.Location.LocationCode != location ||
		registered.Location.Revision != 1 || registered.Location.ZoneCode != zone || registered.Location.LocationTypeCode == nil ||
		*registered.Location.LocationTypeCode != locationType || registered.Location.Priority == nil || *registered.Location.Priority != 5 ||
		registered.Location.Registrant != "https://id.syn.example/dex#SYN-OPERATOR-01" || registered.Location.RegisteredAt == "" {
		t.Fatalf("首次登记的答复 = %s", first.Body.String())
	}
	replayed := registerLocation("token-a", warehouseLocationDocument(warehouse, location, 1, zone, locationType, intPtr(5), false))
	if answer := decodeLocationAnswer(t, replayed); replayed.Code != http.StatusOK || answer.Outcome != "ALREADY_REGISTERED" || answer.Location.RegisteredAt != registered.Location.RegisteredAt {
		t.Fatalf("原样重投：%d %s，want 200 ALREADY_REGISTERED 并回显当初那一笔", replayed.Code, replayed.Body.String())
	}
	if conflict := registerLocation("token-a", warehouseLocationDocument(warehouse, location, 1, zone, locationType, intPtr(6), false)); conflict.Code != http.StatusOK ||
		decodeLocationAnswer(t, conflict).Outcome != "CONTENT_CONFLICT" {
		t.Fatalf("同一修订号换内容：%d %s，want 200 CONTENT_CONFLICT", conflict.Code, conflict.Body.String())
	}
	expectRefusal("把库位改到另一个库区", registerLocation("token-a", warehouseLocationDocument(warehouse, location, 2, "SYN-ZONE-02", locationType, intPtr(5), false)),
		"IMMUTABLE_FIELD", "zoneCode")
	if skipped := registerLocation("token-a", warehouseLocationDocument(warehouse, location, 3, zone, locationType, intPtr(5), false)); skipped.Code != http.StatusOK {
		t.Fatalf("跳号：%d %s", skipped.Code, skipped.Body.String())
	} else if answer := decodeLocationAnswer(t, skipped); answer.Refusal != "REVISION_OUT_OF_SEQUENCE" || answer.LatestRevision == nil || *answer.LatestRevision != 1 {
		t.Fatalf("跳号的答复 = %s，want 回显在册最新修订号 1", skipped.Body.String())
	}
	expectCreated("只改优先级", registerLocation("token-a", warehouseLocationDocument(warehouse, location, 2, zone, locationType, intPtr(9), false)))

	// 没有库位类型与优先级的库位：登记得进，回显里那两个键整键缺席；优先级 0 与没有优先级是两份内容。
	bare := registerLocation("token-a", warehouseLocationDocument(warehouse, "SYN-LOC-BARE", 1, zone, "", nil, false))
	expectCreated("不带类型与优先级的库位", bare)
	if strings.Contains(bare.Body.String(), "locationTypeCode") || strings.Contains(bare.Body.String(), "priority") {
		t.Fatalf("没有类型与优先级的回显里那两个键该整键缺席：%s", bare.Body.String())
	}
	if conflict := registerLocation("token-a", warehouseLocationDocument(warehouse, "SYN-LOC-BARE", 1, zone, "", intPtr(0), false)); decodeLocationAnswer(t, conflict).Outcome != "CONTENT_CONFLICT" {
		t.Fatalf("同一修订号带优先级 0 对不带：%s，want CONTENT_CONFLICT", conflict.Body.String())
	}
	zeroPriority := registerLocation("token-a", warehouseLocationDocument(warehouse, "SYN-LOC-ZERO", 1, zone, "", intPtr(0), false))
	expectCreated("优先级 0 的库位", zeroPriority)
	if answer := decodeLocationAnswer(t, zeroPriority); answer.Location.Priority == nil || *answer.Location.Priority != 0 || !strings.Contains(zeroPriority.Body.String(), `"priority":0`) {
		t.Fatalf("优先级 0 的回显 = %s，want priority 为 0 且在场", zeroPriority.Body.String())
	}
	// 二号仓下同编码的库位是另一个库位；库位编码逐字保存。
	const verbatim = " Syn-Loc 09 "
	expectCreated("二号仓同编码的库位", registerLocation("token-a", warehouseLocationDocument("SYN-WH-02", location, 1, "SYN-ZONE-W2", "", nil, false)))
	expectCreated("逐字编码的库位", registerLocation("token-a", warehouseLocationDocument(warehouse, verbatim, 1, zone, "", nil, false)))

	// 列表与修订史。
	list := decodeLocationList(t, warehouseRequest(router, http.MethodGet, locationListLine, "token-a", ""))
	if list.Outcome != "LOCATIONS_LISTED" || len(list.Locations) != 5 || list.Page.Total == nil || *list.Page.Total != 5 {
		t.Fatalf("列表 = %+v，want 五个库位（各自的当前修订）", list)
	}
	type current struct {
		revision     int
		zone         string
		locationType *string
		priority     *int
	}
	byKey := map[string]current{}
	for _, row := range list.Locations {
		byKey[row.WarehouseCode+"/"+row.LocationCode] = current{row.Revision, row.ZoneCode, row.LocationTypeCode, row.Priority}
	}
	if got := byKey[warehouse+"/"+location]; got.revision != 2 || got.zone != zone || got.locationType == nil || *got.locationType != locationType ||
		got.priority == nil || *got.priority != 9 {
		t.Fatalf("列表里 %s 的当前修订 = %+v，want 修订 2、带类型、优先级 9", location, got)
	}
	if got := byKey[warehouse+"/SYN-LOC-BARE"]; got.revision != 1 || got.locationType != nil || got.priority != nil {
		t.Fatalf("列表里没有类型与优先级的库位 = %+v，want 两格都没有", got)
	}
	if got := byKey[warehouse+"/SYN-LOC-ZERO"]; got.priority == nil || *got.priority != 0 {
		t.Fatalf("列表里优先级 0 的库位 = %+v，want 优先级给了、是 0", got)
	}
	if got := byKey["SYN-WH-02/"+location]; got.revision != 1 || got.zone != "SYN-ZONE-W2" {
		t.Fatalf("二号仓同编码的库位 = %+v，want 修订 1、属于它自己仓下的库区", got)
	}
	byZone := decodeLocationList(t, warehouseRequest(router, http.MethodGet, locationListLine+"?warehouseCode="+warehouse+"&zoneCode="+zone, "token-a", ""))
	if len(byZone.Locations) != 4 {
		t.Fatalf("按仓与库区筛 = %d 个，want 一号仓 %s 库区下的四个库位", len(byZone.Locations), zone)
	}
	if none := decodeLocationList(t, warehouseRequest(router, http.MethodGet, locationListLine+"?zoneCode=SYN-ZONE-02", "token-a", "")); len(none.Locations) != 0 {
		t.Fatalf("按没有库位的库区筛 = %+v，want 没有", none)
	}
	if none := decodeLocationList(t, warehouseRequest(router, http.MethodGet, locationListLine+"?deactivated=true", "token-a", "")); len(none.Locations) != 0 {
		t.Fatalf("按已停用筛 = %+v，want 没有", none)
	}
	history := warehouseRequest(router, http.MethodGet, locationRevisionsLine+"?warehouseCode="+warehouse+"&locationCode="+location, "token-a", "")
	var revisions locationRevisionsListedAnswer
	if err := json.Unmarshal(history.Body.Bytes(), &revisions); err != nil || history.Code != http.StatusOK {
		t.Fatalf("修订史：%d %s（%v）", history.Code, history.Body.String(), err)
	}
	if revisions.Outcome != "LOCATION_REVISIONS_LISTED" || revisions.WarehouseCode != warehouse || revisions.LocationCode != location || len(revisions.Revisions) != 2 ||
		revisions.Revisions[0].Revision != 1 || revisions.Revisions[1].Revision != 2 ||
		revisions.Revisions[0].Priority == nil || *revisions.Revisions[0].Priority != 5 || *revisions.Revisions[1].Priority != 9 {
		t.Fatalf("修订史 = %s，want 修订 1、2 按序，优先级 5 与 9", history.Body.String())
	}
	verbatimHistory := warehouseRequest(router, http.MethodGet,
		locationRevisionsLine+"?warehouseCode="+warehouse+"&locationCode="+url.QueryEscape(verbatim), "token-a", "")
	var oddRevisions locationRevisionsListedAnswer
	if err := json.Unmarshal(verbatimHistory.Body.Bytes(), &oddRevisions); err != nil || oddRevisions.LocationCode != verbatim || len(oddRevisions.Revisions) != 1 {
		t.Fatalf("逐字库位编码的修订史 = %s", verbatimHistory.Body.String())
	}
	// 没有类型与优先级的库位在库里是 NULL，不是空串或 0。
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.location_revision
		WHERE tenant_id = 'SYN-TENANT-01' AND location_code = 'SYN-LOC-BARE' AND location_type_code IS NULL AND priority IS NULL`); got != 1 {
		t.Fatalf("没有类型与优先级的库位在库里该是两个 NULL，查到 %d 行", got)
	}

	// 另一个租户读不到它：列表空，修订史是空数组，与从没登记过的库位同答；它没登记过仓、库区与库位类型，所以指名它们答查无所指，
	// 与本租户指名没登记过的同答。
	otherList := warehouseRequest(router, http.MethodGet, locationListLine, "token-b", "")
	if otherList.Code != http.StatusOK || !strings.Contains(otherList.Body.String(), `"locations":[]`) {
		t.Fatalf("另一个租户的列表：%d %s，want 200 与空数组", otherList.Code, otherList.Body.String())
	}
	otherHistory := warehouseRequest(router, http.MethodGet, locationRevisionsLine+"?warehouseCode="+warehouse+"&locationCode="+location, "token-b", "")
	neverRegistered := warehouseRequest(router, http.MethodGet, locationRevisionsLine+"?warehouseCode="+warehouse+"&locationCode=SYN-LOC-NEVER", "token-b", "")
	if want := strings.ReplaceAll(neverRegistered.Body.String(), "SYN-LOC-NEVER", location); otherHistory.Code != http.StatusOK || otherHistory.Body.String() != want {
		t.Fatalf("别的租户的同编码库位答 %d %s，与从没登记过的库位答 %d %s 不同：泄露了是否存在",
			otherHistory.Code, otherHistory.Body.String(), neverRegistered.Code, neverRegistered.Body.String())
	}
	foreign := registerLocation("token-b", warehouseLocationDocument(warehouse, location, 1, zone, "", nil, false))
	neverWarehouse := registerLocation("token-b", warehouseLocationDocument("SYN-WH-NEVER", location, 1, zone, "", nil, false))
	if foreign.Code != http.StatusOK || foreign.Body.String() != neverWarehouse.Body.String() {
		t.Fatalf("另一个租户指名本租户登记的仓：%d %s，与指名没登记过的仓 %d %s 不同", foreign.Code, foreign.Body.String(), neverWarehouse.Code, neverWarehouse.Body.String())
	}
	expectRefusal("另一个租户指名本租户的仓", foreign, "REFERENCE_NOT_FOUND", "warehouseCode")
	// 另一个租户登记了自己的同编码仓之后，指名本租户的库区与库位类型仍是查无所指。
	expectCreated("另一个租户登记同编码的仓", post("token-b", warehouseRegistrationsLine, warehouseDocument(warehouse, 1, "另一租户的仓", "AGENT")))
	foreignZone := registerLocation("token-b", warehouseLocationDocument(warehouse, location, 1, zone, "", nil, false))
	neverZone := registerLocation("token-b", warehouseLocationDocument(warehouse, location, 1, "SYN-ZONE-NEVER", "", nil, false))
	if foreignZone.Body.String() != neverZone.Body.String() {
		t.Fatalf("另一个租户指名本租户的库区：%s，与指名没登记过的库区 %s 不同", foreignZone.Body.String(), neverZone.Body.String())
	}
	expectRefusal("另一个租户指名本租户的库区", foreignZone, "REFERENCE_NOT_FOUND", "zoneCode")
	expectCreated("另一个租户登记同编码的库区", post("token-b", zoneRegistrationsLine, zoneDocument(warehouse, zone, 1, "另一租户的存储区", "STORAGE", false)))
	foreignType := registerLocation("token-b", warehouseLocationDocument(warehouse, location, 1, zone, locationType, nil, false))
	neverType := registerLocation("token-b", warehouseLocationDocument(warehouse, location, 1, zone, "SYN-LT-NEVER", nil, false))
	if foreignType.Body.String() != neverType.Body.String() {
		t.Fatalf("另一个租户指名本租户的库位类型：%s，与指名没登记过的库位类型 %s 不同", foreignType.Body.String(), neverType.Body.String())
	}
	expectRefusal("另一个租户指名本租户的库位类型", foreignType, "REFERENCE_NOT_FOUND", "locationTypeCode")
	// 另一个租户登记同编码的库位，从修订 1 起，互不相干。
	expectCreated("另一个租户登记同编码的库位", registerLocation("token-b", warehouseLocationDocument(warehouse, location, 1, zone, "", nil, false)))
	again := decodeLocationList(t, warehouseRequest(router, http.MethodGet, locationListLine+"?warehouseCode="+warehouse+"&zoneCode="+zone, "token-a", ""))
	for _, row := range again.Locations {
		if row.WarehouseCode == warehouse && row.LocationCode == location && (row.Revision != 2 || row.Priority == nil || *row.Priority != 9) {
			t.Fatalf("另一个租户登记后，A 的库位 = %+v，want 不变", row)
		}
	}

	// 把库位类型停用：新库位指名它答所指已停用，原来的库位沿用它只改优先级照常受理。
	expectCreated("停用库位类型", post("token-a", locationTypeRegistrationsLine, locationTypeDocument(locationType, 2, "合成货架位", true)))
	deactivated := registerLocation("token-a", warehouseLocationDocument(warehouse, "SYN-LOC-NEW", 1, zone, locationType, nil, false))
	expectRefusal("新库位指名已停用的库位类型", deactivated, "REFERENCE_DEACTIVATED", "locationTypeCode")
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.location_revision WHERE tenant_id = 'SYN-TENANT-01' AND location_code = 'SYN-LOC-NEW'`); got != 0 {
		t.Fatalf("所指已停用之后册里有 %d 笔 SYN-LOC-NEW，want 不落册", got)
	}
	kept := registerLocation("token-a", warehouseLocationDocument(warehouse, location, 3, zone, locationType, intPtr(10), false))
	if decoded := decodeLocationAnswer(t, kept); kept.Code != http.StatusCreated || decoded.Outcome != "REGISTERED" || decoded.Location.Revision != 3 {
		t.Fatalf("沿用已停用的库位类型只改优先级：%d %s，want 201 REGISTERED 修订 3", kept.Code, kept.Body.String())
	}
	// 库位停用与再启用各是新修订。
	expectCreated("停用库位", registerLocation("token-a", warehouseLocationDocument(warehouse, location, 4, zone, locationType, intPtr(10), true)))
	expectCreated("再启用库位", registerLocation("token-a", warehouseLocationDocument(warehouse, location, 5, zone, locationType, intPtr(10), false)))

	// 仓与库区停用之后，库位照常登记。
	deactivatedWarehouse := `{"warehouseCode":"SYN-WH-02","revision":2,"name":"合成二号仓","kind":"AGENT",` +
		`"address":{"country":"CN","text":"合成路 2 号"},"deactivated":true}`
	expectCreated("停用二号仓", post("token-a", warehouseRegistrationsLine, deactivatedWarehouse))
	expectCreated("停用二号仓的库区", post("token-a", zoneRegistrationsLine, zoneDocument("SYN-WH-02", "SYN-ZONE-W2", 2, "二号仓的存储区", "STORAGE", true)))
	expectCreated("停用的仓与库区下登记库位", registerLocation("token-a", warehouseLocationDocument("SYN-WH-02", "SYN-LOC-D", 1, "SYN-ZONE-W2", "", nil, false)))

	// 本租户在册：SYN-LOC-01 五笔、SYN-LOC-BARE、SYN-LOC-ZERO、逐字编码、二号仓同编码、停用的仓下的库位，各一笔。冲突、改库区、跳号、
	// 查无所指与所指已停用都不落册。
	if got := rowsOf("SYN-TENANT-01"); got != 10 {
		t.Fatalf("SYN-TENANT-01 在册 %d 笔库位修订，want 10", got)
	}
	if got := rowsOf("SYN-TENANT-02"); got != 1 {
		t.Fatalf("SYN-TENANT-02 在册 %d 笔库位修订，want 1", got)
	}
}

// Covers: AT-WF-012（端点那一半）——开关关着时，同样的真库与真渠道（授予齐备、令牌有效）打库位的各口，答 403
// WAREHOUSE_FULFILLMENT_NOT_ENABLED，库位册里一行也没落：登记口没有走到任何编排，库表（空表）原样。
func TestSwitchedOffLocationsNeverReachTheRegisterOnARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, false)

	registration := warehouseRequest(router, http.MethodPost, locationRegistrationsLine, "token-a",
		warehouseLocationDocument("SYN-WH-01", "SYN-LOC-01", 1, "SYN-ZONE-01", "SYN-LT-01", intPtr(1), false))
	list := warehouseRequest(router, http.MethodGet, locationListLine, "token-a", "")
	history := warehouseRequest(router, http.MethodGet, locationRevisionsLine+"?warehouseCode=SYN-WH-01&locationCode=SYN-LOC-01", "token-a", "")
	for name, response := range map[string]*httptest.ResponseRecorder{"registration": registration, "list": list, "history": history} {
		if response.Code != http.StatusForbidden || problemCode(t, response) != "WAREHOUSE_FULFILLMENT_NOT_ENABLED" {
			t.Fatalf("%s：答 %d %s，want 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED", name, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response, name)
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.location_revision`); got != 0 {
		t.Fatalf("开关关着时库位册里有 %d 笔修订，want 0", got)
	}
}
