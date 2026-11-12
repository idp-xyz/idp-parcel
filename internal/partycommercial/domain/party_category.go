package domain

import "time"

// PartyCategory 是业务参与方的类别（PC CONTEXT「参与方身份」节类别一句，ADR-0378 决定一）：它此刻持有的各个角色，
// 被未停用的责任法人钉着的另算我方公司，都不是的为无角色。类别不是业务参与方身份上的一格，是由登记的事实读出的结论；
// 词表是参与方角色各词加下面两个词。
//
// 「谁算客户」「谁算我方公司」只在本文件判一处：读面的 SQL 是它的镜像（postgres 适配器的业务参与方目录读面），两边用
// 同一批夹具对照测试；管理台不再自己推。
type PartyCategory string

const (
	// OurCompanyCategory：被一个未停用的责任法人钉着。
	OurCompanyCategory PartyCategory = "OUR_COMPANY"
	// NoRoleCategory：别的类别一项都不成立。它不与别的类别同现。
	NoRoleCategory PartyCategory = "NO_ROLE"
)

// RoleCategory 是「持有某个角色」这一类别，词就是角色的词。无效角色答空串。
func RoleCategory(role PartyRole) PartyCategory {
	return PartyCategory(role.String())
}

// PartyCategoryWords 按词表次序列出类别的全部词：各角色照枚举次序，然后是我方公司、无角色。读口的筛选维词表、分面与
// 答复行上类别的次序都取这一份，领域加一个角色时它们跟着加。
func PartyCategoryWords() []string {
	var words []string
	for role := CustomerRole; role.valid(); role++ {
		words = append(words, role.String())
	}
	return append(words, string(OurCompanyCategory), string(NoRoleCategory))
}

// Live 回答一段关系是否在用：候选或已生效（CONTEXT 客户角色一句的原话）。到期、撤销、替代是终局，边界之后不再依据它。
func (status RelationshipStatus) Live() bool {
	return status == RelationshipCandidate || status == RelationshipEffective
}

// PartyCategoriesAt 判一个业务参与方在明确时点的类别（ADR-0378 决定一）。held 是它作为持有方的各段关系（各取最新
// 修订；它作相对方的那些不给它角色，不要传进来），customerAccounts 是以它为所属客户参与方的各货主客户账户的生命周期，
// pinningEntities 是钉着它的各责任法人的生命周期。
//
// 每一项都是一次存在性判断，行与行之间不比较、不挑选、不汇总：
//   - 持有某个角色：有一段在用的该角色关系；
//   - 客户角色另有一条：有一个未停用的货主客户账户——两条任一成立即可，登记账户不要求先有客户关系；
//   - 我方公司：有一个未停用的责任法人钉着它。
//
// 「未停用」含生效时点未到的登记：已登记待生效的账户与法人是登好了、等着用的，与候选关系算在用同一条判据。
// 答出的类别照 PartyCategoryWords 的次序，永不为空：一项都不成立时只有无角色一个。时点无效时生命周期判不出状态，
// 账户与法人那两条都不成立。
func PartyCategoriesAt(
	held []PartyRelationship,
	customerAccounts []IdentityLifecycle,
	pinningEntities []IdentityLifecycle,
	at time.Time,
) []PartyCategory {
	holds := make(map[PartyRole]bool)
	for _, relationship := range held {
		if relationship.role.valid() && relationship.status.Live() {
			holds[relationship.role] = true
		}
	}
	if anyNotDeactivated(customerAccounts, at) {
		holds[CustomerRole] = true
	}

	var categories []PartyCategory
	for role := CustomerRole; role.valid(); role++ {
		if holds[role] {
			categories = append(categories, RoleCategory(role))
		}
	}
	if anyNotDeactivated(pinningEntities, at) {
		categories = append(categories, OurCompanyCategory)
	}
	if len(categories) == 0 {
		return []PartyCategory{NoRoleCategory}
	}
	return categories
}

func anyNotDeactivated(lifecycles []IdentityLifecycle, at time.Time) bool {
	for _, lifecycle := range lifecycles {
		switch lifecycle.StatusAt(at) {
		case IdentityRegistered, IdentityEffective:
			return true
		}
	}
	return false
}
