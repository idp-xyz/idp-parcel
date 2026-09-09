# 23 结算政策表单两条裁决落地：客户相对方从业务参与方册选、六维里的合同镜像成壳引用

Category: enhancement
Status: ready-for-agent——2026-09-09 通道 1 代裁（用户授权自决）票 [15](./15-settlement-policy-form.md) 评审留下的两条判断题，裁决已写进
`docs/domain/party-commercial/CONTEXT.md` 结算方式那条规则末尾（客户相对方 = 业务参与方，不是货主客户账户；六维里的合同版本同时作壳引用交出）；
本票是它的落地。**Blocked by [22](./22-publication-form-private-helpers-lift-to-party-shared-layer.md)**：22 重构 `SettlementPolicyPublicationForm.tsx`
的私有件，本票改同一张表单的两格，等它抬完再改，不相撞。**2026-09-09 21:5x：22 已进 main `4209520b`，阻塞边解除**——本票现在改的是接了共享层之后的
`SettlementPolicyPublicationForm.tsx`（`Field` / `useLoaded` 从 `PublicationFormFields` 导入；`ContractPicker` 仍留在本册）
Blocked by: 22（已进 main）

## 两条裁决与理由

1. **客户相对方引用解析到业务参与方册，不是货主客户账户册。** GLOSSARY「货主客户账户」：「一个货主客户账户可以按责任法人、相对方、方向、币种和结算
   政策拥有多个结算账户」——相对方是账户之下细分结算账户的键，与账户不是同一对象；CONTEXT-MAP「结算账户是法人、相对方、方向、币种和结算政策共同确定的
   金额责任边界；两者不能合并」。相对方是承担结算责任的**法律主体**，那是业务参与方册的对象。今天表单从 `listCustomerAccounts` 取选单是票 15 作者按 seed
   的写法（seed 用账户标识），两读都通所以没锁死；现在锁死。`domain.CounterpartyReference` 仍是未绑定册的 `requiredValue`（存在性不由构造门查，
   与其它开放引用同），本票只改**表单选单的来源与读面的显法**，不给领域加存在性校验。
2. **六维里的合同版本同时镜像成版本壳的指名引用 `references.CUSTOMER_CONTRACT`。** 没有它，结算政策在被引合同未发布时照样发布，失去「被引合同
   未发布 → `发布未决`」那道排序门；seed 的结算政策壳本来就带着它，`CustomerContractPublicationForm` 也有 `alsoPaths=['references.ACCEPTANCE_RULE_PACKAGE',…]`
   的先例。表单不给操作者第二个输入格——**从六维里选出的合同自动写进壳引用**（同一个选择、两个落点），载荷层若两处都在场且不同答问题、点名
   `references.CUSTOMER_CONTRACT`。

## 完成判据

1. `SettlementPolicyPublicationForm.tsx`：相对方选单 `load` 换成 `listBusinessParties`（`GET /commercial-business-parties`），选项显示业务参与方的名称 +
   标识、值为参与方标识；目录 403 / 读不到仍退回手填格（既有行为）；不预选。
2. `settlement-policy-form.ts`：载荷生成把六维 `contract{objectId, version}` 的 `objectId` 同时写进壳 `references.CUSTOMER_CONTRACT`；node:test 钉「选了合同 →
   壳引用同值」「没选合同 → 壳无该键」（不送空串）。
3. Go 载荷层 `SettlementPolicyBodyPayload.body`（或壳解码处，作者定、写理由）：`references.CUSTOMER_CONTRACT` 在场且与六维 `contract.objectId` 不同 →
   问题路径 `references.CUSTOMER_CONTRACT`；缺席不补（旧载荷 / 受控批文照发）。传输面测试各一正一反。
4. 读面：第 N 册结算政策的「相对方」列显法不变（仍显引用串）；若 `presentation.ts` 有按册解析名称的机制则接业务参与方册，没有就不加（写进完成记录）。
5. demo seed `publish-batch.json` 的结算政策项：相对方若填的是客户账户标识，改成对应业务参与方标识（SYN 数据内自洽即可）；壳引用已带着，不动；
   `contentDigest` 若因正文改动而变，重算 PCC-1 并写明算法出处（同 15 / 13 的补笔做法）。
6. tsc / run-tests 绿；Go：PC 四包 + `cmd/parcel-api` + `internal/architecture`，**并带 DSN 跑 `./cmd/parcel-commercial/`**（同形票的教训，见
   parallel-sessions「验证」节 2026-09-09 补的那句）。

## 边界

不动 0011 与结算政策领域构造门；不给相对方加存在性校验；不动 settlement-accounting 的读路径；不改 `PublicationDraftFlow`。
