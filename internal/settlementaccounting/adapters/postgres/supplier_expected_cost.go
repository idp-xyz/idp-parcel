package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SupplierExpectedCosts 实现 ports.ExpectedCostView（读取面）与 ports.ExpectedCostRegistry
// （登记面），同一张表。
//
// 读写同表同行模型、放在一个类型里，理由与 parcel-shipment 的取消库一样：分开实现
// 就是第二处定义。表是 settlement-accounting 自己的——CONTEXT 把供应商预期成本判给
// 本上下文独立拥有，账单主张与审核应付是另外的对象，行上一列都不给它们。
//
// 登记面的落点代数曾声明在本包（那时它没有应用层调用方，不预开端口）；UC-SA-002 步 5 的
// BUY 侧形成编排落地后它有了消费方，代数随之搬进 ports.ExpectedCostSaveOutcome。
type SupplierExpectedCosts struct {
	db *bentopg.DB
}

func NewSupplierExpectedCosts(db *bentopg.DB) (*SupplierExpectedCosts, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SupplierExpectedCosts{db: db}, nil
}

var (
	_ ports.ExpectedCostView     = (*SupplierExpectedCosts)(nil)
	_ ports.ExpectedCostRegistry = (*SupplierExpectedCosts)(nil)
)

// LoadExpectedCost 按版本取回预期成本。found=false 是「这个版本不存在」——UC-SA-004
// 逐行匹配指错版本是提交矛盾，不是等谁，所以它与读取失败分成两种答案。
//
// 否定结果不区分「不存在」与「属于另一个租户」：租户条件写在 SQL 里，越权探到的
// 与真不存在长得一样（ADR-0029 同款）。
func (repository *SupplierExpectedCosts) LoadExpectedCost(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.SupplierCostVersionID,
) (domain.SupplierExpectedCost, bool, error) {
	return repository.loadOne(ctx,
		`WHERE tenant_id = $1 AND version = $2`, tenant.String(), version.String())
}

// LoadFirstVersion 按幂等三维取首版（prior_version IS NULL 那一行——部分唯一索引保证最多
// 一条）。found=false 表示这三维还没形成过预期成本。
func (repository *SupplierExpectedCosts) LoadFirstVersion(
	ctx context.Context,
	tenant domain.TenantID,
	occurrence domain.ChargeOccurrenceID,
	feeItem domain.FeeItemReference,
	ruleVersion domain.PurchaseRuleVersionReference,
) (domain.SupplierExpectedCost, bool, error) {
	return repository.loadOne(ctx,
		`WHERE tenant_id = $1 AND occurrence_id = $2 AND fee_item = $3 AND purchase_rule_version = $4
		   AND prior_version IS NULL`,
		tenant.String(), occurrence.String(), feeItem.String(), ruleVersion.String())
}

