package settlementhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// IsolatedCommandIntake 是隔离写路径准入（ADR-0091）在外部资金事实采用口的注入式放行（票 operator-channel/08）。
//
// 它与本包的 IsolatedOperationsReadIntake 同层同款：租户格在构造时由装配点给定，进请求路径后不读任何授权输入——采用口的
// Intake 契约写着「采信报文自称的租户会穿透 ADR-0003 的隔离边界」，隔离形态的认证结果就是开关值。与读面不同的是它会构造
// 命令、命令会落行：可分辨物由租户维的 `SYN-` 前缀承担（ADR-0091 决定三）；前缀门禁、启用与否与启动日志都在
// cmd/parcel-api，本类型只收立得住的租户。
//
// 它不自动采用任何回调：收的是人工 / 受控交进来的载荷（UC-SA-005「不因接收回调自动采用」），与受控 CLI 同一份译装。
//
// 逐口放行（ADR-0091 Consequences）：本类型只实现已成笔的口的 Intake 接口，更正口在装配点仍挂字面量 UnconfiguredIntake{}，
// 且在类型上就装不进本类型。
type IsolatedCommandIntake struct {
	tenant domain.TenantID
}

// 已成笔的口。每放一口在这里多一行断言、多一个方法，装配点多换一行。
var _ ExternalFundsFactRegistrationIntake = (*IsolatedCommandIntake)(nil)

// NewIsolatedCommandIntake 由装配点以显式合成值构造。立不起来的租户在这里拒：装配错误要在启动时暴露，不该等到第一个请求。
func NewIsolatedCommandIntake(tenant string) (*IsolatedCommandIntake, error) {
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		return nil, fmt.Errorf("settlement accounting http: isolated command intake: %w", err)
	}
	return &IsolatedCommandIntake{tenant: tenantID}, nil
}

// IntakeExternalFundsFactRegistration 译一次外部资金事实首版的采用（`/settlement-external-funds-fact-registrations`）。线格式是受控
// CLI `-input` 的载荷去掉 tenantId；本方法把注入的租户拼回那一格，交给 registrationjson 那一份译装——ExternalFundsFactRegistrationIntake
// 的契约写明「登记输入本体的译装已有一份，本包不得另写」。译装拒的一律是用法错误（未知字段、构造门、封闭词表），答 400。
func (intake *IsolatedCommandIntake) IntakeExternalFundsFactRegistration(
	_ context.Context,
	request *http.Request,
) (application.AdoptFundsFactCommand, error) {
	snapshot, err := intake.snapshotWithInjectedTenant(request.Body)
	if err != nil {
		return application.AdoptFundsFactCommand{}, err
	}
	command, err := registrationjson.ExternalFundsFactFromJSON(snapshot)
	if err != nil {
		return application.AdoptFundsFactCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return command, nil
}

// snapshotWithInjectedTenant 读一份登记载荷、拒自报租户，把注入的租户拼回 tenantId 一格。键在场即拒、不看值（`"tenantId": null`
// 也是自报）：值与开关碰巧相同也拒，否则同一份载荷在别的环境里就是穿透 ADR-0003 隔离边界的第一步。只在一个 JSON 对象上拼，
// 尾随的第二个值同样拒；其余键原样交给译装，由它的严格解码判认不认识。
func (intake *IsolatedCommandIntake) snapshotWithInjectedTenant(body io.Reader) ([]byte, error) {
	decoder := json.NewDecoder(body)
	var snapshot map[string]json.RawMessage
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	if snapshot == nil {
		return nil, fmt.Errorf("%w: the payload must be a JSON object", ErrMalformedRequest)
	}
	if _, present := snapshot["tenantId"]; present {
		return nil, fmt.Errorf(
			"%w: tenantId must not be carried in the payload; the tenant grid is filled by the access channel, not by the request",
			ErrMalformedRequest,
		)
	}
	tenant, err := json.Marshal(intake.tenant.String())
	if err != nil {
		return nil, fmt.Errorf("settlement accounting http: encode injected tenant: %w", err)
	}
	snapshot["tenantId"] = tenant
	return json.Marshal(snapshot)
}
