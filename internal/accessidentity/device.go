package accessidentity

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrIncompleteDeviceRegistration 表示设备册的一行缺件，建不成。
var ErrIncompleteDeviceRegistration = errors.New("access identity: device registration is incomplete")

// ErrDeactivationDoesNotMatchDevice 表示一笔停用被挂到了它没停的那台设备上。
var ErrDeactivationDoesNotMatchDevice = errors.New("access identity: deactivation belongs to another device")

// DeviceRegistration 是设备册的一行（ADR-0149 决定二）：设备标识、所属租户与作业范围。行是租户取值（参数登记册
// PAR-INT-10），依据指向该租户的受控证据。只登标识与所属，不做设备管理、不校验设备健康（ADR-0149 越权风险点 2）。
//
// 设备标识在租户内唯一，不像操作者主体那样由发行方全局签发：别的租户登的同名设备是另一台，本租户看不见它。ADR-0023
// 要设备重装、重置或换机时形成新的设备身份——事实身份与序号由设备签发、在同一设备身份内单调——所以一个标识不改指
// 另一台设备，停用之后也不复用。
type DeviceRegistration struct {
	tenantID string
	deviceID string
	scope    WorkScope
	basis    string
}

func NewDeviceRegistration(tenantID, deviceID string, scope WorkScope, basis string) (DeviceRegistration, error) {
	tenant := strings.TrimSpace(tenantID)
	device := strings.TrimSpace(deviceID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || device == "" || !scope.valid() || reference == "" {
		return DeviceRegistration{}, ErrIncompleteDeviceRegistration
	}
	return DeviceRegistration{tenantID: tenant, deviceID: device, scope: scope, basis: reference}, nil
}

func (registration DeviceRegistration) TenantID() string     { return registration.tenantID }
func (registration DeviceRegistration) DeviceID() string     { return registration.deviceID }
func (registration DeviceRegistration) WorkScope() WorkScope { return registration.scope }
func (registration DeviceRegistration) Basis() string        { return registration.basis }

// DeviceDeactivation 是对一台设备的停用：自停用时刻起（含该时刻）它不再在用。停用是终局，没有「再启用」这一格——
// 换机或重置要形成新的设备身份（ADR-0023），旧标识再启用会让序号的单调性失据。时刻与依据缺一不成立，理由同
// GrantRevocation。
type DeviceDeactivation struct {
	tenantID      string
	deviceID      string
	deactivatedAt time.Time
	basis         string
}

func NewDeviceDeactivation(tenantID, deviceID string, deactivatedAt time.Time, basis string) (DeviceDeactivation, error) {
	tenant := strings.TrimSpace(tenantID)
	device := strings.TrimSpace(deviceID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || device == "" || deactivatedAt.IsZero() || reference == "" {
		return DeviceDeactivation{}, ErrIncompleteDeviceRegistration
	}
	return DeviceDeactivation{tenantID: tenant, deviceID: device, deactivatedAt: deactivatedAt, basis: reference}, nil
}

func (deactivation DeviceDeactivation) TenantID() string         { return deactivation.tenantID }
func (deactivation DeviceDeactivation) DeviceID() string         { return deactivation.deviceID }
func (deactivation DeviceDeactivation) DeactivatedAt() time.Time { return deactivation.deactivatedAt }
func (deactivation DeviceDeactivation) Basis() string            { return deactivation.basis }

// RecordedDevice 是册上的一台设备，连同它的停用（若有）。状态「登记 / 停用」由停用对时点导出，不存成状态，理由同
// EffectiveInterval：存一个「已停用」就是存一份会过期的推导。
type RecordedDevice struct {
	registration DeviceRegistration
	deactivation DeviceDeactivation
	deactivated  bool
}

// NewRecordedDevice 的 deactivation 取 nil 表示未停用。
func NewRecordedDevice(registration DeviceRegistration, deactivation *DeviceDeactivation) (RecordedDevice, error) {
	if deactivation == nil {
		return RecordedDevice{registration: registration}, nil
	}
	if deactivation.tenantID != registration.tenantID || deactivation.deviceID != registration.deviceID {
		return RecordedDevice{}, ErrDeactivationDoesNotMatchDevice
	}
	return RecordedDevice{registration: registration, deactivation: *deactivation, deactivated: true}, nil
}

func (device RecordedDevice) Registration() DeviceRegistration { return device.registration }

func (device RecordedDevice) Deactivation() (DeviceDeactivation, bool) {
	return device.deactivation, device.deactivated
}

// InUseAt 答这台设备在 at 那一刻在不在用：在册，且还没到停用时刻。
func (device RecordedDevice) InUseAt(at time.Time) bool {
	return !device.deactivated || at.Before(device.deactivation.deactivatedAt)
}

// ErrDeviceCatalogueTenantRequired 表示按租户列设备时没给出租户。空租户不能列成「全部租户的设备」。
var ErrDeviceCatalogueTenantRequired = errors.New("access identity: device catalogue list needs a tenant")

// DeviceCatalogueStatus 是查阅行上的状态词。登记与停用由停用导出，不另存一份状态。
type DeviceCatalogueStatus string

const (
	// DeviceCatalogueRegistered 是尚未停用的一行。
	DeviceCatalogueRegistered DeviceCatalogueStatus = "REGISTERED"
	// DeviceCatalogueDeactivated 是已经停用的一行。
	DeviceCatalogueDeactivated DeviceCatalogueStatus = "DEACTIVATED"
)

func (status DeviceCatalogueStatus) String() string { return string(status) }

// DeviceCatalogueRow 是设备册查阅交回的一行。只有设备标识、作业范围、登记还是停用；停用了的带停用时刻。
// 不出型号、序列号、使用人、健康状态，也不出依据——依据留在登记行上（ADR-0149 越权风险点 2，票 operator-channel/21）。
type DeviceCatalogueRow struct {
	deviceID      string
	scope         WorkScope
	deactivatedAt time.Time
	deactivated   bool
}

// NewDeviceCatalogueRow 的 deactivatedAt 取 nil 表示登记、未停用。零值时刻不是停用时刻。
func NewDeviceCatalogueRow(deviceID string, scope WorkScope, deactivatedAt *time.Time) (DeviceCatalogueRow, error) {
	device := strings.TrimSpace(deviceID)
	if device == "" || !scope.valid() {
		return DeviceCatalogueRow{}, ErrIncompleteDeviceRegistration
	}
	if deactivatedAt == nil {
		return DeviceCatalogueRow{deviceID: device, scope: scope}, nil
	}
	if deactivatedAt.IsZero() {
		return DeviceCatalogueRow{}, ErrIncompleteDeviceRegistration
	}
	return DeviceCatalogueRow{
		deviceID:      device,
		scope:         scope,
		deactivatedAt: *deactivatedAt,
		deactivated:   true,
	}, nil
}

func (row DeviceCatalogueRow) DeviceID() string     { return row.deviceID }
func (row DeviceCatalogueRow) WorkScope() WorkScope { return row.scope }
func (row DeviceCatalogueRow) Status() DeviceCatalogueStatus {
	if row.deactivated {
		return DeviceCatalogueDeactivated
	}
	return DeviceCatalogueRegistered
}

// DeactivatedAt 只在这一行已停用时交回时刻。登记行没有停用时刻，不是零值时刻。
func (row DeviceCatalogueRow) DeactivatedAt() (time.Time, bool) {
	return row.deactivatedAt, row.deactivated
}

// CatalogueRow 从册上的一台设备投影出查阅行，依据停在登记行里，不进入这一行。
func (device RecordedDevice) CatalogueRow() (DeviceCatalogueRow, error) {
	registration := device.registration
	if deactivation, deactivated := device.Deactivation(); deactivated {
		at := deactivation.DeactivatedAt()
		return NewDeviceCatalogueRow(registration.DeviceID(), registration.WorkScope(), &at)
	}
	return NewDeviceCatalogueRow(registration.DeviceID(), registration.WorkScope(), nil)
}

// DeviceCatalogue 按租户列出已登记设备。空册答空切片，不答未配置；读不动才答 error。
//
// 与 DeviceRegistry 分列：FindDevice 的替身只为一台设备的铸造服务，列册是查阅。塞进同一个接口会让那些替身空实现
// 一个它们碰不到的方法。postgres 的 DeviceRegistry 两者都实现，册仍是一本。
type DeviceCatalogue interface {
	ListDevices(ctx context.Context, tenantID string) ([]DeviceCatalogueRow, error)
}

// DeviceRegistry 是设备册的装载口：按（租户、设备标识）取一台设备，连同它的停用。
//
// 租户在键上：铸造时它取自操作者册上的绑定，所以别的租户登的同名设备在这里查不到。不在册时答 found=false 而不返回
// error，理由同 OperatorRegistry。
type DeviceRegistry interface {
	FindDevice(ctx context.Context, tenantID, deviceID string) (RecordedDevice, bool, error)
}

// DeviceRegistrationOutcome 是设备册登记口的答复。撞键从不覆盖：同内容重放答原结果，异内容答冲突，册上已有的那一行
// 原样不动。
type DeviceRegistrationOutcome string

const (
	DeviceRegistrationRecorded          DeviceRegistrationOutcome = "RECORDED"
	DeviceRegistrationAlreadyRegistered DeviceRegistrationOutcome = "ALREADY_REGISTERED"
	DeviceRegistrationContentConflict   DeviceRegistrationOutcome = "CONTENT_CONFLICT"
	// DeviceNotRegistered 只由 RegisterDeactivation 答：本租户册上没有这台设备。
	DeviceNotRegistered DeviceRegistrationOutcome = "DEVICE_NOT_REGISTERED"
)

func (outcome DeviceRegistrationOutcome) String() string { return string(outcome) }

// DeviceRegistrar 是设备册的登记口。受控批量口与将来的在线口消费它，答复代数一致。写入要落在调用方开的框架事务里，
// 同 OperatorRegistrar。
type DeviceRegistrar interface {
	RegisterDevice(ctx context.Context, registration DeviceRegistration) (DeviceRegistrationOutcome, error)
	RegisterDeactivation(ctx context.Context, deactivation DeviceDeactivation) (DeviceRegistrationOutcome, error)
}