func (repository *SupplierExpectedCosts) loadOne(
	ctx context.Context,
	where string,
	args ...any,
) (domain.SupplierExpectedCost, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SupplierExpectedCost{}, false, fmt.Errorf("load expected cost: %w", err)
	}

	var (
		versionValue                                      string
		occurrenceID, occurrenceReason, occurrenceVersion string
		occurredAt                                        time.Time
		feeItem, ruleVersion, agreement, evaluation       string
		originalCurrency, settlementCurrency              string
		originalMinor, settlementMinor                    int64
		conversion, priorVersion, correctionReason        *string
	)
	err = querier.QueryRow(ctx,
		`SELECT version, occurrence_id, occurrence_reason, occurrence_version, occurred_at,
		        fee_item, purchase_rule_version, agreement_ref, evaluation_ref,
		        original_currency, original_minor, settlement_currency, settlement_minor,
		        conversion_ref, prior_version, correction_reason
		   FROM settlement_accounting.supplier_expected_cost `+where,
		args...,
	).Scan(&versionValue, &occurrenceID, &occurrenceReason, &occurrenceVersion, &occurredAt,
		&feeItem, &ruleVersion, &agreement, &evaluation,
		&originalCurrency, &originalMinor, &settlementCurrency, &settlementMinor,
		&conversion, &priorVersion, &correctionReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SupplierExpectedCost{}, false, nil
	}
	if err != nil {
		return domain.SupplierExpectedCost{}, false, fmt.Errorf("load expected cost: %w", err)
	}
	version, err := domain.NewSupplierCostVersionID(versionValue)
	if err != nil {
		return domain.SupplierExpectedCost{}, false, fmt.Errorf("load expected cost: %w", err)
	}

	spec, err := expectedCostSpecOf(expectedCostColumns{
		version:            version,
		occurrenceID:       occurrenceID,
		occurrenceReason:   occurrenceReason,
		occurrenceVersion:  occurrenceVersion,
		occurredAt:         occurredAt,
		feeItem:            feeItem,
		ruleVersion:        ruleVersion,
		agreement:          agreement,
		evaluation:         evaluation,
		originalCurrency:   originalCurrency,
		originalMinor:      originalMinor,
		settlementCurrency: settlementCurrency,
		settlementMinor:    settlementMinor,
		conversion:         conversion,
		priorVersion:       priorVersion,
		correctionReason:   correctionReason,
	})
	if err != nil {
		return domain.SupplierExpectedCost{}, false, fmt.Errorf("load expected cost: %w", err)
	}

	cost, err := domain.RehydrateSupplierExpectedCost(spec)
	if err != nil {
		return domain.SupplierExpectedCost{}, false, fmt.Errorf("load expected cost: %w", err)
	}
	return cost, true, nil
}

// Save 登记一份预期成本版本（首版或纠错版本同一入口，靠回指分辨）。同（租户+版本）
// 已有行时交回`已登记`而不是错误：那是业务答案，调用方据以读回赢家（ADR-0031）。
//
// 首版撞幂等三维时交回的也是`已登记`：那三维上的部分唯一索引拦的正是「同一发生项、
// 费用项目和规则版本重复形成预期成本」，冲突在这里是重复形成，不是技术故障。
func (repository *SupplierExpectedCosts) Save(
	ctx context.Context,
	tenant domain.TenantID,
	cost domain.SupplierExpectedCost,
	recordedAt time.Time,
) (ports.ExpectedCostSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ExpectedCostSaveOutcomeInvalid, fmt.Errorf("save expected cost: %w", err)
	}

	originalCurrency, originalMinor := cost.OriginalAmount()
	settlementCurrency, settlementMinor := cost.SettlementAmount()

	var conversionRef *string
	if conversion, crossCurrency := cost.Conversion(); crossCurrency {
		value := conversion.String()
		conversionRef = &value
	}
	var priorVersion, correctionReason *string
	prior, corrected := cost.PriorVersion()
	reason, hasReason := cost.CorrectionReason()
	if corrected != hasReason {
		// 领域不变量：纠错版本回指与原因同在同缺。走到这里是领域或适配器的 bug。
		return ports.ExpectedCostSaveOutcomeInvalid, fmt.Errorf(
			"save expected cost: 版本 %s 的纠错两件不成对", cost.Version())
	}
	if corrected {
		priorValue, reasonValue := prior.String(), reason.String()
		priorVersion, correctionReason = &priorValue, &reasonValue
	}

	occurrence := cost.Occurrence()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.supplier_expected_cost
			(tenant_id, version, occurrence_id, occurrence_reason, occurrence_version,
			 occurred_at, fee_item, purchase_rule_version, agreement_ref, evaluation_ref,
			 original_currency, original_minor, settlement_currency, settlement_minor,
			 conversion_ref, prior_version, correction_reason, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		cost.Version().String(),
		occurrence.ID().String(),
		occurrence.Reason().String(),
		occurrence.Version().String(),
		occurrence.OccurredAt().UTC(),
		cost.FeeItem().String(),
		cost.RuleVersion().String(),
		cost.Agreement().String(),
		cost.Evaluation().String(),
		originalCurrency.String(),
		originalMinor,
		settlementCurrency.String(),
		settlementMinor,
		conversionRef,
		priorVersion,
		correctionReason,
		recordedAt.UTC(),
	)
	if err != nil {
		return ports.ExpectedCostSaveOutcomeInvalid, fmt.Errorf("save expected cost: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ExpectedCostAlreadyRecorded, nil
	}
	return ports.ExpectedCostSaved, nil
}

