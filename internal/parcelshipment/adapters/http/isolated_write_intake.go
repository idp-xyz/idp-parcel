package shipmenthttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// IsolatedSubmissionIntake 是隔离写路径准入（ADR-0091）在提交口的注入式放行。
//
// 它与隔离读面的 IsolatedOperationsReadIntake 同层同款，但**多做三件读面不必做的事**，
// 因为提交命令的入参不是一个作用域就能凑齐的：
//
//   - **来源信封**：租户、货主客户账户、来源三样整组注入，请求里写什么都不看（ADR-0003
//     的最高隔离边界在隔离形态下同样不许被自报穿透）。来源请求键是唯一从请求内容派生的
//     一格，见下面 sourceRequestKey 的注释。
//   - **载荷摘要与要素子段**：把管理台草案译成 `SubmissionPayloadSpec` 再走领域侧的
//     `CanonicalizeSubmission`（ADR-0014；pp-seams/05 裁决 3——摘要与寄 / 收地址要素一次产出、一起
//     进命令）。摘要只吃客户声明的引用，不吃本包现签的内部标识——否则同一份内容重发两次会得到
//     两个摘要，重放就被判成`接入冲突`。
//   - **期望规则修订**：向归属权威预取。`UC-PS-001` 步骤 3B 把「准入范围与期望规则修订」
//     判给试点准入控制装配，本包据此照办；编排随后在提交时点重判一次，因此门禁仍拦得住
//     预取与提交之间登记册发生的变化，只是拦不住「调用方揣着上周的修订」——隔离形态里
//     没有那样的调用方。
//
// **本类型对 `PAR-INT-01` 一格都不预设。** 它翻译的是本仓自己那张管理台页面的草案形状，
// 不是任何租户的渠道契约；真渠道的词表、认证与幂等键仍按 ADR-0072 等渠道证据，落在
// `internal/accessidentity/`。两者不会相混：本类型只在隔离形态的装配路径上出现，生产
// 装配里没有通往它的代码路径（ADR-0091 判据三）。
type IsolatedSubmissionIntake struct {
	tenant          domain.TenantID
	customerAccount domain.CustomerAccountID
	source          domain.Source
	admissionScope  domain.AdmissionScope
	ownership       ports.ProductionOwnershipAuthority
	clock           ports.Clock
	requests        platformidentity.Minter
	batches         platformidentity.Minter
	parcels         platformidentity.Minter
}

var _ SubmissionIntake = (*IsolatedSubmissionIntake)(nil)

// 本包现签内部标识用的前缀。它们不带 `SYN-`：合成来源由**行自己的租户维与来源维**表达
// （ADR-0091 决定三），内部标识跟着染色只会让人以为分辨靠的是它。
const (
	isolatedRequestPrefix = "SHR"
	isolatedBatchPrefix   = "SBAT"
	isolatedParcelPrefix  = "DPCL"
)

// IsolatedSubmissionIntakeDeps 是构造本 Intake 的全部输入。四个字符串是装配点给定的
// 合成值，两个协作者是本进程既有的生产实现。
type IsolatedSubmissionIntakeDeps struct {
	Tenant          string
	CustomerAccount string
	Source          string
	ScopeReference  string
	ScopeDigest     string
	Ownership       ports.ProductionOwnershipAuthority
	Clock           ports.Clock
}

// NewIsolatedSubmissionIntake 由装配点以显式合成值构造。立不起来的值在这里拒：装配错误
// 要在启动时暴露，不该等到第一个请求。
func NewIsolatedSubmissionIntake(deps IsolatedSubmissionIntakeDeps) (*IsolatedSubmissionIntake, error) {
	fail := func(err error) (*IsolatedSubmissionIntake, error) {
		return nil, fmt.Errorf("parcel shipment http: isolated submission intake: %w", err)
	}

	tenant, err := domain.NewTenantID(deps.Tenant)
	if err != nil {
		return fail(err)
	}
	account, err := domain.NewCustomerAccountID(deps.CustomerAccount)
	if err != nil {
		return fail(err)
	}
	source, err := domain.NewSource(deps.Source)
	if err != nil {
		return fail(err)
	}
	scopeReference, err := domain.NewAdmissionScopeReference(deps.ScopeReference)
	if err != nil {
		return fail(err)
	}
	scopeDigest, err := domain.NewAdmissionScopeDigest(deps.ScopeDigest)
	if err != nil {
		return fail(err)
	}
	scope, err := domain.NewAdmissionScope(scopeReference, scopeDigest)
	if err != nil {
		return fail(err)
	}
	if deps.Ownership == nil {
		return fail(fmt.Errorf("ownership authority is nil"))
	}
	if deps.Clock == nil {
		return fail(fmt.Errorf("clock is nil"))
	}

	requests, err := platformidentity.NewMinter(isolatedRequestPrefix)
	if err != nil {
		return fail(err)
	}
	batches, err := platformidentity.NewMinter(isolatedBatchPrefix)
	if err != nil {
		return fail(err)
	}
	parcels, err := platformidentity.NewMinter(isolatedParcelPrefix)
	if err != nil {
		return fail(err)
	}

	return &IsolatedSubmissionIntake{
		tenant:          tenant,
		customerAccount: account,
		source:          source,
		admissionScope:  scope,
		ownership:       deps.Ownership,
		clock:           deps.Clock,
		requests:        requests,
		batches:         batches,
		parcels:         parcels,
	}, nil
}

