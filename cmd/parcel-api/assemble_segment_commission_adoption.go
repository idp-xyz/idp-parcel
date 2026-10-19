package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalSegmentCommissionAdopter 把一次采用包进一笔事务：采用册、委托册的写口按框架合同无事务即拒，事务边界归装配点；
// 连线与委托的开始同笔落地。
//
// 编排答`采用成立`以外的任一格都整笔回滚，再交回那一份答复：其余各格本不该留下写入，而编排读回发现委托刚被并发取消的那一格，
// 连线已经落了——编排把撤掉它交给装配（application.AdoptSegmentCommissionHandler.Adopt 的头注）。读口在事务里报了 SQL 错误时
// PostgreSQL 把整笔置为 aborted，照常提交只换回提交错误，未决各格同样回滚后交回。
type transactionalSegmentCommissionAdopter struct {
	transactor bentoapp.Transactor
	inner      *tfapp.AdoptSegmentCommissionHandler
}

// errSegmentCommissionNotAdopted 只在本壳里流转：编排答采用成立以外的格时回调交它，逼事务回滚；回滚之后认出它，交回编排那一份
// 答复。
var errSegmentCommissionNotAdopted = errors.New("parcel-api: segment commission not adopted; transaction rolled back")

var _ tfhttp.SegmentCommissionAdopter = transactionalSegmentCommissionAdopter{}

func (adopter transactionalSegmentCommissionAdopter) Adopt(
	ctx context.Context,
	command tfapp.AdoptSegmentCommissionCommand,
) (tfapp.CommissionAdoptionResult, error) {
	var result tfapp.CommissionAdoptionResult
	err := adopter.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := adopter.inner.Adopt(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if handled.Outcome() != tfapp.CommissionAdopted {
			return errSegmentCommissionNotAdopted
		}
		return nil
	})
	if errors.Is(err, errSegmentCommissionNotAdopted) {
		return result, nil
	}
	if err != nil {
		return tfapp.CommissionAdoptionResult{}, err
	}
	return result, nil
}

// buildSegmentCommissionAdoption 装配 `/transport-fulfillment-segment-commission-adoptions` 背后的真编排（票 demo-chain-gaps/25，
// ADR-0204）。缝全接真：采用册、段登记册（只读）、委托册与时钟。采用不入队：今天没有它的消费者。
func buildSegmentCommissionAdoption(db *bentopg.DB) (tfhttp.SegmentCommissionAdopter, error) {
	adoptions, err := tfpostgres.NewSegmentCommissionAdoptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: segment commission adoptions: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: fulfillment segments: %w", err)
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: transport commissions: %w", err)
	}
	handler := tfapp.NewAdoptSegmentCommissionHandler(tfapp.AdoptSegmentCommissionDeps{
		Adoptions:   adoptions,
		Segments:    segments,
		Commissions: commissions,
		Clock:       systemClock{},
	})
	return transactionalSegmentCommissionAdopter{transactor: db.Transactor(), inner: handler}, nil
}

// transactionalCancellationFaces 把取消委托与取消订舱各包进一笔事务，理由与未决时整笔回滚的做法同订舱两口那只壳
// （transactionalBookingFaces）：写口无事务即拒，读口报错时事务已中止。取消落不上而读回作答的各格都没有写入，提交与回滚
// 无别。两只壳字段相同，这里换成那只壳的类型借它的 within，免得同一段回滚规矩在本包里写两份、日后改一处漏一处。
type transactionalCancellationFaces struct {
	transactor bentoapp.Transactor
	inner      *tfapp.CommissionTransportHandler
}

var (
	_ tfhttp.TransportCommissionCanceller = transactionalCancellationFaces{}
	_ tfhttp.BookingRequestCanceller      = transactionalCancellationFaces{}
)

func (faces transactionalCancellationFaces) CancelCommission(ctx context.Context, command tfapp.CancelCommissionCommand) (tfapp.CommissionResult, error) {
	return transactionalBookingFaces(faces).within(ctx, func(txCtx context.Context) (tfapp.CommissionResult, error) {
		return faces.inner.CancelCommission(txCtx, command)
	})
}

func (faces transactionalCancellationFaces) CancelBooking(ctx context.Context, command tfapp.CancelBookingCommand) (tfapp.CommissionResult, error) {
	return transactionalBookingFaces(faces).within(ctx, func(txCtx context.Context) (tfapp.CommissionResult, error) {
		return faces.inner.CancelBooking(txCtx, command)
	})
}

// buildCancellationFaces 装配 `/transport-fulfillment-transport-commission-cancellations` 与
// `/transport-fulfillment-booking-request-cancellations` 背后的真编排（票 demo-chain-gaps/25，ADR-0204 决定三）。缝全接真：
// 委托册、订舱册（读与取消写口是同一个适配器）与时钟。不装协议核对、委托意图交付、答复册与班次：两口都不提交委托、不碰答复
// 与班次。取消不入队：今天没有它们的消费者。
func buildCancellationFaces(db *bentopg.DB) (transactionalCancellationFaces, error) {
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		return transactionalCancellationFaces{}, fmt.Errorf("parcel-api: transport commissions: %w", err)
	}
	bookings, err := tfpostgres.NewBookingRequests(db)
	if err != nil {
		return transactionalCancellationFaces{}, fmt.Errorf("parcel-api: booking requests: %w", err)
	}
	handler := tfapp.NewCommissionTransportHandler(tfapp.CommissionTransportDeps{
		Commissions:          commissions,
		Bookings:             bookings,
		BookingCancellations: bookings,
		Clock:                systemClock{},
	})
	return transactionalCancellationFaces{transactor: db.Transactor(), inner: handler}, nil
}
