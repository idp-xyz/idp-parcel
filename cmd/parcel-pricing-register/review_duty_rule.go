package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 参考复核职责规则的登记（ADR-0262 决定一）：reference-review-duty-rule 把租户对序列与目录人工复核的那一条规则落进规则册，
// 一租户一条、同内容重放、异内容答冲突。规则未登记时人工复核一律答未配置，所以种子要在序列与目录复核之前先登这一条。
// 在线登记面随治理写面那一族另裁。
const kindReferenceReviewDutyRule = "reference-review-duty-rule"

// referenceReviewDutyRuleRegistrar 把规则登记用例收窄成本工具消费的形状，测试用替身顶上。
type referenceReviewDutyRuleRegistrar interface {
	Handle(ctx context.Context, command application.RegisterReferenceReviewDutyRuleCommand) (application.RegisterReferenceReviewDutyRuleOutcome, error)
}

var _ referenceReviewDutyRuleRegistrar = (*application.RegisterReferenceReviewDutyRuleHandler)(nil)

// reviewDutyRuleDocument 是规则文档的线格式。两格都必须写出来：distinctSubjects 是 true 或 false，requiredGrant 是一格授予名
// 或 null（不要求）。缺键不按「不要求」读——那是放行的方向，规则的缺省朝拦，租户不说就不替它说。
type reviewDutyRuleDocument struct {
	Tenant           string          `json:"tenant"`
	DistinctSubjects *bool           `json:"distinctSubjects"`
	RequiredGrant    json.RawMessage `json:"requiredGrant"`
}

// parseReviewDutyRuleDocument 只做形状翻译，封闭解码：未知键与尾随内容答用法错。规则立不住（空租户、空授予名）也在入库前拒。
func parseReviewDutyRuleDocument(raw []byte) (domain.ReferenceReviewDutyRule, error) {
	var document reviewDutyRuleDocument
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return domain.ReferenceReviewDutyRule{}, fmt.Errorf("规则文档不是合法 JSON 或带了未知键：%w", err)
	}
	// 判尾随内容同 decodeReviewDocument：再解一次必须见到文件尾。
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ReferenceReviewDutyRule{}, fmt.Errorf("规则文档有尾随内容")
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return domain.ReferenceReviewDutyRule{}, fmt.Errorf("规则文档 tenant：%w", err)
	}
	if document.DistinctSubjects == nil {
		return domain.ReferenceReviewDutyRule{}, fmt.Errorf("规则文档缺 distinctSubjects：须否不同主体要由租户写明")
	}
	if len(document.RequiredGrant) == 0 {
		return domain.ReferenceReviewDutyRule{}, fmt.Errorf("规则文档缺 requiredGrant：不要求授予写 null")
	}
	var grantName *string
	if err := json.Unmarshal(document.RequiredGrant, &grantName); err != nil {
		return domain.ReferenceReviewDutyRule{}, fmt.Errorf("规则文档 requiredGrant 须为授予名或 null：%w", err)
	}
	var grant domain.OperatorGrant
	if grantName != nil {
		if grant, err = domain.NewOperatorGrant(*grantName); err != nil {
			return domain.ReferenceReviewDutyRule{}, fmt.Errorf("规则文档 requiredGrant：%w", err)
		}
	}
	rule, err := domain.NewReferenceReviewDutyRule(tenant, *document.DistinctSubjects, grant)
	if err != nil {
		return domain.ReferenceReviewDutyRule{}, fmt.Errorf("规则立不住：%w", err)
	}
	return rule, nil
}

// executeReviewDutyRule 把一份规则文档推进到规则册答案：在事务内交登记用例，再把答案译成退出码。
func executeReviewDutyRule(
	ctx context.Context,
	raw []byte,
	registrar referenceReviewDutyRuleRegistrar,
	transactor bentoapp.Transactor,
) (string, int) {
	rule, err := parseReviewDutyRuleDocument(raw)
	if err != nil {
		return fmt.Sprintf("%s: %v", kindReferenceReviewDutyRule, err), exitUsage
	}
	var outcome application.RegisterReferenceReviewDutyRuleOutcome
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		result, err := registrar.Handle(txCtx, application.RegisterReferenceReviewDutyRuleCommand{Rule: rule})
		outcome = result
		return err
	})
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", kindReferenceReviewDutyRule, err), exitUndecided
	}
	return kindReferenceReviewDutyRule + ": " + outcome.String(), reviewDutyRuleExitCode(outcome)
}

// reviewDutyRuleExitCode：冲突是规则册的治理答案——原行未被顶替，要改规则的人去走治理，不是本工具用错了。
func reviewDutyRuleExitCode(outcome application.RegisterReferenceReviewDutyRuleOutcome) int {
	switch outcome {
	case application.ReferenceReviewDutyRuleRecorded, application.ReferenceReviewDutyRuleAlreadyOnRegister:
		return exitRegistered
	case application.ReferenceReviewDutyRuleConflict:
		return exitGovernance
	case application.ReferenceReviewDutyRuleNotAccepted:
		return exitUsage
	default:
		return exitUndecided
	}
}
