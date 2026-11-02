package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// DeliveryPreAdvices 实现 ports.DeliveryPreAdviceRegister（迁移 0013；ADR-0212 决定二）。只增：写口都是 INSERT … ON CONFLICT
// DO NOTHING，撞键后读回册上那一行回显，没有 UPDATE 或 DELETE。
type DeliveryPreAdvices struct {
	db *bentopg.DB
}

func NewDeliveryPreAdvices(db *bentopg.DB) (*DeliveryPreAdvices, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &DeliveryPreAdvices{db: db}, nil
}

var _ ports.DeliveryPreAdviceRegister = (*DeliveryPreAdvices)(nil)

const preAdviceColumns = `pre_advice_id, customer_account_id, node_ref, expected_arrival_at, parcel_numbers, delivering_party,
	source_channel, source_request_key, content_digest, formed_at, document_number, document_number_absence`

// LockDeliveryPreAdviceSource 取（租户，客户端，来源请求键）的事务级咨询锁，再按来源读回在册那一份。锁到事务结束：同一来源并发的
// 两笔，后一笔等前一笔提交或回滚之后才读，读到的就是前一笔落下的那一份（或前一笔回滚后的空），不会两笔都以为没人落过而各取一个号。
// 键里用 chr(31) 分隔，免得「a」+「bc」与「ab」+「c」拼成同一个键。
func (register *DeliveryPreAdvices) LockDeliveryPreAdviceSource(
	ctx context.Context,
	tenant domain.TenantID,
	source domain.CustomerRequestSource,
) (domain.DeliveryPreAdvice, bool, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return domain.DeliveryPreAdvice{}, false, fmt.Errorf("lock delivery pre-advice source: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('node_operations.delivery_pre_advice.source'),
		        hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		tenant.String(), source.Channel, source.RequestKey,
	); err != nil {
		return domain.DeliveryPreAdvice{}, false, fmt.Errorf("lock delivery pre-advice source: %w", err)
	}
	recorded, found, err := scanPreAdvice(executor.QueryRow(ctx,
		`SELECT `+preAdviceColumns+`
		   FROM node_operations.delivery_pre_advice
		  WHERE tenant_id = $1 AND source_channel = $2 AND source_request_key = $3`,
		tenant.String(), source.Channel, source.RequestKey,
	), tenant)
	if err != nil {
		return domain.DeliveryPreAdvice{}, false, fmt.Errorf("lock delivery pre-advice source: read back: %w", err)
	}
	return recorded, found, nil
}

// DeliveryPreAdviceDocumentNumberUsed 按（租户，单据号）查，走唯一约束带出的索引。
func (register *DeliveryPreAdvices) DeliveryPreAdviceDocumentNumberUsed(ctx context.Context, tenant domain.TenantID, number string) (bool, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("delivery pre-advice document number used: %w", err)
	}
	var used bool
	if err := querier.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM node_operations.delivery_pre_advice WHERE tenant_id = $1 AND document_number = $2)`,
		tenant.String(), number,
	).Scan(&used); err != nil {
		return false, fmt.Errorf("delivery pre-advice document number used: %w", err)
	}
	return used, nil
}

// RecordDeliveryPreAdvice 落一份预报。无 conflict target 的 DO NOTHING 同时吃主键、来源唯一键与（租户，单据号）唯一键；撞上后按来源
// 请求键读回——撞的若是主键（签发的标识重了，128 位随机下不该发生）或单据号（取号编排之外有写入占了这个号），按来源读不到，如实
// 报错，不拿别人的预报回显，开立事务随之回滚。
func (register *DeliveryPreAdvices) RecordDeliveryPreAdvice(
	ctx context.Context,
	preAdvice domain.DeliveryPreAdvice,
) (domain.DeliveryPreAdvice, ports.DeliveryScopeAppendOutcome, error) {
	none := domain.DeliveryPreAdvice{}
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery pre-advice: %w", err)
	}
	documentNumber, documentNumberAbsence, err := documentNumberingColumns(preAdvice.DocumentNumbering())
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery pre-advice: %w", err)
	}
	numbers := make([]string, 0, len(preAdvice.Numbers()))
	for _, number := range preAdvice.Numbers() {
		numbers = append(numbers, number.String())
	}
	var deliveringParty *string
	if clue, present := preAdvice.DeliveringParty(); present {
		value := clue.String()
		deliveringParty = &value
	}
	source := preAdvice.Source()
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.delivery_pre_advice
			(tenant_id, `+preAdviceColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		preAdvice.TenantID().String(), preAdvice.ID().String(), preAdvice.CustomerAccount().String(), preAdvice.Node().String(),
		preAdvice.ExpectedArrivalAt(), numbers, deliveringParty,
		source.Channel, source.RequestKey, source.ContentDigest, preAdvice.FormedAt(), documentNumber, documentNumberAbsence,
	)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery pre-advice: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return preAdvice, ports.DeliveryScopeAppended, nil
	}
	recorded, found, err := scanPreAdvice(executor.QueryRow(ctx,
		`SELECT `+preAdviceColumns+`
		   FROM node_operations.delivery_pre_advice
		  WHERE tenant_id = $1 AND source_channel = $2 AND source_request_key = $3`,
		preAdvice.TenantID().String(), source.Channel, source.RequestKey,
	), preAdvice.TenantID())
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery pre-advice: read back: %w", err)
	}
	if !found {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery pre-advice: the pre-advice ID %q or its document number is already taken", preAdvice.ID())
	}
	return recorded, ports.DeliveryScopeAlreadyRecorded, nil
}

