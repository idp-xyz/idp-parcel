package main

// 本文件承载商业发布审批职责规则的受控登记入口（ADR-0362；参数登记册 PAR-COM-18）：register-approval-duty-rule 子命令。
// 规则是租户治理参数的实例半边：录入者与批准者须否为不同主体、批准者须持哪一格授予，一格不代填——文件里没写的就是没有。
// 一租户一条、撞键不覆盖（ADR-0031 同款判据）：同内容是重放，异内容是冲突，退出码 2 让治理责任方看报告；改规则今天没有入口。

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// approvalDutyRuleDocument 是登记文件的形状（ADR-0101：载荷形状归产品定）。requiredApproverLevel 缺席即不要求等级，
// 不是「缺省等级」——规则本身不含任何默认。
type approvalDutyRuleDocument struct {
	TenantID                 json.RawMessage `json:"tenantId"`
	RequiresDistinctSubjects *bool           `json:"requiresDistinctSubjects"`
	RequiredApproverLevel    string          `json:"requiredApproverLevel,omitempty"`
}

func approvalDutyRuleFromJSON(raw []byte) (domain.ApprovalDutyRule, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document approvalDutyRuleDocument
	if err := decoder.Decode(&document); err != nil {
		return domain.ApprovalDutyRule{}, fmt.Errorf("审批职责规则不是本入口的形状：%w", err)
	}
	tenant, err := registrationjson.DocumentTenant(document.TenantID)
	if err != nil {
		return domain.ApprovalDutyRule{}, err
	}
	// 须否不同主体是规则的正文，不给就是没写规则：不把缺席读成「不要求」。
	if document.RequiresDistinctSubjects == nil {
		return domain.ApprovalDutyRule{}, fmt.Errorf("审批职责规则缺 requiresDistinctSubjects")
	}
	var level domain.AuthorityLevel
	if document.RequiredApproverLevel != "" {
		if level, err = domain.NewAuthorityLevel(document.RequiredApproverLevel); err != nil {
			return domain.ApprovalDutyRule{}, fmt.Errorf("requiredApproverLevel：%w", err)
		}
	}
	rule, err := domain.NewApprovalDutyRule(tenant, *document.RequiresDistinctSubjects, level)
	if err != nil {
		return domain.ApprovalDutyRule{}, err
	}
	return rule, nil
}

func runRegisterApprovalDutyRule(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-approval-duty-rule", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "审批职责规则 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-approval-duty-rule 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读审批职责规则：%v\n", err)
		return exitTechnical
	}
	rule, err := approvalDutyRuleFromJSON(raw)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}

	db, cleanup, err := openDatabase(ctx, getenv)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}
	defer cleanup()

	rules, err := pcpostgres.NewApprovalDutyRules(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造审批职责规则册：%v\n", err)
		return exitTechnical
	}
	var outcome pcports.ApprovalDutyRuleSaveOutcome
	err = db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = rules.SaveApprovalDutyRule(txCtx, rule)
		return saveErr
	})
	if err != nil {
		fmt.Fprintf(errOut, "登记审批职责规则：技术失败：%v\n", err)
		return exitTechnical
	}
	fmt.Fprintf(out, "租户 %v 的审批职责规则：%s%s\n", rule.Tenant(), outcome, approvalDutyRuleDetail(rule))
	if outcome == pcports.ApprovalDutyRuleContentConflict {
		return exitAttention
	}
	return exitLanded
}

func approvalDutyRuleDetail(rule domain.ApprovalDutyRule) string {
	subjects := "录入者与批准者可为同一主体"
	if rule.RequiresDistinctSubjects() {
		subjects = "录入者与批准者须为不同主体"
	}
	if level, required := rule.RequiredApproverLevel(); required {
		return fmt.Sprintf("（%s；批准者须持等级 %v）", subjects, level)
	}
	return fmt.Sprintf("（%s；不要求等级）", subjects)
}
