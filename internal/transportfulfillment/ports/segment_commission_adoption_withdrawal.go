package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 解除履约段采用要的几口（ADR-0232 决定一至三）。另立文件、不并进 ports.go 与 segment_commission_adoption.go，理由同
// SegmentCommissionAdoptionStore 的文件头：那些接口有别处写的替身。

// SegmentCommissionAdoptionWithdrawalRecord 是一次解除越过提交边界留下的东西。Key 是被解除的那一段，代次在 Withdrawal 上。
type SegmentCommissionAdoptionWithdrawalRecord struct {
	Key        SegmentCommissionAdoptionKey
	Withdrawal domain.SegmentCommissionAdoptionWithdrawal
	RecordedAt time.Time
}

// SegmentCommissionAdoptionWithdrawalStore 是解除册：只插、按段读最近一次。
type SegmentCommissionAdoptionWithdrawalStore interface {
	// FindLatestWithdrawal 交这一段最近一次解除（代次最大的那一次）；从没解除过答 found=false。原样再交的解除据它答重放。
	FindLatestWithdrawal(ctx context.Context, key SegmentCommissionAdoptionKey) (SegmentCommissionAdoptionWithdrawalRecord, bool, error)
	// SaveWithdrawal 撞上同一段同一代已有的解除答 AdoptionAlreadyRegistered，不覆盖先到者。
	SaveWithdrawal(ctx context.Context, record SegmentCommissionAdoptionWithdrawalRecord) (AdoptionSaveOutcome, error)
}

// CommissionAdoptionsView 按委托列出它此刻在各段上的当前采用。解除一段之后，委托的开始按其余各段重新派生，要的就是这一张单子。
type CommissionAdoptionsView interface {
	FindCurrentByCommission(ctx context.Context, key TransportCommissionKey) ([]SegmentCommissionAdoptionRecord, error)
}

// TransportCommissionStartHold 取一份委托「开始」的事务级串行锁，直到事务结束。采用（可能标开始或前移）与解除（可能撤回或后移）都在
// 读委托之前握着它：采用读到委托已开始、这一段不更早就不写委托，解除读其余各段再定开始——两笔若并发，各自凭对方还没提交的样子判，
// 会留下「委托已撤回到已提交、段却还挂在它上面」。不在事务里调用由实现拒。
type TransportCommissionStartHold interface {
	HoldCommissionStart(ctx context.Context, key TransportCommissionKey) error
}

// TransportCommissionStartCorrection 是解除对委托册的写口：把开始写成解除判出的样子（撤回则两列清空），只作用于未取消、在册开始恰是
// expected 那一份的行；对不上答 false。不借 Replace 与 AdvanceStart：两者的 WHERE 只认未开始、或只认更早的开始，表达不出后移与撤回。
type TransportCommissionStartCorrection interface {
	CorrectStart(
		ctx context.Context,
		record TransportCommissionRecord,
		expectedBasis domain.ParticipationBasisReference,
		expectedAt time.Time,
	) (bool, error)
}
