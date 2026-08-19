package domain

// 本文件拥有来源事实替代关系的派生问答。替代与冲突裁决（fact_conflict.go）平行而
// 不合并：冲突回答「VE 依据哪一维给多份有效事实排序」，替代回答「源上下文已经说了
// 哪一份取代哪一份」——两者的决定权不在同一侧，替代因此不进 ConflictResolutionBasis。

// CurrentlyEffective 交回这组已接受事实中当前有效——即未被替代——的那些，保持
// 输入顺序。
//
// 「已被替代」是派生问答不是可变标记（CONTEXT 硬句）：一份已接受事实处于已被替代，
// 当且仅当另一份已接受事实指名它为前身。本判断按当下在场的事实集重新回答，替代先于
// 被替代到达因而不需要等待态，也不需要事后回填。关系只在同一源上下文、同一事实引用
// 的版本之间成立：跨源上下文或跨事实引用的指名不构成替代，这里直接不采——那是冲突，
// 按冲突裁决处理，不得借替代关系代为裁决。
//
// 替代链分叉（同一前身被两份以上事实指名）不择一：分叉不改变「前身已被替代」，各
// 后继全部保留（AT-VE-043「保留双方和冲突关系」），投影据此把各方都摆在场、按信息
// 待确认表达；适用异常信号的形成走冲突机制，不在本问答里。
func CurrentlyEffective(facts []AcceptedSourceFact) []AcceptedSourceFact {
	// 键含源上下文与事实引用，正是「关系只在同一源上下文、同一事实引用的版本之间
	// 成立」那条硬句的落点；被替代的后继自己的指名不因此收回——源上下文给出的关系
	// 只增不删。
	type supersessionScope struct {
		source  SourceContext
		fact    string
		version string
	}
	named := make(map[supersessionScope]bool, len(facts))
	for _, fact := range facts {
		if predecessor, given := fact.Supersedes(); given {
			named[supersessionScope{fact.source, fact.fact.String(), predecessor.String()}] = true
		}
	}
	effective := make([]AcceptedSourceFact, 0, len(facts))
	for _, fact := range facts {
		if named[supersessionScope{fact.source, fact.fact.String(), fact.version.String()}] {
			continue
		}
		effective = append(effective, fact)
	}
	return effective
}
