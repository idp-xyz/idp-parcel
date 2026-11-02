package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	tfdocnumber "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/docnumber"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalTransportCommissionSubmitter 把一次委托提交包进一笔事务，理由随 transactionalDelivery：委托册的写口与
// Outbox 入队按框架合同无事务即拒，事务边界归装配点；委托与「委托已提交」意图同笔落地，编排返回 error 时整笔回滚。
// 协议核对读的是 party-commercial 的已提交数据，落在同一笔里不改变它读到什么。
//
// 编排答`未决`时整笔回滚而不提交，再交回那一份答复：读口（协议点读、委托册）在事务里报了 SQL 错误，PostgreSQL 就把
// 整笔事务置为 aborted，照常提交只换回提交错误，线上落成 500 `NO_ANSWER_FORMED`，与 ADR-0202 决定三写的「PC 读不动
// 答未决」对不上；`未决`各格也没有任何东西要落库。先例是 errActionAdmissionUnreadable。
//
// 编排答`编号规则撞号`时同样整笔回滚再交回答复（ADR-0355 决定八）：那时委托行已落、取号编排已从序列取过一段流水，提交就把
// 那一行留成没有号的委托、把那段流水全烧成空号；回滚把两者一起撤掉，下一次开立仍从同一段取起。
type transactionalTransportCommissionSubmitter struct {
	transactor bentoapp.Transactor
	inner      *tfapp.CommissionTransportHandler
}

// errTransportCommissionUndecided 只在本壳里流转：编排答`未决`或`编号规则撞号`时回调交它，逼事务回滚；回滚之后认出它，
// 交回编排那一份答复。
var errTransportCommissionUndecided = errors.New("parcel-api: transport commission undecided; transaction rolled back")

var _ tfhttp.TransportCommissionSubmitter = transactionalTransportCommissionSubmitter{}

func (submitter transactionalTransportCommissionSubmitter) SubmitCommission(
	ctx context.Context,
	command tfapp.SubmitCommissionCommand,
) (tfapp.CommissionResult, error) {
	var result tfapp.CommissionResult
	err := submitter.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := submitter.inner.SubmitCommission(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if rollsBackCommissionFace(handled.Outcome()) {
			return errTransportCommissionUndecided
		}
		return nil
	})
	if errors.Is(err, errTransportCommissionUndecided) {
		return result, nil
	}
	if err != nil {
		return tfapp.CommissionResult{}, err
	}
	return result, nil
}

// rollsBackCommissionFace 答这一格答复要不要让开立事务回滚：`未决`各格没有东西要落、读口报错时事务已 aborted；
// `编号规则撞号`时委托或订舱那一行已落、流水已取，必须一起撤掉（ADR-0355 决定八）。
func rollsBackCommissionFace(outcome tfapp.CommissionOutcome) bool {
	return outcome == tfapp.CommissionUndecided || outcome == tfapp.CommissionNumberingRuleCollision
}

// commissionDocumentNumbers 把编号组件的取号编排接到运输履约的消费侧端口上（ADR-0025、ADR-0355 决定四）。编号组件由
// parcel-api 在同一个 db 上装一次（docnumbering.Build），开立事务与序列取值因而同属一个 DB。
func commissionDocumentNumbers(numbers docnumbering.Components) (*tfdocnumber.Issuer, error) {
	if numbers.Assigner == nil {
		return nil, errors.New("parcel-api: transport fulfillment needs the document number assigner")
	}
	return tfdocnumber.NewIssuer(numbers.Assigner)
}

// buildTransportCommissionSubmission 装配 `/transport-fulfillment-transport-commission-submissions` 背后的真编排（票
// demo-chain-gaps/23，ADR-0202）。缝全接真：委托册、供应商协议核对（PC 的版本点读口经 TF 消费侧适配器，ADR-0025）、
// Outbox 意图交付、取号（编号组件经 TF 消费侧适配器，委托表答「是否已用」并记号，ADR-0355）与时钟。订舱与承运应答的册子
// 留给票 24 接：本口只提交委托，编排的那几条缝在这里不装。
//
// 「委托已提交」信封照 ADR-0066 不登派发路由，派发对它如实答 `dispatch.no_subscriber` 直到失败预算耗尽——那是记录里认下
// 的代价，不是装配缺口（ADR-0202 决定六）。
func buildTransportCommissionSubmission(db *bentopg.DB, numbers docnumbering.Components) (tfhttp.TransportCommissionSubmitter, error) {
	issuer, err := commissionDocumentNumbers(numbers)
	if err != nil {
		return nil, err
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: transport commissions: %w", err)
	}
	contents, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: supplier agreement contents: %w", err)
	}
	agreements, err := tfpartycommercial.NewSupplierAgreementSource(contents)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: supplier agreement source: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := tfpostgres.NewOutboxTransportCommissionHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: transport commission handoff: %w", err)
	}
	handler := tfapp.NewCommissionTransportHandler(tfapp.CommissionTransportDeps{
		Commissions:       commissions,
		Agreements:        agreements,
		Downstream:        downstream,
		Clock:             clock,
		Numbers:           issuer,
		CommissionNumbers: commissions,
	})
	return transactionalTransportCommissionSubmitter{transactor: db.Transactor(), inner: handler}, nil
}

