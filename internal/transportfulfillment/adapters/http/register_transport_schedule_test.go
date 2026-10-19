package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证周期安排与具体班次两口（ADR-0201）的传输层纪律：新落一版 201、其余答案 200 且 outcome 区分；载荷逐字段
// 翻译、租户只取认证结果、自报租户与解不出的时刻答坏报文；未配置 Intake 不读体。编排是真处理器接内存册子替身。
// 夹具全为合成值（S 级）。

type scheduleTemplateStore struct {
	current map[string]ports.ScheduleTemplateRecord
}

func (store *scheduleTemplateStore) FindCurrentTemplate(_ context.Context, key ports.ScheduleTemplateKey) (ports.ScheduleTemplateRecord, bool, error) {
	record, found := store.current[key.TenantID.String()+"|"+key.Code.String()]
	return record, found, nil
}

func (store *scheduleTemplateStore) SaveTemplateVersion(_ context.Context, record ports.ScheduleTemplateRecord) (ports.ScheduleTemplateSaveOutcome, error) {
	store.current[record.Key.TenantID.String()+"|"+record.Key.Code.String()] = record
	return ports.ScheduleTemplateSaved, nil
}

type transportScheduleStore struct {
	current map[string]ports.ScheduleRecord
	findErr error
}

func (store *transportScheduleStore) FindByKey(_ context.Context, key ports.ScheduleKey) (ports.ScheduleRecord, bool, error) {
	if store.findErr != nil {
		return ports.ScheduleRecord{}, false, store.findErr
	}
	record, found := store.current[key.TenantID.String()+"|"+key.Schedule.String()]
	return record, found, nil
}

func (store *transportScheduleStore) Save(_ context.Context, record ports.ScheduleRecord) (ports.ScheduleSaveOutcome, error) {
	store.current[record.Key.TenantID.String()+"|"+record.Key.Schedule.String()] = record
	return ports.ScheduleSaved, nil
}

type scheduleClock struct{}

func (scheduleClock) Now() time.Time { return time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC) }

func scheduleRegistrar() (*application.RegisterTransportScheduleHandler, *transportScheduleStore) {
	schedules := &transportScheduleStore{current: map[string]ports.ScheduleRecord{}}
	return application.NewRegisterTransportScheduleHandler(application.RegisterTransportScheduleDeps{
		Templates: &scheduleTemplateStore{current: map[string]ports.ScheduleTemplateRecord{}},
		Schedules: schedules,
		Clock:     scheduleClock{},
	}), schedules
}

const scheduleTemplateBody = `{"template": "SYN-TPL-1", "connection": "SYN-CONN-1", "fromNode": "SYN-NODE-A",
	"toNode": "SYN-NODE-B", "weekdays": ["MON", "WED"], "departureLocalTime": "22:00", "durationMinutes": 600,
	"timeZone": "Asia/Shanghai", "serviceScope": "SYN-SCOPE-01", "effectiveFrom": "2026-01-01"}`

func postSchedule(t *testing.T, handler http.Handler, body string) (*httptest.ResponseRecorder, map[string]json.RawMessage) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, registryRequest("t", body))
	var fields map[string]json.RawMessage
	if response.Code < 300 {
		if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
			t.Fatalf("decode %s: %v", response.Body.String(), err)
		}
	}
	return response, fields
}

