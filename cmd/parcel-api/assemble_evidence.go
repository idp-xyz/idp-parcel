package main

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// evidenceFaces 把证据的登记、评价与披露版本准备各包进一笔事务：证据册的写口按框架合同无事务即拒，事务边界归装配点。
// 壳复用关务各决定口那一只：未决整笔回滚、再交回编排那一份答复。不发布意图——证据项是被引的本体，今天没有订阅者。
type evidenceFaces struct {
	submission customsDecisionTransaction[visibilityapp.SubmitEvidenceCommand, visibilityapp.ManageEvidenceResult]
	appraisal  customsDecisionTransaction[visibilityapp.AppraiseEvidenceCommand, visibilityapp.ManageEvidenceResult]
	disclosure customsDecisionTransaction[visibilityapp.PrepareEvidenceDisclosureCommand, visibilityapp.ManageEvidenceResult]
}

func (faces evidenceFaces) SubmitEvidence(ctx context.Context, command visibilityapp.SubmitEvidenceCommand) (visibilityapp.ManageEvidenceResult, error) {
	return faces.submission.Handle(ctx, command)
}

func (faces evidenceFaces) Appraise(ctx context.Context, command visibilityapp.AppraiseEvidenceCommand) (visibilityapp.ManageEvidenceResult, error) {
	return faces.appraisal.Handle(ctx, command)
}

func (faces evidenceFaces) PrepareDisclosure(ctx context.Context, command visibilityapp.PrepareEvidenceDisclosureCommand) (visibilityapp.ManageEvidenceResult, error) {
	return faces.disclosure.Handle(ctx, command)
}

// buildEvidenceFaces 装配证据三口背后的真编排：证据册（本体与操作证据是同一只适配器的两面）、证据项标识签发器与生产
// 时钟。本产品只记内容指纹，不保存材料本体。
func buildEvidenceFaces(db *bentopg.DB) (evidenceFaces, error) {
	items, err := vepostgres.NewEvidenceItems(db)
	if err != nil {
		return evidenceFaces{}, fmt.Errorf("parcel-api: evidence items: %w", err)
	}
	identities, err := veidentity.NewEvidenceItems()
	if err != nil {
		return evidenceFaces{}, fmt.Errorf("parcel-api: evidence identities: %w", err)
	}
	handler := visibilityapp.NewManageEvidenceHandler(visibilityapp.ManageEvidenceDeps{
		Decisions: items, Evidence: items, Identities: identities, Clock: systemClock{},
	})
	undecided := func(result visibilityapp.ManageEvidenceResult) bool {
		return result.Outcome() == visibilityapp.ManageEvidenceUndecided
	}
	transactor := db.Transactor()
	return evidenceFaces{
		submission: customsDecisionTransaction[visibilityapp.SubmitEvidenceCommand, visibilityapp.ManageEvidenceResult]{
			transactor: transactor, handle: handler.SubmitEvidence, rollBack: undecided,
		},
		appraisal: customsDecisionTransaction[visibilityapp.AppraiseEvidenceCommand, visibilityapp.ManageEvidenceResult]{
			transactor: transactor, handle: handler.Appraise, rollBack: undecided,
		},
		disclosure: customsDecisionTransaction[visibilityapp.PrepareEvidenceDisclosureCommand, visibilityapp.ManageEvidenceResult]{
			transactor: transactor, handle: handler.PrepareDisclosure, rollBack: undecided,
		},
	}, nil
}
