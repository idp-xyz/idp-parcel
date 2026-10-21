package tfhttp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// Covers: AT-TF-288（ADR-0230 决定七）——补拍口挂运营决定能力面「派送重问」：租户取认证结果，线格式仍是段、对象与这一拍的业务时间；
// 请求里没有任何地点、时间窗或条件（ADR-0114 决定三），不收作业事实的设备与作业范围头。作业事实那一面不再装得进这一口。
func TestTheDeliveryDispatchTriggerIsAnOperatorDecision(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	command, err := intake.IntakeDeliveryDispatchTrigger(context.Background(), bearerRequest("t",
		`{"segment":"SYN-SEGMENT-08-09","object":"SYN-PARCEL-08-09","occurredAt":"2026-09-25T07:00:00+08:00"}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.TenantID.String() != operatorTenant || command.Segment != "SYN-SEGMENT-08-09" || command.Object != "SYN-PARCEL-08-09" ||
		!command.OccurredAt.Equal(time.Date(2026, 9, 24, 23, 0, 0, 0, time.UTC)) || command.ExpectedHandover != nil {
		t.Fatalf("command = %+v，与认证结果与载荷不符", command)
	}
	if _, err := intake.IntakeDeliveryDispatchTrigger(context.Background(), bearerRequest("t",
		`{"segment":"s","object":"o","place":"SYN-PLACE/x"}`)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("带地点的补拍：err = %v, want ErrMalformedRequest——地点由执行器向所有者取", err)
	}
	if isA[tfhttp.DeliveryDispatchTriggerIntake](any(workFactIntakeForTest(t))) {
		t.Fatal("作业事实那一面仍装得进补拍口")
	}
}

// Covers: AT-TF-285 的线格式那一半（ADR-0230 决定四）——按册重问只收问法与件数，租户取认证结果；问法的词由编排认，载荷里夹带租户、
// 段或对象一律 400。
func TestTheReaskIntakeTakesOnlyScopeAndLimit(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	command, err := intake.IntakeDeliveryDispatchReask(context.Background(), bearerRequest("t", `{"scope":"DUE","limit":50}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.TenantID.String() != operatorTenant || command.Scope != application.ReaskDuePending || command.Limit != 50 {
		t.Fatalf("command = %+v", command)
	}
	if command, _ := intake.IntakeDeliveryDispatchReask(context.Background(), bearerRequest("t", `{"scope":"SOME","limit":50}`)); command.Scope != application.ReaskScopeInvalid {
		t.Fatalf("集外问法 = %s, want 零值交给编排答未受理", command.Scope)
	}
	for name, body := range map[string]string{
		"夹带租户": `{"scope":"ALL","limit":5,"tenantId":"SYN-TENANT-02"}`,
		"夹带对象": `{"scope":"ALL","limit":5,"object":"SYN-PARCEL-1"}`,
	} {
		if _, err := intake.IntakeDeliveryDispatchReask(context.Background(), bearerRequest("t", body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
}