// Covers: ADR-0022 与 ADR-0201 决定四——首登 201 带回那一版；同内容再登 200 已存在；异内容 201 下一版。
func TestScheduleTemplateEndpointAnswersByWhetherAVersionWasPersisted(t *testing.T) {
	registrar, _ := scheduleRegistrar()
	endpoint := tfhttp.NewRegisterScheduleTemplateEndpoint(registryIntake(t), registrar)

	response, fields := postSchedule(t, endpoint, scheduleTemplateBody)
	if response.Code != http.StatusCreated || string(fields["outcome"]) != `"TEMPLATE_REGISTERED"` {
		t.Fatalf("首登 = %d %s", response.Code, response.Body.String())
	}
	var template map[string]json.RawMessage
	if err := json.Unmarshal(fields["template"], &template); err != nil {
		t.Fatalf("decode template: %v", err)
	}
	if string(template["version"]) != `1` || string(template["weekdays"]) != `["MON","WED"]` ||
		string(template["departureLocalTime"]) != `"22:00"` || string(template["connection"]) != `"SYN-CONN-1"` {
		t.Fatalf("周期安排回显走样：%s", response.Body.String())
	}

	response, fields = postSchedule(t, endpoint, scheduleTemplateBody)
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"EXISTING_TEMPLATE"` {
		t.Fatalf("同内容再登 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postSchedule(t, endpoint, strings.Replace(scheduleTemplateBody, `"22:00"`, `"23:00"`, 1))
	if response.Code != http.StatusCreated || string(fields["outcome"]) != `"TEMPLATE_REVISED"` {
		t.Fatalf("异内容再登 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postSchedule(t, endpoint, strings.Replace(scheduleTemplateBody, `"MON"`, `"MONDAY"`, 1))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"INPUT_NOT_ACCEPTED"` {
		t.Fatalf("文法外的星期词 = %d %s（形成了的业务答案，不是坏报文）", response.Code, response.Body.String())
	}
}

