package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证容量池登记口（ADR-0201 决定一）与物化登记带池（决定三）的传输层纪律：新建了池 201、其余答案 200 且
// outcome 区分；载荷逐字段翻译、租户只取认证结果；未配置 Intake 不读体。编排是真处理器接内存册子替身。夹具全为
// 合成值（S 级）。

type capacityPoolStore struct {
	current map[string]ports.CapacityPoolRecord
	findErr error
}

func (store *capacityPoolStore) FindByKey(_ context.Context, key ports.CapacityPoolKey) (ports.CapacityPoolRecord, bool, error) {
	if store.findErr != nil {
		return ports.CapacityPoolRecord{}, false, store.findErr
	}
	record, found := store.current[key.TenantID.String()+"|"+key.Pool.String()]
	return record, found, nil
}

func (store *capacityPoolStore) Save(_ context.Context, record ports.CapacityPoolRecord) (ports.PoolSaveOutcome, error) {
	store.current[record.Key.TenantID.String()+"|"+record.Key.Pool.String()] = record
	return ports.PoolSaved, nil
}

func (store *capacityPoolStore) Replace(context.Context, ports.CapacityPoolRecord) (bool, error) {
	return false, errors.New("建池口不该转换池")
}

func poolRegistrar(schedules *transportScheduleStore) (*application.PrepareTransportOpportunityHandler, *capacityPoolStore) {
	pools := &capacityPoolStore{current: map[string]ports.CapacityPoolRecord{}}
	return application.NewPrepareTransportOpportunityHandler(application.PrepareTransportOpportunityDeps{
		Pools:     pools,
		Schedules: schedules,
		Clock:     scheduleClock{},
	}), pools
}

const capacityPoolBody = `{"schedule": "SYN-TPL-1/2026-10-21", "dimension": "WEIGHT", "unit": "KG", "capacity": 5000}`