// RecordDeliveryPreAdviceWithdrawal 落一份撤回。插入只从「这份预报属撤回方的客户账户」那一行选出：别的账户的预报与不存在的预报同样
// 选不出行。撞键后先按来源请求键读回（重放或来源冲突，调用方比），读不到再按预报读回（这份预报已被别的请求撤回）。
func (register *DeliveryPreAdvices) RecordDeliveryPreAdviceWithdrawal(
	ctx context.Context,
	withdrawal domain.DeliveryPreAdviceWithdrawal,
) (domain.DeliveryPreAdviceWithdrawal, ports.DeliveryScopeAppendOutcome, error) {
	none := domain.DeliveryPreAdviceWithdrawal{}
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery pre-advice withdrawal: %w", err)
	}
	tenant := withdrawal.TenantID()
	source := withdrawal.Source()
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.delivery_pre_advice_withdrawal
			(tenant_id, pre_advice_id, customer_account_id, source_channel, source_request_key, content_digest, withdrawn_at)
		 SELECT pre_advice.tenant_id, pre_advice.pre_advice_id, $3, $4, $5, $6, $7
		   FROM node_operations.delivery_pre_advice AS pre_advice
		  WHERE pre_advice.tenant_id = $1 AND pre_advice.pre_advice_id = $2 AND pre_advice.customer_account_id = $3
		 ON CONFLICT DO NOTHING`,
		tenant.String(), withdrawal.PreAdvice().String(), withdrawal.CustomerAccount().String(),
		source.Channel, source.RequestKey, source.ContentDigest, withdrawal.WithdrawnAt(),
	)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery pre-advice withdrawal: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return withdrawal, ports.DeliveryScopeAppended, nil
	}
	const withdrawalColumns = `pre_advice_id, customer_account_id, source_channel, source_request_key, content_digest, withdrawn_at`
	recorded, found, err := scanWithdrawal(executor.QueryRow(ctx,
		`SELECT `+withdrawalColumns+`
		   FROM node_operations.delivery_pre_advice_withdrawal
		  WHERE tenant_id = $1 AND source_channel = $2 AND source_request_key = $3`,
		tenant.String(), source.Channel, source.RequestKey,
	), tenant)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery pre-advice withdrawal: read back: %w", err)
	}
	if found {
		return recorded, ports.DeliveryScopeAlreadyRecorded, nil
	}
	recorded, found, err = scanWithdrawal(executor.QueryRow(ctx,
		`SELECT `+withdrawalColumns+`
		   FROM node_operations.delivery_pre_advice_withdrawal
		  WHERE tenant_id = $1 AND pre_advice_id = $2 AND customer_account_id = $3`,
		tenant.String(), withdrawal.PreAdvice().String(), withdrawal.CustomerAccount().String(),
	), tenant)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery pre-advice withdrawal: read back: %w", err)
	}
	if found {
		return recorded, ports.DeliveryScopeAlreadyRecorded, nil
	}
	return none, ports.DeliveryScopeTargetNotFound, nil
}

// FindDeliveryPreAdvice 读一份预报连同它的撤回。走 ReadExecutor：在事务里调时读得到本事务刚落的撤回。
func (register *DeliveryPreAdvices) FindDeliveryPreAdvice(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.DeliveryPreAdviceID,
) (domain.DeliveryPreAdviceState, bool, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return domain.DeliveryPreAdviceState{}, false, fmt.Errorf("find delivery pre-advice: %w", err)
	}
	preAdvice, found, err := scanPreAdvice(querier.QueryRow(ctx,
		`SELECT `+preAdviceColumns+`
		   FROM node_operations.delivery_pre_advice
		  WHERE tenant_id = $1 AND pre_advice_id = $2`,
		tenant.String(), id.String(),
	), tenant)
	if err != nil {
		return domain.DeliveryPreAdviceState{}, false, fmt.Errorf("find delivery pre-advice: %w", err)
	}
	if !found {
		return domain.DeliveryPreAdviceState{}, false, nil
	}
	withdrawal, _, err := scanWithdrawal(querier.QueryRow(ctx,
		`SELECT pre_advice_id, customer_account_id, source_channel, source_request_key, content_digest, withdrawn_at
		   FROM node_operations.delivery_pre_advice_withdrawal
		  WHERE tenant_id = $1 AND pre_advice_id = $2`,
		tenant.String(), id.String(),
	), tenant)
	if err != nil {
		return domain.DeliveryPreAdviceState{}, false, fmt.Errorf("find delivery pre-advice: withdrawal: %w", err)
	}
	return domain.DeliveryPreAdviceState{PreAdvice: preAdvice, Withdrawal: withdrawal}, true, nil
}

// scanPreAdvice 读一行预报并经形成门还原：读回的也要过一遍不变量，库里那一行若被绕过写口改坏了，在这里报错，不交出一份自相矛盾的
// 预报。号码不再过形状门，理由见 domain.NewPreAdvisedParcelNumber。
func scanPreAdvice(row pgx.Row, tenant domain.TenantID) (domain.DeliveryPreAdvice, bool, error) {
	var (
		id, account, node, channel, requestKey, digest string
		expectedAt, formedAt                           time.Time
		numbers                                        []string
		deliveringParty                                *string
		documentNumber, documentNumberAbsence          *string
	)
	err := row.Scan(&id, &account, &node, &expectedAt, &numbers, &deliveringParty, &channel, &requestKey, &digest, &formedAt,
		&documentNumber, &documentNumberAbsence)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeliveryPreAdvice{}, false, nil
	}
	if err != nil {
		return domain.DeliveryPreAdvice{}, false, err
	}
	spec := domain.DeliveryPreAdviceSpec{
		TenantID:          tenant,
		ExpectedArrivalAt: expectedAt,
		Source:            domain.CustomerRequestSource{Channel: channel, RequestKey: requestKey, ContentDigest: digest},
		FormedAt:          formedAt,
	}
	if spec.ID, err = domain.NewDeliveryPreAdviceID(id); err != nil {
		return domain.DeliveryPreAdvice{}, false, err
	}
	if spec.CustomerAccount, err = domain.NewCustomerAccountReference(account); err != nil {
		return domain.DeliveryPreAdvice{}, false, err
	}
	if spec.Node, err = domain.NewNodeReference(node); err != nil {
		return domain.DeliveryPreAdvice{}, false, err
	}
	for _, raw := range numbers {
		number, err := domain.NewPreAdvisedParcelNumber(raw)
		if err != nil {
			return domain.DeliveryPreAdvice{}, false, err
		}
		spec.Numbers = append(spec.Numbers, number)
	}
	if deliveringParty != nil {
		if spec.DeliveringParty, err = domain.NewDeliveringPartyReference(*deliveringParty); err != nil {
			return domain.DeliveryPreAdvice{}, false, err
		}
	}
	preAdvice, err := domain.FormDeliveryPreAdvice(spec)
	if err != nil {
		return domain.DeliveryPreAdvice{}, false, err
	}
	numbering, err := domain.RehydrateDocumentNumbering(stringOrEmpty(documentNumber), stringOrEmpty(documentNumberAbsence))
	if err != nil {
		return domain.DeliveryPreAdvice{}, false, err
	}
	return preAdvice.WithDocumentNumbering(numbering), true, nil
}

func scanWithdrawal(row pgx.Row, tenant domain.TenantID) (domain.DeliveryPreAdviceWithdrawal, bool, error) {
	var (
		id, account, channel, requestKey, digest string
		withdrawnAt                              time.Time
	)
	err := row.Scan(&id, &account, &channel, &requestKey, &digest, &withdrawnAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeliveryPreAdviceWithdrawal{}, false, nil
	}
	if err != nil {
		return domain.DeliveryPreAdviceWithdrawal{}, false, err
	}
	spec := domain.DeliveryPreAdviceWithdrawalSpec{
		TenantID:    tenant,
		Source:      domain.CustomerRequestSource{Channel: channel, RequestKey: requestKey, ContentDigest: digest},
		WithdrawnAt: withdrawnAt,
	}
	if spec.PreAdvice, err = domain.NewDeliveryPreAdviceID(id); err != nil {
		return domain.DeliveryPreAdviceWithdrawal{}, false, err
	}
	if spec.CustomerAccount, err = domain.NewCustomerAccountReference(account); err != nil {
		return domain.DeliveryPreAdviceWithdrawal{}, false, err
	}
	withdrawal, err := domain.FormDeliveryPreAdviceWithdrawal(spec)
	if err != nil {
		return domain.DeliveryPreAdviceWithdrawal{}, false, err
	}
	return withdrawal, true, nil
}