// Covers: ADR-0201 决定五与 ADR-0022——按日期物化 201 带回派生身份与出处；周期安排不在册、那天不开各是 200 一格；
// 册子读不动答 200 未决带续办引用。
func TestTransportScheduleEndpointMaterializesAndAnswersEachGrade(t *testing.T) {
	registrar, schedules := scheduleRegistrar()
	templateEndpoint := tfhttp.NewRegisterScheduleTemplateEndpoint(registryIntake(t), registrar)
	endpoint := tfhttp.NewRegisterTransportScheduleEndpoint(registryIntake(t), registrar)

	response, fields := postSchedule(t, endpoint, `{"template": "SYN-TPL-1", "runningDate": "2026-10-21"}`)
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"SCHEDULE_TEMPLATE_NOT_FOUND"` {
		t.Fatalf("周期安排不在册 = %d %s", response.Code, response.Body.String())
	}
	if response, _ := postSchedule(t, templateEndpoint, scheduleTemplateBody); response.Code != http.StatusCreated {
		t.Fatalf("登周期安排 = %d %s", response.Code, response.Body.String())
	}

	response, fields = postSchedule(t, endpoint, `{"template": "SYN-TPL-1", "runningDate": "2026-10-21"}`)
	if response.Code != http.StatusCreated || string(fields["outcome"]) != `"SCHEDULE_REGISTERED"` {
		t.Fatalf("物化 = %d %s", response.Code, response.Body.String())
	}
	var schedule map[string]json.RawMessage
	if err := json.Unmarshal(fields["schedule"], &schedule); err != nil {
		t.Fatalf("decode schedule: %v", err)
	}
	if string(schedule["scheduleId"]) != `"SYN-TPL-1/2026-10-21"` || string(schedule["origin"]) != `"TEMPLATE"` ||
		string(schedule["runningDate"]) != `"2026-10-21"` || string(schedule["templateVersion"]) != `1` ||
		string(schedule["departsAt"]) != `"2026-10-21T14:00:00Z"` || string(schedule["arrivesAt"]) != `"2026-10-22T00:00:00Z"` {
		t.Fatalf("班次回显走样：%s", response.Body.String())
	}
	response, fields = postSchedule(t, endpoint, `{"template": "SYN-TPL-1", "runningDate": "2026-10-21"}`)
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"EXISTING_SCHEDULE"` {
		t.Fatalf("同一日期再登 = %d %s", response.Code, response.Body.String())
	}
	response, fields = postSchedule(t, endpoint, `{"template": "SYN-TPL-1", "runningDate": "2026-10-22"}`)
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"SCHEDULE_TEMPLATE_NOT_RUNNING"` {
		t.Fatalf("那天不开 = %d %s", response.Code, response.Body.String())
	}

	schedules.findErr = errors.New("registry down")
	response, fields = postSchedule(t, endpoint, `{"template": "SYN-TPL-1", "runningDate": "2026-10-21"}`)
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"REGISTRATION_UNDECIDED"` ||
		string(fields["undecidedReason"]) != `"TRANSPORT_SCHEDULE_REGISTRY_UNAVAILABLE"` || len(fields["continuationReference"]) == 0 {
		t.Fatalf("册子读不动 = %d %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0201 决定一——载荷逐字段翻译、租户取认证结果；自报租户与解不出的时刻答坏报文（400）；未出示令牌在解
// 载荷之前就答凭证被拒；未配置 Intake 一律 403。
func TestScheduleRegistrationIntakesTranslateFieldByField(t *testing.T) {
	intake := registryIntake(t)
	command, err := intake.IntakeTransportScheduleRegistration(context.Background(), registryRequest("t",
		`{"schedule": "SYN-ADHOC-1", "connection": "SYN-CONN-1", "fromNode": "SYN-NODE-A", "toNode": "SYN-NODE-B",
		  "departsAt": "2026-10-21T02:00:00Z", "arrivesAt": "2026-10-21T05:00:00Z", "serviceScope": "SYN-SCOPE-01"}`))
	if err != nil {
		t.Fatalf("临时班次翻译：%v", err)
	}
	if command.TenantID.String() != "SYN-TENANT-01" || command.Schedule != "SYN-ADHOC-1" ||
		!command.ArrivesAt.Equal(time.Date(2026, 10, 21, 5, 0, 0, 0, time.UTC)) || command.Template != "" {
		t.Fatalf("command = %+v", command)
	}
	templateCommand, err := intake.IntakeScheduleTemplateRegistration(context.Background(), registryRequest("t", scheduleTemplateBody))
	if err != nil || templateCommand.TenantID.String() != "SYN-TENANT-01" || len(templateCommand.Weekdays) != 2 ||
		templateCommand.DurationMinutes != 600 || templateCommand.EffectiveUntil != "" {
		t.Fatalf("周期安排翻译 = %+v，%v", templateCommand, err)
	}

	for name, body := range map[string]string{
		"自报租户":    `{"tenantId": "SYN-TENANT-02", "template": "SYN-TPL-1", "runningDate": "2026-10-21"}`,
		"解不出的时刻":  `{"schedule": "SYN-ADHOC-1", "departsAt": "tomorrow"}`,
		"未知键":     `{"template": "SYN-TPL-1", "runningDate": "2026-10-21", "direction": "CN-US"}`,
		"载荷后多一份":  `{"template": "SYN-TPL-1"} {}`,
		"大小写变体租户": `{"TenantID": "SYN-TENANT-02", "template": "SYN-TPL-1"}`,
	} {
		if _, err := intake.IntakeTransportScheduleRegistration(context.Background(), registryRequest("t", body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v，要 ErrMalformedRequest", name, err)
		}
	}
	if _, err := intake.IntakeScheduleTemplateRegistration(context.Background(), registryRequest("t", `{"tenantId": "SYN-TENANT-02"}`)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("周期安排自报租户：err = %v", err)
	}
	if _, err := intake.IntakeScheduleTemplateRegistration(context.Background(), registryRequest("", `not json`)); !errors.Is(err, tfhttp.ErrOperatorCredentialRejected) {
		t.Fatalf("未出示令牌：err = %v", err)
	}

	registrar, _ := scheduleRegistrar()
	for name, endpoint := range map[string]http.Handler{
		"周期安排": tfhttp.NewRegisterScheduleTemplateEndpoint(tfhttp.UnconfiguredIntake{}, registrar),
		"班次":   tfhttp.NewRegisterTransportScheduleEndpoint(tfhttp.UnconfiguredIntake{}, registrar),
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, registryRequest("t", scheduleTemplateBody))
		if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s 未配置 = %d %s", name, response.Code, response.Body.String())
		}
	}
}
