package main

// 本文件把作业设备册的两种登记批译成领域类型（票 operator-channel/10），纪律同 translate.go：未知字段、缺件与形状错在
// 触库之前拒收，一格不代填——没有缺省作业范围，也没有缺省停用时刻。作业范围的形状与「作业事实登记」授予批共用。

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// workScopeDocument 是批文里的作业范围：种类与该租户给它的引用（参数登记册 PAR-INT-08、PAR-INT-10）。
type workScopeDocument struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

type deviceBatchDocument struct {
	TenantID string               `json:"tenantId"`
	Devices  []deviceItemDocument `json:"devices"`
}

// deviceItemDocument 只有标识、作业范围与依据：设备的型号、序列号、健康状况不进册（ADR-0149 越权风险点 2），未知字段
// 拒收会把误填的一格当场拦下。
type deviceItemDocument struct {
	DeviceID  string             `json:"deviceId"`
	WorkScope *workScopeDocument `json:"workScope"`
	Basis     string             `json:"basis"`
}

type deviceDeactivationBatchDocument struct {
	TenantID      string                           `json:"tenantId"`
	Deactivations []deviceDeactivationItemDocument `json:"deactivations"`
}

type deviceDeactivationItemDocument struct {
	DeviceID      string    `json:"deviceId"`
	DeactivatedAt time.Time `json:"deactivatedAt"`
	Basis         string    `json:"basis"`
}

func workScopeFrom(document *workScopeDocument) (accessidentity.WorkScope, error) {
	if document == nil {
		return accessidentity.WorkScope{}, fmt.Errorf("workScope 缺席：作业范围不代填")
	}
	kind, err := accessidentity.ParseWorkScopeKind(document.Kind)
	if err != nil {
		return accessidentity.WorkScope{}, fmt.Errorf("workScope.kind %q：%w", document.Kind, err)
	}
	scope, err := accessidentity.NewWorkScope(kind, document.Reference)
	if err != nil {
		return accessidentity.WorkScope{}, fmt.Errorf("workScope.reference：%w", err)
	}
	return scope, nil
}

func deviceBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document deviceBatchDocument
	if err := decodeStrict(raw, &document, "设备登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Devices), "设备登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Devices))
	for _, entry := range document.Devices {
		scope, err := workScopeFrom(entry.WorkScope)
		if err != nil {
			return nil, fmt.Errorf("devices/%s：%w", entry.DeviceID, err)
		}
		registration, err := accessidentity.NewDeviceRegistration(document.TenantID, entry.DeviceID, scope, entry.Basis)
		if err != nil {
			return nil, fmt.Errorf("devices/%s：%w", entry.DeviceID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("设备 %s（%s %s）", registration.DeviceID(), scope.Kind(), scope.Reference()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.devices.RegisterDevice(ctx, registration)
				return deviceOutcome(answer), err
			},
		})
	}
	return items, nil
}

func deviceDeactivationBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document deviceDeactivationBatchDocument
	if err := decodeStrict(raw, &document, "设备停用批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Deactivations), "设备停用批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Deactivations))
	for _, entry := range document.Deactivations {
		if entry.DeactivatedAt.IsZero() {
			return nil, fmt.Errorf("deactivations/%s：deactivatedAt 缺席：停用时刻不代填", entry.DeviceID)
		}
		deactivation, err := accessidentity.NewDeviceDeactivation(document.TenantID, entry.DeviceID, entry.DeactivatedAt, entry.Basis)
		if err != nil {
			return nil, fmt.Errorf("deactivations/%s：%w", entry.DeviceID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("停用设备 %s", deactivation.DeviceID()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.devices.RegisterDeactivation(ctx, deactivation)
				return deviceOutcome(answer), err
			},
		})
	}
	return items, nil
}
