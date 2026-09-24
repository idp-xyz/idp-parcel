package customshttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
)

// 本文件对隔离写路径准入在关务主链命令面的注入式放行（ADR-0091；票 operator-channel/08）证传输面：租户格只来自注入、
// 外部事实的身份与时间照 ADR-0023 从载荷如实收、自报租户即拒、形状错即拒、未放行的口在类型上装不进去。
// 期望值取自载荷字面量而不是重算：这里证的是「译装没换字」。

const isolatedCommandTenant = "SYN-TENANT-01"

var isolatedReceivedAt = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

func isolatedCommandIntakeForTest(t *testing.T) *customshttp.IsolatedCommandIntake {
	t.Helper()
	intake, err := customshttp.NewIsolatedCommandIntake(customshttp.IsolatedCommandIntakeDeps{
		Tenant: isolatedCommandTenant,
		Clock:  fixedClock{at: isolatedReceivedAt},
	})
	if err != nil {
		t.Fatalf("构造隔离命令 Intake：%v", err)
	}
	return intake
}

func commandRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(body))
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	request.URL.RawQuery = "tenant=TENANT-9"
	return request
}

const isolatedExternalResultBody = `{"sourceId":"SYN-CUSTOMS-RESP/0001","layer":"RELEASE_RESULT","role":"CUSTOMS_AUTHORITY",` +
	`"rawSemantics":"SYN 放行回执 R01","claimedVersion":"SYN-DECL-V-0001","attempt":1,"scope":"SYN-DECL-UNIT-0001",` +
	`"occurredAt":"2026-09-24T10:30:00+08:00","release":{"kind":"CONDITIONAL","authority":"SYN-AUTHORITY/customs-sg",` +
	`"condition":"SYN 待补提单副本"}}`

// Covers: ResultIntake 契约「租户归属只能来自通道认证结果；来源标识、层、原文语义与发生时间照 ADR-0023 从报文体收——
// 服务端不代铸外部事实的身份与时间」。接收时间是服务端自己的时刻（ADR-0023「服务端仍记录自己的接收时间，与发生时间
// 分开保存」），取注入时钟，不从载荷收。
func TestIsolatedCommandIntakeTranslatesExternalResultWithInjectedTenant(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakeResult(context.Background(), commandRequest(isolatedExternalResultBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != isolatedCommandTenant {
		t.Fatalf("TenantID = %q, want %q（注入值，不是请求里的自报）", got, isolatedCommandTenant)
	}
	if command.SourceID != "SYN-CUSTOMS-RESP/0001" || command.Layer.String() != "RELEASE_RESULT" || command.Role != "CUSTOMS_AUTHORITY" ||
		command.RawSemantics != "SYN 放行回执 R01" || command.ClaimedVersion != "SYN-DECL-V-0001" || command.Attempt != 1 ||
		command.Scope != "SYN-DECL-UNIT-0001" {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
	if want := time.Date(2026, 9, 24, 2, 30, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("OccurredAt = %s, want %s（外部事实的发生时间，不是服务端时钟）", command.OccurredAt, want)
	}
	if !command.ReceivedAt.Equal(isolatedReceivedAt) {
		t.Fatalf("ReceivedAt = %s, want %s（服务端自己的接收时刻）", command.ReceivedAt, isolatedReceivedAt)
	}
	if command.Release == nil || command.Release.Kind.String() != "CONDITIONAL" ||
		command.Release.Authority.String() != "SYN-AUTHORITY/customs-sg" || command.Release.Condition != "SYN 待补提单副本" {
		t.Fatalf("Release = %+v，与载荷不符", command.Release)
	}
}

// Covers: 非放行层不带放行三件时命令上就没有这一格——缺席照缺席交给编排，不补一个零值放行。
func TestIsolatedCommandIntakeLeavesAnAbsentReleaseAbsent(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakeResult(context.Background(), commandRequest(
		`{"sourceId":"s","layer":"REGULATORY_RECEIPT","role":"r","rawSemantics":"x","claimedVersion":"v","scope":"u","occurredAt":"2026-09-24T10:30:00+08:00"}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.Release != nil {
		t.Fatalf("Release = %+v, want nil", command.Release)
	}
}

// Covers: 自报租户即拒、尾随内容与未知键即拒、层与放行种类取封闭词（词表外是用法错误）、时刻解不出即拒——都答 400，
// 编排一次也不该被调到。服务端的接收时间不许被载荷改写：带 receivedAt 按未知键拒。
func TestIsolatedCommandIntakeRefusesMalformedExternalResults(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)
	rest := strings.TrimPrefix(isolatedExternalResultBody, "{")
	for name, body := range map[string]string{
		"自报租户":    `{"tenantId":"` + isolatedCommandTenant + `",` + rest,
		"自报接收时间":  `{"receivedAt":"2026-09-24T09:00:00Z",` + rest,
		"尾随内容":    isolatedExternalResultBody + ` {"x":1}`,
		"不是 JSON": "!!not-json!!",
		"空载荷":     "",
		"词表外的层":   `{"sourceId":"s","layer":"FINAL_ANSWER","rawSemantics":"x","claimedVersion":"v"}`,
		"词表外的放行":  `{"sourceId":"s","layer":"RELEASE_RESULT","rawSemantics":"x","claimedVersion":"v","release":{"kind":"MOSTLY"}}`,
		"时刻不是时刻":  `{"sourceId":"s","layer":"REGULATORY_RECEIPT","rawSemantics":"x","claimedVersion":"v","occurredAt":"昨天"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeResult(context.Background(), commandRequest(body))
			if !errors.Is(err, customshttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

// Covers: 立不起来的注入在构造时拒，不等第一个请求。
func TestNewIsolatedCommandIntakeRejectsBlankInjection(t *testing.T) {
	if _, err := customshttp.NewIsolatedCommandIntake(customshttp.IsolatedCommandIntakeDeps{Tenant: " ", Clock: fixedClock{}}); err == nil {
		t.Fatal("空租户被接受")
	}
	if _, err := customshttp.NewIsolatedCommandIntake(customshttp.IsolatedCommandIntakeDeps{Tenant: isolatedCommandTenant}); err == nil {
		t.Fatal("缺时钟被接受——接收时间就没有出处")
	}
}

// Covers: ADR-0091 Consequences「命令面按端点逐口放行」——本类型只实现放行了的口；配置登记族（04 那族）、协作与核对两口
// 与查阅行在类型上就装不进去。
func TestIsolatedCommandIntakeServesOnlyAdmittedLines(t *testing.T) {
	var intake any = isolatedCommandIntakeForTest(t)
	if _, ok := intake.(customshttp.ResultIntake); !ok {
		t.Fatal("外部结果口该已放行")
	}
	for name, refused := range map[string]bool{
		"解释规则登记口（04 那族）":   isA[customshttp.InterpretationRuleRegistrationIntake](intake),
		"门禁目录登记口（04 那族）":   isA[customshttp.GateCatalogRegistrationIntake](intake),
		"税费协作登记口（04 那族）":   isA[customshttp.DutyCollaborationRegistrationIntake](intake),
		"税费核对登记口（04 那族）":   isA[customshttp.DutyPaymentVerificationRegistrationIntake](intake),
		"查阅行（归隔离读 Intake）": isA[customshttp.CatalogueQueryIntake](intake),
	} {
		if refused {
			t.Fatalf("%s 不在放行名单，隔离命令 Intake 不该装得进", name)
		}
	}
}

func isA[Interface any](value any) bool {
	_, ok := value.(Interface)
	return ok
}
