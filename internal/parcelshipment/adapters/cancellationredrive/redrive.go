// Package cancellationredrive 拥有「取消被认定不成立 → 被这份取消挡住的来源重新进入原来的编排」这一份续办（ADR-0295 决定四）。被取消挡住的各条链共用它：
// 网络服务的收寄采用、网络服务履约结果的终局采用、面单渠道服务的终局判断。各条链各抄一份重驱，等于对「重驱用什么目标、怎样算入账」立多套口径，而这条续办
// 漏重驱一条，就是一件已解开冲突的包裹永远没有责任起点或终局。
//
// 它放在 parcel-shipment 的适配器层而不是应用层：它要复用各链消费门已经定下的「应用结果 → 入账或回滚」那两张译表（adoptconsume、finalconsume），那两张表认得
// psapplication 的封闭结果集合，应用层不能反向依赖适配器。
package cancellationredrive

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/adoptconsume"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var (
	// ErrRedriveTargetNotFound 表示这个租户下当前没有可采认的已接受委托声明了该包裹。委托可能还没落到已接受，重投会改变结果；是续办，不是毒丸。
	ErrRedriveTargetNotFound = errors.New("parcel shipment cancellation redrive: current accepted parcel target not found")
	// ErrRedriveUntranslatable 表示信封里的某一维译不成本上下文的值，或回放原料被库面改坏了。重投同样内容不会改变结果，现场要查的是那一行为什么长成这样，
	// 所以不进未决名单。
	ErrRedriveUntranslatable = errors.New("parcel shipment cancellation redrive: untranslatable reference")
)

// IntakeAdopter 是收寄采用编排在本适配器侧的窄口。真实装配接 *application.AdoptNetworkIntakeHandler。
type IntakeAdopter interface {
	Handle(ctx context.Context, command psapplication.AdoptNetworkIntakeCommand) (psapplication.AdoptNetworkIntakeResult, error)
}

// FinalAdopter 是终局采用编排在本适配器侧的窄口。真实装配接 *application.FormParcelFinalHandler。
type FinalAdopter interface {
	Handle(ctx context.Context, command psapplication.FormParcelFinalCommand) (psapplication.FormParcelFinalResult, error)
}

// LabelFinalJudge 是面单渠道终局判断各路共用的处理方核的口（与 adapters/transportfulfillment 同形）：按（租户 + 包裹）反查目标委托、折命令、判断、
// 把结果译成消费结论。真实装配接 labelfinal.ParcelJudgmentCore。
type LabelFinalJudge interface {
	JudgeParcel(ctx context.Context, tenantRaw string, parcelRaw string, pickup psdomain.CarrierFirstEffectivePickupSpec) error
}

// Redriver 是 psinbox.NotUpheldCancellationHandler 的真实处理方。
type Redriver struct {
	replays psports.CancellationBlockedReplays
	targets psports.CurrentAcceptedParcelTargetView
	pickups psports.CarrierPickupBoundaryView
	intake  IntakeAdopter
	finals  FinalAdopter
	label   LabelFinalJudge
	lock    psports.CancellationBoundaryLock
}

func NewRedriver(
	replays psports.CancellationBlockedReplays,
	targets psports.CurrentAcceptedParcelTargetView,
	pickups psports.CarrierPickupBoundaryView,
	intake IntakeAdopter,
	finals FinalAdopter,
	label LabelFinalJudge,
	lock psports.CancellationBoundaryLock,
) (*Redriver, error) {
	switch {
	case replays == nil:
		return nil, fmt.Errorf("parcel shipment cancellation redrive: blocked replay reader is nil")
	case targets == nil:
		return nil, fmt.Errorf("parcel shipment cancellation redrive: parcel target view is nil")
	case pickups == nil:
		return nil, fmt.Errorf("parcel shipment cancellation redrive: carrier pickup boundary view is nil")
	case intake == nil:
		return nil, fmt.Errorf("parcel shipment cancellation redrive: intake adopter is nil")
	case finals == nil:
		return nil, fmt.Errorf("parcel shipment cancellation redrive: final adopter is nil")
	case label == nil:
		return nil, fmt.Errorf("parcel shipment cancellation redrive: label final judge is nil")
	case lock == nil:
		return nil, fmt.Errorf("parcel shipment cancellation redrive: cancellation boundary lock is nil")
	}
	return &Redriver{replays: replays, targets: targets, pickups: pickups, intake: intake, finals: finals, label: label, lock: lock}, nil
}

