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

// 本文件的操作者、租户、仓与库区都是隔离合成值，只记 `S`。认证与装配沿用 assemble_warehouse_fulfillment_test.go：令牌 token-a 是
// 租户 SYN-TENANT-01 的操作者，token-b 是租户 SYN-TENANT-02 的操作者，其余一概走生产的铸造器、认证适配器、Intake、编排与真库。

func zoneDocument(warehouse, code string, revision int, name, usage string, deactivated bool) string {
	return fmt.Sprintf(`{"warehouseCode":%q,"zoneCode":%q,"revision":%d,"name":%q,"usage":%q,"deactivated":%t}`,
		warehouse, code, revision, name, usage, deactivated)
}

type zoneRegistrationAnswer struct {
	Outcome        string `json:"outcome"`
	Refusal        string `json:"refusal"`
	Field          string `json:"field"`
	LatestRevision *int   `json:"latestRevision"`
	Zone           struct {
		WarehouseCode string `json:"warehouseCode"`
		ZoneCode      string `json:"zoneCode"`
		Revision      int    `json:"revision"`
		Name          string `json:"name"`
		Usage         string `json:"usage"`
		Deactivated   bool   `json:"deactivated"`
		Registrant    string `json:"registrant"`
		RegisteredAt  string `json:"registeredAt"`
	} `json:"zone"`
}

func decodeZoneAnswer(t *testing.T, response *httptest.ResponseRecorder) zoneRegistrationAnswer {
	t.Helper()
	var answer zoneRegistrationAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复体 %q：%v", response.Body.String(), err)
	}
	return answer
}

type zoneListedAnswer struct {
	Outcome string `json:"outcome"`
	Zones   []struct {
		WarehouseCode string `json:"warehouseCode"`
		ZoneCode      string `json:"zoneCode"`
		Revision      int    `json:"revision"`
		Name          string `json:"name"`
		Usage         string `json:"usage"`
		Deactivated   bool   `json:"deactivated"`
	} `json:"zones"`
	Page struct {
		Total *int64 `json:"total"`
	} `json:"page"`
}

func decodeZoneList(t *testing.T, response *httptest.ResponseRecorder) zoneListedAnswer {
	t.Helper()
	var answer zoneListedAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || response.Code != http.StatusOK {
		t.Fatalf("列表：%d %s（%v）", response.Code, response.Body.String(), err)
	}
	return answer
}

type zoneRevisionsListedAnswer struct {
	Outcome       string `json:"outcome"`
	WarehouseCode string `json:"warehouseCode"`
	ZoneCode      string `json:"zoneCode"`
	Revisions     []struct {
		Revision int    `json:"revision"`
		Name     string `json:"name"`
	} `json:"revisions"`
}

