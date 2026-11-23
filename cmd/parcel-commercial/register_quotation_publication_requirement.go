package main

// 本文件承载报价发布前提的受控登记入口：register-quotation-publication-requirement 子命令。
// requires 必须出现，缺了不是「不要求」。一租户一条、撞键不覆盖：同内容是重放，异内容是冲突。

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
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

type quotationPublicationRequirementDocument struct {
	TenantID json.RawMessage `json:"tenantId"`
	Requires *bool           `json:"requires"`
}

func quotationPublicationRequirementFromJSON(raw []byte) (domain.QuotationPublicationRequirement, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document quotationPublicationRequirementDocument
	if err := decoder.Decode(&document); err != nil {
		return domain.QuotationPublicationRequirement{}, fmt.Errorf("报价发布前提不是本入口的形状：%w", err)
	}
	tenant, err := registrationjson.DocumentTenant(document.TenantID)
	if err != nil {
		return domain.QuotationPublicationRequirement{}, err
	}
	if document.Requires == nil {
		return domain.QuotationPublicationRequirement{}, fmt.Errorf("报价发布前提缺 requires")
	}
	rule, err := domain.NewQuotationPublicationRequirement(tenant, *document.Requires)
	if err != nil {
		return domain.QuotationPublicationRequirement{}, err
	}
	return rule, nil
}

func runRegisterQuotationPublicationRequirement(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-quotation-publication-requirement", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "报价发布前提 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-quotation-publication-requirement 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读报价发布前提：%v\n", err)
		return exitTechnical
	}
	rule, err := quotationPublicationRequirementFromJSON(raw)
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

	view, err := pcpostgres.NewQuotationPublication(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造报价发布前提册：%v\n", err)
		return exitTechnical
	}
	var outcome pcapplication.QuotationPublicationRequirementOutcome
	err = db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = pcapplication.RegisterQuotationPublicationRequirement(txCtx, view, rule)
		return saveErr
	})
	if err != nil {
		fmt.Fprintf(errOut, "登记报价发布前提：技术失败：%v\n", err)
		return exitTechnical
	}
	fmt.Fprintf(out, "租户 %v 的报价发布前提：%s（要求 %t）\n", rule.Tenant(), outcome, rule.Requires())
	if outcome == pcapplication.QuotationPublicationRequirementConflict {
		return exitAttention
	}
	return exitLanded
}
