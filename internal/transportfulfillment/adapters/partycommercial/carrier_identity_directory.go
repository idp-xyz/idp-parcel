// Package partycommercial 是 transport-fulfillment 对 party-commercial 参与方身份登记册的消费侧适配器
// （ADR-0025：只有本包可以同时导入两个上下文；只翻译不判断，翻译必须是全函数）。
//
// 它只答一件事：某个承运主体身份在 PC 上登了没有。判断值只引用已登记的参与方或运营法人身份，本上下文
// 不为承运方铸身份（TF CONTEXT Boundaries；ADR-0103 决定六）——所以这里没有任何写口，也不读名称：
// 名称在册上找不到身份时归「待确认（承运主体身份未登记）」，那是 TF 的判断值，不是本包的答复。
package partycommercial

import (
	"context"
	"errors"
	"fmt"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUntranslatableAnswer 与其余跨上下文适配器的同名哨兵同义：某一侧交出了词汇表之外的内容，是编程错误
// 不是业务答案。
var ErrUntranslatableAnswer = errors.New("transport fulfillment partycommercial adapter: untranslatable answer")

// BusinessPartySource 是参与方册的最新修订读口，按 PC 的 PartyIdentityRegistry 原形取。
//
// 只取这一个方法而不依赖整个 PartyIdentityRegistry：那个口带着写口，消费侧拿到它就拿到了往 PC 册子里写
// 身份的能力，而「本上下文不铸身份」正是要在类型上就表达出来的那句话。
type BusinessPartySource interface {
	LoadLatestBusinessParty(
		ctx context.Context,
		tenant pcdomain.TenantID,
		party pcdomain.PartyID,
	) (pcdomain.BusinessPartyRegistration, bool, error)
}

// LegalEntitySource 是法人册的最新修订读口，判据同 BusinessPartySource。
type LegalEntitySource interface {
	LoadLatestLegalEntity(
		ctx context.Context,
		tenant pcdomain.TenantID,
		entity pcdomain.LegalEntityReference,
	) (pcdomain.LegalEntityRegistration, bool, error)
}

// CarrierIdentityDirectory 实现 tfports.CarrierIdentityDirectory：外部参与方查参与方册，自营运营法人查法人册
// （ADR-0103 决定三的两支各查各的）。
//
// **答的是存在性，不是生效与否。** PC 的 Load* 交回 found=false 即「从未登记」；已停用的身份仍然在册——停用
// 挡的是新的商业决定（PC CONTEXT），而判断记的是谁承运过这一事实，一份停用后的身份被判为某段的实际承运商
// 不是 PC 的新决定，PC 也不因此改变它任何商业角色（决定六）。生效时点同理不在这里读：多答一格就是在消费侧
// 复制一份会过期的推导。
type CarrierIdentityDirectory struct {
	parties  BusinessPartySource
	entities LegalEntitySource
}

// NewCarrierIdentityDirectory 两本册子都要：缺一本，对应那一支就永远答不出，而「答不出」在读口上会长成
// error，日后有人会把它读成 PC 坏了。
func NewCarrierIdentityDirectory(parties BusinessPartySource, entities LegalEntitySource) (*CarrierIdentityDirectory, error) {
	if parties == nil {
		return nil, fmt.Errorf("transport fulfillment partycommercial adapter: business party source is nil")
	}
	if entities == nil {
		return nil, fmt.Errorf("transport fulfillment partycommercial adapter: legal entity source is nil")
	}
	return &CarrierIdentityDirectory{parties: parties, entities: entities}, nil
}

var _ tfports.CarrierIdentityDirectory = (*CarrierIdentityDirectory)(nil)

// IdentityRegistered 答承运主体身份在 PC 上登了没有。
//
// 依赖调不通一律作为错误上抛，绝不折成 false：把「读不到」读成「未登记」，一次故障就会在 TF 那一侧长成一版
// 待确认（承运主体身份未登记），而两者的恢复动作不同（ADR-0029）。租户按原值传过去——跨越租户必须在签名上
// 看得见（ADR-0003），适配器不替换也不省略它。
func (directory *CarrierIdentityDirectory) IdentityRegistered(
	ctx context.Context,
	tenant tfdomain.TenantID,
	subject tfdomain.CarrierSubject,
) (bool, error) {
	pcTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	switch subject.Kind() {
	case tfdomain.ExternalCarrierParty:
		party, err := pcdomain.NewPartyID(subject.Reference())
		if err != nil {
			return false, fmt.Errorf("%w: party reference: %v", ErrUntranslatableAnswer, err)
		}
		_, found, err := directory.parties.LoadLatestBusinessParty(ctx, pcTenant, party)
		if err != nil {
			return false, fmt.Errorf("load latest business party: %w", err)
		}
		return found, nil
	case tfdomain.OwnOperatingLegalEntity:
		entity, err := pcdomain.NewLegalEntityReference(subject.Reference())
		if err != nil {
			return false, fmt.Errorf("%w: legal entity reference: %v", ErrUntranslatableAnswer, err)
		}
		_, found, err := directory.entities.LoadLatestLegalEntity(ctx, pcTenant, entity)
		if err != nil {
			return false, fmt.Errorf("load latest legal entity: %w", err)
		}
		return found, nil
	default:
		return false, fmt.Errorf("%w: carrier subject kind %q", ErrUntranslatableAnswer, subject.Kind())
	}
}
