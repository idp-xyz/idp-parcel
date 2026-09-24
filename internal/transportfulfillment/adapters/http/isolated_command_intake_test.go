package tfhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 本文件对隔离写路径准入在运输履约主链命令面的注入式放行（ADR-0091；票 operator-channel/08）证传输面：租户格只来自
// 注入、事实内容与发生时间照 ADR-0023 从载荷如实收、自报租户即拒、形状错即拒、未放行的口在类型上装不进去。
// 期望值取自载荷字面量而不是重算：这里证的是「译装没换字」。成形与否（对象引用立不立得住、控制证据缺不缺）不在这里证
// ——那是编排答`未受理`的格，本 Intake 不预判。

const isolatedCommandTenant = "SYN-TENANT-01"

func isolatedCommandIntakeForTest(t *testing.T) *tfhttp.IsolatedCommandIntake {
	t.Helper()
	intake, err := tfhttp.NewIsolatedCommandIntake(tfhttp.IsolatedCommandIntakeDeps{Tenant: isolatedCommandTenant})
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

// isolatedLine 是一口已放行的线：怎么调它的 Intake、一份合规载荷。自报租户、尾随内容与类型锁三条对每一口都成立，
// 表驱动逐口跑——每放一口在这里加一行。
type isolatedLine struct {
	intake func(*tfhttp.IsolatedCommandIntake, *http.Request) error
	valid  string
}

var isolatedLines = map[string]isolatedLine{
	"/transport-fulfillment/offsite-pickups": {
		intake: func(intake *tfhttp.IsolatedCommandIntake, request *http.Request) error {
			_, err := intake.IntakePickupRegistration(context.Background(), request)
			return err
		},
		valid: offsitePickupRegistrationBody,
	},
}

const offsitePickupRegistrationBody = `{"object":"SYN-PARCEL-08-01","task":"SYN-TASK-08-01","attempt":"SYN-ATTEMPT-08-01",` +
	`"place":"SYN-PLACE/shipper-dock-01","control":"SYN-CONTROL/signed-pickup-01","executedBy":"SYN-COURIER-08",` +
	`"occurredAt":"2026-09-24T08:30:00+08:00","segment":"SYN-SEGMENT-08-01","plannedSegment":"SYN-PLANNED-08-01",` +
	`"segmentServiceAction":"OFFSITE_PICKUP"}`

// Covers: PickupRegistrationIntake 契约「租户只能来自认证结果、事实内容从请求体收」+ `Segment`/`PlannedSegment` 必须收——
// 隔离形态的认证结果是开关值；十格逐字来自载荷，请求头与查询串里的自报一律无视。
func TestIsolatedCommandIntakeTranslatesOffsitePickupRegistrationWithInjectedTenant(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakePickupRegistration(context.Background(), commandRequest(offsitePickupRegistrationBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != isolatedCommandTenant {
		t.Fatalf("TenantID = %q, want %q（注入值，不是请求里的自报）", got, isolatedCommandTenant)
	}
	for name, pair := range map[string][2]string{
		"object":               {command.Object, "SYN-PARCEL-08-01"},
		"task":                 {command.Task, "SYN-TASK-08-01"},
		"attempt":              {command.Attempt, "SYN-ATTEMPT-08-01"},
		"place":                {command.Place, "SYN-PLACE/shipper-dock-01"},
		"control":              {command.Control, "SYN-CONTROL/signed-pickup-01"},
		"executedBy":           {command.ExecutedBy, "SYN-COURIER-08"},
		"segment":              {command.Segment, "SYN-SEGMENT-08-01"},
		"plannedSegment":       {command.PlannedSegment, "SYN-PLANNED-08-01"},
		"segmentServiceAction": {command.SegmentServiceAction, "OFFSITE_PICKUP"},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
	if want := time.Date(2026, 9, 24, 0, 30, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("OccurredAt = %s, want %s（设备记录的业务时间，不是服务端时钟）", command.OccurredAt, want)
	}
}

// Covers: 时刻缺席交给编排答`未受理`（200，形成了的业务答案），解不出才是坏报文（400）——同本包判断载荷的分格。
func TestIsolatedCommandIntakeLeavesAnAbsentPickupInstantToTheOrchestration(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakePickupRegistration(context.Background(),
		commandRequest(`{"object":"SYN-PARCEL-08-01","attempt":"SYN-ATTEMPT-08-01"}`))
	if err != nil {
		t.Fatalf("缺时刻被当成坏报文：%v", err)
	}
	if !command.OccurredAt.IsZero() {
		t.Fatalf("OccurredAt = %s, want 零值——缺席不是服务端补一个时钟值的理由", command.OccurredAt)
	}
}

// Covers: 载荷里带 tenantId 即拒（MALFORMED_REQUEST），值与开关相同也拒——每一口都一样。
func TestIsolatedCommandIntakeRefusesSelfReportedTenantOnEveryAdmittedLine(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)
	for pattern, line := range isolatedLines {
		t.Run(pattern, func(t *testing.T) {
			body := `{"tenantId":"` + isolatedCommandTenant + `",` + strings.TrimPrefix(line.valid, "{")
			err := line.intake(intake, commandRequest(body))
			if !errors.Is(err, tfhttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
			if !strings.Contains(err.Error(), "tenantId") {
				t.Fatalf("拒绝理由没点名 tenantId：%v", err)
			}
		})
	}
}

// Covers: 一份载荷只许一个文档、只许认识的键——尾随的第二个 JSON 值与未知键都拒，放过它们就是无声丢掉一段输入。
func TestIsolatedCommandIntakeRefusesTrailingContentAndUnknownKeysOnEveryAdmittedLine(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)
	for pattern, line := range isolatedLines {
		t.Run(pattern, func(t *testing.T) {
			for name, body := range map[string]string{
				"尾随内容":    line.valid + ` {"x":1}`,
				"未知键":     `{"unknownKey":"x",` + strings.TrimPrefix(line.valid, "{"),
				"不是 JSON": "!!not-json!!",
				"空载荷":     "",
			} {
				if err := line.intake(intake, commandRequest(body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
					t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
				}
			}
		})
	}
}

// Covers: 时刻给了却解不出是坏报文（400）。
func TestIsolatedCommandIntakeRefusesAnUnparsablePickupInstant(t *testing.T) {
	_, err := isolatedCommandIntakeForTest(t).IntakePickupRegistration(context.Background(),
		commandRequest(`{"object":"SYN-PARCEL-08-01","attempt":"SYN-ATTEMPT-08-01","occurredAt":"昨天"}`))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: 立不起来的注入在构造时拒，不等第一个请求（同隔离读 Intake 的纪律）。
func TestNewIsolatedCommandIntakeRejectsBlankTenant(t *testing.T) {
	if _, err := tfhttp.NewIsolatedCommandIntake(tfhttp.IsolatedCommandIntakeDeps{Tenant: "   "}); err == nil {
		t.Fatal("空租户被接受")
	}
}

// Covers: ADR-0091 Consequences「命令面按端点逐口放行，不是一次全开」——本类型只实现放行了的口；更正口、登记册配置口与
// 查阅行在类型上就装不进去。每放一口，该口的断言从「装不进」挪到「该已放行」。
func TestIsolatedCommandIntakeServesOnlyAdmittedLines(t *testing.T) {
	var intake any = isolatedCommandIntakeForTest(t)
	if _, ok := intake.(tfhttp.PickupRegistrationIntake); !ok {
		t.Fatal("场外揽收登记口该已放行")
	}
	for name, refused := range map[string]bool{
		"揽收更正口（同族未列）":      isA[tfhttp.PickupCorrectionIntake](intake),
		"交接更正口（同族未列）":      isA[tfhttp.HandoverCorrectionIntake](intake),
		"交付更正口（同族未列）":      isA[tfhttp.DeliveryProofCorrectionIntake](intake),
		"装载分配口（票面未列）":      isA[tfhttp.LoadAssignmentIntake](intake),
		"终止参与口（票面未列）":      isA[tfhttp.ParticipationTerminationIntake](intake),
		"承运凭证登记口（04 那族）":   isA[tfhttp.CredentialIntake](intake),
		"有效时间规则登记口（04 那族）": isA[tfhttp.EffectiveTimeRuleIntake](intake),
		"总单登记口（04 那族）":     isA[tfhttp.MasterDocumentIntake](intake),
		"查阅行（归隔离读 Intake）": isA[tfhttp.CatalogueQueryIntake](intake),
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
