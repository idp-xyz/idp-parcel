package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件把票面点名的结算编排接进生产装配。审核没有单独的构造函数，它是
// ReceiveSupplierBillHandler.Audit。确认与截单先问触发册：没登记答未配置，不调用编排。
// 账期不在这本册里。

type settlementOrchestrations struct {
	transactor bentoapp.Transactor
	moments    *saapplication.AdmitSettlementMomentHandler
	confirm    *saapplication.ConfirmChargeHandler
	cutoff     *saapplication.CutOffPublishStatementHandler
	adjust     *saapplication.RecordChargeAdjustmentHandler
	allocate   *saapplication.AllocateCostsHandler
	supplier   *saapplication.ReceiveSupplierBillHandler
	claims     *saapplication.SettleClaimAmountsHandler
}

func buildSettlementOrchestrations(db *bentopg.DB) (settlementOrchestrations, error) {
	none := settlementOrchestrations{}
	if db == nil {
		return none, fmt.Errorf("parcel-api: settlement orchestrations: db is nil")
	}
	clock := systemClock{}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement orchestrations outbox: %w", err)
	}
	charges, err := sapostgres.NewCustomerCharges(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customer charges: %w", err)
	}
	conditions, err := sapostgres.NewChargeConfirmationConditions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: confirmation conditions: %w", err)
	}
	catalogues, err := sapostgres.NewSettlementCatalogues(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement catalogues: %w", err)
	}
	confirmHandoff, err := sapostgres.NewOutboxChargeConfirmationHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: charge confirmation handoff: %w", err)
	}
	adjustments, err := sapostgres.NewChargeAdjustments(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: charge adjustments: %w", err)
	}
	statements, err := sapostgres.NewCustomerStatements(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customer statements: %w", err)
	}
	inclusions, err := sapostgres.NewSubsequentInclusions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: subsequent inclusions: %w", err)
	}
	disputes, err := sapostgres.NewStatementDisputes(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: statement disputes: %w", err)
	}
	statementHandoff, err := sapostgres.NewOutboxStatementHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: statement handoff: %w", err)
	}
	allocations, err := sapostgres.NewCostAllocations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: cost allocations: %w", err)
	}
	results, err := sapostgres.NewOperatingResults(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: operating results: %w", err)
	}
	rules, err := sapostgres.NewAllocationRuleApplicability(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: allocation rules: %w", err)
	}
	operatingHandoff, err := sapostgres.NewOutboxOperatingHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: operating handoff: %w", err)
	}
	receptions, err := sapostgres.NewBillReceptions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: bill receptions: %w", err)
	}
	costs, err := sapostgres.NewSupplierExpectedCosts(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: supplier expected costs: %w", err)
	}
	ceilings, err := sapostgres.NewAuditEscalationCeilings(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: audit escalation ceilings: %w", err)
	}
	payables, err := sapostgres.NewAuditedPayables(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: audited payables: %w", err)
	}
	creditNotes, err := sapostgres.NewSupplierCreditNotes(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: supplier credit notes: %w", err)
	}
	billHandoff, err := sapostgres.NewOutboxSupplierBillHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: supplier bill handoff: %w", err)
	}
	claimAmounts, err := sapostgres.NewCustomerClaimAmounts(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: claim amounts: %w", err)
	}
	receivables, err := sapostgres.NewRecoveryReceivables(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: recovery receivables: %w", err)
	}
	acknowledgements, err := sapostgres.NewRecoveryAcknowledgements(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: recovery acknowledgements: %w", err)
	}
	claimAdjustments, err := sapostgres.NewClaimAmountAdjustments(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: claim amount adjustments: %w", err)
	}
	grammars, err := sapostgres.NewAmountGrammars(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: amount grammars: %w", err)
	}
	claimHandoff, err := sapostgres.NewOutboxClaimSettlementHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: claim settlement handoff: %w", err)
	}
	momentStore, err := sapostgres.NewSettlementMoments(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement moments: %w", err)
	}
	moments, err := saapplication.NewAdmitSettlementMomentHandler(momentStore)
	if err != nil {
		return none, fmt.Errorf("parcel-api: admit settlement moment: %w", err)
	}
	return settlementOrchestrations{
		transactor: db.Transactor(),
		moments:    moments,
		confirm: saapplication.NewConfirmChargeHandler(saapplication.ConfirmChargeDeps{
			Charges: charges, Conditions: conditions, Facts: catalogues, Downstream: confirmHandoff, Clock: clock,
		}),
		cutoff: saapplication.NewCutOffPublishStatementHandler(saapplication.CutOffPublishStatementDeps{
			Statements: statements, Charges: charges, Adjustments: adjustments, Inclusions: inclusions,
			Disputes: disputes, Downstream: statementHandoff, Clock: clock,
		}),
		adjust: saapplication.NewRecordChargeAdjustmentHandler(saapplication.RecordChargeAdjustmentDeps{
			Charges: charges, Adjustments: adjustments, Clock: clock,
		}),
		allocate: saapplication.NewAllocateCostsHandler(saapplication.AllocateCostsDeps{
			Allocations: allocations, Results: results, Rules: rules, Downstream: operatingHandoff, Clock: clock,
		}),
		supplier: saapplication.NewReceiveSupplierBillHandler(saapplication.ReceiveSupplierBillDeps{
			Receptions: receptions, Costs: costs, Authority: catalogues, Accounts: catalogues, Escalation: ceilings,
			Payables: payables, CreditNotes: creditNotes, Downstream: billHandoff, Clock: clock,
		}),
		claims: saapplication.NewSettleClaimAmountsHandler(saapplication.SettleClaimAmountsDeps{
			Amounts: claimAmounts, Receivables: receivables, Acknowledgements: acknowledgements,
			Adjustments: claimAdjustments, Rules: catalogues, Grammar: grammars, Downstream: claimHandoff, Clock: clock,
		}),
	}, nil
}