// submissionDraft 是管理台提交页送上来的形状（`apps/admin-web` 的 `ShipmentRequestDraft`）。
// 它是本仓自己那张页面的草案形状，不是已发布的渠道 Schema——页面注释与本注释同此一句。
type submissionDraft struct {
	CustomerShipmentReference string  `json:"customerShipmentReference"`
	RequestedServiceProduct   string  `json:"requestedServiceProduct"`
	RequestEffectiveAt        *string `json:"requestEffectiveAt"`
	SenderRelation            string  `json:"senderRelation"`
	SenderAddress             string  `json:"senderAddress"`
	// SenderPostalCode / SenderCountryCode 与收件那一对是寄 / 收两范围的地址要素（PS CONTEXT「地址要素」；pp-seams/05）：
	// 译成按封闭要素名命名的范围条目，随同一次规范化既进摘要又挑成要素子段进命令。四格都可缺席——缺席即那一格没报。
	// 它们是本仓自己那张页面的草案字段，不是任何租户的渠道字段名。
	SenderPostalCode        string                `json:"senderPostalCode"`
	SenderCountryCode       string                `json:"senderCountryCode"`
	RecipientRelation       string                `json:"recipientRelation"`
	RecipientAddress        string                `json:"recipientAddress"`
	RecipientPostalCode     string                `json:"recipientPostalCode"`
	RecipientCountryCode    string                `json:"recipientCountryCode"`
	DestinationServiceScope string                `json:"destinationServiceScope"`
	Parcels                 []declaredParcelDraft `json:"parcels"`
}

type declaredParcelDraft struct {
	CustomerParcelReference string `json:"customerParcelReference"`
	DeclaredWeightValue     string `json:"declaredWeightValue"`
	DeclaredWeightUnit      string `json:"declaredWeightUnit"`
	DeclaredLength          string `json:"declaredLength"`
	DeclaredWidth           string `json:"declaredWidth"`
	DeclaredHeight          string `json:"declaredHeight"`
	DeclaredDimensionsUnit  string `json:"declaredDimensionsUnit"`
	GoodsDescription        string `json:"goodsDescription"`
	Quantity                string `json:"quantity"`
	DeclaredValue           string `json:"declaredValue"`
	Currency                string `json:"currency"`
	OriginCountry           string `json:"originCountry"`
	ServiceNotes            string `json:"serviceNotes"`
}

