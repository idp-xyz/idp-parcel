package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// transactionalETAFormer 把一次 ETA 录入包进一笔事务：线锁、预测版本、人工录入留痕与交给可见性判断的意图同生同灭。
// 未决、或意图没交出去，整笔回滚；后一种不把编排那份「已形成」的答复交回——那一版并没有落。
type transactionalETAFormer struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.FormETAHandler
}

var (
	errETAEntryUndecided      = errors.New("parcel-api: eta entry undecided")
	errETAEntryHandoffNotSent = errors.New("parcel-api: eta version rolled back because its handoff was not sent")
)

func (former transactionalETAFormer) FormETA(ctx context.Context, command visibilityapp.FormETACommand) (visibilityapp.FormETAResult, error) {
	var result visibilityapp.FormETAResult
	err := former.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		formed, err := former.inner.FormETA(txCtx, command)
		if err != nil {
			return err
		}
		result = formed
		if formed.Outcome() == visibilityapp.FormETAUndecided {
			return errETAEntryUndecided
		}
		if formed.HandoffReference() != "" {
			return errETAEntryHandoffNotSent
		}
		return nil
	})
	switch {
	case err == nil, errors.Is(err, errETAEntryUndecided):
		return result, nil
	default:
		return visibilityapp.FormETAResult{}, err
	}
}

// buildETAEntry 装配预计到达手工录入口背后的真编排：预测册（预测与人工录入留痕是同一只适配器的两面）、版本标识签发器
// 与交给客户视图链的 Outbox 意图。这是 ETA 形成的第一个生产装配点；下游的 ETA 可见性判断早已接在 parcel-dispatch 上，
// 此前没有预测来源入队。缺口那一半（FormVisibilityGap）不经这个口，对应的两格不接。
func buildETAEntry(db *bentopg.DB) (transactionalETAFormer, error) {
	none := transactionalETAFormer{}
	clock := systemClock{}
	predictions, err := vepostgres.NewETAs(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: eta predictions: %w", err)
	}
	identities, err := veidentity.NewETAVersions()
	if err != nil {
		return none, fmt.Errorf("parcel-api: eta version identities: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: eta outbox store: %w", err)
	}
	viewChain, err := vepostgres.NewOutboxETAHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: eta handoff: %w", err)
	}
	handler := visibilityapp.NewFormETAHandler(visibilityapp.FormETADeps{
		Predictions: predictions, Identities: identities, ViewChain: viewChain, Clock: clock, ManualEntries: predictions,
	})
	return transactionalETAFormer{transactor: db.Transactor(), inner: handler}, nil
}
