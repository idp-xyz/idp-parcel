// Package customscompliance 是 settlement-accounting 读 customs-compliance 的消费侧适配器
// （`internal/<consumer>/adapters/<provider>/`，与 CC 读 SA 的 adapters/settlementaccounting 同一位置纪律）：
// 跨上下文翻译只落在这里，`application` 不 import 提供方（票 sa-cc/09 做法 2）。本包只读提供方的只读半边，
// 不写它的库。
package customscompliance

import (
	"context"
	"errors"
	"fmt"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUntranslatableReference 表示信封带来的引用在提供方词汇里构造不出来（空白租户 / 范围 / 税费 / 资金事实
// / 指纹）。那是引用坏了，不是等谁——但消费者译码已经拒过五维缺席，这里只可能是两侧词汇表出了分歧。
var ErrUntranslatableReference = errors.New(
	"settlement accounting customscompliance adapter: duty payment verification reference does not translate")

// DutyVerificationReader 是本适配器向提供方要的只读半边：ccports.DutyVerificationStore 的 FindVerification 一口。
// 在这里另立窄接口而不直接收 DutyVerificationStore，是让写口在类型上就不进本上下文——本包只读提供方，
// 收整个 Store 等于把 SaveVerification 也摆进依赖图，而没有任何一条路该走它。ccpostgres.DutyPaymentReconciliation
// 结构上满足它，CC 一字不动。
type DutyVerificationReader interface {
	FindVerification(ctx context.Context, key ccports.DutyVerificationKey) (ccports.DutyVerificationRecord, bool, error)
}

// CustomsDutyPaymentVerificationView 实现 saports.DutyPaymentVerificationView：把本上下文的核对引用四维译成
// 提供方的幂等键，问 CC 那一版在不在册。
//
// 只答在不在（票 sa-cc/09 红线：三态由 CC 读口按引用答，本上下文不复制结论）。读回来的 DutyVerificationRecord
// 里有三轴与依据，这里一格都不转述——采用一格用不着，转述一次就多一处口径；实际代垫判断形成时再按同一
// 引用回读。
type CustomsDutyPaymentVerificationView struct {
	reader DutyVerificationReader
}

func NewCustomsDutyPaymentVerificationView(reader DutyVerificationReader) (*CustomsDutyPaymentVerificationView, error) {
	if reader == nil {
		return nil, fmt.Errorf("settlement accounting customscompliance adapter: duty verification reader is nil")
	}
	return &CustomsDutyPaymentVerificationView{reader: reader}, nil
}

var _ saports.DutyPaymentVerificationView = (*CustomsDutyPaymentVerificationView)(nil)

// DutyPaymentVerificationExists 按信封所指的那一版问。提供方答「没有」原样交回 false——那一版还没落，或
// 存的是另一版指纹，两者都不该拿当前版顶替（票 lc/24 的教训）。
func (view *CustomsDutyPaymentVerificationView) DutyPaymentVerificationExists(
	ctx context.Context,
	tenant sadomain.TenantID,
	verification sadomain.DutyPaymentVerificationReference,
) (bool, error) {
	key, err := customsVerificationKey(tenant, verification)
	if err != nil {
		return false, err
	}
	_, found, err := view.reader.FindVerification(ctx, key)
	if err != nil {
		return false, fmt.Errorf("find duty payment verification: %w", err)
	}
	return found, nil
}

// customsVerificationKey 把本上下文的引用译成提供方的幂等键。四维一对一：申报范围 → DecisionScopeReference、
// 税费义务 → AssessedDutyReference、资金事实 → ExternalFundsFactReference、版本指纹 → Digest。
func customsVerificationKey(
	tenant sadomain.TenantID,
	verification sadomain.DutyPaymentVerificationReference,
) (ccports.DutyVerificationKey, error) {
	ccTenant, err := ccdomain.NewTenantID(tenant.String())
	if err != nil {
		return ccports.DutyVerificationKey{}, fmt.Errorf("%w: tenant: %v", ErrUntranslatableReference, err)
	}
	scope, err := ccdomain.NewDecisionScopeReference(verification.Scope().String())
	if err != nil {
		return ccports.DutyVerificationKey{}, fmt.Errorf("%w: scope: %v", ErrUntranslatableReference, err)
	}
	duty, err := ccdomain.NewAssessedDutyReference(verification.Duty().String())
	if err != nil {
		return ccports.DutyVerificationKey{}, fmt.Errorf("%w: duty: %v", ErrUntranslatableReference, err)
	}
	funds, err := ccdomain.NewExternalFundsFactReference(verification.Funds().String())
	if err != nil {
		return ccports.DutyVerificationKey{}, fmt.Errorf("%w: funds: %v", ErrUntranslatableReference, err)
	}
	if verification.Version().String() == "" {
		return ccports.DutyVerificationKey{}, fmt.Errorf("%w: version digest is blank", ErrUntranslatableReference)
	}
	return ccports.DutyVerificationKey{
		TenantID: ccTenant,
		Duty:     duty,
		Funds:    funds,
		Scope:    scope,
		Digest:   verification.Version().String(),
	}, nil
}
