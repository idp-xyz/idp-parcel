package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// transactionalManualLodgement 把一次线下递交标记包进一笔事务：发送事实与递交留痕同生同灭。未决整笔回滚，再交回
// 编排那一份答复。
type transactionalManualLodgement struct {
	transactor bentoapp.Transactor
	inner      *customsapp.RecordManualLodgementHandler
}

var errManualLodgementUndecided = errors.New("parcel-api: manual lodgement undecided")

func (lodgement transactionalManualLodgement) Record(ctx context.Context, command customsapp.RecordManualLodgementCommand) (customsapp.RecordManualLodgementResult, error) {
	var result customsapp.RecordManualLodgementResult
	err := lodgement.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		recorded, err := lodgement.inner.Record(txCtx, command)
		if err != nil {
			return err
		}
		result = recorded
		if recorded.Outcome() == customsapp.ManualLodgementUndecided {
			return errManualLodgementUndecided
		}
		return nil
	})
	if err != nil && !errors.Is(err, errManualLodgementUndecided) {
		return customsapp.RecordManualLodgementResult{}, err
	}
	return result, nil
}

// buildManualLodgement 装配线下递交标记口背后的真编排：提交版本册、发送事实册（发送事实与递交留痕是同一只适配器的
// 两面）、就绪与授权两个读口——与 parcel-dispatch 里系统发送那一路用的是同一组构造，只是没有通道册与连接器。
func buildManualLodgement(db *bentopg.DB) (transactionalManualLodgement, error) {
	none := transactionalManualLodgement{}
	submissions, err := ccpostgres.NewDeclarationSubmissions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: declaration submissions for manual lodgement: %w", err)
	}
	sends, err := ccpostgres.NewSubmissionSends(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: submission sends for manual lodgement: %w", err)
	}
	readiness, err := ccpostgres.NewReadinessView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: readiness view for manual lodgement: %w", err)
	}
	authority, err := ccpostgres.NewSubmissionAuthorityView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: submission authority view for manual lodgement: %w", err)
	}
	handler, err := customsapp.NewRecordManualLodgementHandler(customsapp.RecordManualLodgementDeps{
		Submissions: submissions, SendFacts: sends, Readiness: readiness, Authority: authority, Lodgements: sends, Clock: systemClock{},
	})
	if err != nil {
		return none, fmt.Errorf("parcel-api: record manual lodgement: %w", err)
	}
	return transactionalManualLodgement{transactor: db.Transactor(), inner: handler}, nil
}
