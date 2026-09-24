# 从外来原型反看本仓：运营工作台层的四个缺口

Category: enhancement
Status: in-progress
出处：用户 2026-09-25 经 IDP 队列通道 3 连问「`IDP_Parcel_Commercial_Party_UX_V1.0_Full_Package` 对我们有借鉴意义吗 → 你直接跑起来看看 →
你的看法 → 反过来看，我们的实现有巨大缺陷吗 → 按你的理解和建议，你自己独立完成吧」。本规格是那几轮评估的落笔，票按缺口拆。

评估对象在仓外（本机 `/home/tops/workspace/IDP_Parcel_Commercial_Party_UX_V1.0_Full_Package`），不入仓：一份 AI 建站工具生成的可点击前端原型，
演示数据存浏览器 sessionStorage、没有后端，底座是它自己的「业务交互原型 V3」（价卡、分区、附加费、注入方案、发货、交接、追踪、售后、对账），
这一版在上面加「商业合作方」工作台。仓内同名的 [`docs/reference/IDP_Parcel_Commercial_Party_UX_V1.0.md`](../../docs/reference/IDP_Parcel_Commercial_Party_UX_V1.0.md)
是另一份东西（文字方案，主张按票绑定发件 / 出口 / 付款等角色，已判整体不采纳）；这份代码原型反而保守，只把客户账户、采购账户、合同、价卡挂到
一个主体下查看，那正是本仓 `业务参与方` + `参与方关系` 已有的模型。

## 评估结论（原型本身）

**值得借的，都在界面层**：按对手方组织的详情（一份档案连到每一种合作关系）；待办提醒直达处理界面；登记时的疑似重复提示（只提示、不拦、
同名不自动合并）；评价解释的排法（计费起点 → 目的地、命中分区、依据、逐项费用与规则版本并排）。

**不借的，与现行 CONTEXT 冲突**：角色是贴在主体上的永久标签、没有范围依据有效期，且按名称推断（名称等于某值即加「承运商」）——参与方关系
是时态事实、不得从名称推断；「月结 / 预付」是客户账户上的全局字段——预付或账期是结算政策在明确范围内的属性；合作方页面从订单现算可用授信——
信用暴露归 `settlement-accounting`；编辑原地覆盖——身份登记按修订版本化、不可覆盖；提醒阈值写死（合同 30 天、余额低于 100）——红线。
计价、分区、附加费规则与它自拟预期值的 76 条检查：本仓 `parcel-pricing` CONTEXT 已覆盖且更严，最多记 `S`，不借。

## 反看本仓：模型与机制不缺，缺的是「人怎么用」这一层

取证钉 main `71e41e6d`。管理台登记的页面全部对真端点（`page-registry.tsx` 的 `liveIds`），模型比原型严得多，演示动线停在委托是缺租户取值
时的诚实结果——这三样不是缺陷。缺的是运营工作台层：

1. **查阅按登记册切开，没有按对象的全景。** 参与方详情抽屉只有身份字段与修订历史；它是谁的客户、有哪些客户账户、是不是本集团法人、签了
   哪些供应商协议、适用哪些结算政策，要在几个页面之间自己拼。委托详情同理（只有委托自身的标识、版本与时间）。
2. **首发范围内的试算没有入口。** `parcel-pricing` CONTEXT「评价对象为试算对象时……产品已确认需要多供应商比价择优，因此试算能力进入首发」；
   领域里有 `SubjectEstimate` 与 `NewEstimateSubject`（`internal/parcelpricing/domain/input.go`），且已被小包托运的接受前估价在进程内使用
   （`internal/parcelshipment/adapters/parcelpricing/estimation_amount.go`）；但没有面向运营的独立入口——`internal/*/adapters/http` 与 `cmd/parcel-api`
   无试算端点，管理台无试算页（`estimate` 在管理台只命中结算的「预估」状态词）。客户问「1 公斤到某邮编多少钱」，产品今天答不了。
3. **开户没有就绪反馈。** 给一个货主开户要走参与方、客户账户、客户合同、结算政策、信用政策、接单规则包、价格绑定等若干册；漏了哪一册，
   要等委托被拒才知道。`UC-PC-002` 的解析本来就会明确答出无适用依据 / 未配置 / 适用冲突，缺一个「不落账、只看答案」的入口。
4. **价卡录入仍是粘 JSON。** [ADR-0101](../../docs/adr/0101-operator-facing-registration-payload-shape-is-product-defined.md) 决定二至五已定
   「产品发布的版本化导入模板 → parcel-pricing 持久化草稿 → 批准 → 发布（交既有 `RegisterPriceCard`）」，Consequences 列了草稿册、导入 /
   批准 / 发布用例、端点与管理台「导入价卡」「草稿」两签；代码里 `internal/parcelpricing` 无草稿、`.scratch/` 无对应实施票，价卡页的「登记价卡」
   签仍是 `RegistrationPanel` 粘登记快照 JSON。

另记一格、本批不立票：身份登记表单要操作者自编参与方标识、自填修订号（有建议值）。标识该不该由系统签发属产品取舍，ADR-0101 决定八已放各册
自裁表单形态，留作下一轮评估输入。

## 子票

| 号 | 题 | 形态 | 阻塞边 | 状态 |
|---|---|---|---|---|
| [01](./issues/01-party-overview-read-only-aggregate.md) | 参与方全景（只读聚合） | 前端切片，按 workflow「前端切片」在 `main` 上做 | — | resolved |
| [02](./issues/02-estimate-evaluation-entry.md) | 试算入口：定用例与取舍 | 设计票：落成 ADR-0152 与 UC-PP-001 | — | resolved |
| [05](./issues/05-estimate-endpoint-backend.md) | 试算后端：编排、补齐读数共用件与 `POST /pricing-estimates` | 走并行会话那条路 | — | ready-for-agent |
| [06](./issues/06-estimate-admin-page.md) | 管理台试算页 | 前端切片 | 05 | draft |
| [03](./issues/03-customer-account-onboarding-readiness.md) | 开户就绪清单 | 先建模：按客户账户做一次不落账的解析 | — | draft |
| [04](./issues/04-price-card-template-import-per-adr-0101.md) | 落地 ADR-0101 价卡导入 | 后端 + 前端，ADR 已接受，需先拆票 | — | draft |

## 不做

- 不改任何 `CONTEXT.md` 或 ADR，不引入原型的领域模型、数字或视觉风格。
- 全景不接跨上下文信号（授信、余额、订单）：那要 `settlement-accounting`、`parcel-shipment` 按参与方的读口，归各自上下文立票。
- 原型不入仓；评估截图只留在本机，不作任何验收依据。
