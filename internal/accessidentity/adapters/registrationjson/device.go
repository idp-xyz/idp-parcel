package registrationjson

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// DeviceItem 是登一台作业设备的单项，形状与受控命令行 parcel-access-register 相同。租户不进载荷，在线口取自信封。
type DeviceItem struct {
	DeviceID  string     `json:"deviceId"`
	WorkScope *WorkScope `json:"workScope"`
	Basis     string     `json:"basis"`
}

// DeviceDeactivationItem 是停用一台作业设备的单项。停用时刻不代填。
type DeviceDeactivationItem struct {
	DeviceID      string    `json:"deviceId"`
	DeactivatedAt time.Time `json:"deactivatedAt"`
	Basis         string    `json:"basis"`
}

// DeviceFrom 把设备登记载荷译成领域注册项。
func DeviceFrom(tenantID string, entry DeviceItem) (accessidentity.DeviceRegistration, error) {
	if entry.WorkScope == nil {
		return accessidentity.DeviceRegistration{}, fmt.Errorf("device %q: workScope is required", entry.DeviceID)
	}
	kind, err := accessidentity.ParseWorkScopeKind(entry.WorkScope.Kind)
	if err != nil {
		return accessidentity.DeviceRegistration{}, fmt.Errorf("device %q: workScope.kind %q: %w", entry.DeviceID, entry.WorkScope.Kind, err)
	}
	scope, err := accessidentity.NewWorkScope(kind, entry.WorkScope.Reference)
	if err != nil {
		return accessidentity.DeviceRegistration{}, fmt.Errorf("device %q: workScope: %w", entry.DeviceID, err)
	}
	return accessidentity.NewDeviceRegistration(tenantID, entry.DeviceID, scope, entry.Basis)
}

// DeviceDeactivationFrom 把设备停用载荷译成领域停用项。
func DeviceDeactivationFrom(tenantID string, entry DeviceDeactivationItem) (accessidentity.DeviceDeactivation, error) {
	if entry.DeactivatedAt.IsZero() {
		return accessidentity.DeviceDeactivation{}, fmt.Errorf("device %q: deactivatedAt is required", entry.DeviceID)
	}
	return accessidentity.NewDeviceDeactivation(tenantID, entry.DeviceID, entry.DeactivatedAt, entry.Basis)
}
