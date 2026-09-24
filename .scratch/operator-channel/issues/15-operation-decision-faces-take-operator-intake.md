# 15 委托与履约的运营决定口：操作者渠道增「运营决定」能力面，委托侧五口与 TF 四个管理台写面换操作者 Intake

Category: enhancement
Status: ready-for-agent——2026-09-24 随 ADR-0151 立（用户同日「同意你的决定，开干」）；2026-09-25 通道 2 按用户「开干前，全面审查，确保确实如此」复核后改定范围、阻塞与完成判据，复核记录见文末 Comments
Blocked by: 03、14
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`internal/accessidentity`（能力面授予格「运营决定」）；`cmd/parcel-api` 端点表里下列各口的 Intake 装配与装配测试；`internal/parcelshipment/adapters/http` 与 `internal/transportfulfillment/adapters/http` 各口的操作者 Intake。
出处：[ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 决定一、二、三、六；[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二、四；[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四。

## 做什么

1. 操作者册的授予多一个能力面「运营决定」，授予按租户 × 决定种类登记：复核完成、主动拒绝、授权处置、受控关闭、受控重开、关段、建派送任务、装载分配、终止参与。
2. 下列各口在装配点换成操作者 Intake，每换一口，该口的「未配置即拒」测试改写为答复格测试：
   - 委托侧：`/shipment-requests/manual-review-completions`、`/shipment-requests/rejections`、`/shipment-requests/authorized-dispositions`、`/shipment-requests/continued-attempt-closures`、`/shipment-requests/continued-attempt-reopenings`；
   - TF：`/transport-fulfillment-segment-closures`、`/transport-fulfillment-dispatch-task-registrations`、`/transport-fulfillment-load-assignment-registrations`、`/transport-fulfillment-participation-terminations`。
   关段与建派送任务两口今天经写开关放行，换口的同一笔撤下隔离放行（ADR-0150）。
3. 身份格照 ADR-0151 决定二逐口取：
   - 租户与提交操作者只从信封来；
   - 复核人、拒绝决定人、处置人取提交操作者，有没有权仍由编排问 party-commercial；
   - 受控关闭与重开的命令里没有提交操作者的格，决定方、授权角色与授权依据快照由授权答复给出；请求方与重开时的货主账户在操作者渠道上怎么形成，先与 PS owner 对齐，定之前 Intake 不收这两格。
4. 委托寻址：Intake 按信封的租户与请求指名的委托（或包裹）在服务端查出委托的来源身份（租户、客户账户、来源、来源请求键），不从请求体收；查不到与越权探针同答。
5. ADR-0149 决定四两条（ADR-0151 决定三）：
   - 铸信封前接 14 的准入范围判断，不覆盖答「不在准入范围」；
   - 逐口核载荷规范化——领域已有同身份重放与内容冲突代数的口只核对，逐口记下凭哪条代数；没有的补一版带形状版本的规范化形状。未核完的口不开真渠道。

## 不做

- 客户侧各口（ADR-0139，归用户）；主链事实的更正口（10、11）。
- 角色模型与授权请求坐标的推导（[psb/07](../../product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)）；party-commercial 授权动作词表里的「授权处置」一格（ADR-0132 越权风险点 2；此前无票，2026-09-25 记到 psb/07 第 2 项下）。
- TF 决定要不要记提交操作者作操作证据（归 TF owner，ADR-0151 越权风险点 5）。

## 完成判据

- 上列各口：发行方未配置 / 令牌无效 / 无此决定种类的授予 / 不在准入范围四格各答其格，带测试；装配测试与端点表仍一一对照。
- 租户、提交操作者与委托来源身份都不从请求体收，请求体带即 `400`；复核人、拒绝决定人、处置人等于提交操作者；受控关闭与重开的命令里不出现提交操作者（带测试）。
- 越过 Intake 之后：TF 各口答业务结果；委托侧五口到达编排，在各自的授权缺口补上之前，生产装配如实停在该口今天的格——复核完成 `NO_ANSWER_FORMED`，主动拒绝「拒绝授权不可用」，受控关闭与重开「未决 · 授权口不可用」，授权处置「授权规则未配置」；装配测试以合成的授权映射或授权器证已授权时答业务结果（同 `manualReviewOrchestrationWith` 的做法）。
- 关段与建派送任务两口的隔离放行已撤，其隔离用例改写为真渠道答复格用例。

## Comments

### 复核记录（通道 2，2026-09-25）

用户令「开干前，全面审查，确保确实如此」。通道 4 起草的 ADR-0151 与本票在 23:22 落盘后会话中断，未提交；本记录逐条对照代码与文档复核草稿。

**属实，照旧：**
- 委托侧五口挂字面量 `UnconfiguredIntake{}`、没有隔离放行，也没有任何票接住（08 归类表记「未归」）。
- ADR-0100 决定一已把 `PAR-INT-01` 收回到客户生产委托接入渠道，这几口注释里的「属 `PAR-INT-01`（实例半边）」自 2026-09-03 起就过期了。
- 这几口的操作者是运营企业自己的人，不是货主客户；客户侧的撤回、取消、补充、资料修订等的去处等的是用户对 ADR-0139 的决定，不是租户证据。

**改了的：**
1. **实际决定方从哪来**。草稿写「受控关闭与重开、主动拒绝、授权处置的实际决定方由命令声明、由商业授权核定」，五口没有一口对得上：
   - 拒绝与处置的适配器写「采信自报的决定人 / 处置人等于让任何调用方替任何角色拒单 / 决定去向……Intake 只交出『谁在请求』」；
   - 受控关闭命令写「决定方、授权角色、授权依据快照不在命令里，由授权答复给出」。
   照草稿做就是从请求体采信决定人。改为 ADR-0151 决定二。
2. **TF 四个管理台写面被拆进两族**。关段与建派送任务早由 ADR-0149 决定一列进作业事实登记（要设备），草稿却以「管理台判断、没有设备」把同组的装载分配与终止参与另归新能力面；票 tf-segment-lifecycle-closure/07 把四口一并定为「运营写决定」。改为四口整组归「运营决定」，部分停用 ADR-0149 决定一两项（ADR-0151 决定六）。
3. **ADR-0055 决定五两项**。草稿没说载荷规范化与准入范围对运营决定口适不适用；运营决定形成生产事实，ADR-0100 豁免登记写面的理由对它不成立。补 ADR-0151 决定三，本票 Blocked by 加 14。
4. **委托寻址**。五口命令都带委托来源身份，操作者信封里没有；草稿只列为风险点，改为本票「做什么」第 4 条。
5. **生产上走不到业务结果**。复核、拒绝、受控关闭与重开的授权请求映射在生产装配里是 nil（psb/07 第 1 项），授权处置装的是未配置授权器（PC 授权动作词表缺这一格，ADR-0132 越权风险点 2）；草稿的完成判据「七口答业务结果」在生产装配上不成立。改为逐口如实停在今天的格、装配测试以合成授权证已授权路径。
6. **清扫票 16 不存在**。ADR 与 psb/15 引了「同日做完」的 16，而全仓还有几十处过期注释（含本票五口自己的 Intake 注释）；本次补立并做完。
