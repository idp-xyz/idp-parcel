package main

// 本文件承载参与方身份与关系的受控登记入口（票 admin-remainder-mechanism-batch/01）：
// register-parties 与 deactivate-party-identity 两个子命令。登记是操作者动作，走本
// CLI 不占 parcel-api 端点面（判据同 publish 的文件注释）。
//
// 批内各项独立成败、逐项各起事务（AT-PC-011 同款纪律）：文件内 parties → entities →
// accounts → relationships 的次序让批内前项先落库，后项的引用检查天然看得见前项；
// 某项被拒不撤已落的前项，重跑同一批已落项以重放回答。

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

type partyBatchDocument struct {
	TenantID         string                      `json:"tenantId"`
	BusinessParties  []businessPartyDocument     `json:"businessParties,omitempty"`
	LegalEntities    []legalEntityDocument       `json:"legalEntities,omitempty"`
	CustomerAccounts []customerAccountDocument   `json:"customerAccounts,omitempty"`
	Relationships    []partyRelationshipDocument `json:"relationships,omitempty"`
}

type businessPartyDocument struct {
	PartyID       string    `json:"partyId"`
	Name          string    `json:"name"`
	Revision      int       `json:"revision"`
	Basis         string    `json:"basis"`
	EffectiveFrom time.Time `json:"effectiveFrom"`
}

// legalEntityDocument 的身份层三格与在线口 legalEntities 项逐字同形（ADR-0145 决定一、二）：键可缺，缺不缺由用例判。
type legalEntityDocument struct {
	LegalEntityID               string                   `json:"legalEntityId"`
	PartyID                     string                   `json:"partyId"`
	Revision                    int                      `json:"revision"`
	Basis                       string                   `json:"basis"`
	EffectiveFrom               time.Time                `json:"effectiveFrom"`
	RegistrationCountry         *string                  `json:"registrationCountry,omitempty"`
	LifetimeRegistrationNumbers []lifetimeNumberDocument `json:"lifetimeRegistrationNumbers,omitempty"`
	IdentityCorrectionBasis     *string                  `json:"identityCorrectionBasis,omitempty"`
}

type lifetimeNumberDocument struct {
	TypeCode string `json:"typeCode"`
	Number   string `json:"number"`
}

type customerAccountDocument struct {
	AccountID       string    `json:"accountId"`
	CustomerPartyID string    `json:"customerPartyId"`
	Revision        int       `json:"revision"`
	Basis           string    `json:"basis"`
	EffectiveFrom   time.Time `json:"effectiveFrom"`
}