// Covers: AT-WF-001、AT-WF-002、AT-WF-003、AT-WF-009、AT-WF-010（库区的部分）、UC-WF-001「权限、隔离与审计」——开关开着，经生产装配、真认证
// 适配器、真编排与真库走通：一个仓都没登记时登记库区答 REFERENCE_NOT_FOUND 并指出仓那一格、不落册，列表与修订史答空；登记仓之后在它
// 下面登记库区（201 REGISTERED）、原样重投（200 ALREADY_REGISTERED，回显当初那一笔）、同一修订号换内容（CONTENT_CONFLICT）、改用途
// （IMMUTABLE_FIELD 指出 usage）、跳号（修订错位并回显在册最新修订号）、改名（修订 2）；列表与修订史读到它，按仓编码与是否停用筛得出；
// 另一个租户读不到它，也登记不了它的仓下的库区（与没登记过的仓同答），并可以登记同编码的仓与库区，互不相干；仓停用之后库区照常登记。
func TestZonesAreRegisteredAndReadBackThroughTheProductionAssembly(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, true)
	const warehouse, zone = "SYN-WH-01", "SYN-ZONE-01"
	registerZone := func(token, body string) *httptest.ResponseRecorder {
		return warehouseRequest(router, http.MethodPost, zoneRegistrationsLine, token, body)
	}
	registerWarehouse := func(token, body string) *httptest.ResponseRecorder {
		return warehouseRequest(router, http.MethodPost, warehouseRegistrationsLine, token, body)
	}

	// AT-WF-009：一个仓都没登记。
	refused := registerZone("token-a", zoneDocument(warehouse, zone, 1, "合成存储区", "STORAGE", false))
	if refused.Code != http.StatusOK {
		t.Fatalf("一个仓都没登记时登记库区：%d %s，want 200", refused.Code, refused.Body.String())
	}
	if answer := decodeZoneAnswer(t, refused); answer.Outcome != "NOT_ACCEPTED" || answer.Refusal != "REFERENCE_NOT_FOUND" || answer.Field != "warehouseCode" {
		t.Fatalf("一个仓都没登记时登记库区 = %s，want NOT_ACCEPTED/REFERENCE_NOT_FOUND 指出 warehouseCode", refused.Body.String())
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.zone_revision`); got != 0 {
		t.Fatalf("被拒之后册里有 %d 笔库区修订，want 不落册", got)
	}
	if empty := decodeZoneList(t, warehouseRequest(router, http.MethodGet, zoneListLine, "token-a", "")); empty.Outcome != "ZONES_LISTED" || len(empty.Zones) != 0 {
		t.Fatalf("空册的列表 = %+v，want 空列表", empty)
	}
	emptyList := warehouseRequest(router, http.MethodGet, zoneListLine, "token-a", "")
	emptyHistory := warehouseRequest(router, http.MethodGet, zoneRevisionsLine+"?warehouseCode="+warehouse+"&zoneCode="+zone, "token-a", "")
	if !strings.Contains(emptyList.Body.String(), `"zones":[]`) || !strings.Contains(emptyHistory.Body.String(), `"revisions":[]`) {
		t.Fatalf("空册的列表与修订史该是空数组：%s、%s", emptyList.Body.String(), emptyHistory.Body.String())
	}

	// 登记仓，再在它下面登记库区。
	if response := registerWarehouse("token-a", warehouseDocument(warehouse, 1, "合成一号仓", "SELF_OPERATED")); response.Code != http.StatusCreated {
		t.Fatalf("登记仓：%d %s", response.Code, response.Body.String())
	}
	first := registerZone("token-a", zoneDocument(warehouse, zone, 1, "合成存储区", "STORAGE", false))
	if first.Code != http.StatusCreated {
		t.Fatalf("首次登记库区：%d %s，want 201", first.Code, first.Body.String())
	}
	registered := decodeZoneAnswer(t, first)
	if registered.Outcome != "REGISTERED" || registered.Zone.WarehouseCode != warehouse || registered.Zone.ZoneCode != zone ||
		registered.Zone.Revision != 1 || registered.Zone.Usage != "STORAGE" ||
		registered.Zone.Registrant != "https://id.syn.example/dex#SYN-OPERATOR-01" || registered.Zone.RegisteredAt == "" {
		t.Fatalf("首次登记的答复 = %s", first.Body.String())
	}

	replayed := registerZone("token-a", zoneDocument(warehouse, zone, 1, "合成存储区", "STORAGE", false))
	if replayed.Code != http.StatusOK {
		t.Fatalf("原样重投：%d %s，want 200", replayed.Code, replayed.Body.String())
	}
	if answer := decodeZoneAnswer(t, replayed); answer.Outcome != "ALREADY_REGISTERED" || answer.Zone.RegisteredAt != registered.Zone.RegisteredAt {
		t.Fatalf("原样重投的答复 = %s，want ALREADY_REGISTERED 并回显当初那一笔", replayed.Body.String())
	}
	if conflict := registerZone("token-a", zoneDocument(warehouse, zone, 1, "合成存储区（改名）", "STORAGE", false)); conflict.Code != http.StatusOK ||
		decodeZoneAnswer(t, conflict).Outcome != "CONTENT_CONFLICT" {
		t.Fatalf("同一修订号换内容：%d %s，want 200 CONTENT_CONFLICT", conflict.Code, conflict.Body.String())
	}
	if immutable := registerZone("token-a", zoneDocument(warehouse, zone, 2, "合成存储区", "PICKING", false)); immutable.Code != http.StatusOK {
		t.Fatalf("改用途：%d %s", immutable.Code, immutable.Body.String())
	} else if answer := decodeZoneAnswer(t, immutable); answer.Outcome != "NOT_ACCEPTED" || answer.Refusal != "IMMUTABLE_FIELD" || answer.Field != "usage" {
		t.Fatalf("改用途的答复 = %s", immutable.Body.String())
	}
	if skipped := registerZone("token-a", zoneDocument(warehouse, zone, 3, "合成存储区", "STORAGE", false)); skipped.Code != http.StatusOK {
		t.Fatalf("跳号：%d %s", skipped.Code, skipped.Body.String())
	} else if answer := decodeZoneAnswer(t, skipped); answer.Refusal != "REVISION_OUT_OF_SEQUENCE" || answer.LatestRevision == nil || *answer.LatestRevision != 1 {
		t.Fatalf("跳号的答复 = %s，want 回显在册最新修订号 1", skipped.Body.String())
	}
	if renamed := registerZone("token-a", zoneDocument(warehouse, zone, 2, "合成存储区（新名）", "STORAGE", false)); renamed.Code != http.StatusCreated {
		t.Fatalf("修订 2：%d %s，want 201", renamed.Code, renamed.Body.String())
	}
	// 同一个库区编码在另一个仓下是另一个库区；仓编码与库区编码都逐字保存。
	if response := registerWarehouse("token-a", warehouseDocument("SYN-WH-02", 1, "合成二号仓", "AGENT")); response.Code != http.StatusCreated {
		t.Fatalf("登记二号仓：%d %s", response.Code, response.Body.String())
	}
	const verbatim = " Syn-Zone 09 "
	for _, body := range []string{
		zoneDocument("SYN-WH-02", zone, 1, "二号仓的存储区", "PICKING", false),
		zoneDocument(warehouse, verbatim, 1, "逐字编码的库区", "DEFECTIVE", false),
	} {
		if response := registerZone("token-a", body); response.Code != http.StatusCreated {
			t.Fatalf("登记 %s：%d %s", body, response.Code, response.Body.String())
		}
	}

	list := decodeZoneList(t, warehouseRequest(router, http.MethodGet, zoneListLine, "token-a", ""))
	if list.Outcome != "ZONES_LISTED" || len(list.Zones) != 3 || list.Page.Total == nil || *list.Page.Total != 3 {
		t.Fatalf("列表 = %+v，want 三个库区（各自的当前修订）", list)
	}
	byKey := map[string]int{}
	names := map[string]string{}
	for _, row := range list.Zones {
		byKey[row.WarehouseCode+"/"+row.ZoneCode] = row.Revision
		names[row.WarehouseCode+"/"+row.ZoneCode] = row.Name
	}
	if byKey[warehouse+"/"+zone] != 2 || names[warehouse+"/"+zone] != "合成存储区（新名）" ||
		byKey["SYN-WH-02/"+zone] != 1 || byKey[warehouse+"/"+verbatim] != 1 {
		t.Fatalf("列表读到的当前修订 = %v %v，want 一号仓的库区为修订 2、二号仓同编码的库区与逐字编码的库区为修订 1", byKey, names)
	}
	filtered := decodeZoneList(t, warehouseRequest(router, http.MethodGet, zoneListLine+"?warehouseCode=SYN-WH-02", "token-a", ""))
	if len(filtered.Zones) != 1 || filtered.Zones[0].WarehouseCode != "SYN-WH-02" {
		t.Fatalf("按仓编码筛 = %+v，want 只有二号仓的库区", filtered)
	}
	if none := decodeZoneList(t, warehouseRequest(router, http.MethodGet, zoneListLine+"?deactivated=true", "token-a", "")); len(none.Zones) != 0 {
		t.Fatalf("按已停用筛 = %+v，want 没有", none)
	}

	history := warehouseRequest(router, http.MethodGet, zoneRevisionsLine+"?warehouseCode="+warehouse+"&zoneCode="+zone, "token-a", "")
	var revisions zoneRevisionsListedAnswer
	if err := json.Unmarshal(history.Body.Bytes(), &revisions); err != nil || history.Code != http.StatusOK {
		t.Fatalf("修订史：%d %s（%v）", history.Code, history.Body.String(), err)
	}
	if revisions.Outcome != "ZONE_REVISIONS_LISTED" || revisions.WarehouseCode != warehouse || revisions.ZoneCode != zone || len(revisions.Revisions) != 2 ||
		revisions.Revisions[0].Revision != 1 || revisions.Revisions[1].Revision != 2 || revisions.Revisions[0].Name != "合成存储区" {
		t.Fatalf("修订史 = %s，want 修订 1、2 按序", history.Body.String())
	}
	verbatimHistory := warehouseRequest(router, http.MethodGet,
		zoneRevisionsLine+"?warehouseCode="+warehouse+"&zoneCode="+url.QueryEscape(verbatim), "token-a", "")
	var oddRevisions zoneRevisionsListedAnswer
	if err := json.Unmarshal(verbatimHistory.Body.Bytes(), &oddRevisions); err != nil || oddRevisions.ZoneCode != verbatim || len(oddRevisions.Revisions) != 1 {
		t.Fatalf("逐字库区编码的修订史 = %s", verbatimHistory.Body.String())
	}

	// 另一个租户读不到它：列表空，修订史是空数组，与从没登记过的库区同答；它没登记仓，所以在同编码的仓下登记库区答查无所指，
	// 与本租户指名没登记的仓同答。
	otherList := warehouseRequest(router, http.MethodGet, zoneListLine, "token-b", "")
	if otherList.Code != http.StatusOK || !strings.Contains(otherList.Body.String(), `"zones":[]`) {
		t.Fatalf("另一个租户的列表：%d %s，want 200 与空数组", otherList.Code, otherList.Body.String())
	}
	otherHistory := warehouseRequest(router, http.MethodGet, zoneRevisionsLine+"?warehouseCode="+warehouse+"&zoneCode="+zone, "token-b", "")
	neverRegistered := warehouseRequest(router, http.MethodGet, zoneRevisionsLine+"?warehouseCode="+warehouse+"&zoneCode=SYN-ZONE-NEVER", "token-b", "")
	if want := strings.ReplaceAll(neverRegistered.Body.String(), "SYN-ZONE-NEVER", zone); otherHistory.Code != http.StatusOK || otherHistory.Body.String() != want {
		t.Fatalf("别的租户的同编码库区答 %d %s，与从没登记过的库区答 %d %s 不同：泄露了是否存在",
			otherHistory.Code, otherHistory.Body.String(), neverRegistered.Code, neverRegistered.Body.String())
	}
	foreign := registerZone("token-b", zoneDocument(warehouse, zone, 1, "另一租户的存储区", "STORAGE", false))
	neverWarehouse := registerZone("token-b", zoneDocument("SYN-WH-NEVER", zone, 1, "另一租户的存储区", "STORAGE", false))
	if foreign.Code != http.StatusOK || foreign.Body.String() != neverWarehouse.Body.String() {
		t.Fatalf("另一个租户指名本租户登记的仓：%d %s，与指名没登记过的仓 %d %s 不同", foreign.Code, foreign.Body.String(), neverWarehouse.Code, neverWarehouse.Body.String())
	}
	if answer := decodeZoneAnswer(t, foreign); answer.Refusal != "REFERENCE_NOT_FOUND" || answer.Field != "warehouseCode" {
		t.Fatalf("另一个租户指名本租户登记的仓的答复 = %s", foreign.Body.String())
	}
	// 另一个租户登记同编码的仓与库区，从修订 1 起，互不相干。
	if response := registerWarehouse("token-b", warehouseDocument(warehouse, 1, "另一租户的仓", "AGENT")); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记同编码的仓：%d %s", response.Code, response.Body.String())
	}
	if response := registerZone("token-b", zoneDocument(warehouse, zone, 1, "另一租户的存储区", "STORAGE", false)); response.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记同编码的库区：%d %s，want 201", response.Code, response.Body.String())
	}
	again := decodeZoneList(t, warehouseRequest(router, http.MethodGet, zoneListLine, "token-a", ""))
	for _, row := range again.Zones {
		if row.WarehouseCode == warehouse && row.ZoneCode == zone && (row.Revision != 2 || row.Name != "合成存储区（新名）") {
			t.Fatalf("另一个租户登记后，A 的库区 = %+v，want 不变", row)
		}
	}

	// 仓停用之后，库区照常登记新修订：停用只挡此后新受理的请求与新下发的选择，不挡登记。
	deactivatedWarehouse := `{"warehouseCode":"SYN-WH-01","revision":2,"name":"合成一号仓","kind":"SELF_OPERATED",` +
		`"address":{"country":"CN","text":"合成路 1 号"},"deactivated":true}`
	if response := registerWarehouse("token-a", deactivatedWarehouse); response.Code != http.StatusCreated {
		t.Fatalf("停用仓：%d %s", response.Code, response.Body.String())
	}
	if response := registerZone("token-a", zoneDocument(warehouse, zone, 3, "合成存储区（仓停用中）", "STORAGE", false)); response.Code != http.StatusCreated {
		t.Fatalf("停用的仓下登记库区修订 3：%d %s，want 201", response.Code, response.Body.String())
	}
	if response := registerZone("token-a", zoneDocument(warehouse, "SYN-ZONE-03", 1, "停用的仓下的新库区", "OUTBOUND_STAGING", false)); response.Code != http.StatusCreated {
		t.Fatalf("停用的仓下登记新库区：%d %s，want 201", response.Code, response.Body.String())
	}

	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.zone_revision WHERE tenant_id = $1`, "SYN-TENANT-01"); got != 6 {
		t.Fatalf("SYN-TENANT-01 在册 %d 笔库区修订，want 6（一号仓的库区修订 1、2、3，二号仓同编码库区一笔，逐字编码库区一笔，停用的仓下新库区一笔；"+
			"冲突、改用途、跳号与查无所指都不落册）", got)
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.zone_revision WHERE tenant_id = $1`, "SYN-TENANT-02"); got != 1 {
		t.Fatalf("SYN-TENANT-02 在册 %d 笔库区修订，want 1", got)
	}
}

// Covers: AT-WF-012（端点那一半）——开关关着时，同样的真库与真渠道（授予齐备、令牌有效）打库区的三个口，答 403
// WAREHOUSE_FULFILLMENT_NOT_ENABLED，库区册里一行也没落：登记口没有走到任何编排，库表（空表）原样。
func TestSwitchedOffZonesNeverReachTheRegisterOnARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, false)

	registration := warehouseRequest(router, http.MethodPost, zoneRegistrationsLine, "token-a", zoneDocument("SYN-WH-01", "SYN-ZONE-01", 1, "合成存储区", "STORAGE", false))
	list := warehouseRequest(router, http.MethodGet, zoneListLine, "token-a", "")
	history := warehouseRequest(router, http.MethodGet, zoneRevisionsLine+"?warehouseCode=SYN-WH-01&zoneCode=SYN-ZONE-01", "token-a", "")
	for name, response := range map[string]*httptest.ResponseRecorder{"registration": registration, "list": list, "history": history} {
		if response.Code != http.StatusForbidden || problemCode(t, response) != "WAREHOUSE_FULFILLMENT_NOT_ENABLED" {
			t.Fatalf("%s：答 %d %s，want 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED", name, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response, name)
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.zone_revision`); got != 0 {
		t.Fatalf("开关关着时库区册里有 %d 笔修订，want 0", got)
	}
}