// Confirm 在确认触发已登记时才调用确认编排。没登记答未配置。
func (orchestrations settlementOrchestrations) Confirm(
	ctx context.Context,
	command saapplication.ConfirmChargeCommand,
) (saapplication.SettlementMomentOutcome, saapplication.ConfirmChargeResult, error) {
	var outcome saapplication.SettlementMomentOutcome
	var result saapplication.ConfirmChargeResult
	err := orchestrations.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		admitted, admitErr := orchestrations.moments.Admit(txCtx, command.TenantID, sadomain.SettlementMomentConfirm)
		if admitErr != nil {
			return admitErr
		}
		outcome = admitted
		if admitted != saapplication.SettlementMomentAdmitted {
			return nil
		}
		handled, handleErr := orchestrations.confirm.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return 0, saapplication.ConfirmChargeResult{}, err
	}
	return outcome, result, nil
}

// CutOff 在截单触发已登记时才调用截单发布。没登记答未配置。账期仍在命令里，这里不填。
func (orchestrations settlementOrchestrations) CutOff(
	ctx context.Context,
	command saapplication.PublishStatementCommand,
) (saapplication.SettlementMomentOutcome, saapplication.StatementResult, error) {
	var outcome saapplication.SettlementMomentOutcome
	var result saapplication.StatementResult
	err := orchestrations.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		admitted, admitErr := orchestrations.moments.Admit(txCtx, command.TenantID, sadomain.SettlementMomentCutOff)
		if admitErr != nil {
			return admitErr
		}
		outcome = admitted
		if admitted != saapplication.SettlementMomentAdmitted {
			return nil
		}
		published, publishErr := orchestrations.cutoff.Publish(txCtx, command)
		if publishErr != nil {
			return publishErr
		}
		result = published
		return nil
	})
	if err != nil {
		return 0, saapplication.StatementResult{}, err
	}
	return outcome, result, nil
}
