package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/adapters/registrationjson"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件把受控批量口的两种批文折成命令。发布批的外壳与逐项翻译在 registrationjson，与操作者渠道的在线发布口
// 共用（票 operator-channel/18）；这里只留整批的解码与「第几项」的定位。解析键登记译成 parcel-shipment 的登记面，
// 留在本包。纪律同发布那一份：未知字段拒收、集合外取值拒收、缺件不代填。

func publishCommandsFromJSON(raw []byte) ([]pcapplication.PublishCommercialAuthorityCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document registrationjson.PublicationBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("发布批不是本入口的形状：%w", err)
	}
	if len(document.Items) == 0 {
		return nil, fmt.Errorf("发布批没有任何项")
	}
	commands := make([]pcapplication.PublishCommercialAuthorityCommand, 0, len(document.Items))
	for index, item := range document.Items {
		command, err := registrationjson.PublicationCommand(item)
		if err != nil {
			return nil, fmt.Errorf("第 %d 项：%w", index+1, err)
		}
		commands = append(commands, command)
	}
	return commands, nil
}

// ---- 解析键登记的 JSON 形状 ----

type resolutionKeyDocument struct {
	TenantID          string    `json:"tenantId"`
	CustomerAccountID string    `json:"customerAccountId"`
	Scope             string    `json:"scope"`
	LegalEntity       string    `json:"legalEntity"`
	AnchorPolicy      string    `json:"anchorPolicyVersion"`
	AnchorAt          time.Time `json:"anchorAt"`
	RequiredBases     []string  `json:"requiredBases"`
	// Settlement 只在必需依据含 SETTLEMENT_POLICY 时给出，且只有三维——合同维由闭包解出
	// 的客户合同来填，登记面结构上就没有它（ADR-0080）。
	Settlement *settlementSelectorDocument `json:"settlement,omitempty"`
	// Credit 只在必需依据含 CREDIT_POLICY 时给出，且只有两维——法人与时点不在其中，键上已有
	// 法人候选与锚点，重复携带就允许两者不一致（ADR-0127 决定二）。
	Credit *creditSelectorDocument `json:"credit,omitempty"`
}

type settlementSelectorDocument struct {
	Counterparty string `json:"counterparty"`
	ChargeScope  string `json:"chargeScope"`
	Currency     string `json:"currency"`
}

type creditSelectorDocument struct {
	Level      string `json:"level"`
	ChargeType string `json:"chargeType"`
}

func keyRegistrationFromJSON(raw []byte) (pspartycommercial.ResolutionKeyRegistration, error) {
	none := pspartycommercial.ResolutionKeyRegistration{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document resolutionKeyDocument
	if err := decoder.Decode(&document); err != nil {
		return none, fmt.Errorf("解析键登记不是本入口的形状：%w", err)
	}

	registration := pspartycommercial.ResolutionKeyRegistration{AnchorAt: document.AnchorAt}
	var err error
	if registration.TenantID, err = psdomain.NewTenantID(document.TenantID); err != nil {
		return none, err
	}
	if registration.CustomerAccountID, err = psdomain.NewCustomerAccountID(document.CustomerAccountID); err != nil {
		return none, err
	}
	if registration.Scope, err = pcdomain.NewCommercialScopeReference(document.Scope); err != nil {
		return none, err
	}
	if registration.LegalEntity, err = pcdomain.NewLegalEntityReference(document.LegalEntity); err != nil {
		return none, err
	}
	if registration.AnchorPolicy, err = pcdomain.NewAnchorPolicyVersion(document.AnchorPolicy); err != nil {
		return none, err
	}
	for _, name := range document.RequiredBases {
		kind, err := registrationjson.CommercialKindFrom(name)
		if err != nil {
			return none, err
		}
		registration.RequiredBases = append(registration.RequiredBases, kind)
	}
	// 三维逐维过构造门。缺席整节即三维全缺，登记面据此判「本次不要结算依据」；给了节却
	// 少一维在这里就响亮失败，不折成缺席——那会让一次打错字段名变成一句「没登记结算」。
	if document.Settlement != nil {
		if registration.SettlementCounterparty, err = pcdomain.NewCounterpartyReference(
			document.Settlement.Counterparty); err != nil {
			return none, err
		}
		if registration.SettlementChargeScope, err = pcdomain.NewChargeScopeReference(
			document.Settlement.ChargeScope); err != nil {
			return none, err
		}
		if registration.SettlementCurrency, err = pcdomain.NewCurrencyCode(
			document.Settlement.Currency); err != nil {
			return none, err
		}
	}
	// 信用两维同一纪律：缺席整节即两维全缺，给了节却少一维在这里响亮失败。两维都走非空引用值
	// 的构造门而没有集外判读——商业权限等级是租户的版本化业务授权，party-commercial 不预设它
	// 有哪几档，费用类型同理。
	if document.Credit != nil {
		if registration.CreditLevel, err = pcdomain.NewAuthorityLevel(
			document.Credit.Level); err != nil {
			return none, err
		}
		if registration.CreditChargeType, err = pcdomain.NewChargeTypeReference(
			document.Credit.ChargeType); err != nil {
			return none, err
		}
	}
	return registration, nil
}
