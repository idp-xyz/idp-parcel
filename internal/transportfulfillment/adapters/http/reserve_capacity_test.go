package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证容量预占口与释放口（ADR-0203 决定三至六）的传输层纪律：新预占、新释放 201，其余答案 200 且 outcome 区分；
// 各格交回的池、预占、释放与余量逐字段透出；载荷逐字段翻译、租户只取认证结果、带越过上限一类键是坏报文。编排是真处理器
// 接内存池册替身。夹具全为合成值（S 级）：池 SYN-TPL-1/2026-10-21/WEIGHT 容量 5000 KG。

const decisionPool = "SYN-TPL-1/2026-10-21/WEIGHT"

// decisionPoolStore 是能换存的内存池册。
type decisionPoolStore struct {
	current map[string]ports.CapacityPoolRecord
	findErr error
}

func (store *decisionPoolStore) FindByKey(_ context.Context, key ports.CapacityPoolKey) (ports.CapacityPoolRecord, bool, error) {
	if store.findErr != nil {
		return ports.CapacityPoolRecord{}, false, store.findErr
	}
	record, found := store.current[key.TenantID.String()+"|"+key.Pool.String()]
	return record, found, nil
}

func (store *decisionPoolStore) Save(_ context.Context, record ports.CapacityPoolRecord) (ports.PoolSaveOutcome, error) {
	store.current[record.Key.TenantID.String()+"|"+record.Key.Pool.String()] = record
	return ports.PoolSaved, nil
}

func (store *decisionPoolStore) Replace(_ context.Context, record ports.CapacityPoolRecord) (bool, error) {
	key := record.Key.TenantID.String() + "|" + record.Key.Pool.String()
	if _, found := store.current[key]; !found {
		return false, nil
	}
	store.current[key] = record
	return true, nil
}

