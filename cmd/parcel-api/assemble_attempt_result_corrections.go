package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件是尝试结果更正两口的组合根（票 docs-consistency/code/90，ADR-0297）：`/transport-fulfillment/offsite-pickup-attempt-result-corrections`、
// `/transport-fulfillment/delivery-attempt-result-corrections` 与运营决定面的两口背后的真编排。

// errAttemptResultNotCorrected 只在本壳里流转：更正编排答「成立」以外的任一格都逼事务回滚。揽收一侧更正成揽收到手时对象级揽收登记写在
// 更正追加之前——登记成了而更正没追加成（冲突、未决、读不动），登记就得随之撤回，否则留下一份没有更正回指的场外揽收。其余格本不该
// 留下写入，回滚对它们无别。回滚之后认出它，交回编排那一份答复。
var errAttemptResultNotCorrected = errors.New("parcel-api: attempt result not corrected; transaction rolled back")

type transactionalPickupAttemptResultCorrection struct {
	transactor bentoapp.Transactor
	inner      *tfapp.CorrectPickupAttemptResultHandler
}

var _ tfhttp.PickupAttemptResultCorrector = transactionalPickupAttemptResultCorrection{}

func (correction transactionalPickupAttemptResultCorrection) Correct(
	ctx context.Context,
	command tfapp.CorrectPickupAttemptResultCommand,
) (tfapp.CorrectPickupAttemptResultResult, error) {
	var result tfapp.CorrectPickupAttemptResultResult
	err := correction.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := correction.inner.Correct(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if handled.Outcome() != tfapp.AttemptResultCorrected {
			return errAttemptResultNotCorrected
		}
		return nil
	})
	if errors.Is(err, errAttemptResultNotCorrected) {
		return result, nil
	}
	if err != nil {
		return tfapp.CorrectPickupAttemptResultResult{}, err
	}
	return result, nil
}

// buildPickupAttemptResultCorrection 装配揽收结果更正：更正册与对象级揽收登记同一个库。更正成揽收到手时转交的登记编排与首登口
// （buildControlFactOrchestrations 的 pickupRegistration）是同一组缝——揽收登记册、段登记册、实际承运商判断登记册、结果版本签发、
// 登记意图交付、时钟——于是进段、铸判断首版、交 parcel-shipment 采认与首登逐格同形；Segments 与 Judgments 一并交入，不留缺席：缺席时
// 编排对进段既不做也不报，答复与成功逐格同形，只有读回段才看得出来。
func buildPickupAttemptResultCorrection(db *bentopg.DB) (tfhttp.PickupAttemptResultCorrector, error) {
	corrections, err := tfpostgres.NewPickupResultCorrections(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: pickup result corrections: %w", err)
	}
	return newPickupAttemptResultCorrection(db, corrections)
}

// newPickupAttemptResultCorrection 是 buildPickupAttemptResultCorrection 去掉更正册的那一半：更正册由调用方给，其余缝全接真。
// 另成一个函数，只为让装配测试能换一只写不进的更正册，证对象级揽收登记写在更正追加之前时事务壳真的整笔回滚。
func newPickupAttemptResultCorrection(db *bentopg.DB, corrections tfports.PickupResultCorrections) (tfhttp.PickupAttemptResultCorrector, error) {
	pickups, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: offsite pickup registrations: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: fulfillment segments: %w", err)
	}
	judgments, err := tfpostgres.NewActualCarrierJudgments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: actual carrier judgments: %w", err)
	}
	versions, err := tfpostgres.NewResultVersions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: pickup result versions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := tfpostgres.NewOutboxOffsitePickupRegistrationHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: offsite pickup registration handoff: %w", err)
	}
	registrar := tfapp.NewRegisterOffsitePickupHandler(tfapp.RegisterOffsitePickupDeps{
		Pickups:    pickups,
		Segments:   segments,
		Judgments:  judgments,
		Versions:   versions,
		Downstream: downstream,
		Clock:      clock,
	})
	return transactionalPickupAttemptResultCorrection{
		transactor: db.Transactor(),
		inner: tfapp.NewCorrectPickupAttemptResultHandler(tfapp.CorrectPickupAttemptResultDeps{
			Corrections: corrections,
			Pickups:     registrar,
			Clock:       clock,
		}),
	}, nil
}

type transactionalDeliveryAttemptResultCorrection struct {
	transactor bentoapp.Transactor
	inner      *tfapp.CorrectDeliveryAttemptResultHandler
}

var _ tfhttp.DeliveryAttemptResultCorrector = transactionalDeliveryAttemptResultCorrection{}

func (correction transactionalDeliveryAttemptResultCorrection) Correct(
	ctx context.Context,
	command tfapp.CorrectDeliveryAttemptResultCommand,
) (tfapp.CorrectDeliveryAttemptResultResult, error) {
	var result tfapp.CorrectDeliveryAttemptResultResult
	err := correction.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := correction.inner.Correct(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if handled.Outcome() != tfapp.AttemptResultCorrected {
			return errAttemptResultNotCorrected
		}
		return nil
	})
	if errors.Is(err, errAttemptResultNotCorrected) {
		return result, nil
	}
	if err != nil {
		return tfapp.CorrectDeliveryAttemptResultResult{}, err
	}
	return result, nil
}

// buildDeliveryAttemptResultCorrection 装配派送结果更正：只有更正册一只适配器。更正成妥投不形成别的事实，有效交付仍由交付登记另登。
func buildDeliveryAttemptResultCorrection(db *bentopg.DB) (tfhttp.DeliveryAttemptResultCorrector, error) {
	corrections, err := tfpostgres.NewDeliveryResultCorrections(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery result corrections: %w", err)
	}
	return transactionalDeliveryAttemptResultCorrection{
		transactor: db.Transactor(),
		inner: tfapp.NewCorrectDeliveryAttemptResultHandler(tfapp.CorrectDeliveryAttemptResultDeps{
			Corrections: corrections,
			Clock:       systemClock{},
		}),
	}, nil
}
