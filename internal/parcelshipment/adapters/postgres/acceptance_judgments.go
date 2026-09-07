package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// AcceptanceJudgments 同时实现 ports.AcceptanceJudgmentRecorder 与
// ports.RecordedJudgmentReader：接受判断任务上采用了什么，与提交决定前读回什么，是同一批
// 行的两面。拆成两个类型只会让「写进去的形状」与「读出来的形状」各有一处定义。
//
// 一切查询按（租户 + 委托标识）圈定。委托标识的唯一性本就按租户圈定，只凭标识定不到一份
// 委托——跨租户同号的两份就会读到彼此的判断，而 ADR-0003 把租户定为最高数据隔离边界。
//
// 判断与处理尝试只追加：同键重复到达用 ON CONFLICT DO NOTHING 译成重放（撞键的 INSERT 不
// 打中止事务，编排还要在同一事务里继续）。四个写口都只交回 error，因此「已经记过了」没有
// 可交回的业务取值——把它译成错误会让编排报`判断未记录`，而库里确实记着一份。
type AcceptanceJudgments struct {
	db *bentopg.DB
}

func NewAcceptanceJudgments(db *bentopg.DB) (*AcceptanceJudgments, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &AcceptanceJudgments{db: db}, nil
}

// RecordReachabilityJudgment 把一个已采用的可达性判断追加到任务上。
//
// 同一成员的重判各占一行，键上带提交版本与时点：重判必须以新时点发起（`AT-PS-037`），同版本
// 同成员同时点再来一次是权威重放既有判断，保留先到者。版本在键里是为了让新版本的判断在时点
// 策略把两版钉在同一时点时仍成为自己那一版的行，而不是被旧版那份吞成重放（迁移 0018）。
func (repository *AcceptanceJudgments) RecordReachabilityJudgment(
	ctx context.Context,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
	version domain.SubmissionVersionID,
	judgment domain.ReachabilityJudgment,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record reachability judgment: %w", err)
	}

	asOf := judgment.AsOf()
	_, err = executor.Exec(ctx,
		`INSERT INTO parcel_shipment.acceptance_reachability_judgment
			(tenant_id, shipment_request_id, submission_version, parcel_id, as_of_at,
			 judgment_id, judgment_value, basis_ref,
			 as_of_kind, as_of_semantics, as_of_policy)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		requestID.String(),
		version.String(),
		judgment.DeclaredParcelID().String(),
		asOf.At().UTC(),
		nullableText(judgment.JudgmentID().String()),
		judgment.Value().String(),
		nullableText(judgment.Basis().String()),
		asOf.Kind().String(),
		asOf.Semantics().String(),
		asOf.PolicyVersion().String(),
	)
	if err != nil {
		return fmt.Errorf("record reachability judgment: %w", err)
	}
	return nil
}

// RecordFinancialControlResult 把一次已采用的接受前财务控制结果追加到任务上。
//
// 不按成员分行：控制作用在整份委托上。键上同样带提交版本与时点，理由与可达性一致。
func (repository *AcceptanceJudgments) RecordFinancialControlResult(
	ctx context.Context,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
	version domain.SubmissionVersionID,
	result domain.FinancialControlResult,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record financial control result: %w", err)
	}

	asOf := result.AsOf()
	_, err = executor.Exec(ctx,
		`INSERT INTO parcel_shipment.acceptance_financial_control
			(tenant_id, shipment_request_id, submission_version, as_of_at,
			 result_id, control_outcome, basis_ref,
			 as_of_kind, as_of_semantics, as_of_policy)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		requestID.String(),
		version.String(),
		asOf.At().UTC(),
		nullableText(result.ResultID().String()),
		result.Outcome().String(),
		nullableText(result.Basis().String()),
		asOf.Kind().String(),
		asOf.Semantics().String(),
		asOf.PolicyVersion().String(),
	)
	if err != nil {
		return fmt.Errorf("record financial control result: %w", err)
	}
	return nil
}

