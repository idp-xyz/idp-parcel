package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件的操作者、租户与仓都是隔离合成值，只记 `S`。认证用可控的假件：令牌 token-a 是租户 SYN-TENANT-01 的操作者，token-b 是
// 租户 SYN-TENANT-02 的操作者，两人都持登记册配置写与主数据与运营查阅读的授予；其余一概走生产的铸造器、认证适配器、Intake、
// 编排与真库。

type warehouseOperatorVerifier struct{}

func (warehouseOperatorVerifier) VerifyOperatorCredential(_ context.Context, credential accessidentity.OperatorCredential) (accessidentity.OperatorSubject, error) {
	switch credential.Token() {
	case "token-a":
		return accessidentity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-01")
	case "token-b":
		return accessidentity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-02")
	}
	return accessidentity.OperatorSubject{}, accessidentity.ErrCredentialRejected
}

// twoTenantOperators 是操作者册的替身：SYN-OPERATOR-01 绑 SYN-TENANT-01，SYN-OPERATOR-02 绑 SYN-TENANT-02，各持登记册配置写与
// 主数据与运营查阅读的授予。
type twoTenantOperators struct{}

func (twoTenantOperators) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	tenants := map[string]string{"SYN-OPERATOR-01": "SYN-TENANT-01", "SYN-OPERATOR-02": "SYN-TENANT-02"}
	tenant, known := tenants[subject.Subject()]
	if !known {
		return accessidentity.OperatorStanding{}, false, nil
	}
	binding, err := accessidentity.NewOperatorBinding(subject, tenant, "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	var grants []accessidentity.RecordedGrant
	for index, face := range []accessidentity.CapabilityFace{accessidentity.CapabilityRegistryConfigurationWrite, accessidentity.CapabilityMasterDataAndOperationsRead} {
		grant, err := accessidentity.NewOperatorGrant(tenant, fmt.Sprintf("SYN-GRANT-%d", index), subject, face, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
		recorded, err := accessidentity.NewRecordedGrant(grant, nil)
		if err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewOperatorStanding(binding, grants)
	return standing, err == nil, err
}

// warehouseProductionRouter 经登记处的生产构造（feature.production）出仓配特性的各行，接真库；开关取 warehouseFulfillment。
func warehouseProductionRouter(t *testing.T, db *bentopg.DB, warehouseFulfillment bool) http.Handler {
	t.Helper()
	minter, err := buildOperatorMinter(warehouseOperatorVerifier{}, twoTenantOperators{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*warehouseFulfillmentWiring)(nil)) {
			continue
		}
		endpoints, err := feature.production(
			endpointSources{db: db, warehouseFulfillment: warehouseFulfillment},
			endpointChannels{operatorRegistries: registries})
		if err != nil {
			t.Fatalf("装配仓配特性：%v", err)
		}
		return httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints)
	}
	t.Fatal("登记处没有仓配履约这个特性")
	return nil
}

func warehouseRequest(router http.Handler, method, target, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func warehouseDocument(code string, revision int, name, kind string) string {
	return fmt.Sprintf(`{"warehouseCode":%q,"revision":%d,"name":%q,"kind":%q,`+
		`"address":{"country":"CN","text":"合成路 1 号"},"businessTimeZone":"Asia/Shanghai","colocatedNode":"SYN-NODE-01"}`,
		code, revision, name, kind)
}

type warehouseAnswer struct {
	Outcome        string `json:"outcome"`
	Refusal        string `json:"refusal"`
	Field          string `json:"field"`
	LatestRevision *int   `json:"latestRevision"`
	Warehouse      struct {
		WarehouseCode string `json:"warehouseCode"`
		Revision      int    `json:"revision"`
		Name          string `json:"name"`
		Registrant    string `json:"registrant"`
		RegisteredAt  string `json:"registeredAt"`
	} `json:"warehouse"`
}

func decodeWarehouseAnswer(t *testing.T, response *httptest.ResponseRecorder) warehouseAnswer {
	t.Helper()
	var answer warehouseAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("答复体 %q：%v", response.Body.String(), err)
	}
	return answer
}

