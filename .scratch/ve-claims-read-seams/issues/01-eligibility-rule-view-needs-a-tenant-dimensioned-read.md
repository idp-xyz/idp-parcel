# VE 资格规则视图缺带租户维的读法——真适配器是单租户装配形状，多租户入口接不上

Category: enhancement
Status: ready-for-agent

自[接线票 04](../../parcel-api-remaining-endpoint-wiring/issues/04-ve-claims-wiring.md)
Comments 里的既知事实提出立票（该票 resolved 时如实记了成因但没有后续票承载）；
[第二十六轮重盘](../../mechanism-reinventory-r26/report.md)第三节点名为端口计数看不见的
机制余量之一。

## 事实

- 登记面本身已存在（claim_contract_scope/授权目录，经 `parcel-ve-register` 可登），真适配器
  `vepostgres.ClaimEligibilityRules` 也在——判据 A/B 都记「已实现」，端口盘点看不见这条缝。
- 接不上是因为该适配器把**租户钉在装配期**（为受控登记口而设），而 `parcel-api` 是多租户
  入口、今天没有租户可钉。钉空租户不行：视图会以「声明不在场」的业务答案顶「没接」，把恢复
  动作指向登记参数；租户真登记后答案也不变——接错看着像接对。
- 现状以报错桩顶位（`cmd/parcel-api/assemble_claims.go` 的 `unconfiguredEligibilityRules`，
  按端口合同「依赖调不通作为错误返回」如实报错，编排停在 `ELIGIBILITY_RULES_UNAVAILABLE`
  指名到缝的未决）。

## 要做什么

给这条缝一个带租户维的读法：查询携带租户的多租户读适配器（租户从查询来、不从构造期来），
`parcel-api` 换掉报错桩接真。单租户形状的既有适配器为受控登记口而设，保留不动——两个形状
各答各的调用面，不合并（合并会让登记口拿到跨租户读）。

## 完成标准

- `parcel-api` 的资格缝接真后：未登记租户答「声明不在场」的业务格（found=false 一族），
  不再是报错桩的技术未决；已登记（含合成 `SYN-` 种子）租户按册答。
- 装配测试对真库钉住两态；全仓真库套件绿。
- 接线票 04 记的「资格未决指名到缝」断言随之更新（那格从 UNAVAILABLE 换成按册答案）。
