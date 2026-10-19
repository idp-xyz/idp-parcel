package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SellEvaluationRequests 实现 ports.SellEvaluationRequestRegistry（登记面）与 ports.SellEvaluationRequestView（只读
// 半边），同一张表、同一份行模型——分开实现就是第二处定义。
//
// 幂等由库上的唯一约束守：主键（租户+铸造 ID）拦同一份重放，自然键（租户+包裹+计费时点）拦另一个 ID 带同一件包裹
// 同一时点再来（ADR-0190 决定一、二）。撞上哪一条都答`已存在`、不覆盖先到者（写入代数同 ADR-0031）。
type SellEvaluationRequests struct {
	db *bentopg.DB
}

func NewSellEvaluationRequests(db *bentopg.DB) (*SellEvaluationRequests, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SellEvaluationRequests{db: db}, nil
}

var (
	_ ports.SellEvaluationRequestRegistry = (*SellEvaluationRequests)(nil)
	_ ports.SellEvaluationRequestView     = (*SellEvaluationRequests)(nil)
)

// Save 登记一份 SELL 评价请求，状态格落`已提交`。撞主键或撞自然键唯一约束都交回`已存在`而不是错误：那是业务答案，
// 编排据以按自然键读回先到者、交回原 ID。
//
// 写之前先核键与对象说的是同一件事：主键列来自 record.Key、内容列来自 record.Request，两者之间库上没有约束能拦；
// 零值对象的 ID 与零值键相等，单比相等拦不住它，所以键的两维先各核非空。记录若带着`已提交`之外的状态格同样拒——
// 登记口写下的只能是「问了什么」，形成的结论另有写口。
func (registry *SellEvaluationRequests) Save(
	ctx context.Context,
	record ports.SellEvaluationRequestRecord,
) (ports.EvaluationRequestSaveOutcome, error) {
	if strings.TrimSpace(record.Key.TenantID.String()) == "" || strings.TrimSpace(record.Key.Request.String()) == "" {
		return ports.EvaluationRequestSaveOutcomeInvalid,
			fmt.Errorf("save sell evaluation request: the tenant or the request ID in the key is blank")
	}
	if record.Key.Request != record.Request.ID() {
		return ports.EvaluationRequestSaveOutcomeInvalid,
			fmt.Errorf("save sell evaluation request: key %q disagrees with the request %q it claims to index",
				record.Key.Request.String(), record.Request.ID().String())
	}
	if record.State != domain.SellRequestSubmitted || record.FormedCharge.String() != "" {
		return ports.EvaluationRequestSaveOutcomeInvalid,
			fmt.Errorf("save sell evaluation request: a registration is %s without a charge, got %q with charge %q",
				domain.SellRequestSubmitted, record.State.String(), record.FormedCharge.String())
	}

	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.EvaluationRequestSaveOutcomeInvalid, fmt.Errorf("save sell evaluation request: %w", err)
	}

	request := record.Request
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.sell_evaluation_request
			(tenant_id, request_id, parcel_id, account_id, settlement_currency, pricing_scope_id,
			 billing_moment, source_fact_ref, pricing_basis_at, recorded_at, state)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Request.String(),
		request.Parcel().String(),
		request.Account().String(),
		request.SettlementCurrency().String(),
		request.PricingScope().String(),
		request.Moment().String(),
		request.Source().String(),
		request.PricingBasisAt().UTC(),
		record.RecordedAt.UTC(),
		domain.SellRequestSubmitted.String(),
	)
	if err != nil {
		return ports.EvaluationRequestSaveOutcomeInvalid, fmt.Errorf("save sell evaluation request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.EvaluationRequestAlreadyRequested, nil
	}
	return ports.EvaluationRequestSaved, nil
}

// FindByID 按（租户+铸造 ID）取回。否定结果不区分「不存在」与「属于另一个租户」：租户条件写在 SQL 里，越权探到的
// 与真不存在长得一样（ADR-0029 同款）。
func (registry *SellEvaluationRequests) FindByID(
	ctx context.Context,
	key ports.SellEvaluationRequestKey,
) (ports.SellEvaluationRequestRecord, bool, error) {
	return registry.findOne(ctx, key.TenantID,
		`WHERE tenant_id = $1 AND request_id = $2`,
		key.TenantID.String(), key.Request.String())
}

// FindByNaturalKey 按（租户+包裹+计费时点）取回先到者。编排在 Save 答`已存在`时走这里读回赢家。
func (registry *SellEvaluationRequests) FindByNaturalKey(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.SellEvaluationRequestNaturalKey,
) (ports.SellEvaluationRequestRecord, bool, error) {
	return registry.findOne(ctx, tenant,
		`WHERE tenant_id = $1 AND parcel_id = $2 AND billing_moment = $3`,
		tenant.String(), key.Parcel.String(), key.Moment.String())
}