var _ psinbox.NotUpheldCancellationHandler = (*Redriver)(nil)

// HandleNotUpheldCancellation 把被这份取消挡住的来源逐一重新交回原来的编排：先取包裹取消边界锁（各条链进来时再取是重入），再按当前已接受委托取目标，
// 然后依回放原料重驱收寄采用与终局采用；冲突若落在面单渠道服务的首次有效收寄上，再按 TF 的首次有效收寄链尾现读，交面单终局判断。每条链的结果经它自己消费门的
// 译表译成入账或回滚——未决、发布意图没交出都回滚整笔，重投会改变结果。
//
// 重驱幂等：来源已形成采用或终局的，编排答已有结果，不写第二份；取消又被别的取消替代而再挡住来源的，编排照常把它记成新取消名下的冲突或不采用。
func (redriver *Redriver) HandleNotUpheldCancellation(ctx context.Context, notUpheld psinbox.NotUpheldCancellation) error {
	tenant, err := psdomain.NewTenantID(notUpheld.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrRedriveUntranslatable, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(notUpheld.Parcel)
	if err != nil {
		return fmt.Errorf("%w: parcel: %v", ErrRedriveUntranslatable, err)
	}
	cancellation, err := psdomain.NewParcelCancellationID(notUpheld.Cancellation)
	if err != nil {
		return fmt.Errorf("%w: cancellation: %v", ErrRedriveUntranslatable, err)
	}
	if notUpheld.BoundaryKind != psdomain.NetworkIntakeBoundary.String() &&
		notUpheld.BoundaryKind != psdomain.CarrierFirstEffectivePickupBoundary.String() {
		return fmt.Errorf("%w: boundary kind %q", ErrRedriveUntranslatable, notUpheld.BoundaryKind)
	}

	if err := redriver.lock.LockCancellationBoundary(ctx, tenant, parcel); err != nil {
		return fmt.Errorf("parcel shipment cancellation redrive: lock cancellation boundary: %w", err)
	}
	target, found, err := redriver.targets.FindCurrentAcceptedByParcel(ctx, tenant, parcel)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: parcel %q", ErrRedriveTargetNotFound, parcel)
	}

	replays, err := redriver.replays.ListCancellationBlockedReplays(ctx, tenant, parcel, cancellation)
	if err != nil {
		return fmt.Errorf("parcel shipment cancellation redrive: list blocked replays: %w", err)
	}
	for _, replay := range replays {
		switch {
		case replay.Intake != nil:
			result, err := redriver.intake.Handle(ctx, psapplication.AdoptNetworkIntakeCommand{
				Identity:          target.Identity(),
				ShipmentRequestID: target.ShipmentRequestID(),
				SubmissionVersion: target.SubmissionVersion(),
				Source:            *replay.Intake,
			})
			if err != nil {
				return err
			}
			if err := adoptconsume.Consumption(result); err != nil {
				return err
			}
		case replay.Final != nil:
			result, err := redriver.finals.Handle(ctx, psapplication.FormParcelFinalCommand{
				Identity:          target.Identity(),
				ShipmentRequestID: target.ShipmentRequestID(),
				Outcome:           *replay.Final,
			})
			if err != nil {
				return err
			}
			if err := finalconsume.Consumption(result); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: a blocked replay carries neither an intake source nor a final outcome", ErrRedriveUntranslatable)
		}
	}

	if notUpheld.BoundaryKind != psdomain.CarrierFirstEffectivePickupBoundary.String() {
		return nil
	}
	// 面单渠道服务的边界事实是 TF 判过的首次有效收寄，本上下文不存它的副本，按链尾现读：更正出新一代的，就以新一代重判。没有收寄或只是待确认，没有可据以判终局的东西。
	boundary, err := redriver.pickups.FindCurrentCarrierPickup(ctx, tenant, parcel)
	if err != nil {
		return err
	}
	if boundary.State != psports.CarrierPickupBoundaryFormed {
		return nil
	}
	return redriver.label.JudgeParcel(ctx, notUpheld.TenantID, notUpheld.Parcel, boundary.Pickup)
}