// IntakeSubmission 把一次管理台提交译成提交命令。
//
// 请求体只供给「客户可声明」的那几组（`UC-PS-001` 输入语义契约）；来源信封整组不看请求。
// 形状级失败包 ErrMalformedRequest（4xx，重发同样内容不会好）；归属权威调不通不包它
// （5xx，运维救依赖）——两格的恢复动作不同，判据同 ADR-0029。
func (intake *IsolatedSubmissionIntake) IntakeSubmission(
	ctx context.Context,
	request *http.Request,
) (application.SubmitShipmentRequestCommand, error) {
	var draft submissionDraft
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&draft); err != nil {
		return application.SubmitShipmentRequestCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}

	identity, err := intake.sourceIdentity(draft)
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}

	customerReference, customerParcels, err := customerDeclaredReferences(draft)
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}
	effectiveAt, err := declaredEffectiveAt(draft)
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}
	// 只调这一次：摘要与要素子段由同一份规范化输入一次产出、一起进命令（pp-seams/05 裁决 3）。再为要素单独算一遍，
	// 内容与摘要就有了各说各话的口子——结构上不给它留位置。
	canonical, err := domain.CanonicalizeSubmission(domain.SubmissionPayloadSpec{
		RequestReference:  customerReference,
		EffectiveAt:       effectiveAt,
		DeclaredParcelIDs: customerParcels,
		Scope:             scopeEntries(draft),
		Service:           serviceEntries(draft),
	})
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}

	internalRequest, internalBatch, internalParcels, err := intake.mintInternalIdentities(len(customerParcels))
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}

	// 归属权威调不通时上抛裸错：那是依赖故障，不是这次请求的形状问题。
	decision, err := intake.ownership.DecideProductionOwnership(ctx, intake.admissionScope)
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, fmt.Errorf("isolated submission intake: decide production ownership: %w", err)
	}

	now := intake.clock.Now().UTC()
	return application.SubmitShipmentRequestCommand{
		Identity:          identity,
		PayloadDigest:     canonical.Digest(),
		OccurredAt:        now,
		ReceivedAt:        now,
		BatchID:           internalBatch,
		ShipmentRequestID: internalRequest,
		DeclaredParcelIDs: internalParcels,
		AdmissionScope:    intake.admissionScope,
		ExpectedRevision:  decision.Revision(),
		DeclaredElements:  canonical.DeclaredElements(),
	}, nil
}

