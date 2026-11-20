package main

// 本文件承载签署证据要求的受控登记入口：register-signing-evidence-requirement 子命令。
// 两个布尔都必须出现，缺一格不是「不要」。一租户一条、撞键不覆盖：同内容是重放，异内容是冲突。

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

type signingEvidenceRequirementDocument struct {
	TenantID                  json.RawMessage `json:"tenantId"`
	RequiresCustomerContract  *bool           `json:"requiresCustomerContract"`
	RequiresSupplierAgreement *bool           `json:"requiresSupplierAgreement"`
}

func signingEvidenceRequirementFromJSON(raw []byte) (domain.SigningEvidenceRequirement, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document signingEvidenceRequirementDocument
	if err := decoder.Decode(&document); err != nil {
		return domain.SigningEvidenceRequirement{}, fmt.Errorf("签署证据要求不是本入口的形状：%w", err)
	}
	tenant, err := registrationjson.DocumentTenant(document.TenantID)
	if err != nil {
		return domain.SigningEvidenceRequirement{}, err
	}
	if document.RequiresCustomerContract == nil || document.RequiresSupplierAgreement == nil {
		return domain.SigningEvidenceRequirement{}, fmt.Errorf("签署证据要求缺 requiresCustomerContract 或 requiresSupplierAgreement")
	}
	rule, err := domain.NewSigningEvidenceRequirement(tenant, *document.RequiresCustomerContract, *document.RequiresSupplierAgreement)
	if err != nil {
		return domain.SigningEvidenceRequirement{}, err
	}
	return rule, nil
}

func runRegisterSigningEvidenceRequirement(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-signing-evidence-requirement", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "签署证据要求 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-signing-evidence-requirement 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读签署证据要求：%v\n", err)
		return exitTechnical
	}
	rule, err := signingEvidenceRequirementFromJSON(raw)
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

	rules, err := pcpostgres.NewSigningEvidences(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造签署证据册：%v\n", err)
		return exitTechnical
	}
	var outcome pcports.SigningEvidenceRequirementSaveOutcome
	err = db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = rules.SaveSigningEvidenceRequirement(txCtx, rule)
		return saveErr
	})
	if err != nil {
		fmt.Fprintf(errOut, "登记签署证据要求：技术失败：%v\n", err)
		return exitTechnical
	}
	fmt.Fprintf(out, "租户 %v 的签署证据要求：%s（客户合同 %t，供应商协议 %t）\n",
		rule.Tenant(), outcome, rule.RequiresCustomerContract(), rule.RequiresSupplierAgreement())
	if outcome == pcports.SigningEvidenceRequirementContentConflict {
		return exitAttention
	}
	return exitLanded
}