// transactionalBookingFaces 把订舱提交与承运答复各包进一笔事务，理由与未决、编号规则撞号时整笔回滚的做法同委托提交那只壳：
// 订舱册、答复册与承运答复挡回册的写口按框架合同无事务即拒，挡回与那一次答复同笔；读口（委托、班次、订舱、答复）或挡回册在
// 事务里报了 SQL 错误，PostgreSQL 就把整笔事务置为 aborted，照常提交只换回提交错误。`未决`各格都不落库，回滚不丢东西；
// 撞号时订舱那一行与取过的流水一起撤掉。
type transactionalBookingFaces struct {
	transactor bentoapp.Transactor
	inner      *tfapp.CommissionTransportHandler
}

// errBookingFaceUndecided 只在本壳里流转：编排答`未决`或`编号规则撞号`时回调交它，逼事务回滚；回滚之后认出它，交回编排那一份
// 答复。
var errBookingFaceUndecided = errors.New("parcel-api: booking face undecided; transaction rolled back")

var (
	_ tfhttp.BookingRequestSubmitter = transactionalBookingFaces{}
	_ tfhttp.BookingAnswerer         = transactionalBookingFaces{}
)

func (faces transactionalBookingFaces) SubmitBooking(ctx context.Context, command tfapp.SubmitBookingCommand) (tfapp.CommissionResult, error) {
	return faces.within(ctx, func(txCtx context.Context) (tfapp.CommissionResult, error) {
		return faces.inner.SubmitBooking(txCtx, command)
	})
}

func (faces transactionalBookingFaces) AnswerBooking(ctx context.Context, command tfapp.AnswerBookingCommand) (tfapp.CommissionResult, error) {
	return faces.within(ctx, func(txCtx context.Context) (tfapp.CommissionResult, error) {
		return faces.inner.AnswerBooking(txCtx, command)
	})
}

func (faces transactionalBookingFaces) within(
	ctx context.Context,
	handle func(context.Context) (tfapp.CommissionResult, error),
) (tfapp.CommissionResult, error) {
	var result tfapp.CommissionResult
	err := faces.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := handle(txCtx)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if rollsBackCommissionFace(handled.Outcome()) {
			return errBookingFaceUndecided
		}
		return nil
	})
	if errors.Is(err, errBookingFaceUndecided) {
		return result, nil
	}
	if err != nil {
		return tfapp.CommissionResult{}, err
	}
	return result, nil
}

// buildBookingFaces 装配 `/transport-fulfillment-booking-request-submissions` 与 `/transport-fulfillment/booking-answers`
// 背后的真编排（票 demo-chain-gaps/24，ADR-0202 决定二、五）。缝全接真：委托册（订舱核委托在册）、订舱册（答复口把订舱连同
// 服务方一次读回，ADR-0239 决定三）、答复册、承运答复挡回册、班次登记（只读口）、订舱取号（订舱表答「是否已用」并记号，
// ADR-0355）与时钟。与委托提交口不共用一只编排实例：这一只不装协议核对、委托意图交付与委托取号——两口都不提交委托，装了也
// 只是让这只实例多一条够不着的路。订舱与答复都不入队：今天没有它们的消费者。承运答复不取号，照旧按订舱标识认单。
func buildBookingFaces(db *bentopg.DB, numbers docnumbering.Components) (transactionalBookingFaces, error) {
	issuer, err := commissionDocumentNumbers(numbers)
	if err != nil {
		return transactionalBookingFaces{}, err
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		return transactionalBookingFaces{}, fmt.Errorf("parcel-api: transport commissions: %w", err)
	}
	bookings, err := tfpostgres.NewBookingRequests(db)
	if err != nil {
		return transactionalBookingFaces{}, fmt.Errorf("parcel-api: booking requests: %w", err)
	}
	answers, err := tfpostgres.NewBookingAnswers(db)
	if err != nil {
		return transactionalBookingFaces{}, fmt.Errorf("parcel-api: booking answers: %w", err)
	}
	bounces, err := tfpostgres.NewBookingAnswerBounces(db)
	if err != nil {
		return transactionalBookingFaces{}, fmt.Errorf("parcel-api: booking answer bounces: %w", err)
	}
	schedules, err := tfpostgres.NewTransportSchedules(db)
	if err != nil {
		return transactionalBookingFaces{}, fmt.Errorf("parcel-api: transport schedules: %w", err)
	}
	handler := tfapp.NewCommissionTransportHandler(tfapp.CommissionTransportDeps{
		Commissions:          commissions,
		Bookings:             bookings,
		BookingsWithProvider: bookings,
		Answers:              answers,
		AnswerBounces:        bounces,
		Schedules:            schedules,
		Clock:                systemClock{},
		Numbers:              issuer,
		BookingNumbers:       bookings,
	})
	return transactionalBookingFaces{transactor: db.Transactor(), inner: handler}, nil
}
