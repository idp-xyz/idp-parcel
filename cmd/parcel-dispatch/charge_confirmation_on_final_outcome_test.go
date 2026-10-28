package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件钉 ADR-0336 决定一、二在生产依赖图上的两种先后：演示租户登了确认时点、并为运费项目登了「有效交付已终局」那一种
// 确认条件。
//   - 费用先形成、终局后到：SELL 评价已记录信封一拍形成预估客户费用，同笔登上七项确认事实；PS 终局信封再一拍给它记依据、
//     确认，交出费用确认信封。
//   - 终局先到、费用后形成：终局行先在 PS 册上，评价已记录那一拍形成费用时回读到它，同笔记依据并确认。
// 真库、真路由表，证据只记 S。

// Covers: AT-SA-217、AT-SA-218、AT-SA-219——真库装配。
func TestAFormedChargeIsConfirmedOnTheEffectiveDeliveryFinalInEitherOrder(t *testing.T) {
	for name, finalFirst := range map[string]bool{"费用先形成": false, "终局先到": true} {
		t.Run(name, func(t *testing.T) {
			beat, db, store := wiredBeat(t)
			tenant := saValue(t, sadomain.NewTenantID, demoTenant)
			seedConfirmingTenant(t, db, tenant)

			finalKey := effectiveDeliveryFinalKey(t, "SYN-PARCEL-01")
			finalRecord := effectiveDeliveryFinalRecord(t, finalKey)
			finals, err := pspostgres.NewFinalOutcomes(db)
			if err != nil {
				t.Fatal(err)
			}
			saveFinal := func() {
				mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
					_, err := finals.Save(txCtx, finalRecord)
					return err
				})
			}
			if finalFirst {
				saveFinal()
			}

			evaluations, err := pppostgres.NewEvaluations(db)
			if err != nil {
				t.Fatal(err)
			}
			handoff, err := pppostgres.NewOutboxEvaluationHandoff(db, store, systemClock{})
			if err != nil {
				t.Fatal(err)
			}
			evaluation := sellChargeEvaluation(t, "SYN-EVAL-SELL-CONFIRM", "SELLREQ-SYN-CONFIRM", "SYN-PARCEL-01",
				sellChargeCard(t, "REMOTE_AREA_SURCHARGE"))
			mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
				if _, err := evaluations.Save(txCtx, evaluation); err != nil {
					return err
				}
				return handoff.HandOffEvaluation(txCtx, ppports.EvaluationHandoffIntent{Evaluation: evaluation})
			})
			if _, err := beat.DispatchOnce(t.Context()); err != nil {
				t.Fatalf("评价那一拍：%v", err)
			}
			if status := outboxStatus(t, db, "SYN-EVAL-SELL-CONFIRM"); status != "PUBLISHED" {
				t.Fatalf("评价信封 status = %q；失败码 = %q", status, recordedFailureCode(t, db, "SYN-EVAL-SELL-CONFIRM"))
			}

			charges, err := sapostgres.NewCustomerCharges(db)
			if err != nil {
				t.Fatal(err)
			}
			chargeID := saValue(t, sadomain.NewCustomerChargeID, "SELLCC-SELLREQ-SYN-CONFIRM")
			catalogues, err := sapostgres.NewSettlementCatalogues(db)
			if err != nil {
				t.Fatal(err)
			}
			facts, registered, err := catalogues.LoadConfirmedChargeFacts(t.Context(), tenant, chargeID)
			if err != nil || !registered || facts.SettlementAccount.String() != "SYN-SA-SHIPPER-01-RECEIVABLE" ||
				facts.ContractBasis.String() != "SYN-CONTRACT-01" || facts.SourceFact.String() != "SYN-SOURCE-FACT-SELLREQ-SYN-CONFIRM" {
				t.Fatalf("确认事实 = %+v registered=%v err=%v", facts, registered, err)
			}

			if !finalFirst {
				charge, _, err := charges.FindByID(t.Context(), tenant, chargeID)
				if err != nil || charge.Stage() != sadomain.ChargeEstimated {
					t.Fatalf("终局之前费用阶段 = %s err = %v，want 预估", charge.Stage(), err)
				}
				saveFinal()
				enqueueForBeat(t, db, store, "final-1", sainbox.FinalOutcomeFormedEventType,
					`{"tenantId":"`+demoTenant+`","parcel":"SYN-PARCEL-01","kind":"EFFECTIVE_DELIVERY","version":"SYN-OUTCOME-V1"}`)
				if _, err := beat.DispatchOnce(t.Context()); err != nil {
					t.Fatalf("终局那一拍：%v", err)
				}
				if status := outboxStatus(t, db, "final-1"); status != "PUBLISHED" {
					t.Fatalf("终局信封 status = %q；失败码 = %q", status, recordedFailureCode(t, db, "final-1"))
				}
			}

			charge, found, err := charges.FindByID(t.Context(), tenant, chargeID)
			if err != nil || !found || charge.Stage() != sadomain.ChargeConfirmed {
				t.Fatalf("费用阶段 = %s found=%v err=%v，want 已确认", charge.Stage(), found, err)
			}
			basis, _ := charge.Confirmation()
			if basis.String() != "final-outcome/SYN-PARCEL-01/EFFECTIVE_DELIVERY/SYN-OUTCOME-V1" {
				t.Fatalf("确认依据 = %q", basis)
			}
			querier, err := db.ReadExecutor(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			var confirmations int
			if err := querier.QueryRow(t.Context(),
				`SELECT count(*) FROM bento.outbox WHERE event_type = 'settlement-accounting.charge-confirmation.formed'`,
			).Scan(&confirmations); err != nil {
				t.Fatal(err)
			}
			if confirmations != 1 {
				t.Fatalf("费用确认信封 = %d 封，want 1", confirmations)
			}
		})
	}
}

