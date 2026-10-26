package registrationjson_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件证资金映射的两格新目标与核销、核销撤销两份载荷的译装：正——逐格到命令，不推导分配、不补映射清单；反——
// 词表外、有构造门的标识空白、未知字段，一律在入库前拒并指名差哪格。分配有没有映射背书、守不守恒、超没超出目标的
// 未核销都是用例与领域的门，这里不代判。

const fundsApplicationDocument = `{
	"tenantId": "SYN-T1",
	"applicationRef": "SYN-APPLICATION-1",
	"factRef": "SYN-FACT-1",
	"targetCurrency": "XTS",
	"mappingRefs": ["SYN-MAPPING-STMT-1", "SYN-MAPPING-COMP-1"],
	"allocations": [
		{"mappingRef": "SYN-MAPPING-STMT-1", "targetKind": "STATEMENT", "targetRef": "SYN-STMT-1", "direction": "DEBIT", "amountMinor": 5000},
		{"mappingRef": "SYN-MAPPING-COMP-1", "targetKind": "CUSTOMER_COMPENSATION", "targetRef": "SYN-COMP-1", "direction": "DEBIT", "amountMinor": 1500}
	],
	"basis": "SYN-PAYMENT-INSTRUCTION-1",
	"appliedAt": "2026-09-03T08:00:00Z"
}`

const fundsApplicationReversalDocument = `{
	"tenantId": "SYN-T1",
	"applicationRef": "SYN-APPLICATION-1",
	"basis": "SYN-FUNDS-RETURNED-1",
	"reversedAt": "2026-09-04T08:00:00Z"
}`

const fundsMappingDocument = `{
	"tenantId": "SYN-T1",
	"mappingRef": "SYN-MAPPING-COMP-1",
	"factRef": "SYN-FACT-1",
	"targetKind": "CUSTOMER_COMPENSATION",
	"targetRef": "SYN-COMP-1",
	"basis": "SYN-PAYMENT-INSTRUCTION-1",
	"mappedAt": "2026-09-02T08:00:00Z"
}`

// Covers: `AT-SA-116`、`AT-SA-117` 的入口面——资金映射的目标词表收客户赔付与追偿认可两格，与领域 String() 同字；
// 词表外的名字（含把认可叫成 ACKNOWLEDGEMENT 的那一种）照旧拒。
func TestFundsMappingFromJSONTakesTheClaimAmountTargets(t *testing.T) {
	for name, kind := range map[string]domain.SettlementTargetKind{
		"CUSTOMER_COMPENSATION": domain.TargetCustomerCompensation,
		"RECOVERY_RECEIVABLE":   domain.TargetRecoveryReceivable,
	} {
		raw := strings.Replace(fundsMappingDocument, `"CUSTOMER_COMPENSATION"`, `"`+name+`"`, 1)
		command, err := registrationjson.FundsMappingFromJSON([]byte(raw))
		if err != nil {
			t.Fatalf("%s 译装被拒：%v", name, err)
		}
		if command.TargetKind != kind || command.Target != "SYN-COMP-1" {
			t.Fatalf("%s 译成 %s/%s", name, command.TargetKind, command.Target)
		}
	}
	raw := strings.Replace(fundsMappingDocument, `"CUSTOMER_COMPENSATION"`, `"RECOVERY_ACKNOWLEDGEMENT"`, 1)
	if _, err := registrationjson.FundsMappingFromJSON([]byte(raw)); err == nil || !strings.Contains(err.Error(), "targetKind") {
		t.Fatalf("词表外的目标：err = %v，要拒并指名 targetKind", err)
	}
}

// Covers: 核销载荷逐格到 ApplySettlementCommand——映射清单与分配原样带过去，不从分配里推映射、不并片、不排序；方向与
// 目标种类按封闭词表译回领域常量；金额与时刻原样。
func TestFundsApplicationFromJSONCarriesEveryFieldWithFidelity(t *testing.T) {
	command, err := registrationjson.FundsApplicationFromJSON([]byte(fundsApplicationDocument))
	if err != nil {
		t.Fatalf("译装被拒：%v", err)
	}
	if command.TenantID.String() != "SYN-T1" || command.Application != "SYN-APPLICATION-1" || command.Fact != "SYN-FACT-1" {
		t.Fatalf("身份失真：%+v", command)
	}
	if command.TargetCurrency != "XTS" || command.Basis != "SYN-PAYMENT-INSTRUCTION-1" {
		t.Fatalf("币种或依据失真：%+v", command)
	}
	if want := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC); !command.AppliedAt.Equal(want) {
		t.Fatalf("appliedAt = %s，要 %s", command.AppliedAt, want)
	}
	if !reflect.DeepEqual(command.MappingRefs, []string{"SYN-MAPPING-STMT-1", "SYN-MAPPING-COMP-1"}) {
		t.Fatalf("mappingRefs = %v", command.MappingRefs)
	}
	want := []application.AllocationDirective{
		{Mapping: "SYN-MAPPING-STMT-1", TargetKind: domain.TargetStatement, Target: "SYN-STMT-1", Direction: domain.AllocationDebit, AmountMinor: 5000},
		{Mapping: "SYN-MAPPING-COMP-1", TargetKind: domain.TargetCustomerCompensation, Target: "SYN-COMP-1", Direction: domain.AllocationDebit, AmountMinor: 1500},
	}
	if !reflect.DeepEqual(command.Allocations, want) {
		t.Fatalf("allocations = %+v，要 %+v", command.Allocations, want)
	}
}