// sourceIdentity 铸来源信封。租户、客户账户与来源整组来自注入；**来源请求键是唯一从
// 请求内容派生的一格**，取客户委托参考。
//
// 这不是对 `PAR-INT-01`「来源请求键由哪个渠道字段铸成」那一格的回答，也不许被读成那个：
// 真渠道的推导口在 `internal/accessidentity` 留着未决（`RequestKeyDerivation` 仓内零生产
// 实现），本包碰都不碰它。这里取客户委托参考只是因为隔离形态需要一个稳定的重放语义——
// 同一份委托参考再发一次要被认出是重放，而不是长出第二份委托。
func (intake *IsolatedSubmissionIntake) sourceIdentity(
	draft submissionDraft,
) (domain.SourceIdentity, error) {
	reference := strings.TrimSpace(draft.CustomerShipmentReference)
	if reference == "" {
		return domain.SourceIdentity{}, fmt.Errorf("%w: customerShipmentReference is blank", ErrMalformedRequest)
	}
	key, err := domain.NewSourceRequestKey(reference)
	if err != nil {
		return domain.SourceIdentity{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	identity, err := domain.NewSourceIdentity(intake.tenant, intake.customerAccount, intake.source, key)
	if err != nil {
		return domain.SourceIdentity{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return identity, nil
}

// declaredEffectiveAt 取客户请求生效时间。键缺席交回零值，而零值在
// `CanonicalizeSubmissionPayload` 里编成「未声明」那一格——值与缺失/显式存在状态都进
// 摘要是 CONTEXT 硬句，所以这里绝不能拿「零值」冒充「声明了但为空」，也不能拿
// occurredAt 顶替。键在场但解不出时间是形状错，如实答 4xx。
func declaredEffectiveAt(draft submissionDraft) (time.Time, error) {
	if draft.RequestEffectiveAt == nil {
		return time.Time{}, nil
	}
	declared, err := time.Parse(time.RFC3339, strings.TrimSpace(*draft.RequestEffectiveAt))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: requestEffectiveAt: %v", ErrMalformedRequest, err)
	}
	return declared, nil
}

// customerDeclaredReferences 取客户声明的委托参考与逐件包裹参考。摘要吃的是这一组，不是
// 本包现签的内部标识——理由在类型注释的第二条。
func customerDeclaredReferences(
	draft submissionDraft,
) (domain.ShipmentRequestID, []domain.DeclaredParcelID, error) {
	reference, err := domain.NewShipmentRequestID(strings.TrimSpace(draft.CustomerShipmentReference))
	if err != nil {
		return domain.ShipmentRequestID{}, nil, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if len(draft.Parcels) == 0 {
		return domain.ShipmentRequestID{}, nil, fmt.Errorf("%w: at least one parcel is required", ErrMalformedRequest)
	}
	parcels := make([]domain.DeclaredParcelID, 0, len(draft.Parcels))
	for _, parcel := range draft.Parcels {
		declared, err := domain.NewDeclaredParcelID(strings.TrimSpace(parcel.CustomerParcelReference))
		if err != nil {
			return domain.ShipmentRequestID{}, nil, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
		}
		parcels = append(parcels, declared)
	}
	return reference, parcels, nil
}

// mintInternalIdentities 现签内部标识。它们**不从客户参考派生**——`SubmissionIdentityFactory`
// 的端口注释把这条写死了：「客户参考号不得变成内部标识」。代价是同一份内容重发两次会
// 签出两组新标识，但那两组都用不上：重放在来源身份上就被认出，命令里的标识根本走不到建单。
func (intake *IsolatedSubmissionIntake) mintInternalIdentities(
	parcelCount int,
) (domain.ShipmentRequestID, domain.SubmissionBatchID, []domain.DeclaredParcelID, error) {
	requestValue, err := intake.requests.Next()
	if err != nil {
		return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
	}
	requestID, err := domain.NewShipmentRequestID(requestValue)
	if err != nil {
		return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
	}
	batchValue, err := intake.batches.Next()
	if err != nil {
		return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
	}
	batchID, err := domain.NewSubmissionBatchID(batchValue)
	if err != nil {
		return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
	}
	parcels := make([]domain.DeclaredParcelID, 0, parcelCount)
	for index := 0; index < parcelCount; index++ {
		parcelValue, err := intake.parcels.Next()
		if err != nil {
			return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
		}
		parcelID, err := domain.NewDeclaredParcelID(parcelValue)
		if err != nil {
			return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
		}
		parcels = append(parcels, parcelID)
	}
	return requestID, batchID, parcels, nil
}

// scopeEntries 与 serviceEntries 是本形状的规范化词表：哪个草案字段进摘要的哪一段。
// 缺席的字段整条不进——`CanonicalContentEntry` 把「显式清空」与「条目缺席」定为两种内容，
// 给缺席字段补一条空值条目会把前者的语义安在后者头上。
//
// 邮编与国家 / 地区码四格不用本文件自造的点号名，而取 domain.AddressElementEntryName 拼出的封闭名字：
// 那是 AddressElementsOf 唯一认的名字，本形状用别的名字它们就只进摘要、进不了要素子段。
func scopeEntries(draft submissionDraft) []domain.CanonicalContentEntry {
	sender, delivery := domain.SenderPlaceDataGroup(), domain.DeliveryPlaceDataGroup()
	return canonicalEntries(map[string]string{
		"sender.relation":          draft.SenderRelation,
		"sender.address":           draft.SenderAddress,
		"recipient.relation":       draft.RecipientRelation,
		"recipient.address":        draft.RecipientAddress,
		"destination.serviceScope": draft.DestinationServiceScope,
		domain.AddressElementEntryName(sender, domain.PostalCodeElement):    draft.SenderPostalCode,
		domain.AddressElementEntryName(sender, domain.CountryCodeElement):   draft.SenderCountryCode,
		domain.AddressElementEntryName(delivery, domain.PostalCodeElement):  draft.RecipientPostalCode,
		domain.AddressElementEntryName(delivery, domain.CountryCodeElement): draft.RecipientCountryCode,
	})
}

func serviceEntries(draft submissionDraft) []domain.CanonicalContentEntry {
	entries := map[string]string{"service.requestedProduct": draft.RequestedServiceProduct}
	for _, parcel := range draft.Parcels {
		reference := strings.TrimSpace(parcel.CustomerParcelReference)
		for name, value := range map[string]string{
			"weight.value":      parcel.DeclaredWeightValue,
			"weight.unit":       parcel.DeclaredWeightUnit,
			"dimensions.length": parcel.DeclaredLength,
			"dimensions.width":  parcel.DeclaredWidth,
			"dimensions.height": parcel.DeclaredHeight,
			"dimensions.unit":   parcel.DeclaredDimensionsUnit,
			"goods.description": parcel.GoodsDescription,
			"goods.quantity":    parcel.Quantity,
			"goods.value":       parcel.DeclaredValue,
			"goods.currency":    parcel.Currency,
			"goods.origin":      parcel.OriginCountry,
			"service.notes":     parcel.ServiceNotes,
		} {
			entries["parcel["+reference+"]."+name] = value
		}
	}
	return canonicalEntries(entries)
}

func canonicalEntries(values map[string]string) []domain.CanonicalContentEntry {
	entries := make([]domain.CanonicalContentEntry, 0, len(values))
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		entry, err := domain.NewCanonicalContentEntry(name, value)
		if err != nil {
			// 名字是本文件的字面量常量，构造不出来只可能是这里写错了。
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}