// expectedCostColumns 是一行预期成本的原样取值。三个指针列对应库里的三个可空列：
// 换算步骤只在跨币种时在场，回指与原因只在纠错版本上在场。
type expectedCostColumns struct {
	version            domain.SupplierCostVersionID
	occurrenceID       string
	occurrenceReason   string
	occurrenceVersion  string
	occurredAt         time.Time
	feeItem            string
	ruleVersion        string
	agreement          string
	evaluation         string
	originalCurrency   string
	originalMinor      int64
	settlementCurrency string
	settlementMinor    int64
	conversion         *string
	priorVersion       *string
	correctionReason   *string
}

// expectedCostSpecOf 把一行折回重建规格。每个引用各自过自己的构造函数——空白值在
// 这里就报错，而不是让一份缺件的成本走到重建门前才被拒。
func expectedCostSpecOf(columns expectedCostColumns) (domain.RehydrateSupplierExpectedCostSpec, error) {
	spec := domain.RehydrateSupplierExpectedCostSpec{
		Version:         columns.version,
		OriginalMinor:   columns.originalMinor,
		SettlementMinor: columns.settlementMinor,
	}

	occurrence, err := rebuildChargeOccurrence(
		columns.occurrenceID, columns.occurrenceReason,
		columns.occurrenceVersion, columns.occurredAt)
	if err != nil {
		return domain.RehydrateSupplierExpectedCostSpec{}, err
	}
	spec.Occurrence = occurrence

	if spec.FeeItem, err = domain.NewFeeItemReference(columns.feeItem); err != nil {
		return domain.RehydrateSupplierExpectedCostSpec{}, err
	}
	if spec.RuleVersion, err = domain.NewPurchaseRuleVersionReference(columns.ruleVersion); err != nil {
		return domain.RehydrateSupplierExpectedCostSpec{}, err
	}
	if spec.Agreement, err = domain.NewSupplierAgreementReference(columns.agreement); err != nil {
		return domain.RehydrateSupplierExpectedCostSpec{}, err
	}
	if spec.Evaluation, err = domain.NewBuyEvaluationReference(columns.evaluation); err != nil {
		return domain.RehydrateSupplierExpectedCostSpec{}, err
	}
	if spec.OriginalCurrency, err = domain.NewCurrencyCode(columns.originalCurrency); err != nil {
		return domain.RehydrateSupplierExpectedCostSpec{}, err
	}
	if spec.SettlementCurrency, err = domain.NewCurrencyCode(columns.settlementCurrency); err != nil {
		return domain.RehydrateSupplierExpectedCostSpec{}, err
	}

	if columns.conversion != nil {
		if spec.Conversion, err = domain.NewConversionStepReference(*columns.conversion); err != nil {
			return domain.RehydrateSupplierExpectedCostSpec{}, err
		}
	}
	if columns.priorVersion != nil {
		if spec.PriorVersion, err = domain.NewSupplierCostVersionID(*columns.priorVersion); err != nil {
			return domain.RehydrateSupplierExpectedCostSpec{}, err
		}
	}
	if columns.correctionReason != nil {
		if spec.CorrectionReason, err = domain.NewCostCorrectionReason(*columns.correctionReason); err != nil {
			return domain.RehydrateSupplierExpectedCostSpec{}, err
		}
	}
	return spec, nil
}

func rebuildChargeOccurrence(
	id, reason, version string,
	occurredAt time.Time,
) (domain.TransportChargeOccurrence, error) {
	occurrenceID, err := domain.NewChargeOccurrenceID(id)
	if err != nil {
		return domain.TransportChargeOccurrence{}, err
	}
	occurrenceReason, err := domain.NewOccurrenceReasonReference(reason)
	if err != nil {
		return domain.TransportChargeOccurrence{}, err
	}
	occurrenceVersion, err := domain.NewOccurrenceVersion(version)
	if err != nil {
		return domain.TransportChargeOccurrence{}, err
	}
	return domain.NewTransportChargeOccurrence(
		occurrenceID, occurrenceReason, occurrenceVersion, occurredAt)
}