// Covers: 核销载荷的拒绝面——形状坏、词表外、标识空白在入库前拒并指名那一格。贷向指向赔付、分配超额、没有映射背书
// 不在此拒：那是领域与用例的`未受理`、`分配失衡`格。
func TestFundsApplicationFromJSONRejectsBadVocabularyAndBlankIdentifiers(t *testing.T) {
	cases := map[string]struct {
		raw   string
		names string
	}{
		"未知字段":     {strings.Replace(fundsApplicationDocument, `"basis"`, `"typo": 1, "basis"`, 1), "typo"},
		"分配里的未知字段": {strings.Replace(fundsApplicationDocument, `"amountMinor": 5000}`, `"amountMinor": 5000, "amount": 1}`, 1), "amount"},
		"方向词表外":    {strings.Replace(fundsApplicationDocument, `"direction": "DEBIT", "amountMinor": 5000`, `"direction": "DEBT", "amountMinor": 5000`, 1), "direction"},
		"目标种类词表外":  {strings.Replace(fundsApplicationDocument, `"targetKind": "STATEMENT"`, `"targetKind": "CHARGE"`, 1), "targetkind"},
		"核销引用空白":   {strings.Replace(fundsApplicationDocument, `"applicationRef": "SYN-APPLICATION-1"`, `"applicationRef": " "`, 1), "applicationref"},
		"事实引用空白":   {strings.Replace(fundsApplicationDocument, `"factRef": "SYN-FACT-1"`, `"factRef": ""`, 1), "factref"},
		"目标币种空白":   {strings.Replace(fundsApplicationDocument, `"targetCurrency": "XTS"`, `"targetCurrency": ""`, 1), "targetcurrency"},
		"映射清单里有空白": {strings.Replace(fundsApplicationDocument, `["SYN-MAPPING-STMT-1", `, `["", `, 1), "mappingrefs"},
		"分配的映射空白":  {strings.Replace(fundsApplicationDocument, `{"mappingRef": "SYN-MAPPING-STMT-1"`, `{"mappingRef": " "`, 1), "mappingref"},
		"分配的目标空白":  {strings.Replace(fundsApplicationDocument, `"targetRef": "SYN-STMT-1"`, `"targetRef": ""`, 1), "targetref"},
		"依据空白":     {strings.Replace(fundsApplicationDocument, `"basis": "SYN-PAYMENT-INSTRUCTION-1"`, `"basis": ""`, 1), "basis"},
		"租户空白":     {strings.Replace(fundsApplicationDocument, `"tenantId": "SYN-T1"`, `"tenantId": ""`, 1), "tenantid"},
	}
	for name, test := range cases {
		_, err := registrationjson.FundsApplicationFromJSON([]byte(test.raw))
		if err == nil {
			t.Fatalf("%s：译装没拒", name)
		}
		if !strings.Contains(strings.ToLower(err.Error()), test.names) {
			t.Fatalf("%s：错误 %q 没指名 %s", name, err, test.names)
		}
	}
}

// Covers: 核销撤销载荷逐格到 ReverseApplicationCommand；它只有核销引用、撤销依据与时刻——撤销只登记核销关系失效，
// 不带新的分配（AT-SA-112、AT-SA-113），带了按未知字段拒。
func TestFundsApplicationReversalFromJSONCarriesOnlyTheReversal(t *testing.T) {
	command, err := registrationjson.FundsApplicationReversalFromJSON([]byte(fundsApplicationReversalDocument))
	if err != nil {
		t.Fatalf("译装被拒：%v", err)
	}
	if command.TenantID.String() != "SYN-T1" || command.Application != "SYN-APPLICATION-1" || command.Basis != "SYN-FUNDS-RETURNED-1" {
		t.Fatalf("撤销失真：%+v", command)
	}
	if want := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC); !command.ReversedAt.Equal(want) {
		t.Fatalf("reversedAt = %s，要 %s", command.ReversedAt, want)
	}

	cases := map[string]struct {
		raw   string
		names string
	}{
		"带分配":    {strings.Replace(fundsApplicationReversalDocument, `"basis"`, `"allocations": [], "basis"`, 1), "allocations"},
		"核销引用空白": {strings.Replace(fundsApplicationReversalDocument, `"applicationRef": "SYN-APPLICATION-1"`, `"applicationRef": ""`, 1), "applicationref"},
		"撤销依据空白": {strings.Replace(fundsApplicationReversalDocument, `"basis": "SYN-FUNDS-RETURNED-1"`, `"basis": " "`, 1), "basis"},
		"租户空白":   {strings.Replace(fundsApplicationReversalDocument, `"tenantId": "SYN-T1"`, `"tenantId": ""`, 1), "tenantid"},
	}
	for name, test := range cases {
		_, err := registrationjson.FundsApplicationReversalFromJSON([]byte(test.raw))
		if err == nil {
			t.Fatalf("%s：译装没拒", name)
		}
		if !strings.Contains(strings.ToLower(err.Error()), test.names) {
			t.Fatalf("%s：错误 %q 没指名 %s", name, err, test.names)
		}
	}
}
