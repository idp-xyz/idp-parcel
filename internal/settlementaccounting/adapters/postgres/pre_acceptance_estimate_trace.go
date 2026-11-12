package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// PreAcceptanceEstimateTraces 是受理前估价留痕（ADR-0179 决定六、ADR-0379 决定一、二）。写口写在环境事务里；按委托的读口与它同一个
// 适配器：读写共用 estimateTrialRow 一份行模型，写侧改键名读侧跟着改，不会静默读空。
type PreAcceptanceEstimateTraces struct {
	db *bentopg.DB
}

func NewPreAcceptanceEstimateTraces(db *bentopg.DB) (*PreAcceptanceEstimateTraces, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &PreAcceptanceEstimateTraces{db: db}, nil
}

var (
	_ ports.PreAcceptanceEstimateTraces = (*PreAcceptanceEstimateTraces)(nil)
	_ ports.PreAcceptanceEstimateRead   = (*PreAcceptanceEstimateTraces)(nil)
)

// estimateTrialRow 是 trials 列里一件的形状。形成了的估价每一件都有合计，所以合计与币种两格不设缺席。计价重量与费用行是补记的两格
// （迁移 0057），缺键即没记：补记之前的行没有，补记之后某件译不出那一格也没有。
type estimateTrialRow struct {
	Parcel        string                       `json:"parcel"`
	Evaluation    string                       `json:"evaluation"`
	Versions      []string                     `json:"versions"`
	TotalMinor    int64                        `json:"totalMinor"`
	Currency      string                       `json:"currency"`
	PricingWeight *estimateTrialWeightRow      `json:"pricingWeight,omitempty"`
	ChargeLines   []estimateTrialChargeLineRow `json:"chargeLines,omitempty"`
}

type estimateTrialWeightRow struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

type estimateTrialChargeLineRow struct {
	Code     string `json:"code"`
	Effect   string `json:"effect"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// RecordEstimate 插一行；键撞上即在库里逐列比内容，分重放与冲突，不覆盖。比较放在 SQL 里做：时点按同一种编码落库与比较，
// trials 按 jsonb 的相等比（补记的十进制两格按数比，见 comparableTrials），Go 侧不必再猜库里那份截到了哪一位。trial_at 在比较之列、recorded_at 不在：跨重驱是重放靠的是判断时点
// 不动（端口 PreAcceptanceEstimateTraces 的头注写了这个前提）。
//
// 已落的是补记之前的旧行时，只比它当时有的那几格：服务产品不比，trials 拿去掉补记两键的那一份比。旧行不回填（ADR-0379 决定一），
// 一次在迁移 0057 之前落了留痕、之后才重驱的控制请求拿到的是同一份估价，按全部内容比它永远答冲突，施加编排就停在`请求冲突`推不动了。
func (repository *PreAcceptanceEstimateTraces) RecordEstimate(
	ctx context.Context,
	trace ports.PreAcceptanceEstimateTrace,
) (ports.PreAcceptanceEstimateTraceEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("record pre-acceptance estimate: %w", err)
	}
	estimate := trace.Estimate
	rows, err := estimateTrialRows(estimate)
	if err != nil {
		return 0, fmt.Errorf("record pre-acceptance estimate: %w", err)
	}
	trials, err := json.Marshal(rows)
	if err != nil {
		return 0, fmt.Errorf("record pre-acceptance estimate: %w", err)
	}
	legacy, err := json.Marshal(withoutDetail(rows))
	if err != nil {
		return 0, fmt.Errorf("record pre-acceptance estimate: %w", err)
	}
	var product *string
	if declared, present := estimate.ServiceProduct(); present {
		product = &declared
	}
	key := []any{
		trace.Tenant.String(),
		trace.Scope.LegalEntity().String(),
		trace.Scope.Account().String(),
		trace.Scope.Currency().String(),
		trace.Request.String(),
	}
	content := []any{
		trace.ShipmentRequest,
		trace.SubmissionVersion,
		trace.At.UTC(),
		estimate.Form().String(),
		estimate.PricingScope().String(),
		estimate.PriceCard(),
		estimate.TotalMinor(),
		trials,
		product,
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.pre_acceptance_estimate_trace
			(tenant_id, legal_entity, account_id, currency, control_request_id,
			 shipment_request_ref, submission_version_ref, trial_at, form, pricing_scope_id,
			 price_card, total_minor, trials, service_product, recorded_at, detail_recorded)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, true)
		 ON CONFLICT ON CONSTRAINT pre_acceptance_estimate_trace_pkey DO NOTHING`,
		append(append(append([]any{}, key...), content...), trace.RecordedAt.UTC())...,
	)
	if err != nil {
		return 0, fmt.Errorf("record pre-acceptance estimate: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.PreAcceptanceEstimateTraceRecorded, nil
	}

	var same bool
	if err := executor.QueryRow(ctx,
		`SELECT shipment_request_ref = $6 AND submission_version_ref = $7 AND trial_at = $8
		        AND form = $9 AND pricing_scope_id = $10 AND price_card = $11 AND total_minor = $12
		        AND CASE WHEN detail_recorded
		                 THEN `+comparableTrials("trials")+` IS NOT DISTINCT FROM `+comparableTrials("$13::jsonb")+`
		                      AND service_product IS NOT DISTINCT FROM $14::text
		                 ELSE trials = $15::jsonb
		            END
		   FROM settlement_accounting.pre_acceptance_estimate_trace
		  WHERE tenant_id = $1 AND legal_entity = $2 AND account_id = $3 AND currency = $4
		    AND control_request_id = $5`,
		append(append(append([]any{}, key...), content...), legacy)...,
	).Scan(&same); err != nil {
		return 0, fmt.Errorf("record pre-acceptance estimate: %w", err)
	}
	if same {
		return ports.PreAcceptanceEstimateTraceReplay, nil
	}
	return ports.PreAcceptanceEstimateTraceConflict, nil
}

