package labelfinal

import (
	"context"
	"fmt"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ContinuedAttemptDecisionJudgmentAdapter 是 psinbox.ContinuedAttemptDecisionJudgmentConsumer 的真实处理方：
// 关闭 / 重开决定落册后交出的指针式信封（lc/27）→ 与 lc/26 共用的核。它不读回登记册——判断读全册且读
// 当下，信封里的决定标识只用于幂等与追溯；命令不带 FirstEffectivePickup，判断走关闭路径。两种决定都经
// 这里，分格归 JudgeLabelServiceFinal：关闭之后按 CONTEXT 判失败 / 服务结果 / 不形成，重开之后判
// NOT_FINAL（继续尝试仍开放）——这里不看种类、不挑。
type ContinuedAttemptDecisionJudgmentAdapter struct {
	core *ParcelJudgmentCore
}

func NewContinuedAttemptDecisionJudgmentAdapter(core *ParcelJudgmentCore) (*ContinuedAttemptDecisionJudgmentAdapter, error) {
	if core == nil {
		return nil, fmt.Errorf("parcel shipment label final: judgment core is nil")
	}
	return &ContinuedAttemptDecisionJudgmentAdapter{core: core}, nil
}

var _ psinbox.ContinuedAttemptDecisionJudgmentDueHandler = (*ContinuedAttemptDecisionJudgmentAdapter)(nil)

// HandleContinuedAttemptDecisionJudgmentDue 按信封的（租户 + 包裹）判一次终局。
func (adapter *ContinuedAttemptDecisionJudgmentAdapter) HandleContinuedAttemptDecisionJudgmentDue(
	ctx context.Context,
	due psinbox.ContinuedAttemptDecisionJudgmentDue,
) error {
	return adapter.core.JudgeParcel(ctx, due.TenantID, due.Parcel, psdomain.CarrierFirstEffectivePickupSpec{})
}
