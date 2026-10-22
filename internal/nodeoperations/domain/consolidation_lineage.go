package domain

// ContainmentLineage 是一件集运单元此刻的直接父级链路：目标自身在先，逐级直接父级，到根为止（CONTEXT「嵌套集运时
// 通过直接父级链路派生间接包含」）。链路由仓储沿当前容纳索引走出来——每件对象同一时点最多一个直接物理父级，
// 所以它是一条线而不是一棵树。
type ContainmentLineage []ConsolidationUnitID

// ClosesCycle 判把 member 移入链路起点那件单元会不会让包含关系成环（CONTEXT「集运包含关系必须无循环」）。成员就是
// 起点自身，或是起点的某一级祖先，成环；交回从起点到那一级的那一段，供答复点名要在哪里断开。成员不在链路上不成环。
//
// 成员是作业实物标识、链路上是集运单元标识，两者按同串认：袋套袋时同一只袋在两边各有一个名字，而两个名字就是同一串，
// 认法与权威运输交接认对象同一条（ADR-0214 决定二「包裹号与集运单元号同串时按集运单元判」）。
func (lineage ContainmentLineage) ClosesCycle(member HandlingUnitID) (ContainmentLineage, bool) {
	for index, ancestor := range lineage {
		if ancestor.String() == member.String() {
			return append(ContainmentLineage(nil), lineage[:index+1]...), true
		}
	}
	return nil, false
}
