package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// transactionalTrackingAdopter 把一条素材的认领包进一笔事务：事实版本、人工录入留痕与交给 visibility-exception 的意图
// 同生同灭。未决整笔回滚，再交回编排那一份答复——一批里各条各自一笔，一条未决不拖累别的条。
type transactionalTrackingAdopter struct {
	transactor bentoapp.Transactor
	inner      *tfapp.AdoptTrackingMaterialHandler
}

var errTrackingAdoptionUndecided = errors.New("parcel-api: tracking adoption undecided")

var errTrackingHandoffNotSent = errors.New("parcel-api: tracking fact rolled back because its handoff was not sent")

func (adopter transactionalTrackingAdopter) Adopt(ctx context.Context, material tfports.TrackingMaterial) (tfapp.AdoptTrackingMaterialResult, error) {
	var result tfapp.AdoptTrackingMaterialResult
	err := adopter.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		adopted, err := adopter.inner.Adopt(txCtx, material)
		if err != nil {
			return err
		}
		result = adopted
		if adopted.Outcome() == tfapp.TrackingAdoptionUndecided || adopted.HandoffReference() != "" {
			return errTrackingAdoptionUndecided
		}
		return nil
	})
	if err != nil && !errors.Is(err, errTrackingAdoptionUndecided) {
		return tfapp.AdoptTrackingMaterialResult{}, err
	}
	switch result.Outcome() {
	case tfapp.TrackingFactAdopted, tfapp.TrackingFactAdoptedPendingJudgment:
		if result.HandoffReference() != "" {
			// 编排答的是「已认领、意图没交出去」，而这一笔已经整笔回滚：事实并没有落。把那份答复原样交回，调用方会读到
			// 一个不存在的事实标识。如实答没形成答案，重投即可。
			return tfapp.AdoptTrackingMaterialResult{}, errTrackingHandoffNotSent
		}
	}
	return result, nil
}

// buildManualTrackingAdopter 装配承运商轨迹手工录入口背后的真编排：外部轨迹事实册（事实、留痕与人工录入留痕是同一只
// 适配器的三面）、事实身份签发器、外部承运凭证解析口、有效时间规则目录，以及交给 visibility-exception 的 Outbox 意图。
// 这是认领编排的第一个生产装配点——轨迹源适配器那一路仍等第一家真源，接上时用的是同一组构造。
func buildManualTrackingAdopter(db *bentopg.DB) (transactionalTrackingAdopter, error) {
	none := transactionalTrackingAdopter{}
	clock := systemClock{}
	facts, err := tfpostgres.NewExternalTrackingFacts(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: manual tracking facts: %w", err)
	}
	identities, err := tfpostgres.NewResultVersions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: manual tracking identities: %w", err)
	}
	credentials, err := tfpostgres.NewExternalCarrierCredentials(db, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: manual tracking credentials: %w", err)
	}
	rules, err := tfpostgres.NewEffectiveTimeRuleCatalogue(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: manual tracking effective time rules: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: manual tracking outbox store: %w", err)
	}
	downstream, err := tfpostgres.NewOutboxExternalTrackingFactHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: manual tracking handoff: %w", err)
	}
	handler := tfapp.NewAdoptTrackingMaterialHandler(tfapp.AdoptTrackingMaterialDeps{
		Facts: facts, Identities: identities, Credentials: credentials, Rules: rules, Ledger: facts,
		Downstream: downstream, Clock: clock, ManualEntries: facts,
	})
	return transactionalTrackingAdopter{transactor: db.Transactor(), inner: handler}, nil
}