// capacityDecisions 交回接着一个 5000 KG 池的编排与它的池册。
func capacityDecisions(t *testing.T) (*application.PrepareTransportOpportunityHandler, *decisionPoolStore) {
	t.Helper()
	tenant, err := domain.NewTenantID(operatorTenant)
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := domain.NewScheduleReference("SYN-TPL-1/2026-10-21")
	if err != nil {
		t.Fatal(err)
	}
	unit, err := domain.NewQuantityUnitReference("KG")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := domain.EstablishCapacityPool(domain.CapacityPoolSpec{
		TenantID: tenant, Schedule: schedule, Dimension: domain.CapacityWeight, Unit: unit, Capacity: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &decisionPoolStore{current: map[string]ports.CapacityPoolRecord{}}
	if _, err := store.Save(context.Background(), ports.CapacityPoolRecord{
		Key: ports.CapacityPoolKey{TenantID: tenant, Pool: pool.Pool()}, ContentDigest: "d", Pool: pool,
		RecordedAt: time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	return application.NewPrepareTransportOpportunityHandler(application.PrepareTransportOpportunityDeps{
		Pools: store,
		Clock: scheduleClock{},
	}), store
}

func reservationBody(reservation string, quantity int, unit string) string {
	return `{"pool": "` + decisionPool + `", "reservation": "` + reservation + `", "quantity": ` + strconv.Itoa(quantity) +
		`, "unit": "` + unit + `", "source": {"kind": "OPERATOR_DECLARATION", "reference": "SYN-DEMAND-DECL-01/v1"},` +
		` "validUntil": "2026-10-21T12:00:00Z", "reservedAt": "2026-10-19T08:00:00Z"}`
}

func releaseBody(release, reservation string, quantity int) string {
	return `{"pool": "` + decisionPool + `", "reservation": "` + reservation + `", "release": "` + release +
		`", "quantity": ` + strconv.Itoa(quantity) + `, "unit": "KG", "source": {"kind": "OPERATOR_DECLARATION", "reference": "SYN-DEMAND-DECL-01/v2"},` +
		` "releasedAt": "2026-10-20T08:00:00Z"}`
}

func postDecision(t *testing.T, handler http.Handler, token, body string) (*httptest.ResponseRecorder, map[string]json.RawMessage) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, bearerRequest(token, body))
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	return response, fields
}

func nested(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return fields
}

// Covers: ADR-0022 与 `AT-TF-137` 至 `AT-TF-140`——池不在册 200 POOL_NOT_FOUND；新预占 201 带回池与那一份预占（三量、单位、
// 来源、判断时点、有效期逐字段）；原样重交 200 已有预占；异内容 200 预占冲突并带回在册那一份；单位不符 200 带回池；超出
// 硬上限 200 带余量；来源种类不在封闭词里 200 未受理；池册读不动 200 未决带续办引用。
func TestCapacityReservationEndpointAnswersEachGrade(t *testing.T) {
	handler, store := capacityDecisions(t)
	endpoint := tfhttp.NewReserveCapacityEndpoint(operatorIntake(t, authenticatorFake{}), handler)

	response, fields := postDecision(t, endpoint, "t", strings.Replace(reservationBody("SYN-RES-1", 1200, "KG"), decisionPool, "SYN-TPL-1/2026-10-22/WEIGHT", 1))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"POOL_NOT_FOUND"` || fields["pool"] != nil {
		t.Fatalf("池不在册 = %d %s", response.Code, response.Body.String())
	}

	response, fields = postDecision(t, endpoint, "t", reservationBody("SYN-RES-1", 1200, "KG"))
	if response.Code != http.StatusCreated || string(fields["outcome"]) != `"CAPACITY_RESERVED"` {
		t.Fatalf("预占 = %d %s", response.Code, response.Body.String())
	}
	pool := nested(t, fields["pool"])
	if string(pool["poolId"]) != `"`+decisionPool+`"` || string(pool["unit"]) != `"KG"` || string(pool["capacity"]) != `"5000"` {
		t.Fatalf("pool = %s", fields["pool"])
	}
	reservation := nested(t, fields["reservation"])
	for key, want := range map[string]string{
		"reservation": `"SYN-RES-1"`, "quantity": `"1200"`, "released": `"0"`, "consumed": `"0"`, "unit": `"KG"`,
		"reservedAt": `"2026-10-19T08:00:00Z"`, "validUntil": `"2026-10-21T12:00:00Z"`,
		"source": `{"kind":"OPERATOR_DECLARATION","reference":"SYN-DEMAND-DECL-01/v1"}`,
	} {
		if string(reservation[key]) != want {
			t.Fatalf("reservation.%s = %s, want %s", key, reservation[key], want)
		}
	}
	if fields["release"] != nil || fields["availableQuantity"] != nil {
		t.Fatalf("预占口不该带 release 或 availableQuantity：%s", response.Body.String())
	}

	response, fields = postDecision(t, endpoint, "t", reservationBody("SYN-RES-1", 1200, "KG"))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"EXISTING_RESERVATION"` || fields["reservation"] == nil {
		t.Fatalf("原样重交 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postDecision(t, endpoint, "t", reservationBody("SYN-RES-1", 1300, "KG"))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"RESERVATION_CONFLICT"` ||
		string(nested(t, fields["reservation"])["quantity"]) != `"1200"` {
		t.Fatalf("异内容 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postDecision(t, endpoint, "t", reservationBody("SYN-RES-2", 100, "T"))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"UNIT_MISMATCH"` || fields["pool"] == nil || fields["reservation"] != nil {
		t.Fatalf("单位不符 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postDecision(t, endpoint, "t", reservationBody("SYN-RES-2", 3801, "KG"))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"CAPACITY_EXCEEDED"` ||
		string(fields["availableQuantity"]) != `"3800"` || fields["pool"] == nil {
		t.Fatalf("超出硬上限 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postDecision(t, endpoint, "t", strings.Replace(reservationBody("SYN-RES-2", 10, "KG"), "OPERATOR_DECLARATION", "MEMBER_COUNT", 1))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"INPUT_NOT_ACCEPTED"` {
		t.Fatalf("来源种类不在封闭词里 = %d %s（形成了的业务答案，不是坏报文）", response.Code, response.Body.String())
	}

	store.findErr = errors.New("registry down")
	response, fields = postDecision(t, endpoint, "t", reservationBody("SYN-RES-3", 10, "KG"))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"OPPORTUNITY_UNDECIDED"` ||
		string(fields["undecidedReason"]) != `"POOL_STORE_UNAVAILABLE"` || fields["continuationReference"] == nil {
		t.Fatalf("池册读不动 = %d %s", response.Code, response.Body.String())
	}
}

// Covers: `AT-TF-139` 与 `AT-TF-031` 的传输层半边——载荷没有越过硬上限的键，带 `approval`、`override` 一类键的载荷是坏报文
// （400），走不到编排；自报租户同样是坏报文；未出示令牌 401，认证先于解载荷。
func TestCapacityDecisionPayloadsHaveNoOverrideAndNoTenant(t *testing.T) {
	handler, _ := capacityDecisions(t)
	reserve := tfhttp.NewReserveCapacityEndpoint(operatorIntake(t, authenticatorFake{}), handler)
	release := tfhttp.NewReleaseCapacityEndpoint(operatorIntake(t, authenticatorFake{}), handler)
	smuggled := map[string]struct {
		endpoint http.Handler
		body     string
	}{
		"approval":        {reserve, strings.Replace(reservationBody("SYN-RES-1", 9999, "KG"), `{"pool"`, `{"approval": "SYN-APPROVAL-1", "pool"`, 1)},
		"override":        {reserve, strings.Replace(reservationBody("SYN-RES-1", 9999, "KG"), `{"pool"`, `{"override": true, "pool"`, 1)},
		"tenant":          {reserve, strings.Replace(reservationBody("SYN-RES-1", 10, "KG"), `{"pool"`, `{"tenantId": "SYN-TENANT-02", "pool"`, 1)},
		"bad instant":     {reserve, strings.Replace(reservationBody("SYN-RES-1", 10, "KG"), "2026-10-19T08:00:00Z", "yesterday", 1)},
		"string quantity": {reserve, strings.Replace(reservationBody("SYN-RES-1", 10, "KG"), `"quantity": 10`, `"quantity": "10"`, 1)},
		"release tenant":  {release, strings.Replace(releaseBody("SYN-REL-1", "SYN-RES-1", 10), `{"pool"`, `{"tenantId": "SYN-TENANT-02", "pool"`, 1)},
	}
	for name, testCase := range smuggled {
		response := httptest.NewRecorder()
		testCase.endpoint.ServeHTTP(response, bearerRequest("t", testCase.body))
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), `"outcome"`) {
			t.Fatalf("%s = %d %s, want 400 without outcome", name, response.Code, response.Body.String())
		}
	}
	for name, endpoint := range map[string]http.Handler{"reserve": reserve, "release": release} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, bearerRequest("", "!!not-json!!"))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s 未出示令牌 = %d %s", name, response.Code, response.Body.String())
		}
	}
}

// Covers: `AT-TF-141`、`AT-TF-142` 的传输层半边——新释放 201 带回那一次释放与它所落的预占（已释放随之增加）；原样重交
// 200 已有释放、不再还一份；同一释放引用改了数量 200 释放冲突并带回在册那一次；预占不在册 200；守恒撞线 200 带回那一份
// 预占的三量；释放时点早于预占的判断时点 200 预占尚未生效，带回那一份预占的判断时点。
func TestCapacityReleaseEndpointAnswersEachGrade(t *testing.T) {
	handler, _ := capacityDecisions(t)
	reserve := tfhttp.NewReserveCapacityEndpoint(operatorIntake(t, authenticatorFake{}), handler)
	endpoint := tfhttp.NewReleaseCapacityEndpoint(operatorIntake(t, authenticatorFake{}), handler)
	if response, _ := postDecision(t, reserve, "t", reservationBody("SYN-RES-1", 1200, "KG")); response.Code != http.StatusCreated {
		t.Fatalf("预占 = %d %s", response.Code, response.Body.String())
	}

	response, fields := postDecision(t, endpoint, "t", releaseBody("SYN-REL-1", "SYN-RES-1", 200))
	if response.Code != http.StatusCreated || string(fields["outcome"]) != `"CAPACITY_RELEASED"` {
		t.Fatalf("释放 = %d %s", response.Code, response.Body.String())
	}
	release := nested(t, fields["release"])
	for key, want := range map[string]string{
		"release": `"SYN-REL-1"`, "reservation": `"SYN-RES-1"`, "quantity": `"200"`, "unit": `"KG"`,
		"releasedAt": `"2026-10-20T08:00:00Z"`, "source": `{"kind":"OPERATOR_DECLARATION","reference":"SYN-DEMAND-DECL-01/v2"}`,
	} {
		if string(release[key]) != want {
			t.Fatalf("release.%s = %s, want %s", key, release[key], want)
		}
	}
	if string(nested(t, fields["reservation"])["released"]) != `"200"` {
		t.Fatalf("释放后预占 = %s", fields["reservation"])
	}

	response, fields = postDecision(t, endpoint, "t", releaseBody("SYN-REL-1", "SYN-RES-1", 200))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"EXISTING_RELEASE"` ||
		string(nested(t, fields["reservation"])["released"]) != `"200"` {
		t.Fatalf("原样重交 = %d %s（不再还一份）", response.Code, response.Body.String())
	}
	response, fields = postDecision(t, endpoint, "t", releaseBody("SYN-REL-1", "SYN-RES-1", 300))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"RELEASE_CONFLICT"` ||
		string(nested(t, fields["release"])["quantity"]) != `"200"` {
		t.Fatalf("异内容 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postDecision(t, endpoint, "t", releaseBody("SYN-REL-2", "SYN-RES-9", 1))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"RESERVATION_NOT_FOUND"` || fields["reservation"] != nil {
		t.Fatalf("预占不在册 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postDecision(t, endpoint, "t", releaseBody("SYN-REL-3", "SYN-RES-1", 1001))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"RESERVATION_OVERDRAWN"` ||
		string(nested(t, fields["reservation"])["released"]) != `"200"` || fields["release"] != nil {
		t.Fatalf("守恒撞线 = %d %s", response.Code, response.Body.String())
	}
	early := strings.Replace(releaseBody("SYN-REL-4", "SYN-RES-1", 1), "2026-10-20T08:00:00Z", "2026-10-19T07:00:00Z", 1)
	response, fields = postDecision(t, endpoint, "t", early)
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"RESERVATION_NOT_YET_EFFECTIVE"` ||
		string(nested(t, fields["reservation"])["reservedAt"]) != `"2026-10-19T08:00:00Z"` || fields["release"] != nil {
		t.Fatalf("早于判断时点 = %d %s", response.Code, response.Body.String())
	}
}

// Covers: 两口的载荷逐字段翻译成命令，租户取认证结果。
func TestCapacityDecisionsTranslateFieldByField(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	reserve, err := intake.IntakeCapacityReservation(context.Background(), bearerRequest("t", reservationBody("SYN-RES-1", 1200, "KG")))
	if err != nil {
		t.Fatal(err)
	}
	if reserve.TenantID.String() != operatorTenant || reserve.Pool != decisionPool || reserve.Reservation != "SYN-RES-1" ||
		reserve.Quantity != 1200 || reserve.Unit != "KG" || reserve.SourceKind != "OPERATOR_DECLARATION" ||
		reserve.SourceReference != "SYN-DEMAND-DECL-01/v1" ||
		!reserve.ValidUntil.Equal(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)) ||
		!reserve.At.Equal(time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("reserve command = %+v", reserve)
	}
	release, err := intake.IntakeCapacityRelease(context.Background(), bearerRequest("t", releaseBody("SYN-REL-1", "SYN-RES-1", 200)))
	if err != nil {
		t.Fatal(err)
	}
	if release.TenantID.String() != operatorTenant || release.Pool != decisionPool || release.Reservation != "SYN-RES-1" ||
		release.Release != "SYN-REL-1" || release.Quantity != 200 || release.Unit != "KG" ||
		release.SourceKind != "OPERATOR_DECLARATION" || release.SourceReference != "SYN-DEMAND-DECL-01/v2" ||
		!release.At.Equal(time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("release command = %+v", release)
	}
}
