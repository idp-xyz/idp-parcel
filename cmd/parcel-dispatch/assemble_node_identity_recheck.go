package main

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	noapplication "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

// recheckPendingIdentitiesConsumer 接 node-operations 的重核消费者（ADR-0199 决定三）。同一个消费者两扇门，复用同一只重核编排：
//   - PS 外部标识关联已形成的门铃 → 按号重核待识别实物；
//   - 落未核对行时本上下文自投的门铃 → 只重核指名那一件（决定三第三个触发）。
//
// 结果唯一即追加一版关联收寄并交收寄意图（意图与追加版同一笔，由消费门那一笔事务承担）。交回的一个 Consumer 按事件类型把信封
// 分给两扇门，路由表上两类各挂一行、挂的是同一个值。
//
// 身份核对读不动译成未决：信封停在 inbox 外等 PS 恢复，失败码说的是在等依赖，不是发布失败。落册、签发与入队的失败照旧记发布
// 失败——那几格要运维看的是本上下文自己的存储。
func recheckPendingIdentitiesConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	recheck, err := nodeIdentityRecheck(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	processing, err := noparcel.NewRecheckOnRelationFormedAdapter(recheck)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck on relation formed: %w", err)
	}
	consumer, err := noinbox.NewExternalIdentifierRelationFormedConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck pending identities consumer: %w", err)
	}
	named, err := noinbox.NewRecheckNamedPendingIdentity(recheck)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck the named pending identity: %w", err)
	}
	doorbell, err := noinbox.NewUncheckedPendingIdentityRecordedConsumer(db.Transactor(), inboxStore, named)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: unchecked pending identity doorbell consumer: %w", err)
	}
	routed, err := dispatch.WithUndecidedSentinels(recheckGates{
		noinbox.ExternalIdentifierRelationFormedEventType: consumer,
		noinbox.UncheckedPendingIdentityRecordedEventType: doorbell,
	}, noapplication.ErrParcelIdentityUnreadable)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck pending identities undecided translation: %w", err)
	}
	return routed, nil
}

// recheckGates 按事件类型把信封分给重核消费者的两扇门。认不得的类型交回错误，与路由表撞上无订阅者同样响亮——它只可能是路由表
// 把别的类型挂到了这个值上。
type recheckGates map[eventing.EventType]dispatch.Consumer

func (gates recheckGates) Consume(ctx context.Context, envelope eventing.Envelope) error {
	gate, found := gates[envelope.Type]
	if !found {
		return fmt.Errorf("parcel-dispatch: recheck pending identities: unexpected event type %q", envelope.Type)
	}
	return gate.Consume(ctx, envelope)
}

// nodeIdentityRecheck 装重核编排：收寄库、版本签发、收寄意图交付走 NO 真库口，身份核对读 PS 的外部标识关联册，服务结果读 PS
// 的取消与终局两册（与 parcel-api 的收寄编排同一组口，重核走的是收寄步骤 4 那一份身份判断）。编排本身有两个入口，按号重核由
// PS 门铃那一扇门用，只看指名那一件的 Recheck 由自投门铃那一扇门用，见 recheckPendingIdentitiesConsumer。
func nodeIdentityRecheck(db *bentopg.DB, outboxStore *outbox.Store, clock systemClock) (*noapplication.RecheckPendingIdentityHandler, error) {
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck receptions: %w", err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck intake result versions: %w", err)
	}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck node intake handoff: %w", err)
	}
	identifiers, err := pspostgres.NewExternalIdentifiers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck external identifiers: %w", err)
	}
	identity, err := noparcel.NewParcelIdentityView(identifiers)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck parcel identity view: %w", err)
	}
	services, err := nodeIdentityRecheckServiceResults(db)
	if err != nil {
		return nil, err
	}
	recheck, err := noapplication.NewRecheckPendingIdentityHandler(noapplication.RecheckPendingIdentityDeps{
		Identity:       identity,
		ServiceResults: services,
		Receptions:     receptions,
		Pending:        receptions,
		Versions:       versions,
		Downstream:     downstream,
		Clock:          clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: identity recheck orchestration: %w", err)
	}
	return recheck, nil
}

// nodeIdentityRecheckServiceResults 装重核的服务结果读口，外面包一层 savepoint：重核跑在消费门那一笔事务里，服务结果读不动要
// 照常追加关联收寄、核对态记读不动（ADR-0186），一条 SQL 级读错误不得把那一笔打成中止态。
func nodeIdentityRecheckServiceResults(db *bentopg.DB) (noports.ParcelServiceResultView, error) {
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck parcel cancellations: %w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck final outcomes: %w", err)
	}
	view, err := noparcel.NewServiceResultView(cancellations, finals)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck parcel service result view: %w", err)
	}
	isolated, err := nopostgres.NewSavepointedServiceResultView(db, view)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: recheck parcel service result savepoint: %w", err)
	}
	return isolated, nil
}
