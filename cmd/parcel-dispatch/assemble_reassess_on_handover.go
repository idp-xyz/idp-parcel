package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrtransportfulfillment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/transportfulfillment"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// reassessOnHandoverUndecidedSentinels 只给「交接 → NR 路由复核」这一路（ADR-0215）。与追踪投影、派送发起两路分开列：
// 同一封信扇出给几路、各记各的 inbox 账，本路还会停在 PS 读口与复核编排上，合用一份会把别路没有的格子也宣布成「等依赖」；
// 与收寄采用那一路（nrparcelshipment.ErrReassessmentUndecided）也分开，运维要分得出复核是哪一种触发停住的。
//
// 不在名单里：nrtransportfulfillment.ErrHandoverRecordInconsistent 与 nrparcelshipment.ErrAcceptedRequestInconsistent
// （仓储不变量已破）、两个包的 ErrUntranslatableAnswer（编程错误）、psdomain.ErrAmbiguousParcelTarget（两份已接受委托
// 声明同一件包裹，要人去看）——重投都不会变。不是复核触发的交接与不是当前已接受包裹的对象不在任何名单里：它们入账，
// 不是错误。
var reassessOnHandoverUndecidedSentinels = []error{
	nrtransportfulfillment.ErrHandoverNotVisible,
	nrparcelshipment.ErrAcceptedParcelUnreadable,
	nrtransportfulfillment.ErrHandoverReassessmentUndecided,
}

// reassessOnHandoverConsumer 接 TF 权威交接登记 → NR 路由复核（UC-NR-003，ADR-0215）：信封只带交接键，处理方按键读回
// 交接，只对「`已交接`且声明了接收节点」的那一版复核，所携地理投影经消费侧适配器取受理基线那一版提交的。
func reassessOnHandoverConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	settings dispatchSettings,
	clock systemClock,
) (dispatch.Consumer, error) {
	reassess, err := reassessRouteHandler(db, clock)
	if err != nil {
		return nil, err
	}
	return reassessOnHandoverConsumerOver(db, inboxStore, reassess, settings.purpose)
}

// reassessOnHandoverConsumerOver 把交接登记册、PS 委托仓储与消费门接到给定的复核编排上。生产装配交生产编排，真库用例
// 交证据视图换成替身的编排，两边的交接读取、PS 读取与消费门是同一份接法。
//
// 不另包事务：消费门已在 inbox 那一笔事务里调处理方，复核落库与 inbox 的已处理同笔提交。
func reassessOnHandoverConsumerOver(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	reassess *nrapplication.ReassessRouteHandler,
	purpose nrdomain.ServicePurpose,
) (*nrinbox.TransportHandoverConsumer, error) {
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reassess on handover transport handovers: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reassess on handover shipment requests: %w", err)
	}
	accepted, err := nrparcelshipment.NewAcceptedCarriedGeoSource(requests, requests)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: accepted carried geo source: %w", err)
	}
	handler, err := nrtransportfulfillment.NewReassessOnHandoverAdapter(handovers, accepted, reassess, purpose)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reassess on handover: %w", err)
	}
	consumer, err := nrinbox.NewTransportHandoverConsumer(db.Transactor(), inboxStore, handler)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reassess on handover consumer: %w", err)
	}
	return consumer, nil
}

// reassessRouteHandler 是 UC-NR-003 复核编排的生产接法。收寄采用那一路（networkIntakeConsumer）与交接这一路共用这一份：
// 两种触发复核的是同一份路由判断，证据、自动改路事实与判断库接法一处定义，不会一路改了另一路还停在旧接法上。
func reassessRouteHandler(db *bentopg.DB, clock systemClock) (*nrapplication.ReassessRouteHandler, error) {
	evidence, err := initialRouteEvidence(db, clock, customsApplicabilitySource(db))
	if err != nil {
		return nil, err
	}
	return reassessRouteHandlerOver(db, clock, evidence)
}

// reassessRouteHandlerOver 把判断库各册接到给定的证据视图与时钟上。各册都是 db 上的无状态包装，每次现建。
func reassessRouteHandlerOver(
	db *bentopg.DB,
	clock nrports.Clock,
	evidence nrports.InitialRouteEvidenceView,
) (*nrapplication.ReassessRouteHandler, error) {
	routeStore, err := nrpostgres.NewInitialRoutes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: initial route store: %w", err)
	}
	applicabilities, err := nrpostgres.NewPlanApplicabilities(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: plan applicabilities: %w", err)
	}
	reassessments, err := nrpostgres.NewRouteReassessments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: route reassessments: %w", err)
	}
	handoffLog, err := nrpostgres.NewRouteHandoffLogs(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: route handoff log: %w", err)
	}
	identities, err := nrpostgres.NewRouteIdentities(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: route identities: %w", err)
	}
	rerouteFacts, err := nrpostgres.NewAutoRerouteFactsCatalog(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: auto reroute facts: %w", err)
	}
	return nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes:        routeStore,
		Evidence:      evidence,
		Applicability: applicabilities,
		Store:         reassessments,
		Log:           handoffLog,
		Identities:    identities,
		Clock:         clock,
		// 自动改路四条件的事实目录已就位（审计票 05）：按判断键取当前陈述，从未
		// 登记的键照端口第二格答未配置——失效照常落库、改路评估整段不做，与先前
		// 显式 nil 在空册上的行为等价；登记过的键才走 7B/7C。改善阈值、改路条件
		// 与权限、冻结边界的取值属 PAR-NET-14 实例半边，目录只存登记方折算完的
		// 陈述与出处，不种任何默认行。
		AutoReroute: rerouteFacts,
	}), nil
}
