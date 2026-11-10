echo "== 合作关系网演示（party-commercial：我方各家公司各挂客户、代理、供应商与转售；证据 S） =="
# 票 business-party-relationship-graph/03：给往来单位「关联」签的关系图与「我方关联」一册铺数据。排在 800 之后——关系的
# 相对方要用到 800 登记的我方公司。运营方一号与上海名下的客户挂到超过两处关系图的合成门槛（party-relationship-graph.ts 的
# relationshipGraphRoleLimit、OurCompanyNetworkPanel.tsx 的 networkLimits），好让合成节点在演示里看得见；深圳对末端承运方
# 持有渠道账号，图右列也有一段；香港、上海各有一段待确认（不带批准）。
"$BIN/parcel-commercial" register-parties -input "$SEEDS/commercial/register-relationship-network-demo.json"