// FindCurrentByParcel 交回这件包裹登记最晚的那份，定序规则在 ports.SellEvaluationRequestView 头注。
func (registry *SellEvaluationRequests) FindCurrentByParcel(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelReference,
) (ports.SellEvaluationRequestRecord, bool, error) {
	return registry.findOne(ctx, tenant,
		`WHERE tenant_id = $1 AND parcel_id = $2 ORDER BY recorded_at DESC, request_id DESC LIMIT 1`,
		tenant.String(), parcel.String())
}

func (registry *SellEvaluationRequests) findOne(
	ctx context.Context,
	tenant domain.TenantID,
	where string,
	args ...any,
) (ports.SellEvaluationRequestRecord, bool, error) {
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SellEvaluationRequestRecord{}, false, fmt.Errorf("find sell evaluation request: %w", err)
	}

	var (
		columns sellEvaluationRequestColumns
		charge  *string
	)
	err = querier.QueryRow(ctx,
		`SELECT request_id, parcel_id, account_id, settlement_currency, pricing_scope_id,
		        billing_moment, source_fact_ref, pricing_basis_at, recorded_at, state, customer_charge_id
		   FROM settlement_accounting.sell_evaluation_request `+where,
		args...,
	).Scan(&columns.requestID, &columns.parcel, &columns.account, &columns.currency, &columns.pricingScope,
		&columns.moment, &columns.source, &columns.pricingBasisAt, &columns.recordedAt, &columns.state, &charge)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SellEvaluationRequestRecord{}, false, nil
	}
	if err != nil {
		return ports.SellEvaluationRequestRecord{}, false, fmt.Errorf("find sell evaluation request: %w", err)
	}
	if charge != nil {
		columns.charge = *charge
	}

	record, err := rebuildSellEvaluationRequest(tenant, columns)
	if err != nil {
		return ports.SellEvaluationRequestRecord{}, false, fmt.Errorf("find sell evaluation request: %w", err)
	}
	return record, true, nil
}

// sellEvaluationRequestColumns 是一行 SELL 评价请求的原样取值。customer_charge_id 是唯一可空的列，读成空串：
// 有没有费用标识由状态格判，不由指针是否为 nil 判。表上的 inserted_at 是库侧审计列，不进领域。
type sellEvaluationRequestColumns struct {
	requestID      string
	parcel         string
	account        string
	currency       string
	pricingScope   string
	moment         string
	source         string
	pricingBasisAt time.Time
	recordedAt     time.Time
	state          string
	charge         string
}

// rebuildSellEvaluationRequest 把一行折回记录。请求没有独立的重建门：SubmitSellEvaluationRequest 不相信任何输入，
// 走它就是复验。状态格与费用标识的配对再核一次——库上的 CHECK 守着同一条，这里报错只为迁移改坏约束时不静默。
func rebuildSellEvaluationRequest(
	tenant domain.TenantID,
	columns sellEvaluationRequestColumns,
) (ports.SellEvaluationRequestRecord, error) {
	none := ports.SellEvaluationRequestRecord{}
	id, err := domain.NewSellEvaluationRequestID(columns.requestID)
	if err != nil {
		return none, err
	}
	parcel, err := domain.NewParcelReference(columns.parcel)
	if err != nil {
		return none, err
	}
	account, err := domain.NewSettlementAccountID(columns.account)
	if err != nil {
		return none, err
	}
	currency, err := domain.NewCurrencyCode(columns.currency)
	if err != nil {
		return none, err
	}
	scope, err := domain.NewEstimatePricingScope(columns.pricingScope)
	if err != nil {
		return none, err
	}
	moment, err := domain.ParcelBillingMomentFromName(columns.moment)
	if err != nil {
		return none, err
	}
	source, err := domain.NewSourceFactReference(columns.source)
	if err != nil {
		return none, err
	}
	request, err := domain.SubmitSellEvaluationRequest(domain.SellEvaluationRequestSpec{
		ID:                 id,
		Parcel:             parcel,
		Account:            account,
		SettlementCurrency: currency,
		PricingScope:       scope,
		Moment:             moment,
		Source:             source,
		PricingBasisAt:     columns.pricingBasisAt,
	})
	if err != nil {
		return none, err
	}
	state, err := domain.SellEvaluationRequestStateFromName(columns.state)
	if err != nil {
		return none, err
	}
	var charge domain.CustomerChargeID
	if state.RecordsFormedCharge() {
		charge, err = domain.NewCustomerChargeID(columns.charge)
		if err != nil {
			return none, fmt.Errorf("%s is %s without a formed charge: %w", columns.requestID, state, err)
		}
	} else if columns.charge != "" {
		return none, fmt.Errorf("%s is %s yet carries charge %q", columns.requestID, state, columns.charge)
	}
	return ports.SellEvaluationRequestRecord{
		Key:          ports.SellEvaluationRequestKey{TenantID: tenant, Request: request.ID()},
		Request:      request,
		RecordedAt:   columns.recordedAt.UTC(),
		State:        state,
		FormedCharge: charge,
	}, nil
}
