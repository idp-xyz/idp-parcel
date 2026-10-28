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
	attributionForms, err := sapostgres.NewChargeAttributions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: charge attributions: %w", err)
	}
	arrangements, err := sapostgres.NewStatementArrangements(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: statement arrangements: %w", err)
	}
	// 确认与截单共用这一份判定：归属日随确认形成，确认时没判出的截单当场判（ADR-0269 决定三）。两本册空着时
	// 答未配置，不为此种下任何行。
	attributions, err := saapplication.NewJudgeChargeAttributionHandler(attributionForms, arrangements)
	if err != nil {
		return none, fmt.Errorf("parcel-api: judge charge attribution: %w", err)
	}
	momentStore, err := sapostgres.NewSettlementMoments(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement moments: %w", err)
	}
	moments, err := saapplication.NewAdmitSettlementMomentHandler(momentStore)
	if err != nil {
		return none, fmt.Errorf("parcel-api: admit settlement moment: %w", err)
	}
	occupation, err := occupationConversion(db, clock)
	if err != nil {
		return none, err
	}
	confirm, err := saapplication.NewConfirmChargeHandler(saapplication.ConfirmChargeDeps{
		Charges: charges, Conditions: conditions, Facts: catalogues, Attributions: attributions,
		Downstream: confirmHandoff, Occupation: occupation, Clock: clock,
	})
	if err != nil {
		return none, fmt.Errorf("parcel-api: confirm charge: %w", err)
	}
	cutoff, err := saapplication.NewCutOffPublishStatementHandler(saapplication.CutOffPublishStatementDeps{
		Statements: statements, Charges: charges, Billing: statements, Attributions: attributions,
		AttributionDates: charges, Adjustments: adjustments, Inclusions: inclusions,
		Disputes: disputes, Downstream: statementHandoff, Clock: clock,
	})
	if err != nil {
		return none, fmt.Errorf("parcel-api: cut-off publish statement: %w", err)
	}
	return settlementOrchestrations{
		transactor: db.Transactor(),
		moments:    moments,
		confirm:    confirm,
		cutoff:     cutoff,
		adjust: saapplication.NewRecordChargeAdjustmentHandler(saapplication.RecordChargeAdjustmentDeps{
			Charges: charges, Adjustments: adjustments, Clock: clock,
		}),
		allocate: saapplication.NewAllocateCostsHandler(saapplication.AllocateCostsDeps{
			Allocations: allocations, Results: results, Rules: rules, Downstream: operatingHandoff, Clock: clock,
		}),
		supplier: saapplication.NewReceiveSupplierBillHandler(saapplication.ReceiveSupplierBillDeps{
			Receptions: receptions, Costs: costs, Successions: costs, Authority: catalogues, Accounts: catalogues, Escalation: ceilings,
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

// ReceiveSupplierBill 在一笔事务里接收一份供应商账单：接收行、逐行匹配与发布意图同生同灭。
func (orchestrations settlementOrchestrations) ReceiveSupplierBill(
	ctx context.Context,
	command saapplication.ReceiveSupplierBillCommand,
) (saapplication.ReceiveSupplierBillResult, error) {
	var result saapplication.ReceiveSupplierBillResult
	err := orchestrations.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		received, handleErr := orchestrations.supplier.Handle(txCtx, command)
		result = received
		return handleErr
	})
	if err != nil {
		return saapplication.ReceiveSupplierBillResult{}, err
	}
	return result, nil
}

// AuditSupplierBill 在一笔事务里审核一行：应付行与发布意图同生同灭。
func (orchestrations settlementOrchestrations) AuditSupplierBill(
	ctx context.Context,
	command saapplication.AuditBillLineCommand,
) (saapplication.AuditResult, error) {
	var result saapplication.AuditResult
	err := orchestrations.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		audited, auditErr := orchestrations.supplier.Audit(txCtx, command)
		result = audited
		return auditErr
	})
	if err != nil {
		return saapplication.AuditResult{}, err
	}
	return result, nil
}

// occupationConversion 装费用确认同笔的占用收口（ADR-0343）：冻结与暴露账本、控制作用域锁、转出册，以及重算入账余额与已确认未结
// 的那一口（与客户收款映射同一个 CustomerReceiptPostings、同一个求法）。parcel-api 与 parcel-dispatch 的确认入口装的是同一组册，
// 两个进程各装一份，答案不因从哪个进程进来而不同。
func occupationConversion(db *bentopg.DB, clock systemClock) (*saapplication.ConvertOccupationOnChargeConfirmationHandler, error) {
	book, err := sapostgres.NewPreAcceptanceConversions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: pre-acceptance conversions: %w", err)
	}
	freezes, err := sapostgres.NewFreezeLedgers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: freeze ledgers for conversion: %w", err)
	}
	exposures, err := sapostgres.NewCreditExposureLedgers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: credit exposure ledgers for conversion: %w", err)
	}
	scopeLocks, err := sapostgres.NewPreAcceptanceControlScopeLocks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: control scope locks for conversion: %w", err)
	}
	postings, err := sapostgres.NewCustomerReceiptPostings(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: customer receipt postings for conversion: %w", err)
	}
	handler, err := saapplication.NewConvertOccupationOnChargeConfirmationHandler(saapplication.ConvertOccupationDeps{
		Book: book, Freezes: freezes, Exposures: exposures, ScopeLock: scopeLocks,
		Balances: postings, Books: postings, Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: convert occupation: %w", err)
	}
	return handler, nil
}