func estimateTrialRows(estimate domain.PreAcceptanceEstimate) ([]estimateTrialRow, error) {
	trials := estimate.Trials()
	rows := make([]estimateTrialRow, 0, len(trials))
	for _, trial := range trials {
		minor, currency, present := trial.Total()
		if !present {
			// 形成了的估价里每一件都有合计（FormPerParcelSellTrialSum 的停法），走到这里是调用方交了一份没形成的估价。
			return nil, fmt.Errorf("trial %s carries no total", trial.Parcel())
		}
		row := estimateTrialRow{
			Parcel:     trial.Parcel(),
			Evaluation: trial.Evaluation(),
			Versions:   trial.Versions(),
			TotalMinor: minor,
			Currency:   currency.String(),
		}
		if weight, weighed := trial.PricingWeight(); weighed {
			row.PricingWeight = &estimateTrialWeightRow{Value: weight.Value(), Unit: weight.Unit()}
		}
		if lines, listed := trial.ChargeLines(); listed {
			row.ChargeLines = make([]estimateTrialChargeLineRow, 0, len(lines))
			for _, line := range lines {
				row.ChargeLines = append(row.ChargeLines, estimateTrialChargeLineRow{
					Code: line.Code().String(), Effect: line.Effect().String(),
					Amount: line.Amount(), Currency: line.Currency().String(),
				})
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// comparableTrials 交回一段 SQL：把 trials 换成判重放用的形——每条费用行的金额与每件计价重量的值换成 jsonb 数值，其余各键照原样。
// 两者是计价交来的十进制原文，同一个数跨一次部署可能换了写法（「12.50」与「12.5」），按文本比，一次待续办的重驱就成了
// `请求冲突`；jsonb 的数值按 numeric 比，写法不同的同一个数相等。件序与行序照旧算数：WITH ORDINALITY 按原序收回。不是十进制写法
// 的原文照原文比，不做转换——写口不该写出那种值，转换失败也不该让一次写留痕报错。
func comparableTrials(trials string) string {
	const decimal = `'^-?[0-9]+(\.[0-9]+)?$'`
	asNumber := func(text, original string) string {
		return `CASE WHEN ` + text + ` ~ ` + decimal + ` THEN to_jsonb((` + text + `)::numeric) ELSE ` + original + ` END`
	}
	return `(SELECT jsonb_agg(trial
	            || CASE WHEN trial ? 'chargeLines' THEN jsonb_build_object('chargeLines',
	                   (SELECT jsonb_agg(line || jsonb_build_object('amount', ` + asNumber(`line->>'amount'`, `line->'amount'`) + `)
	                                     ORDER BY line_position)
	                      FROM jsonb_array_elements(trial->'chargeLines') WITH ORDINALITY AS line_rows(line, line_position)))
	               ELSE '{}'::jsonb END
	            || CASE WHEN trial ? 'pricingWeight' THEN jsonb_build_object('pricingWeight',
	                   (trial->'pricingWeight') || jsonb_build_object('value',
	                       ` + asNumber(`trial->'pricingWeight'->>'value'`, `trial->'pricingWeight'->'value'`) + `))
	               ELSE '{}'::jsonb END
	            ORDER BY trial_position)
	   FROM jsonb_array_elements(` + trials + `) WITH ORDINALITY AS trial_rows(trial, trial_position))`
}

// withoutDetail 是同一份估价在补记之前会落成的 trials：拿它与旧行比。
func withoutDetail(rows []estimateTrialRow) []estimateTrialRow {
	legacy := make([]estimateTrialRow, 0, len(rows))
	for _, row := range rows {
		row.PricingWeight, row.ChargeLines = nil, nil
		legacy = append(legacy, row)
	}
	return legacy
}

// ListPreAcceptanceEstimates 按（租户，委托）读各份留痕，连同同一控制请求在两本账上的占用（ADR-0379 决定二）。
//
// 占用按控制请求的整把键接：冻结、信用暴露与留痕三张表的主键同形（租户 + 结算作用域 + 控制请求标识），写在同一事务里，同一请求身份
// 在每本账上至多一行、金额就是这份估价的合计。受限的那一项不入账，接不上即没有占用。次序按试算时点，同刻再按落库时刻与键排，翻来覆去
// 读同一委托次序不变。
func (repository *PreAcceptanceEstimateTraces) ListPreAcceptanceEstimates(
	ctx context.Context,
	tenant domain.TenantID,
	shipmentRequest string,
) ([]ports.PreAcceptanceEstimateEntry, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pre-acceptance estimates: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT trace.submission_version_ref, trace.form, trace.pricing_scope_id, trace.price_card,
		        trace.service_product, trace.detail_recorded, trace.total_minor, trace.currency,
		        trace.trial_at, trace.recorded_at, trace.trials,
		        funds.freeze_id, funds.status, funds.amount_minor, funds.frozen_at,
		        funds.released_at, funds.converted_minor, funds.converted_at,
		        credit.exposure_id, credit.status, credit.amount_minor, credit.exposed_at,
		        credit.released_at, credit.converted_minor, credit.converted_at
		   FROM settlement_accounting.pre_acceptance_estimate_trace AS trace
		   LEFT JOIN settlement_accounting.funds_freeze AS funds
		          ON funds.tenant_id = trace.tenant_id AND funds.legal_entity = trace.legal_entity
		         AND funds.account_id = trace.account_id AND funds.currency = trace.currency
		         AND funds.control_request_id = trace.control_request_id
		   LEFT JOIN settlement_accounting.credit_exposure AS credit
		          ON credit.tenant_id = trace.tenant_id AND credit.legal_entity = trace.legal_entity
		         AND credit.account_id = trace.account_id AND credit.currency = trace.currency
		         AND credit.control_request_id = trace.control_request_id
		  WHERE trace.tenant_id = $1 AND trace.shipment_request_ref = $2
		  ORDER BY trace.trial_at, trace.recorded_at, trace.submission_version_ref,
		           trace.legal_entity, trace.account_id, trace.currency, trace.control_request_id`,
		tenant.String(), shipmentRequest,
	)
	if err != nil {
		return nil, fmt.Errorf("list pre-acceptance estimates: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.PreAcceptanceEstimateEntry, 0)
	for rows.Next() {
		var (
			entry     ports.PreAcceptanceEstimateEntry
			priceCard string
			product   *string
			trialsRaw []byte
			freeze    occupationColumns
			exposure  occupationColumns
		)
		if err := rows.Scan(
			&entry.SubmissionVersion, &entry.Form, &entry.PricingScope, &priceCard,
			&product, &entry.DetailRecorded, &entry.TotalMinor, &entry.Currency,
			&entry.TrialAt, &entry.RecordedAt, &trialsRaw,
			&freeze.id, &freeze.status, &freeze.amountMinor, &freeze.formedAt,
			&freeze.releasedAt, &freeze.convertedMinor, &freeze.convertedAt,
			&exposure.id, &exposure.status, &exposure.amountMinor, &exposure.formedAt,
			&exposure.releasedAt, &exposure.convertedMinor, &exposure.convertedAt,
		); err != nil {
			return nil, fmt.Errorf("list pre-acceptance estimates: %w", err)
		}
		parts, split := domain.SplitVersionReferenceLiteral(priceCard)
		if !split {
			return nil, fmt.Errorf("list pre-acceptance estimates: 价卡字面 %q 不是写口写下的形状", priceCard)
		}
		entry.PriceCard = parts
		if product != nil {
			entry.ServiceProduct = *product
		}
		entry.TrialAt, entry.RecordedAt = entry.TrialAt.UTC(), entry.RecordedAt.UTC()
		if entry.Parcels, err = estimateParcelsOf(trialsRaw); err != nil {
			return nil, fmt.Errorf("list pre-acceptance estimates: %w", err)
		}
		entry.Occupations = make([]ports.PreAcceptanceOccupation, 0, 2)
		for _, side := range []struct {
			kind    ports.PreAcceptanceOccupationKind
			columns occupationColumns
		}{
			{ports.OccupationFundsFreeze, freeze},
			{ports.OccupationCreditExposure, exposure},
		} {
			occupation, present, err := side.columns.occupation(side.kind, entry.Currency)
			if err != nil {
				return nil, fmt.Errorf("list pre-acceptance estimates: %w", err)
			}
			if present {
				entry.Occupations = append(entry.Occupations, occupation)
			}
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pre-acceptance estimates: %w", err)
	}
	return entries, nil
}

func estimateParcelsOf(raw []byte) ([]ports.PreAcceptanceEstimateParcel, error) {
	var rows []estimateTrialRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("trials 解码：%w", err)
	}
	parcels := make([]ports.PreAcceptanceEstimateParcel, 0, len(rows))
	for _, row := range rows {
		parcel := ports.PreAcceptanceEstimateParcel{
			Parcel: row.Parcel, Evaluation: row.Evaluation, TotalMinor: row.TotalMinor, Currency: row.Currency,
		}
		if row.PricingWeight != nil {
			parcel.PricingWeight = &ports.PreAcceptanceParcelPricingWeight{Value: row.PricingWeight.Value, Unit: row.PricingWeight.Unit}
		}
		if row.ChargeLines != nil {
			parcel.ChargeLines = make([]ports.PreAcceptanceParcelChargeLine, 0, len(row.ChargeLines))
			for _, line := range row.ChargeLines {
				parcel.ChargeLines = append(parcel.ChargeLines, ports.PreAcceptanceParcelChargeLine{
					Code: line.Code, Effect: line.Effect, Amount: line.Amount, Currency: line.Currency,
				})
			}
		}
		parcels = append(parcels, parcel)
	}
	return parcels, nil
}

// occupationColumns 是一本账左接过来的那几列；没接上时全为 nil。
type occupationColumns struct {
	id             *string
	status         *int16
	amountMinor    *int64
	formedAt       *time.Time
	releasedAt     *time.Time
	convertedMinor *int64
	convertedAt    *time.Time
}

// occupation 把状态码译成各自账本的词：译法就是两本账写入时存的 uint8(status)。受限那一格从不入账（迁移 0001 的状态约束），读到它
// 或别的码是库里有了写口写不出的行，报错，不拿空串顶。
func (columns occupationColumns) occupation(kind ports.PreAcceptanceOccupationKind, currency string) (ports.PreAcceptanceOccupation, bool, error) {
	if columns.id == nil {
		return ports.PreAcceptanceOccupation{}, false, nil
	}
	if columns.status == nil || columns.amountMinor == nil || columns.formedAt == nil || columns.convertedMinor == nil {
		return ports.PreAcceptanceOccupation{}, false, fmt.Errorf("%s %s 缺必有的列", kind, *columns.id)
	}
	var status string
	switch kind {
	case ports.OccupationFundsFreeze:
		switch code := domain.FreezeStatus(*columns.status); code {
		case domain.FreezeHeld, domain.FreezeReleased, domain.FreezeConverted:
			status = code.String()
		}
	case ports.OccupationCreditExposure:
		switch code := domain.ExposureStatus(*columns.status); code {
		case domain.ExposureRecorded, domain.ExposureReleased, domain.ExposureConverted:
			status = code.String()
		}
	}
	if status == "" {
		return ports.PreAcceptanceOccupation{}, false, fmt.Errorf("%s %s 的状态码 %d 不是账本写得出的", kind, *columns.id, *columns.status)
	}
	return ports.PreAcceptanceOccupation{
		Kind:           kind,
		ID:             *columns.id,
		Status:         status,
		AmountMinor:    *columns.amountMinor,
		Currency:       currency,
		FormedAt:       columns.formedAt.UTC(),
		ReleasedAt:     catalogueInstant(columns.releasedAt),
		ConvertedMinor: *columns.convertedMinor,
		ConvertedAt:    catalogueInstant(columns.convertedAt),
	}, true, nil
}