// seedConfirmingTenant 登这条线要的几本册：SELL 请求指名的账户、运费两条代码的映射、一份`已提交`的请求、运费项目的确认
// 条件、确认时点。值在事务外造好，事务回调里只做写入。
func seedConfirmingTenant(t *testing.T, db *bentopg.DB, tenant sadomain.TenantID) {
	t.Helper()
	accounts, err := sapostgres.NewSettlementAccounts(db)
	if err != nil {
		t.Fatal(err)
	}
	mappings, err := sapostgres.NewFeeItemMappings(db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := sapostgres.NewSellEvaluationRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	conditions, err := sapostgres.NewChargeConfirmationConditions(db)
	if err != nil {
		t.Fatal(err)
	}
	moments, err := sapostgres.NewSettlementMoments(db)
	if err != nil {
		t.Fatal(err)
	}
	account := sellRequestAccount(t)
	request := sellRequestRecord(t, "SELLREQ-SYN-CONFIRM", "SYN-PARCEL-01", "SYN-SCOPE-01")
	feeItem := saValue(t, sadomain.NewFeeItemReference, "SYN-FEE-ITEM-FREIGHT")
	var freight []sadomain.FeeItemMapping
	for _, code := range []string{"BASE_FREIGHT", "REMOTE_AREA_SURCHARGE"} {
		mapping, err := sadomain.NewFeeItemMapping(saValue(t, sadomain.NewPricingChargeCode, code), feeItem,
			saValue(t, sadomain.NewFeeItemMappingBasisReference, "SYN-CLAUSE-FREIGHT-01"))
		if err != nil {
			t.Fatal(err)
		}
		freight = append(freight, mapping)
	}
	at := beatInstant()
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		if _, err := accounts.Save(txCtx, tenant, account, at); err != nil {
			return err
		}
		for _, mapping := range freight {
			if _, err := mappings.SaveFeeItemMapping(txCtx, tenant, mapping, at); err != nil {
				return err
			}
		}
		if _, err := requests.Save(txCtx, request); err != nil {
			return err
		}
		if _, err := conditions.SaveConfirmationCondition(txCtx, tenant, feeItem, sadomain.EffectiveDeliveryFinalizedBasis(), at); err != nil {
			return err
		}
		_, err := moments.SaveSettlementMoment(txCtx, tenant, sadomain.SettlementMomentConfirm, at)
		return err
	})
}

func effectiveDeliveryFinalKey(t *testing.T, parcel string) psports.FinalAdoptionKey {
	t.Helper()
	tenant, err := psdomain.NewTenantID(demoTenant)
	if err != nil {
		t.Fatal(err)
	}
	parcelID, err := psdomain.NewDeclaredParcelID(parcel)
	if err != nil {
		t.Fatal(err)
	}
	version, err := psdomain.NewResponsibilityOutcomeVersion("SYN-OUTCOME-V1")
	if err != nil {
		t.Fatal(err)
	}
	return psports.FinalAdoptionKey{TenantID: tenant, Parcel: parcelID, Kind: psdomain.EffectiveDeliveryOutcome, Version: version}
}

func effectiveDeliveryFinalRecord(t *testing.T, key psports.FinalAdoptionKey) psports.FinalOutcomeRecord {
	t.Helper()
	occurredAt := beatInstant().Add(-time.Hour)
	decision, _ := psdomain.NewResponsibilityDecisionReference("SYN-DECISION-1")
	execution, _ := psdomain.NewExecutionEvidenceReference("SYN-POD-1")
	outcome, err := psdomain.NewResponsibilityOutcome(psdomain.ResponsibilityOutcomeSpec{
		Kind: key.Kind, Parcel: key.Parcel, Decision: decision, Execution: execution,
		Version: key.Version, OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("责任结果：%v", err)
	}
	finalVersion, _ := psdomain.NewFinalOutcomeVersionID("SYN-FIN-1")
	finalKind, _ := psdomain.NewFinalKindReference("SYN-DELIVERED")
	rule, _ := psdomain.NewFinalRuleVersionReference("SYN-FINAL-RULE-V1")
	final, err := psdomain.FormParcelFinalOutcome(psdomain.ParcelFinalOutcomeSpec{
		Version: finalVersion, Parcel: key.Parcel, Kind: finalKind, Source: outcome, RuleVersion: rule,
	})
	if err != nil {
		t.Fatalf("终局：%v", err)
	}
	return psports.FinalOutcomeRecord{Key: key, ContentDigest: "SYN-DIGEST-FINAL-1", Finalized: true, Final: final, AdoptedAt: beatInstant()}
}
