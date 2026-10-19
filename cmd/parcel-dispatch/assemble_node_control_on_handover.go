package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	notf "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/transportfulfillment"
	noapplication "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// nodeControlOnHandoverUndecidedSentinels 只给「交接 → NO 节点控制转出」这一路（ADR-0214 决定六）。与同一封信上其余各路的名单分开
// 列：各路各记各的 inbox 账，本路只会停在交接登记册上，合用一份会把别路的格子也宣布成「等依赖」。
//
// 不在名单里：notf.ErrControlTransferDefect（信封键过不了本上下文的构造门、编排答出封闭集合以外的格）、
// notf.ErrHandoverRecordInconsistent 与 notf.ErrUntranslatableAnswer（提供方记录坏了）、noapplication.ErrUnexpectedControlTransitionSave
// （转移册答出集合以外的落册结果）——重投都不会变。入账不推进的各格不是错误，不在任何名单里。
var nodeControlOnHandoverUndecidedSentinels = []error{
	notf.ErrControlTransferUndecided,
}

// transferNodeControlOnHandoverConsumer 接 TF 权威交接登记 → NO 节点控制转出（ADR-0214，UC-NO-005）：信封只带交接键，消费侧适配器按
// 键读回那一版，编排按正式包裹身份对上作业实物、取实物锁、转出它此刻唯一在身的控制或入账不推进。
//
// 不另包事务：消费门已在 inbox 那一笔事务里调处理方，实物锁、转移行与 inbox 的已处理同笔提交；同一 ctx 带着同一笔。转移册与对象认法
// 是同一只适配器：两问都读收寄册与转移册。
func transferNodeControlOnHandoverConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	clock noports.Clock,
) (*noinbox.TransportHandoverRegisteredConsumer, error) {
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node control transport handovers: %w", err)
	}
	source, err := notf.NewAuthoritativeHandoverSource(handovers)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node control authoritative handover source: %w", err)
	}
	transitions, err := nopostgres.NewPhysicalControlTransitions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node control transitions: %w", err)
	}
	transfer, err := noapplication.NewTransferControlOnHandoverHandler(noapplication.TransferControlOnHandoverDeps{
		Handovers: source,
		Objects:   transitions,
		Controls:  transitions,
		Clock:     clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node control transfer orchestration: %w", err)
	}
	processing, err := notf.NewTransferControlOnHandoverAdapter(transfer)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: transfer node control on handover: %w", err)
	}
	consumer, err := noinbox.NewTransportHandoverRegisteredConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node control on handover consumer: %w", err)
	}
	return consumer, nil
}
