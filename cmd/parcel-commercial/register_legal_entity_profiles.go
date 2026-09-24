package main

// 本文件承载法人资料的受控登记入口（票 legal-entity-profile/03）：register-legal-entity-profiles 子命令，与在线口
// /commercial-legal-entity-profile-registrations 消费同一个登记用例。
//
// 批内各项独立成败、逐项各起事务（AT-PC-011 同款纪律）：某项被拒不撤已落的前项，重跑同一批已落项以重放回答。批文
// 一格不代填：invoiceTitle 缺席就是这笔修订不带开票资料，开立时答`资料不全`（ADR-0145 决定六），不拿法人名称去补
// 抬头；给了空串照样拒收，不当缺席。税务登记号与联系人可缺，给了就逐项过领域构造门。

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

type legalEntityProfileBatchDocument struct {
	TenantID string                           `json:"tenantId"`
	Profiles []legalEntityProfileItemDocument `json:"profiles"`
}

type legalEntityProfileItemDocument struct {
	LegalEntityID          string                                `json:"legalEntityId"`
	Revision               int                                   `json:"revision"`
	Basis                  string                                `json:"basis"`
	EffectiveFrom          time.Time                             `json:"effectiveFrom"`
	RegisteredAddress      *legalEntityProfileAddressDocument    `json:"registeredAddress"`
	TaxRegistrationNumbers []legalEntityProfileTaxNumberDocument `json:"taxRegistrationNumbers,omitempty"`
	InvoiceTitle           *string                               `json:"invoiceTitle,omitempty"`
	Contacts               []legalEntityProfileContactDocument   `json:"contacts,omitempty"`
}

type legalEntityProfileAddressDocument struct {
	Country string   `json:"country"`
	Lines   []string `json:"lines"`
}

type legalEntityProfileTaxNumberDocument struct {
	TypeCode string `json:"typeCode"`
	Number   string `json:"number"`
}

type legalEntityProfileContactDocument struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// legalEntityProfileBatchCommand 是翻译产物里的一项：标签供回显。
type legalEntityProfileBatchCommand struct {
	label   string
	command pcapplication.RegisterLegalEntityProfileCommand
}

func legalEntityProfileBatchFromJSON(raw []byte) ([]legalEntityProfileBatchCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document legalEntityProfileBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("法人资料登记批不是本入口的形状：%w", err)
	}
	tenant, err := pcdomain.NewTenantID(document.TenantID)
	if err != nil {
		return nil, err
	}
	if len(document.Profiles) == 0 {
		return nil, fmt.Errorf("法人资料登记批没有任何项")
	}

	commands := make([]legalEntityProfileBatchCommand, 0, len(document.Profiles))
	for _, item := range document.Profiles {
		command, err := legalEntityProfileCommandFrom(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("profiles/%s/r%d：%w", item.LegalEntityID, item.Revision, err)
		}
		commands = append(commands, legalEntityProfileBatchCommand{
			label:   fmt.Sprintf("法人资料 %s r%d", item.LegalEntityID, item.Revision),
			command: command,
		})
	}
	return commands, nil
}

func legalEntityProfileCommandFrom(
	tenant pcdomain.TenantID,
	item legalEntityProfileItemDocument,
) (pcapplication.RegisterLegalEntityProfileCommand, error) {
	none := pcapplication.RegisterLegalEntityProfileCommand{}
	entity, err := pcdomain.NewLegalEntityReference(item.LegalEntityID)
	if err != nil {
		return none, err
	}
	basis, err := pcdomain.NewLegalEntityProfileBasisReference(item.Basis)
	if err != nil {
		return none, err
	}
	if item.EffectiveFrom.IsZero() {
		return none, fmt.Errorf("effectiveFrom 缺席：生效时点不代填")
	}
	if item.RegisteredAddress == nil {
		return none, fmt.Errorf("registeredAddress 缺席：注册地址是每笔资料修订必带的一格")
	}
	country, err := pcdomain.NewRegistrationCountryCode(item.RegisteredAddress.Country)
	if err != nil {
		return none, err
	}
	address, err := pcdomain.NewRegisteredAddress(country, item.RegisteredAddress.Lines)
	if err != nil {
		return none, err
	}

	taxNumbers := make([]pcdomain.TaxRegistrationNumber, 0, len(item.TaxRegistrationNumbers))
	for _, number := range item.TaxRegistrationNumbers {
		typeCode, err := pcdomain.NewRegistrationNumberTypeCode(number.TypeCode)
		if err != nil {
			return none, err
		}
		value, err := pcdomain.NewRegistrationNumber(number.Number)
		if err != nil {
			return none, err
		}
		tax, err := pcdomain.NewTaxRegistrationNumber(typeCode, value)
		if err != nil {
			return none, err
		}
		taxNumbers = append(taxNumbers, tax)
	}

	var invoicing *pcdomain.InvoicingDetails
	if item.InvoiceTitle != nil {
		title, err := pcdomain.NewInvoiceTitle(*item.InvoiceTitle)
		if err != nil {
			return none, err
		}
		details, err := pcdomain.NewInvoicingDetails(title)
		if err != nil {
			return none, err
		}
		invoicing = &details
	}

	contacts := make([]pcdomain.LegalEntityContact, 0, len(item.Contacts))
	for _, contact := range item.Contacts {
		built, err := pcdomain.NewLegalEntityContact(contact.Name, contact.Email, contact.Phone)
		if err != nil {
			return none, err
		}
		contacts = append(contacts, built)
	}

	return pcapplication.RegisterLegalEntityProfileCommand{
		Tenant:        tenant,
		Entity:        entity,
		Revision:      item.Revision,
		Basis:         basis,
		EffectiveFrom: item.EffectiveFrom,
		Address:       address,
		TaxNumbers:    taxNumbers,
		Invoicing:     invoicing,
		Contacts:      contacts,
	}, nil
}

func runRegisterLegalEntityProfiles(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-legal-entity-profiles", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "法人资料登记批 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-legal-entity-profiles 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读登记批：%v\n", err)
		return exitTechnical
	}
	commands, err := legalEntityProfileBatchFromJSON(raw)
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

	profiles, err := pcpostgres.NewLegalEntityProfiles(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造法人资料登记册：%v\n", err)
		return exitTechnical
	}
	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造身份登记册：%v\n", err)
		return exitTechnical
	}
	numberTypes, err := pcpostgres.NewRegistrationNumberTypes(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造注册号类型目录：%v\n", err)
		return exitTechnical
	}
	handler := pcapplication.NewRegisterLegalEntityProfileHandler(profiles, identities, numberTypes)

	attention := false
	for index, item := range commands {
		var result pcapplication.LegalEntityProfileResult
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = handler.Register(txCtx, item.command)
			return handleErr
		})
		label := fmt.Sprintf("第 %d/%d 项 %s", index+1, len(commands), item.label)
		if err != nil {
			fmt.Fprintf(errOut, "%s：技术失败，批在此停下：%v\n", label, err)
			return exitTechnical
		}
		fmt.Fprintf(out, "%s：%s%s\n", label, result.Outcome(), legalEntityProfileCauseDetail(result))
		switch result.Outcome() {
		case pcapplication.LegalEntityProfileContentConflict,
			pcapplication.LegalEntityProfileNotAccepted:
			attention = true
		}
	}
	if attention {
		return exitAttention
	}
	return exitLanded
}

func legalEntityProfileCauseDetail(result pcapplication.LegalEntityProfileResult) string {
	if cause := result.Cause(); cause != nil {
		return fmt.Sprintf("（原因：%v）", cause)
	}
	return ""
}
