package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	veps "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// veETAVisibilityUndecidedSentinels 只给「ETA 预测新版本 → ETA 客户可见性判断 → 客户视图重派生」这一路
// （UC-VE-003 `AT-VE-187`、UC-VE-008）。
//
// 歧义账户也在名单里：同包裹被多份已接受委托同时声明是机制拒绝自动采认（ADR-0060、AT-VE-152），运维要去 PS 侧解开
// 歧义，解开前这封信如实卡着——不任选，也不折成「无判断」。
//
// 不在名单里：ErrETAPredictionUntranslatable 与 ErrETAPredictionInconsistent（引用坏了 / 仓储不变量已破，编程错误）、
// ErrCustomerAccountUntranslatable（同前）、veconsume.ErrCustomerViewHandoffPending（要查 outbox 下游）、
// veps.ErrUnexpectedETAVisibilityOutcome、veconsume.ErrUnexpectedCustomerViewOutcome。
var veETAVisibilityUndecidedSentinels = []error{
	veps.ErrDerivedProjectionUnreadable,
	veps.ErrCustomerAccountUnavailable,
	veps.ErrAmbiguousCustomerAccount,
	veps.ErrETAVisibilityUndecided,
	veconsume.ErrCustomerViewUndecided,
}

// judgeETAVisibilityOnPredictionConsumer 接 VE 自己的内部 ETA 预测新版本 → ETA 客户可见性判断，判断落库后在同一事务里
// 重走客户视图派生（UC-VE-003、CONTEXT「ETA 新版本……必须重新派生客户视图并形成追加更正或明确替代关系」）。
//
// 账户维经 veps.ParcelCustomerAccountLookup 从 PS 的按包裹反查取回（ADR-0060 零/一/多三格）；规则册、判断库与候选 ETA
// 库是 VE 自己的库；客户视图派生复用租户现绑包装，披露策略与判断读口都在包装里按命令租户构造。
func judgeETAVisibilityOnPredictionConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	projections, err := vepostgres.NewProjections(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta visibility projections: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta visibility parcel targets: %w", err)
	}
	accounts, err := veps.NewParcelCustomerAccountLookup(requests)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta visibility account lookup: %w", err)
	}
	etas, err := vepostgres.NewETAs(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta visibility predictions: %w", err)
	}
	rules, err := vepostgres.NewETAVisibilityRules(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta visibility rules: %w", err)
	}
	judgements, err := vepostgres.NewETAVisibilityJudgements(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta visibility judgements: %w", err)
	}
	judgementIdentities, err := veidentity.NewETAVisibilityJudgementVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta visibility judgement versions: %w", err)
	}
	views, err := vepostgres.NewCustomerViews(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta visibility customer views: %w", err)
	}
	viewIdentities, err := veidentity.NewCustomerViewVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta visibility view versions: %w", err)
	}
	downstream, err := vepostgres.NewOutboxCustomerViewHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta visibility view handoff: %w", err)
	}
	processing, err := veps.NewJudgeETAVisibilityOnPredictionAdapter(
		projections,
		accounts,
		veapplication.NewJudgeETAVisibilityHandler(veapplication.JudgeETAVisibilityDeps{
			Predictions: etas,
			Rules:       rules,
			Judgements:  judgements,
			Identities:  judgementIdentities,
			Clock:       clock,
		}),
		&tenantBoundCustomerViewDerive{
			db:         db,
			views:      views,
			identities: viewIdentities,
			downstream: downstream,
			clock:      clock,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: judge eta visibility on prediction: %w", err)
	}
	consumer, err := veinbox.NewETAPredictionConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: eta prediction consumer: %w", err)
	}
	return consumer, nil
}
