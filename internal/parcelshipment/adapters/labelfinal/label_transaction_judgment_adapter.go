package labelfinal

import (
	"context"
	"fmt"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// LabelTransactionJudgmentAdapter 是 psinbox.LabelTransactionJudgmentConsumer 的真实处理方：面单交易
// 定案 / 后续动作两拍交出的指针式信封（lc/26）→ 共用核。它不读回交易——判断读全册且读当下，信封里
// 的交易标识只用于幂等与追溯；命令不带 FirstEffectivePickup，判断走关闭路径。
type LabelTransactionJudgmentAdapter struct {
	core *ParcelJudgmentCore
}

func NewLabelTransactionJudgmentAdapter(core *ParcelJudgmentCore) (*LabelTransactionJudgmentAdapter, error) {
	if core == nil {
		return nil, fmt.Errorf("parcel shipment label final: judgment core is nil")
	}
	return &LabelTransactionJudgmentAdapter{core: core}, nil
}

var _ psinbox.LabelTransactionJudgmentDueHandler = (*LabelTransactionJudgmentAdapter)(nil)

// HandleLabelTransactionJudgmentDue 按信封的（租户 + 包裹）判一次终局。
func (adapter *LabelTransactionJudgmentAdapter) HandleLabelTransactionJudgmentDue(
	ctx context.Context,
	due psinbox.LabelTransactionJudgmentDue,
) error {
	return adapter.core.JudgeParcel(ctx, due.TenantID, due.Parcel, psdomain.CarrierFirstEffectivePickupSpec{})
}