type warehouseListAnswer struct {
	Outcome    string `json:"outcome"`
	Warehouses []struct {
		WarehouseCode string `json:"warehouseCode"`
		Revision      int    `json:"revision"`
		Name          string `json:"name"`
		Registrant    string `json:"registrant"`
	} `json:"warehouses"`
	Page struct {
		Total *int64 `json:"total"`
	} `json:"page"`
}

type warehouseRevisionsAnswer struct {
	Outcome       string `json:"outcome"`
	WarehouseCode string `json:"warehouseCode"`
	Revisions     []struct {
		Revision int    `json:"revision"`
		Name     string `json:"name"`
	} `json:"revisions"`
}

// Covers: AT-WF-001（仓的部分）、AT-WF-009（仓的部分）、AT-WF-010、UC-WF-001「权限、隔离与审计」——开关开着，经生产装配、真
// 认证适配器、真编排与真库走通：登记一个仓（201 REGISTERED，登记责任方是操作者的「发行方#sub」）、原样重投（200
// ALREADY_REGISTERED，回显当初那一笔）、同一修订号换内容（200 CONTENT_CONFLICT）、改种类（IMMUTABLE_FIELD）、跳号（修订错位并回显
// 在册最新修订号）、再登记修订 2；列表读到当前修订、修订史读到两笔；另一个租户读不到它（列表空、修订史空数组，与没登记过同答），
// 并且可以登记同编码的仓，互不相干；仓编码逐字保存、逐字读回。
func TestWarehousesAreRegisteredAndReadBackThroughTheProductionAssembly(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, true)
	const code = "SYN-WH-01"
	register := func(token, body string) *httptest.ResponseRecorder {
		return warehouseRequest(router, http.MethodPost, warehouseRegistrationsLine, token, body)
	}

	first := register("token-a", warehouseDocument(code, 1, "合成一号仓", "SELF_OPERATED"))
	if first.Code != http.StatusCreated {
		t.Fatalf("首次登记：%d %s，want 201", first.Code, first.Body.String())
	}
	registered := decodeWarehouseAnswer(t, first)
	if registered.Outcome != "REGISTERED" || registered.Warehouse.WarehouseCode != code || registered.Warehouse.Revision != 1 ||
		registered.Warehouse.Registrant != "https://id.syn.example/dex#SYN-OPERATOR-01" || registered.Warehouse.RegisteredAt == "" {
		t.Fatalf("首次登记的答复 = %s", first.Body.String())
	}

	replayed := register("token-a", warehouseDocument(code, 1, "合成一号仓", "SELF_OPERATED"))
	if replayed.Code != http.StatusOK {
		t.Fatalf("原样重投：%d %s，want 200", replayed.Code, replayed.Body.String())
	}
	if answer := decodeWarehouseAnswer(t, replayed); answer.Outcome != "ALREADY_REGISTERED" || answer.Warehouse.RegisteredAt != registered.Warehouse.RegisteredAt {
		t.Fatalf("原样重投的答复 = %s，want ALREADY_REGISTERED 并回显当初那一笔", replayed.Body.String())
	}

	if conflict := register("token-a", warehouseDocument(code, 1, "合成一号仓（改名）", "SELF_OPERATED")); conflict.Code != http.StatusOK ||
		decodeWarehouseAnswer(t, conflict).Outcome != "CONTENT_CONFLICT" {
		t.Fatalf("同一修订号换内容：%d %s，want 200 CONTENT_CONFLICT", conflict.Code, conflict.Body.String())
	}
	if immutable := register("token-a", warehouseDocument(code, 2, "合成一号仓", "AGENT")); immutable.Code != http.StatusOK {
		t.Fatalf("改种类：%d %s", immutable.Code, immutable.Body.String())
	} else if answer := decodeWarehouseAnswer(t, immutable); answer.Outcome != "NOT_ACCEPTED" || answer.Refusal != "IMMUTABLE_FIELD" || answer.Field != "kind" {
		t.Fatalf("改种类的答复 = %s", immutable.Body.String())
	}
	if skipped := register("token-a", warehouseDocument(code, 4, "合成一号仓", "SELF_OPERATED")); skipped.Code != http.StatusOK {
		t.Fatalf("跳号：%d %s", skipped.Code, skipped.Body.String())
	} else if answer := decodeWarehouseAnswer(t, skipped); answer.Refusal != "REVISION_OUT_OF_SEQUENCE" || answer.LatestRevision == nil || *answer.LatestRevision != 1 {
		t.Fatalf("跳号的答复 = %s，want 回显在册最新修订号 1", skipped.Body.String())
	}
	if renamed := register("token-a", warehouseDocument(code, 2, "合成一号仓（新名）", "SELF_OPERATED")); renamed.Code != http.StatusCreated {
		t.Fatalf("修订 2：%d %s，want 201", renamed.Code, renamed.Body.String())
	}
	// 逐字保存：带首尾空白与大小写混写的仓编码是另一个仓。
	const verbatim = " Syn-Wh 09 "
	if odd := register("token-a", warehouseDocument(verbatim, 1, "合成九号仓", "AGENT")); odd.Code != http.StatusCreated {
		t.Fatalf("逐字仓编码：%d %s", odd.Code, odd.Body.String())
	}

	list := warehouseRequest(router, http.MethodGet, warehouseListLine, "token-a", "")
	var listed warehouseListAnswer
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil || list.Code != http.StatusOK {
		t.Fatalf("列表：%d %s（%v）", list.Code, list.Body.String(), err)
	}
	if listed.Outcome != "WAREHOUSES_LISTED" || len(listed.Warehouses) != 2 || listed.Page.Total == nil || *listed.Page.Total != 2 {
		t.Fatalf("列表 = %s，want 两个仓（各自的当前修订）", list.Body.String())
	}
	byCode := map[string]int{}
	names := map[string]string{}
	for _, row := range listed.Warehouses {
		byCode[row.WarehouseCode] = row.Revision
		names[row.WarehouseCode] = row.Name
	}
	if byCode[code] != 2 || names[code] != "合成一号仓（新名）" || byCode[verbatim] != 1 {
		t.Fatalf("列表读到的当前修订 = %v %v，want %s 为修订 2、%q 逐字为修订 1", byCode, names, code, verbatim)
	}

	history := warehouseRequest(router, http.MethodGet, warehouseRevisionsLine+"?warehouseCode="+code, "token-a", "")
	var revisions warehouseRevisionsAnswer
	if err := json.Unmarshal(history.Body.Bytes(), &revisions); err != nil || history.Code != http.StatusOK {
		t.Fatalf("修订史：%d %s（%v）", history.Code, history.Body.String(), err)
	}
	if revisions.Outcome != "WAREHOUSE_REVISIONS_LISTED" || revisions.WarehouseCode != code || len(revisions.Revisions) != 2 ||
		revisions.Revisions[0].Revision != 1 || revisions.Revisions[1].Revision != 2 || revisions.Revisions[0].Name != "合成一号仓" {
		t.Fatalf("修订史 = %s，want 修订 1、2 按序", history.Body.String())
	}
	verbatimHistory := warehouseRequest(router, http.MethodGet, warehouseRevisionsLine+"?warehouseCode="+url.QueryEscape(verbatim), "token-a", "")
	var oddRevisions warehouseRevisionsAnswer
	if err := json.Unmarshal(verbatimHistory.Body.Bytes(), &oddRevisions); err != nil || oddRevisions.WarehouseCode != verbatim || len(oddRevisions.Revisions) != 1 {
		t.Fatalf("逐字仓编码的修订史 = %s", verbatimHistory.Body.String())
	}

	// 另一个租户读不到它：列表空，修订史是空数组，与从没登记过的仓同答。
	otherList := warehouseRequest(router, http.MethodGet, warehouseListLine, "token-b", "")
	if otherList.Code != http.StatusOK || !strings.Contains(otherList.Body.String(), `"warehouses":[]`) {
		t.Fatalf("另一个租户的列表：%d %s，want 200 与空数组", otherList.Code, otherList.Body.String())
	}
	otherHistory := warehouseRequest(router, http.MethodGet, warehouseRevisionsLine+"?warehouseCode="+code, "token-b", "")
	neverRegistered := warehouseRequest(router, http.MethodGet, warehouseRevisionsLine+"?warehouseCode=SYN-WH-NEVER", "token-b", "")
	if otherHistory.Code != http.StatusOK || !strings.Contains(otherHistory.Body.String(), `"revisions":[]`) {
		t.Fatalf("另一个租户的修订史：%d %s，want 200 与空数组", otherHistory.Code, otherHistory.Body.String())
	}
	if want := strings.ReplaceAll(neverRegistered.Body.String(), "SYN-WH-NEVER", code); neverRegistered.Code != otherHistory.Code || otherHistory.Body.String() != want {
		t.Fatalf("别的租户的同编码仓答 %d %s，与从没登记过的仓答 %d %s 不同：泄露了是否存在",
			otherHistory.Code, otherHistory.Body.String(), neverRegistered.Code, neverRegistered.Body.String())
	}
	// 另一个租户登记同编码的仓从修订 1 起，互不相干；A 的当前修订不变。
	if other := register("token-b", warehouseDocument(code, 1, "另一租户的仓", "AGENT")); other.Code != http.StatusCreated {
		t.Fatalf("另一个租户登记同编码：%d %s，want 201", other.Code, other.Body.String())
	}
	var again warehouseListAnswer
	if err := json.Unmarshal(warehouseRequest(router, http.MethodGet, warehouseListLine, "token-a", "").Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	for _, row := range again.Warehouses {
		if row.WarehouseCode == code && (row.Revision != 2 || row.Name != "合成一号仓（新名）") {
			t.Fatalf("另一个租户登记后，A 的 %s = %+v，want 不变", code, row)
		}
	}

	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.warehouse_revision WHERE tenant_id = $1`, "SYN-TENANT-01"); got != 3 {
		t.Fatalf("SYN-TENANT-01 在册 %d 笔修订，want 3（%s 两笔、逐字仓一笔；冲突、改种类与跳号都不落册）", got, code)
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.warehouse_revision WHERE tenant_id = $1`, "SYN-TENANT-02"); got != 1 {
		t.Fatalf("SYN-TENANT-02 在册 %d 笔修订，want 1", got)
	}
}