type partyRelationshipDocument struct {
	RelationshipID    string                        `json:"relationshipId"`
	Revision          int                           `json:"revision"`
	Holder            string                        `json:"holder"`
	Counterparty      string                        `json:"counterparty"`
	Role              string                        `json:"role"`
	Scope             string                        `json:"scope"`
	Basis             string                        `json:"basis"`
	EffectiveStartsAt time.Time                     `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time                    `json:"effectiveEndsAt,omitempty"`
	Approval          *relationshipApprovalDocument `json:"approval,omitempty"`
}

type relationshipApprovalDocument struct {
	Reference  string    `json:"reference"`
	ApprovedAt time.Time `json:"approvedAt"`
}

type deactivationBatchDocument struct {
	TenantID      string                 `json:"tenantId"`
	Deactivations []deactivationDocument `json:"deactivations"`
}

type deactivationDocument struct {
	Kind     string    `json:"kind"`
	ID       string    `json:"id"`
	Revision int       `json:"revision"`
	Basis    string    `json:"basis"`
	At       time.Time `json:"at"`
}

// partyBatchCommand 是翻译产物里的一项：标签供回显，执行闭包对着处理器跑。
type partyBatchCommand struct {
	label   string
	execute func(context.Context, *pcapplication.RegisterPartyIdentityHandler) (pcapplication.PartyRegistryResult, error)
}

func partyBatchFromJSON(raw []byte) ([]partyBatchCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document partyBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("参与方登记批不是本入口的形状：%w", err)
	}
	tenant, err := pcdomain.NewTenantID(document.TenantID)
	if err != nil {
		return nil, err
	}
	total := len(document.BusinessParties) + len(document.LegalEntities) +
		len(document.CustomerAccounts) + len(document.Relationships)
	if total == 0 {
		return nil, fmt.Errorf("参与方登记批没有任何项")
	}

	commands := make([]partyBatchCommand, 0, total)
	for _, item := range document.BusinessParties {
		command, err := businessPartyCommandFrom(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("businessParties/%s：%w", item.PartyID, err)
		}
		commands = append(commands, command)
	}
	for _, item := range document.LegalEntities {
		command, err := legalEntityCommandFrom(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("legalEntities/%s：%w", item.LegalEntityID, err)
		}
		commands = append(commands, command)
	}
	for _, item := range document.CustomerAccounts {
		command, err := customerAccountCommandFrom(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("customerAccounts/%s：%w", item.AccountID, err)
		}
		commands = append(commands, command)
	}
	for _, item := range document.Relationships {
		command, err := relationshipCommandFrom(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("relationships/%s：%w", item.RelationshipID, err)
		}
		commands = append(commands, command)
	}
	return commands, nil
}

func businessPartyCommandFrom(tenant pcdomain.TenantID, item businessPartyDocument) (partyBatchCommand, error) {
	none := partyBatchCommand{}
	partyID, err := pcdomain.NewPartyID(item.PartyID)
	if err != nil {
		return none, err
	}
	name, err := pcdomain.NewPartyName(item.Name)
	if err != nil {
		return none, err
	}
	party, err := pcdomain.NewBusinessParty(tenant, partyID, name)
	if err != nil {
		return none, err
	}
	basis, err := pcdomain.NewIdentityBasisReference(item.Basis)
	if err != nil {
		return none, err
	}
	command := pcapplication.RegisterBusinessPartyCommand{
		Party:         party,
		Revision:      item.Revision,
		Basis:         basis,
		EffectiveFrom: item.EffectiveFrom,
	}
	return partyBatchCommand{
		label: fmt.Sprintf("参与方 %s r%d", item.PartyID, item.Revision),
		execute: func(ctx context.Context, handler *pcapplication.RegisterPartyIdentityHandler) (pcapplication.PartyRegistryResult, error) {
			return handler.RegisterBusinessParty(ctx, command)
		},
	}, nil
}

func legalEntityCommandFrom(tenant pcdomain.TenantID, item legalEntityDocument) (partyBatchCommand, error) {
	none := partyBatchCommand{}
	entity, err := pcdomain.NewLegalEntityReference(item.LegalEntityID)
	if err != nil {
		return none, err
	}
	party, err := pcdomain.NewPartyID(item.PartyID)
	if err != nil {
		return none, err
	}
	basis, err := pcdomain.NewIdentityBasisReference(item.Basis)
	if err != nil {
		return none, err
	}
	command := pcapplication.RegisterLegalEntityCommand{
		Tenant:        tenant,
		Entity:        entity,
		Party:         party,
		Revision:      item.Revision,
		Basis:         basis,
		EffectiveFrom: item.EffectiveFrom,
	}
	if item.RegistrationCountry != nil {
		country, err := pcdomain.NewRegistrationCountryCode(*item.RegistrationCountry)
		if err != nil {
			return none, err
		}
		command.RegistrationCountry = &country
	}
	for _, number := range item.LifetimeRegistrationNumbers {
		typeCode, err := pcdomain.NewRegistrationNumberTypeCode(number.TypeCode)
		if err != nil {
			return none, err
		}
		value, err := pcdomain.NewRegistrationNumber(number.Number)
		if err != nil {
			return none, err
		}
		lifetime, err := pcdomain.NewLifetimeRegistrationNumber(typeCode, value)
		if err != nil {
			return none, err
		}
		command.LifetimeNumbers = append(command.LifetimeNumbers, lifetime)
	}
	if item.IdentityCorrectionBasis != nil {
		correction, err := pcdomain.NewIdentityBasisReference(*item.IdentityCorrectionBasis)
		if err != nil {
			return none, err
		}
		command.IdentityCorrectionBasis = &correction
	}
	return partyBatchCommand{
		label: fmt.Sprintf("责任法人 %s r%d", item.LegalEntityID, item.Revision),
		execute: func(ctx context.Context, handler *pcapplication.RegisterPartyIdentityHandler) (pcapplication.PartyRegistryResult, error) {
			return handler.RegisterLegalEntity(ctx, command)
		},
	}, nil
}

func customerAccountCommandFrom(tenant pcdomain.TenantID, item customerAccountDocument) (partyBatchCommand, error) {
	none := partyBatchCommand{}
	account, err := pcdomain.NewCustomerAccountID(item.AccountID)
	if err != nil {
		return none, err
	}
	party, err := pcdomain.NewPartyID(item.CustomerPartyID)
	if err != nil {
		return none, err
	}
	basis, err := pcdomain.NewIdentityBasisReference(item.Basis)
	if err != nil {
		return none, err
	}
	command := pcapplication.RegisterCustomerAccountCommand{
		Tenant:        tenant,
		Account:       account,
		CustomerParty: party,
		Revision:      item.Revision,
		Basis:         basis,
		EffectiveFrom: item.EffectiveFrom,
	}
	return partyBatchCommand{
		label: fmt.Sprintf("客户账户 %s r%d", item.AccountID, item.Revision),
		execute: func(ctx context.Context, handler *pcapplication.RegisterPartyIdentityHandler) (pcapplication.PartyRegistryResult, error) {
			return handler.RegisterCustomerAccount(ctx, command)
		},
	}, nil
}

func relationshipCommandFrom(tenant pcdomain.TenantID, item partyRelationshipDocument) (partyBatchCommand, error) {
	none := partyBatchCommand{}
	id, err := pcdomain.NewRelationshipID(item.RelationshipID)
	if err != nil {
		return none, err
	}
	holder, err := pcdomain.NewPartyID(item.Holder)
	if err != nil {
		return none, err
	}
	counterparty, err := pcdomain.NewPartyID(item.Counterparty)
	if err != nil {
		return none, err
	}
	role, err := partyRoleFromName(item.Role)
	if err != nil {
		return none, err
	}
	scope, err := pcdomain.NewCommercialScopeReference(item.Scope)
	if err != nil {
		return none, err
	}
	basis, err := pcdomain.NewRelationshipBasisReference(item.Basis)
	if err != nil {
		return none, err
	}
	endsAt := time.Time{}
	if item.EffectiveEndsAt != nil {
		endsAt = *item.EffectiveEndsAt
	}
	interval, err := pcdomain.NewEffectiveInterval(item.EffectiveStartsAt, endsAt)
	if err != nil {
		return none, err
	}
	command := pcapplication.RegisterPartyRelationshipCommand{
		Tenant:   tenant,
		ID:       id,
		Revision: item.Revision,
		Spec: pcdomain.PartyRelationshipSpec{
			Holder:       holder,
			Counterparty: counterparty,
			Role:         role,
			Scope:        scope,
			Basis:        basis,
			Effective:    interval,
		},
	}
	if item.Approval != nil {
		reference, err := pcdomain.NewApprovalReference(item.Approval.Reference)
		if err != nil {
			return none, err
		}
		command.Approval = &pcapplication.RelationshipApproval{
			Reference:  reference,
			ApprovedAt: item.Approval.ApprovedAt,
		}
	}
	return partyBatchCommand{
		label: fmt.Sprintf("参与方关系 %s r%d", item.RelationshipID, item.Revision),
		execute: func(ctx context.Context, handler *pcapplication.RegisterPartyIdentityHandler) (pcapplication.PartyRegistryResult, error) {
			return handler.RegisterRelationship(ctx, command)
		},
	}, nil
}

// partyRoleFromName 是 domain.PartyRole 封闭集的名称镜像；集合外取值拒收不吸收。
func partyRoleFromName(raw string) (pcdomain.PartyRole, error) {
	for _, role := range []pcdomain.PartyRole{
		pcdomain.CustomerRole, pcdomain.SupplierRole, pcdomain.CarrierAgentRole,
		pcdomain.ResellerRole, pcdomain.AccountHolderRole,
	} {
		if role.String() == raw {
			return role, nil
		}
	}
	return pcdomain.PartyRoleInvalid, fmt.Errorf("未知参与方角色 %q", raw)
}

// identityKindFromName 是 application.PartyIdentityKind 封闭集的名称镜像。
func identityKindFromName(raw string) (pcapplication.PartyIdentityKind, error) {
	for _, kind := range []pcapplication.PartyIdentityKind{
		pcapplication.BusinessPartyIdentity,
		pcapplication.LegalEntityIdentity,
		pcapplication.CustomerAccountIdentity,
	} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return pcapplication.PartyIdentityKindInvalid, fmt.Errorf("未知身份种类 %q", raw)
}

func deactivationCommandsFromJSON(raw []byte) ([]pcapplication.DeactivatePartyIdentityCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document deactivationBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("停用批不是本入口的形状：%w", err)
	}
	tenant, err := pcdomain.NewTenantID(document.TenantID)
	if err != nil {
		return nil, err
	}
	if len(document.Deactivations) == 0 {
		return nil, fmt.Errorf("停用批没有任何项")
	}
	commands := make([]pcapplication.DeactivatePartyIdentityCommand, 0, len(document.Deactivations))
	for index, item := range document.Deactivations {
		kind, err := identityKindFromName(item.Kind)
		if err != nil {
			return nil, fmt.Errorf("第 %d 项：%w", index+1, err)
		}
		basis, err := pcdomain.NewIdentityBasisReference(item.Basis)
		if err != nil {
			return nil, fmt.Errorf("第 %d 项：%w", index+1, err)
		}
		commands = append(commands, pcapplication.DeactivatePartyIdentityCommand{
			Tenant:   tenant,
			Kind:     kind,
			ID:       item.ID,
			Revision: item.Revision,
			Basis:    basis,
			At:       item.At,
		})
	}
	return commands, nil
}

func runRegisterParties(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-parties", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "参与方登记批 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-parties 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读登记批：%v\n", err)
		return exitTechnical
	}
	commands, err := partyBatchFromJSON(raw)
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

	registry, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造身份登记册：%v\n", err)
		return exitTechnical
	}
	numberTypes, err := pcpostgres.NewRegistrationNumberTypes(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造注册号类型目录：%v\n", err)
		return exitTechnical
	}
	handler := pcapplication.NewRegisterPartyIdentityHandler(registry, numberTypes)

	attention := false
	for index, command := range commands {
		var result pcapplication.PartyRegistryResult
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = command.execute(txCtx, handler)
			return handleErr
		})
		label := fmt.Sprintf("第 %d/%d 项 %s", index+1, len(commands), command.label)
		if err != nil {
			fmt.Fprintf(errOut, "%s：技术失败，批在此停下：%v\n", label, err)
			return exitTechnical
		}
		fmt.Fprintf(out, "%s：%s%s\n", label, result.Outcome(), causeDetail(result))
		switch result.Outcome() {
		case pcapplication.PartyIdentityContentConflict,
			pcapplication.PartyIdentityNotAccepted,
			pcapplication.PartyIdentityNotFound:
			attention = true
		}
	}
	if attention {
		return exitAttention
	}
	return exitLanded
}

func runDeactivatePartyIdentity(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("deactivate-party-identity", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "停用批 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "deactivate-party-identity 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读停用批：%v\n", err)
		return exitTechnical
	}
	commands, err := deactivationCommandsFromJSON(raw)
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

	registry, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造身份登记册：%v\n", err)
		return exitTechnical
	}
	// 停用不判号，不接注册号类型目录。
	handler := pcapplication.NewRegisterPartyIdentityHandler(registry, nil)

	attention := false
	for index, command := range commands {
		var result pcapplication.PartyRegistryResult
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = handler.Deactivate(txCtx, command)
			return handleErr
		})
		label := fmt.Sprintf("第 %d/%d 项 停用 %s %s r%d",
			index+1, len(commands), command.Kind, command.ID, command.Revision)
		if err != nil {
			fmt.Fprintf(errOut, "%s：技术失败，批在此停下：%v\n", label, err)
			return exitTechnical
		}
		fmt.Fprintf(out, "%s：%s%s\n", label, result.Outcome(), causeDetail(result))
		switch result.Outcome() {
		case pcapplication.PartyIdentityContentConflict,
			pcapplication.PartyIdentityNotAccepted,
			pcapplication.PartyIdentityNotFound:
			attention = true
		}
	}
	if attention {
		return exitAttention
	}
	return exitLanded
}

func causeDetail(result pcapplication.PartyRegistryResult) string {
	if cause := result.Cause(); cause != nil {
		return fmt.Sprintf("（原因：%v）", cause)
	}
	return ""
}
