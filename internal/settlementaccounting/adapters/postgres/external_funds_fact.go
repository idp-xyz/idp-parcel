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

// ExternalFundsFacts 实现 ports.ExternalFundsFactStore（写入代数同 ADR-0031）。
// 一条事实一行身份（0004 的 external_funds_fact，0021 起只留身份与首次采用时刻）、每个版本一行内容
// （0021 的 external_funds_fact_version）：首版只采用一次，更正是同一身份下回指前版的新版本。
type ExternalFundsFacts struct {
	db *bentopg.DB
}

func NewExternalFundsFacts(db *bentopg.DB) (*ExternalFundsFacts, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ExternalFundsFacts{db: db}, nil
}

// FindByKey 按（租户+事实）交回链头——没有任何一版回指它的那一版。0021 把链钉成线性——一个首版、一个前版
// 只被更正一次、回指必须指向已有版本——所以「无后继」恰好一行，不需要按时刻排序挑「最新」，也不需要标记列。
// 否定结果只回 false。读回经重建门复验更正两半。
func (repository *ExternalFundsFacts) FindByKey(
	ctx context.Context,
	key ports.FundsFactKey,
) (ports.FundsFactRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact: %w", err)
	}

	record, found, err := scanExternalFundsFact(querier.QueryRow(ctx,
		externalFundsFactSelect+`
		  WHERE v.tenant_id = $1
		    AND v.fact_id = $2
		    AND NOT EXISTS (
		        SELECT 1
		          FROM settlement_accounting.external_funds_fact_version AS successor
		         WHERE successor.tenant_id = v.tenant_id
		           AND successor.fact_id = v.fact_id
		           AND successor.corrects = v.version)`,
		key.TenantID.String(),
		key.Fact.String(),
	), key)
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact: %w", err)
	}
	return record, found, nil
}

// FindVersion 按（租户+事实+版本）取回某一版，不管它是不是链头。更正用例靠它判「同版本重放 / 冲突」
// 与核对回指；下游按信封回查走的是 AdoptedFundsFactView，读方拿不到 Save。
func (repository *ExternalFundsFacts) FindVersion(
	ctx context.Context,
	key ports.FundsFactKey,
	version domain.FundsFactVersion,
) (ports.FundsFactRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact version: %w", err)
	}

	record, found, err := scanExternalFundsFact(querier.QueryRow(ctx,
		externalFundsFactSelect+`
		  WHERE v.tenant_id = $1
		    AND v.fact_id = $2
		    AND v.version = $3`,
		key.TenantID.String(),
		key.Fact.String(),
		version.String(),
	), key)
	if err != nil {
		return ports.FundsFactRecord{}, false, fmt.Errorf("find external funds fact version: %w", err)
	}
	return record, found, nil
}

// externalFundsFactSelect 是资金事实一个版本的读回列，读的是版本子表（别名 v）；写侧 FindByKey / FindVersion
// 与只读视图 AdoptedFundsFactView 共用同一段扫描（scanExternalFundsFact），三处读回的形状不会各自漂。
const externalFundsFactSelect = `SELECT v.source_ref, v.payer_ref, v.kind, v.currency, v.amount_minor, v.version, v.occurred_at,
		        v.corrects, v.corrected_at, v.content_digest, v.recorded_at
		   FROM settlement_accounting.external_funds_fact_version AS v`

