package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件是资料修订允许声明（第三族阶段内容声明，0027，ADR-0120）在持久化面的两半：读口挂在
// StageContentDeclarations（它就是这一族的家），写口挂在 CommercialPublications（与其余具名 Save 同居）。
// 单独成文件而不并进两处既有文件，是让这一族的读写在一处看全，也让并行的会话少碰共享文件。

var _ ports.SourceDataAmendmentAllowanceView = (*StageContentDeclarations)(nil)

// amendmentCellRow 是子表一行的字面：三维 + 允许性。读回与写前比对共用一份形。
type amendmentCellRow struct {
	group, stage, intent, allowance string
}

// LoadSourceDataAmendmentAllowance 取回资料修订允许声明。无父行 = 未配置；父行在场即走
// NewSourceDataAmendmentAllowanceContent，未封闭却零格、某格集外都是坏声明，走 error，不得折成未配置。
// 父 LEFT JOIN 子一次取回：封闭且零格是合法正文，父行必须在没有子行时也读得到。
func (repository *StageContentDeclarations) LoadSourceDataAmendmentAllowance(
	ctx context.Context,
	tenant domain.TenantID,
	rulePackage domain.CommercialVersion,
) (domain.SourceDataAmendmentAllowanceContent, bool, error) {
	none := domain.SourceDataAmendmentAllowanceContent{}
	if err := requireOwnedVersion(tenant, rulePackage, "load source data amendment allowance"); err != nil {
		return none, false, err
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load source data amendment allowance: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT parent.closed,
		        child.data_group_ref,
		        child.stage,
		        child.intent,
		        child.allowance
		   FROM party_commercial.source_data_amendment_content AS parent
		   LEFT JOIN party_commercial.source_data_amendment_allowance AS child
		          ON child.tenant_id     = parent.tenant_id
		         AND child.object_kind   = parent.object_kind
		         AND child.object_id     = parent.object_id
		         AND child.version_label = parent.version_label
		  WHERE parent.tenant_id     = $1
		    AND parent.object_kind   = $2
		    AND parent.object_id     = $3
		    AND parent.version_label = $4
		  ORDER BY child.data_group_ref, child.stage, child.intent`,
		tenant.String(),
		uint8(domain.AcceptanceRulePackageObject),
		rulePackage.ObjectID().String(),
		rulePackage.Version().String(),
	)
	if err != nil {
		return none, false, fmt.Errorf("load source data amendment allowance: %w", err)
	}
	defer rows.Close()

	present := false
	closed := false
	var cells []amendmentCellRow
	for rows.Next() {
		var group, stage, intent, allowance *string
		if err := rows.Scan(&closed, &group, &stage, &intent, &allowance); err != nil {
			return none, false, fmt.Errorf("load source data amendment allowance: %w", err)
		}
		present = true
		if group == nil {
			// LEFT JOIN 没配上子行：父行在、零格。是不是合法由构造门按 closed 判。
			continue
		}
		if stage == nil || intent == nil || allowance == nil {
			return none, false, fmt.Errorf("load source data amendment allowance: cell row with missing columns")
		}
		cells = append(cells, amendmentCellRow{group: *group, stage: *stage, intent: *intent, allowance: *allowance})
	}
	if err := rows.Err(); err != nil {
		return none, false, fmt.Errorf("load source data amendment allowance: %w", err)
	}
	if !present {
		return none, false, nil
	}

	rules := make([]domain.SourceDataAmendmentRule, 0, len(cells))
	for _, cell := range cells {
		rule, err := amendmentRuleFromRow(cell)
		if err != nil {
			return none, false, fmt.Errorf("load source data amendment allowance: %w", err)
		}
		rules = append(rules, rule)
	}
	content, err := domain.NewSourceDataAmendmentAllowanceContent(rulePackage, closed, rules)
	if err != nil {
		return none, false, fmt.Errorf("load source data amendment allowance: %w", err)
	}
	return content, true, nil
}

// SaveSourceDataAmendmentAllowance 登记一份规则包的资料修订允许声明（ADR-0120 Decision 六）。先读回再判：
// 同拥有版本、同「封闭」标记、同一份格集合是重放；封闭标记不同、格多一条少一条、同格不同值都是内容
// 冲突，冲突一行不写。封闭且零格是合法正文，父行单独成立。
func (repository *CommercialPublications) SaveSourceDataAmendmentAllowance(
	ctx context.Context,
	content domain.SourceDataAmendmentAllowanceContent,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save source data amendment allowance: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(content.Owner())

	var existingClosed bool
	present, err := scanSingleBool(ctx, executor,
		`SELECT closed
		   FROM party_commercial.source_data_amendment_content
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		&existingClosed, tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save source data amendment allowance: %w", err)
	}
	incoming := content.Rules()
	if present {
		if existingClosed != content.Closed() {
			return ports.DeclarationContentConflict, nil
		}
		existing, err := scanAmendmentCells(ctx, executor,
			`SELECT data_group_ref, stage, intent, allowance
			   FROM party_commercial.source_data_amendment_allowance
			  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
			tenant, kind, objectID, label)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save source data amendment allowance: %w", err)
		}
		if len(existing) != len(incoming) {
			return ports.DeclarationContentConflict, nil
		}
		for _, rule := range incoming {
			key := amendmentCellRow{group: rule.DataGroup.String(), stage: rule.Stage.String(), intent: rule.Intent.String()}
			if existing[key] != rule.Allowance.String() {
				return ports.DeclarationContentConflict, nil
			}
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.source_data_amendment_content
			(tenant_id, object_kind, object_id, version_label, closed)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant, kind, objectID, label, content.Closed(),
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save source data amendment allowance: %w", err)
	}
	for _, rule := range incoming {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.source_data_amendment_allowance
				(tenant_id, object_kind, object_id, version_label, data_group_ref, stage, intent, allowance)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			tenant, kind, objectID, label,
			rule.DataGroup.String(), rule.Stage.String(), rule.Intent.String(), rule.Allowance.String(),
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save source data amendment allowance: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

// scanAmendmentCells 把子表读成「三维 → 允许性」的映射，供写前比对。键里 allowance 留空。
func scanAmendmentCells(
	ctx context.Context,
	executor bentopg.Executor,
	sql string,
	args ...any,
) (map[amendmentCellRow]string, error) {
	rows, err := executor.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cells := make(map[amendmentCellRow]string)
	for rows.Next() {
		var group, stage, intent, allowance string
		if err := rows.Scan(&group, &stage, &intent, &allowance); err != nil {
			return nil, err
		}
		cells[amendmentCellRow{group: group, stage: stage, intent: intent}] = allowance
	}
	return cells, rows.Err()
}

func scanSingleBool(
	ctx context.Context,
	executor bentopg.Executor,
	sql string,
	target *bool,
	args ...any,
) (bool, error) {
	rows, err := executor.Query(ctx, sql, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	present := false
	for rows.Next() {
		if err := rows.Scan(target); err != nil {
			return false, err
		}
		present = true
	}
	return present, rows.Err()
}

// amendmentRuleFromRow 把一行字面译回领域格。集外取值报错不吸收：库上 CHECK 与领域封闭集应当一致，读到
// 集外就是两处分叉，不能静默丢格——丢一格会把「不允许」读成「未声明」或反过来。
func amendmentRuleFromRow(row amendmentCellRow) (domain.SourceDataAmendmentRule, error) {
	group, err := domain.NewSourceDataGroupReference(row.group)
	if err != nil {
		return domain.SourceDataAmendmentRule{}, err
	}
	stage, err := declaredAmendmentStageFrom(row.stage)
	if err != nil {
		return domain.SourceDataAmendmentRule{}, err
	}
	intent, err := declaredAmendmentIntentFrom(row.intent)
	if err != nil {
		return domain.SourceDataAmendmentRule{}, err
	}
	allowance, err := amendmentAllowanceFrom(row.allowance)
	if err != nil {
		return domain.SourceDataAmendmentRule{}, err
	}
	return domain.SourceDataAmendmentRule{DataGroup: group, Stage: stage, Intent: intent, Allowance: allowance}, nil
}

func declaredAmendmentStageFrom(raw string) (domain.DeclaredAmendmentStage, error) {
	for _, stage := range []domain.DeclaredAmendmentStage{
		domain.DeclaredAcceptedNotYetReceived,
		domain.DeclaredReceivedOrMeasured,
		domain.DeclaredLabelledOrBagged,
		domain.DeclaredCustomsDataFormingNotSubmitted,
		domain.DeclaredCustomsSubmitted,
		domain.DeclaredCaseClosedOrServiceCompleted,
	} {
		if stage.String() == raw {
			return stage, nil
		}
	}
	return domain.DeclaredAmendmentStageInvalid, fmt.Errorf("unknown amendment stage %q", raw)
}

func declaredAmendmentIntentFrom(raw string) (domain.DeclaredAmendmentIntent, error) {
	for _, intent := range []domain.DeclaredAmendmentIntent{
		domain.DeclaredSupplementIntent,
		domain.DeclaredCorrectionIntent,
		domain.DeclaredExplicitClearIntent,
	} {
		if intent.String() == raw {
			return intent, nil
		}
	}
	return domain.DeclaredAmendmentIntentInvalid, fmt.Errorf("unknown amendment intent %q", raw)
}

// amendmentAllowanceFrom 只认两值：子表永不登「未声明」，读到它同样是分叉。
func amendmentAllowanceFrom(raw string) (domain.AmendmentAllowance, error) {
	switch raw {
	case domain.AmendmentAllowed.String():
		return domain.AmendmentAllowed, nil
	case domain.AmendmentDisallowed.String():
		return domain.AmendmentDisallowed, nil
	default:
		return domain.AmendmentAllowanceNotDeclared, fmt.Errorf("unknown amendment allowance %q", raw)
	}
}