// Covers: AT-WF-012（端点那一半）——开关关着时，同样的真库与真渠道（授予齐备、令牌有效）打仓配的各行，答 403
// WAREHOUSE_FULFILLMENT_NOT_ENABLED，册里一行也没落：登记口没有走到任何编排，库表（空表）原样。
func TestSwitchedOffWarehouseFulfillmentNeverReachesTheRegisterOnARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	router := warehouseProductionRouter(t, db, false)

	registration := warehouseRequest(router, http.MethodPost, warehouseRegistrationsLine, "token-a", warehouseDocument("SYN-WH-01", 1, "合成一号仓", "SELF_OPERATED"))
	list := warehouseRequest(router, http.MethodGet, warehouseListLine, "token-a", "")
	history := warehouseRequest(router, http.MethodGet, warehouseRevisionsLine+"?warehouseCode=SYN-WH-01", "token-a", "")
	for name, response := range map[string]*httptest.ResponseRecorder{"registration": registration, "list": list, "history": history} {
		if response.Code != http.StatusForbidden || problemCode(t, response) != "WAREHOUSE_FULFILLMENT_NOT_ENABLED" {
			t.Fatalf("%s：答 %d %s，want 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED", name, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response, name)
	}
	if got := countOf(t, pool, `SELECT count(*) FROM warehouse_fulfillment.warehouse_revision`); got != 0 {
		t.Fatalf("开关关着时册里有 %d 笔修订，want 0", got)
	}
}