// scanExternalFundsFact 把一行扫成记录并经重建门复验更正两半。无行只回 false。
func scanExternalFundsFact(row pgx.Row, key ports.FundsFactKey) (ports.FundsFactRecord, bool, error) {
	var source, kindName, currency, version, digest string
	var payer, corrects *string
	var amount int64
	var occurredAt, recordedAt time.Time
	var correctedAt *time.Time
	err := row.Scan(&source, &payer, &kindName, &currency, &amount, &version, &occurredAt,
		&corrects, &correctedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.FundsFactRecord{}, false, nil
	}
	if err != nil {
		return ports.FundsFactRecord{}, false, err
	}

	sourceRef, err := domain.NewFundsSourceRegistrationReference(source)
	if err != nil {
		return ports.FundsFactRecord{}, false, err
	}
	kind, err := fundsFactKindFrom(kindName)
	if err != nil {
		return ports.FundsFactRecord{}, false, err
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return ports.FundsFactRecord{}, false, err
	}
	versionRef, err := domain.NewFundsFactVersion(version)
	if err != nil {
		return ports.FundsFactRecord{}, false, err
	}
	spec := domain.RehydrateExternalFundsFactSpec{
		Fact:        key.Fact,
		Source:      sourceRef,
		Kind:        kind,
		Currency:    currencyCode,
		AmountMinor: amount,
		Version:     versionRef,
		OccurredAt:  occurredAt,
	}
	// NULL 就是「来源未提供」：不造一个空付款人，事实上显式缺席。
	if payer != nil {
		spec.Payer, err = domain.NewFundsPayerReference(*payer)
		if err != nil {
			return ports.FundsFactRecord{}, false, err
		}
	}
	if corrects != nil {
		spec.Corrects, err = domain.NewFundsFactVersion(*corrects)
		if err != nil {
			return ports.FundsFactRecord{}, false, err
		}
	}
	if correctedAt != nil {
		spec.CorrectedAt = *correctedAt
	}
	fact, err := domain.RehydrateExternalFundsFact(spec)
	if err != nil {
		return ports.FundsFactRecord{}, false, err
	}
	return ports.FundsFactRecord{
		Key:           key,
		ContentDigest: digest,
		Fact:          fact,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次资金事实采用——一个版本。两步都在调用方的事务里（RequireExecutor 没有事务就拒，第二步
// 失败时第一步随事务回滚）：身份行 DO NOTHING（首版落下它、后续版本撞见它），版本行 DO NOTHING——同一
// （租户、事实、版本）已在答`已采用`、不覆盖先到者；身份已在而版本是新的则落进去，这正是更正版本的口。
//
// 版本行的 DO NOTHING 不写冲突目标：0021 守「一个首版」「一个前版只被更正一次」的唯一约束撞上时同样折成
// `已采用`，让编排像 AdoptFact 输掉竞态那样读回链头作答（谁先落谁是当前，输家拿到赢家那一版）；顺序到达的
// 同类写入在编排里就被「回指必须等于链头」挡下，到不了这里。于是同一个「回指的不是当前链头」，顺序到达在
// CorrectFact 里答`未受理`（提交矛盾），并发到达在这里折成`已采用`、编排交回赢家那一版——两答不同是有意的：
// 前者是调用方编程错误，后者是谁先落谁是当前；调用方拿到`已采用`时，从交回记录的版本字面 ≠ 命令版本分得出
// 是输掉竞态而非重放。回指一个不存在的版本是外键错，响亮报错不折。
func (repository *ExternalFundsFacts) Save(
	ctx context.Context,
	record ports.FundsFactRecord,
) (ports.FundsFactSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.FundsFactSaveOutcomeInvalid, fmt.Errorf("save external funds fact: %w", err)
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact (tenant_id, fact_id, recorded_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (tenant_id, fact_id) DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Fact.String(),
		record.RecordedAt.UTC(),
	); err != nil {
		return ports.FundsFactSaveOutcomeInvalid, fmt.Errorf("save external funds fact: %w", err)
	}

	currency, amount := record.Fact.Amount()
	corrects, hasCorrects := record.Fact.Corrects()
	correctedAt, _ := record.Fact.CorrectedAt()
	payer, hasPayer := record.Fact.Payer()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact_version
			(tenant_id, fact_id, version, source_ref, payer_ref, kind, currency, amount_minor,
			 occurred_at, corrects, corrected_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Fact.String(),
		record.Fact.Version().String(),
		record.Fact.Source().String(),
		optionalRef(payer, hasPayer),
		record.Fact.Kind().String(),
		currency.String(),
		amount,
		record.Fact.OccurredAt().UTC(),
		optionalRef(corrects, hasCorrects),
		optionalTime(correctedAt, hasCorrects),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.FundsFactSaveOutcomeInvalid, fmt.Errorf("save external funds fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.FundsFactAlreadyAdopted, nil
	}
	return ports.FundsFactSaved, nil
}

func fundsFactKindFrom(raw string) (domain.FundsFactKind, error) {
	switch raw {
	case domain.FundsReceiptConfirmed.String():
		return domain.FundsReceiptConfirmed, nil
	case domain.FundsPaymentFailed.String():
		return domain.FundsPaymentFailed, nil
	case domain.FundsReturned.String():
		return domain.FundsReturned, nil
	default:
		return 0, fmt.Errorf("unknown funds fact kind %q", raw)
	}
}

func optionalTime(value time.Time, present bool) *time.Time {
	if !present {
		return nil
	}
	utc := value.UTC()
	return &utc
}