// RecordProcessingAttempt 追加一条没能推进的处理记录。
//
// 同一续办引用同一时刻是同一次尝试的重放；真卡了两轮会有两个时刻，各占一行——只留最近一条
// 就说不出这份委托卡过几轮、各卡在哪里。
func (repository *AcceptanceJudgments) RecordProcessingAttempt(
	ctx context.Context,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
	attempt domain.ProcessingAttempt,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record processing attempt: %w", err)
	}

	_, err = executor.Exec(ctx,
		`INSERT INTO parcel_shipment.acceptance_processing_attempt
			(tenant_id, shipment_request_id, continuation_ref, attempted_at,
			 reason_ref, resume_path)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		requestID.String(),
		attempt.ContinuationReference().String(),
		attempt.AttemptedAt().UTC(),
		attempt.Reason().String(),
		attempt.ResumePath().String(),
	)
	if err != nil {
		return fmt.Errorf("record processing attempt: %w", err)
	}
	return nil
}

// RecordAdoptedCommercialResolution 记下本轮采用的那次商业解析，后写覆盖。
//
// 覆盖是端口要求的：决定期的提交前重解会采用新的一次解析并再记一次，不覆盖的话下一轮读回
// 的就是已被取代的那次。重复记录同一标识因而是幂等的。
func (repository *AcceptanceJudgments) RecordAdoptedCommercialResolution(
	ctx context.Context,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
	resolution domain.CommercialResolutionID,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record adopted commercial resolution: %w", err)
	}

	_, err = executor.Exec(ctx,
		`INSERT INTO parcel_shipment.acceptance_adopted_resolution
			(tenant_id, shipment_request_id, resolution_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (tenant_id, shipment_request_id) DO UPDATE
		    SET resolution_id = EXCLUDED.resolution_id,
		        adopted_at    = now()`,
		tenant.String(),
		requestID.String(),
		resolution.String(),
	)
	if err != nil {
		return fmt.Errorf("record adopted commercial resolution: %w", err)
	}
	return nil
}

// LoadRecordedJudgments 取回这份委托在这一提交版本上当前采用的全部权威判断。
//
// 三样各自缺席时一律留零值，不凑：财务控制的零值按端口约定就是「尚未形成」，翻译函数据零值
// 形成`无法判定`；采用解析的零值是「还没有任何一轮采用过依据」，编排据它走首次解析而不是
// 重解。在这里造一个空壳结果，等于把一次没执行的控制写成通过。
//
// 两类判断只读本版的行：旧版本的判断是对旧内容作出的，留在库里作历史，不进当前版本的决定
// （端口注释与迁移 0018 头注）。采用解析每份委托一行，不分版本。
func (repository *AcceptanceJudgments) LoadRecordedJudgments(
	ctx context.Context,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
	version domain.SubmissionVersionID,
) (ports.RecordedJudgments, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.RecordedJudgments{}, fmt.Errorf("load recorded judgments: %w", err)
	}

	reachability, err := loadReachabilityJudgments(ctx, querier, tenant, requestID, version)
	if err != nil {
		return ports.RecordedJudgments{}, err
	}
	control, err := loadFinancialControl(ctx, querier, tenant, requestID, version)
	if err != nil {
		return ports.RecordedJudgments{}, err
	}
	adopted, err := loadAdoptedResolution(ctx, querier, tenant, requestID)
	if err != nil {
		return ports.RecordedJudgments{}, err
	}
	return ports.RecordedJudgments{
		Reachability:                reachability,
		FinancialControl:            control,
		AdoptedCommercialResolution: adopted,
	}, nil
}

// loadReachabilityJudgments 在本版内逐成员只交回当前采用的那一份：按时点取最新。
//
// 一个成员交回两份是不行的——形成决定那一步逐条译成校验结果，一份被推翻的`不可达`会连同
// 重判后的`可达`一起进 Decide，而任何一项确定性失败都拒绝整份版本。`AT-PS-037` 要的正是
// 「原结果不再用于接受」，历史留在行里但不参与本轮。
//
// DISTINCT ON 收下 ORDER BY 之后的第一行，所以时点必须降序——升序会把被推翻的那份交回去。
func loadReachabilityJudgments(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
	version domain.SubmissionVersionID,
) ([]domain.ReachabilityJudgment, error) {
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT ON (parcel_id)
		        parcel_id, judgment_id, judgment_value, basis_ref,
		        as_of_at, as_of_kind, as_of_semantics, as_of_policy
		   FROM parcel_shipment.acceptance_reachability_judgment
		  WHERE tenant_id = $1
		    AND shipment_request_id = $2
		    AND submission_version = $3
		  ORDER BY parcel_id, as_of_at DESC`,
		tenant.String(),
		requestID.String(),
		version.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load reachability judgments: %w", err)
	}
	defer rows.Close()

	var judgments []domain.ReachabilityJudgment
	for rows.Next() {
		var (
			parcelID, valueRaw            string
			judgmentID, basisRef          *string
			asOfAt                        time.Time
			kindRaw, semantics, policyRaw string
		)
		if err := rows.Scan(&parcelID, &judgmentID, &valueRaw, &basisRef,
			&asOfAt, &kindRaw, &semantics, &policyRaw); err != nil {
			return nil, fmt.Errorf("load reachability judgments: %w", err)
		}
		judgment, err := rebuildReachabilityJudgment(reachabilityRow{
			parcelID:   parcelID,
			judgmentID: judgmentID,
			value:      valueRaw,
			basis:      basisRef,
			asOf:       asOfRow{at: asOfAt, kind: kindRaw, semantics: semantics, policy: policyRaw},
		})
		if err != nil {
			return nil, fmt.Errorf("load reachability judgments: %w", err)
		}
		judgments = append(judgments, judgment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load reachability judgments: %w", err)
	}
	return judgments, nil
}

// loadFinancialControl 交回本版当前采用的那一次控制结果，同样按时点取最新。没有行时交回零值：
// 「从未形成控制」与「形成了一次通不过的控制」在读的人眼里必须不同。
func loadFinancialControl(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
	version domain.SubmissionVersionID,
) (domain.FinancialControlResult, error) {
	var (
		outcomeRaw                    string
		resultID, basisRef            *string
		asOfAt                        time.Time
		kindRaw, semantics, policyRaw string
	)
	err := querier.QueryRow(ctx,
		`SELECT result_id, control_outcome, basis_ref,
		        as_of_at, as_of_kind, as_of_semantics, as_of_policy
		   FROM parcel_shipment.acceptance_financial_control
		  WHERE tenant_id = $1
		    AND shipment_request_id = $2
		    AND submission_version = $3
		  ORDER BY as_of_at DESC
		  LIMIT 1`,
		tenant.String(),
		requestID.String(),
		version.String(),
	).Scan(&resultID, &outcomeRaw, &basisRef, &asOfAt, &kindRaw, &semantics, &policyRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FinancialControlResult{}, nil
	}
	if err != nil {
		return domain.FinancialControlResult{}, fmt.Errorf("load financial control result: %w", err)
	}

	result, err := rebuildFinancialControl(financialControlRow{
		resultID: resultID,
		outcome:  outcomeRaw,
		basis:    basisRef,
		asOf:     asOfRow{at: asOfAt, kind: kindRaw, semantics: semantics, policy: policyRaw},
	})
	if err != nil {
		return domain.FinancialControlResult{}, fmt.Errorf("load financial control result: %w", err)
	}
	return result, nil
}

func loadAdoptedResolution(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
) (domain.CommercialResolutionID, error) {
	var raw string
	err := querier.QueryRow(ctx,
		`SELECT resolution_id
		   FROM parcel_shipment.acceptance_adopted_resolution
		  WHERE tenant_id = $1
		    AND shipment_request_id = $2`,
		tenant.String(),
		requestID.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CommercialResolutionID{}, nil
	}
	if err != nil {
		return domain.CommercialResolutionID{}, fmt.Errorf("load adopted commercial resolution: %w", err)
	}

	resolution, err := domain.NewCommercialResolutionID(raw)
	if err != nil {
		return domain.CommercialResolutionID{}, fmt.Errorf("load adopted commercial resolution: %w", err)
	}
	return resolution, nil
}

// asOfRow 是时点在库里的四列。判断种类随行存而不由表名推：NewReachabilityJudgment 不校验
// asOf 的种类，按表名重建会把一份领域允许的判断悄悄读成另一类时点。
type asOfRow struct {
	at        time.Time
	kind      string
	semantics string
	policy    string
}

type reachabilityRow struct {
	parcelID   string
	judgmentID *string
	value      string
	basis      *string
	asOf       asOfRow
}

type financialControlRow struct {
	resultID *string
	outcome  string
	basis    *string
	asOf     asOfRow
}

// rebuildReachabilityJudgment 逐字段过领域构造门，`不适用`带依据、其余三值带标识由
// NewReachabilityJudgment 再验一遍——一行坏数据在构造门上暴露，不会变成一份看起来合法的
// 判断（ADR-0028 同款）。
func rebuildReachabilityJudgment(row reachabilityRow) (domain.ReachabilityJudgment, error) {
	parcelID, err := domain.NewDeclaredParcelID(row.parcelID)
	if err != nil {
		return domain.ReachabilityJudgment{}, err
	}
	value, err := reachabilityValueFrom(row.value)
	if err != nil {
		return domain.ReachabilityJudgment{}, err
	}
	asOf, err := row.asOf.judgmentAsOf()
	if err != nil {
		return domain.ReachabilityJudgment{}, err
	}

	spec := domain.ReachabilityJudgmentSpec{ParcelID: parcelID, Value: value, AsOf: asOf}
	if row.judgmentID != nil {
		if spec.JudgmentID, err = domain.NewReachabilityJudgmentID(*row.judgmentID); err != nil {
			return domain.ReachabilityJudgment{}, err
		}
	}
	if row.basis != nil {
		if spec.Basis, err = domain.NewReachabilityBasisReference(*row.basis); err != nil {
			return domain.ReachabilityJudgment{}, err
		}
	}
	return domain.NewReachabilityJudgment(spec)
}

func rebuildFinancialControl(row financialControlRow) (domain.FinancialControlResult, error) {
	outcome, err := financialControlOutcomeFrom(row.outcome)
	if err != nil {
		return domain.FinancialControlResult{}, err
	}
	asOf, err := row.asOf.judgmentAsOf()
	if err != nil {
		return domain.FinancialControlResult{}, err
	}

	var resultID domain.FinancialControlResultID
	if row.resultID != nil {
		if resultID, err = domain.NewFinancialControlResultID(*row.resultID); err != nil {
			return domain.FinancialControlResult{}, err
		}
	}
	var basis domain.ControlBasisReference
	if row.basis != nil {
		if basis, err = domain.NewControlBasisReference(*row.basis); err != nil {
			return domain.FinancialControlResult{}, err
		}
	}
	return domain.NewFinancialControlResult(resultID, outcome, basis, asOf)
}

// judgmentAsOf 从行重建时点。走 NewEchoedAsOfPolicy 是唯一的路：NewJudgmentAsOf 收不下
// 第一阶段的声明，而库里存的本就是权威回显过的那一份。
func (row asOfRow) judgmentAsOf() (domain.JudgmentAsOf, error) {
	kind, err := judgmentKindFrom(row.kind)
	if err != nil {
		return domain.JudgmentAsOf{}, err
	}
	semantics, err := domain.NewAsOfSemanticsReference(row.semantics)
	if err != nil {
		return domain.JudgmentAsOf{}, err
	}
	policyVersion, err := domain.NewAsOfPolicyVersion(row.policy)
	if err != nil {
		return domain.JudgmentAsOf{}, err
	}
	echoed, err := domain.NewEchoedAsOfPolicy(kind, semantics, policyVersion)
	if err != nil {
		return domain.JudgmentAsOf{}, err
	}
	return domain.NewJudgmentAsOf(row.at.UTC(), echoed)
}

func reachabilityValueFrom(raw string) (domain.ReachabilityValue, error) {
	switch raw {
	case domain.ReachabilityReachable.String():
		return domain.ReachabilityReachable, nil
	case domain.ReachabilityUnreachable.String():
		return domain.ReachabilityUnreachable, nil
	case domain.ReachabilityInsufficientEvidence.String():
		return domain.ReachabilityInsufficientEvidence, nil
	case domain.ReachabilityNotApplicable.String():
		return domain.ReachabilityNotApplicable, nil
	default:
		return domain.ReachabilityValueInvalid, fmt.Errorf("unknown reachability value %q", raw)
	}
}

func financialControlOutcomeFrom(raw string) (domain.FinancialControlOutcome, error) {
	switch raw {
	case domain.FinancialControlHeld.String():
		return domain.FinancialControlHeld, nil
	case domain.FinancialControlRestricted.String():
		return domain.FinancialControlRestricted, nil
	case domain.FinancialControlNotApplicable.String():
		return domain.FinancialControlNotApplicable, nil
	case domain.FinancialControlCreditExposed.String():
		return domain.FinancialControlCreditExposed, nil
	default:
		return domain.FinancialControlOutcomeInvalid, fmt.Errorf("unknown financial control outcome %q", raw)
	}
}

func judgmentKindFrom(raw string) (domain.JudgmentKind, error) {
	switch raw {
	case domain.ReachabilityJudgmentKind.String():
		return domain.ReachabilityJudgmentKind, nil
	case domain.FinancialControlJudgmentKind.String():
		return domain.FinancialControlJudgmentKind, nil
	default:
		return domain.JudgmentKindInvalid, fmt.Errorf("unknown judgment kind %q", raw)
	}
}

// nullableText 把领域侧的缺席（零值引用交回空串）写成 NULL。写成空串会让「这一支下权威不
// 签发标识」与「签发了一个空标识」在库里长得一模一样，而 CHECK 也就挡不住后者。
func nullableText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

var (
	_ ports.AcceptanceJudgmentRecorder = (*AcceptanceJudgments)(nil)
	_ ports.RecordedJudgmentReader     = (*AcceptanceJudgments)(nil)
)
