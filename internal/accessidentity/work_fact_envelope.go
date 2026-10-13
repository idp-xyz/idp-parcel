package accessidentity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrDeviceNotInUse 表示令牌有效、操作者在这个作业范围上也有授予，但请求所带的设备不在本租户册上、已停用，或登在
// 别的作业范围上（ADR-0149 决定二另加的那一格，403）。几种情形同答这一格：恢复动作是同一个——管设备的人去登记；
// 登在别的作业范围上的设备，从这个作业范围看就是没登。
var ErrDeviceNotInUse = errors.New("access identity: device is not registered for this work scope or has been deactivated")

// ErrDeviceRegistryUnavailable 表示设备册读不动。它不折进 ErrDeviceNotInUse，理由同 ErrOperatorRegistryUnavailable：
// 折成「设备未登记」会让管设备的人去登一台其实早已登好的设备。
var ErrDeviceRegistryUnavailable = errors.New("access identity: device registry is unavailable")

// WorkFactSourceFrontlineClient 是作业事实信封的来源：一线作业端（ADR-0021、ADR-0149 决定二）。
const WorkFactSourceFrontlineClient = "FRONTLINE_CLIENT"

// WorkFactRequest 是一次作业事实出示所要的：在哪个作业范围上、用哪台设备。
//
// 它不带事实身份与发生时间：两者由设备签发、随命令载荷走，服务端不重签（ADR-0023），铸造这一步没有可改的东西。
type WorkFactRequest struct {
	// TenantID 取空时用操作者册上的绑定；给了就比对，同 OperatorRequest。
	TenantID string
	// WorkScope 是这一笔事实发生的作业范围；授予与设备都按它核。
	WorkScope WorkScope
	DeviceID  string
	// Admission 非空表示这一口的生产写要判准入范围（ADR-0149 决定四第二条）。
	Admission *AdmissionRequirement
}

// WorkFactEnvelope 是经铸造的一线作业身份：租户、操作者主体、设备与作业范围，来源固定为一线作业端（ADR-0149 决定二
// 「信封记操作者主体 + 设备两件，缺一不铸」）。
//
// 它与 OperatorEnvelope 分型：管理台信封里没有设备，拿它去换作业事实口编译期就走不通。字段不导出、包外没有构造函数；
// 零值不可用——Minted 为假、租户与设备为空。它不带铸造时刻，也不带事实身份：那两样随命令载荷走（ADR-0023）。
type WorkFactEnvelope struct {
	tenantID string
	subject  OperatorSubject
	deviceID string
	scope    WorkScope
}

func (envelope WorkFactEnvelope) TenantID() string         { return envelope.tenantID }
func (envelope WorkFactEnvelope) Subject() OperatorSubject { return envelope.subject }
func (envelope WorkFactEnvelope) DeviceID() string         { return envelope.deviceID }
func (envelope WorkFactEnvelope) WorkScope() WorkScope     { return envelope.scope }
func (envelope WorkFactEnvelope) Source() string           { return WorkFactSourceFrontlineClient }

// Minted 答这个信封是不是铸出来的。
func (envelope WorkFactEnvelope) Minted() bool { return envelope.subject.issuer != "" }

// WorkFactMinter 铸造作业事实信封。凭据核验与操作者册沿用操作者族的那两件（ADR-0149 决定二「凭据校验与信封铸造沿
// ADR-0100 决定二、三」），另多一本设备册。
type WorkFactMinter struct {
	verifier  OperatorCredentialVerifier
	operators OperatorRegistry
	devices   DeviceRegistry
	admission AdmissionScope
	now       func() time.Time
}

// NewWorkFactMinter 的 admission 不许缺，取法同 NewOperatorMinter。
func NewWorkFactMinter(
	verifier OperatorCredentialVerifier,
	operators OperatorRegistry,
	devices DeviceRegistry,
	admission AdmissionScope,
	now func() time.Time,
) (*WorkFactMinter, error) {
	if verifier == nil || operators == nil || devices == nil || admission == nil || now == nil {
		return nil, ErrNilDependency
	}
	return &WorkFactMinter{verifier: verifier, operators: operators, devices: devices, admission: admission, now: now}, nil
}

// MintWorkFact 铸造一次作业事实出示的信封：校验令牌 → 查操作者册、按作业范围核授予 → 查设备册 → 判准入范围。
//
// 核验方的三格原样交回；操作者册读不动答 ErrOperatorRegistryUnavailable，不在册、不绑所请求的租户或此刻在这个作业
// 范围上没有生效授予答 ErrOperatorNotGranted；设备册读不动答 ErrDeviceRegistryUnavailable，设备不在本租户册上、已停用
// 或登在别的作业范围上答 ErrDeviceNotInUse；准入范围两格同 MintOperator。租户只取自操作者册上的绑定。
//
// 设备判在授予之后、准入范围之前：没有授予的人不该从答复里看出这台设备登没登，设备不在用的请求不该看出这个租户登没登
// 区间。设备在不在用按铸造那一刻判，不按事实自报的发生时间：发生时间由设备签发（ADR-0023），停用正是为了不再采信这台
// 设备说的话——按它自报的时间判在用，等于让一台已停用的设备给自己倒签。
func (minter *WorkFactMinter) MintWorkFact(
	ctx context.Context,
	credential OperatorCredential,
	request WorkFactRequest,
) (WorkFactEnvelope, error) {
	// 作业范围与设备标识缺席是接入口漏传，不是出示者的错：答成哪一格都会把人指去错的恢复动作。
	if !request.WorkScope.valid() {
		return WorkFactEnvelope{}, errors.New("mint work fact envelope: request names no work scope")
	}
	deviceID := strings.TrimSpace(request.DeviceID)
	if deviceID == "" {
		return WorkFactEnvelope{}, errors.New("mint work fact envelope: request names no device")
	}
	subject, err := minter.verifier.VerifyOperatorCredential(ctx, credential)
	if err != nil {
		return WorkFactEnvelope{}, err
	}
	standing, found, err := minter.operators.FindOperator(ctx, subject)
	if err != nil {
		return WorkFactEnvelope{}, fmt.Errorf("%w: %w", ErrOperatorRegistryUnavailable, err)
	}
	at := minter.now()
	named := strings.TrimSpace(request.TenantID)
	if !found || (named != "" && standing.binding.tenantID != named) || !standing.HoldsWorkFactAt(request.WorkScope, at) {
		return WorkFactEnvelope{}, ErrOperatorNotGranted
	}
	tenant := standing.binding.tenantID
	device, registered, err := minter.devices.FindDevice(ctx, tenant, deviceID)
	if err != nil {
		return WorkFactEnvelope{}, fmt.Errorf("%w: %w", ErrDeviceRegistryUnavailable, err)
	}
	// 册的实现按（租户、设备标识）取，这里仍比一次租户：租户是最高隔离边界（ADR-0003），不托付给实现方记得按键查。
	if !registered || device.registration.tenantID != tenant || device.registration.scope != request.WorkScope || !device.InUseAt(at) {
		return WorkFactEnvelope{}, ErrDeviceNotInUse
	}
	if request.Admission != nil {
		admitted, err := minter.admission.Admits(ctx, tenant, *request.Admission, at)
		if err != nil {
			return WorkFactEnvelope{}, fmt.Errorf("%w: %w", ErrAdmissionScopeUnavailable, err)
		}
		if !admitted {
			return WorkFactEnvelope{}, ErrOutsideAdmissionScope
		}
	}
	return WorkFactEnvelope{tenantID: tenant, subject: subject, deviceID: deviceID, scope: request.WorkScope}, nil
}