// Covers: ADR-0022、ADR-0201 决定一与 `AT-TF-105`——班次不在册答 200 SCHEDULE_NOT_FOUND；班次登了之后建池 201 带回
// 派生的池身份与维度，同内容再登 200 已存在，异内容 200 池冲突并带回库里那一池；文法外的维度 200 未受理；池册读不动
// 200 未决带续办引用。
func TestCapacityPoolEndpointAnswersEachGrade(t *testing.T) {
	scheduleHandler, schedules := scheduleRegistrar()
	registrar, pools := poolRegistrar(schedules)
	endpoint := tfhttp.NewRegisterCapacityPoolEndpoint(registryIntake(t), registrar)

	response, fields := postSchedule(t, endpoint, capacityPoolBody)
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"SCHEDULE_NOT_FOUND"` || fields["pool"] != nil {
		t.Fatalf("班次不在册 = %d %s", response.Code, response.Body.String())
	}

	templates := tfhttp.NewRegisterScheduleTemplateEndpoint(registryIntake(t), scheduleHandler)
	if response, _ := postSchedule(t, templates, scheduleTemplateBody); response.Code != http.StatusCreated {
		t.Fatalf("登周期安排 = %d %s", response.Code, response.Body.String())
	}
	materialize := tfhttp.NewRegisterTransportScheduleEndpoint(registryIntake(t), scheduleHandler)
	if response, _ := postSchedule(t, materialize, `{"template": "SYN-TPL-1", "runningDate": "2026-10-21"}`); response.Code != http.StatusCreated {
		t.Fatalf("登班次 = %d %s", response.Code, response.Body.String())
	}

	response, fields = postSchedule(t, endpoint, capacityPoolBody)
	if response.Code != http.StatusCreated || string(fields["outcome"]) != `"POOL_ESTABLISHED"` {
		t.Fatalf("建池 = %d %s", response.Code, response.Body.String())
	}
	var pool map[string]json.RawMessage
	if err := json.Unmarshal(fields["pool"], &pool); err != nil {
		t.Fatalf("decode pool: %v", err)
	}
	if string(pool["poolId"]) != `"SYN-TPL-1/2026-10-21/WEIGHT"` || string(pool["schedule"]) != `"SYN-TPL-1/2026-10-21"` ||
		string(pool["dimension"]) != `"WEIGHT"` || string(pool["unit"]) != `"KG"` || string(pool["capacity"]) != `"5000"` {
		t.Fatalf("池回显走样：%s", response.Body.String())
	}

	response, fields = postSchedule(t, endpoint, capacityPoolBody)
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"EXISTING_POOL"` {
		t.Fatalf("同内容再登 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postSchedule(t, endpoint, strings.Replace(capacityPoolBody, `5000`, `6000`, 1))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"POOL_CONFLICT"` ||
		!strings.Contains(string(fields["pool"]), `"capacity":"5000"`) {
		t.Fatalf("异内容再登 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postSchedule(t, endpoint, strings.Replace(capacityPoolBody, `"WEIGHT"`, `"weight"`, 1))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"SOURCE_NOT_ACCEPTED"` {
		t.Fatalf("文法外的维度 = %d %s（形成了的业务答案，不是坏报文）", response.Code, response.Body.String())
	}

	pools.findErr = errors.New("registry down")
	response, fields = postSchedule(t, endpoint, capacityPoolBody)
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"OPPORTUNITY_UNDECIDED"` ||
		string(fields["undecidedReason"]) != `"POOL_STORE_UNAVAILABLE"` || len(fields["continuationReference"]) == 0 {
		t.Fatalf("池册读不动 = %d %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0201 决定三与 ADR-0022——周期安排带逐维容量声明登记，回显带 capacities；按日期物化登记一班 201，答案
// 逐池列出各自 outcome；同一日期再登 200，池都已存在；周期安排换版只新加一维，再登同一日期班次已存在而新池建立，
// 新落了池就是 201。
func TestMaterializedScheduleResponseListsItsPools(t *testing.T) {
	registrar, _ := scheduleRegistrar()
	templates := tfhttp.NewRegisterScheduleTemplateEndpoint(registryIntake(t), registrar)
	endpoint := tfhttp.NewRegisterTransportScheduleEndpoint(registryIntake(t), registrar)
	declared := strings.Replace(scheduleTemplateBody, `"effectiveFrom": "2026-01-01"}`,
		`"effectiveFrom": "2026-01-01", "capacities": [{"dimension": "WEIGHT", "unit": "KG", "capacity": 5000}]}`, 1)

	response, fields := postSchedule(t, templates, declared)
	if response.Code != http.StatusCreated || !strings.Contains(string(fields["template"]),
		`"capacities":[{"dimension":"WEIGHT","unit":"KG","capacity":"5000"}]`) {
		t.Fatalf("登带容量的周期安排 = %d %s", response.Code, response.Body.String())
	}

	materialize := `{"template": "SYN-TPL-1", "runningDate": "2026-10-21"}`
	response, fields = postSchedule(t, endpoint, materialize)
	if response.Code != http.StatusCreated || string(fields["outcome"]) != `"SCHEDULE_REGISTERED"` ||
		!strings.Contains(string(fields["pools"]), `"outcome":"POOL_ESTABLISHED","poolId":"SYN-TPL-1/2026-10-21/WEIGHT"`) {
		t.Fatalf("物化登记 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postSchedule(t, endpoint, materialize)
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"EXISTING_SCHEDULE"` ||
		!strings.Contains(string(fields["pools"]), `"outcome":"EXISTING_POOL"`) {
		t.Fatalf("同一日期再登 = %d %s", response.Code, response.Body.String())
	}

	widened := strings.Replace(declared, `"capacity": 5000}]`,
		`"capacity": 5000}, {"dimension": "PIECES", "unit": "PCS", "capacity": 800}]`, 1)
	if response, _ := postSchedule(t, templates, widened); response.Code != http.StatusCreated {
		t.Fatalf("周期安排换版 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postSchedule(t, endpoint, materialize)
	if response.Code != http.StatusCreated || string(fields["outcome"]) != `"EXISTING_SCHEDULE"` ||
		!strings.Contains(string(fields["pools"]), `"outcome":"POOL_ESTABLISHED","poolId":"SYN-TPL-1/2026-10-21/PIECES"`) {
		t.Fatalf("换版后再登 = %d %s（新落了池就是 201）", response.Code, response.Body.String())
	}
}

// Covers: ADR-0201 决定一——建池载荷逐字段翻译、租户取认证结果；自报租户与未知键答坏报文（400 一族）；未出示令牌在
// 解载荷之前就答凭证被拒；未配置 Intake 一律 403。
func TestCapacityPoolIntakeTranslatesFieldByField(t *testing.T) {
	intake := registryIntake(t)
	command, err := intake.IntakeCapacityPoolRegistration(context.Background(), registryRequest("t", capacityPoolBody))
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}
	if command.TenantID.String() != "SYN-TENANT-01" || command.Schedule != "SYN-TPL-1/2026-10-21" ||
		command.Dimension != "WEIGHT" || command.Unit != "KG" || command.Capacity != 5000 {
		t.Fatalf("command = %+v", command)
	}
	for name, body := range map[string]string{
		"自报租户":   `{"tenantId": "SYN-TENANT-02", "schedule": "S", "dimension": "WEIGHT", "unit": "KG", "capacity": 1}`,
		"自起池名":   `{"pool": "POOL-1", "schedule": "S", "dimension": "WEIGHT", "unit": "KG", "capacity": 1}`,
		"容量不是整数": `{"schedule": "S", "dimension": "WEIGHT", "unit": "KG", "capacity": "5000"}`,
	} {
		if _, err := intake.IntakeCapacityPoolRegistration(context.Background(), registryRequest("t", body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v，要 ErrMalformedRequest", name, err)
		}
	}
	if _, err := intake.IntakeCapacityPoolRegistration(context.Background(), registryRequest("", `not json`)); !errors.Is(err, tfhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("未出示令牌：err = %v", err)
	}

	_, schedules := scheduleRegistrar()
	registrar, _ := poolRegistrar(schedules)
	response := httptest.NewRecorder()
	tfhttp.NewRegisterCapacityPoolEndpoint(tfhttp.UnconfiguredIntake{}, registrar).ServeHTTP(response, registryRequest("t", capacityPoolBody))
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("未配置 = %d %s", response.Code, response.Body.String())
	}
}
