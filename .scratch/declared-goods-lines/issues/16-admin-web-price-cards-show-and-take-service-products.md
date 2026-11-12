# 16 管理台：价卡录入与查阅带适用服务产品，试算可选产品

Category: enhancement
Status: in-progress
Blocked by: 14（已提交，未进 main；同在分支 `feat/goods-details-admin`）

依据：[ADR-0382](../../../docs/adr/0382-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md) 决定一、二。

## 要做的

- **价卡编辑器**（`card-editor` 的基础一步与预览）：
  - 方向为 `SELL` 时多一格「适用服务产品」，可多选、可不选；候选取参与方与商业的服务产品目录。
  - 不选即方案文档不带 `serviceProducts` 键；方向不是 `SELL` 时这一格不出现、也不发。
  - 说明写「不选即接其余产品：同一计价范围里没有别的卡指名的那些产品」。
  - 编辑器发的是结构化工作簿（ADR-0349），照规格口走：规格的 card 表列了 `serviceProducts`（模板 `PPT-3` 起）才显示这一格，并照「字段列全」总写这一行——方向为 `SELL` 且选了产品写 `"<标识>;<标识>"`，否则写空值；规格没列就不写、也不显示多选。模板版本一律取规格口给的那一版。
- **价卡列表与详情**：
  - `SELL` 卡显示适用产品：列出指名的产品；不声明显示「其余产品（不指名）」。
  - `BUY`、`INTERNAL` 卡不显示这一格。
  - 导入面的内容预览同样显示。
- **试算页**：
  - 多一格可选的服务产品，发 `serviceProduct`。
  - 不选时说明「只按不指名产品的卡试算」。

## 完成判据

`tsc`、`run-tests` 与 `vite build` 三道门通过；表单与显示的纯函数各有单测（带与不带产品、方向切换时丢弃这一格）。

## 不做的

后端（票 14、15）。

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/admin-price-card-products` 上的 `7f6473689` 与评审修订 `098f3a451`（基线 `37c172945`），全部改动在 `apps/admin-web/`，未推送、未进 main。
- 拣入：在 `feat/goods-details-admin` 上是 `fbe6fcb9f` 与评审修订 `89dec0b07`，另有 `bdb192fc9` 把空白模板的下载链接换到票 14 生成的 PPT-3 文件；拣入后管理台三道门通过（`run-tests` 1358 过 0 败）。
- 为什么在分支上做，不照 workflow.md「前端切片」在 `main` 上直接做：本票被票 14 的模板字段卡着（`PPT-3` 的 `card` 表那一格由后端加），而 declared-goods-lines 这一条线是整条一起落的；先单独进 main，编辑器就会对着还不收这一格的后端。
- 契约（几经改钉，以最后一版为准）：
  - 开工时钉的是方案文档 JSON 的 `serviceProducts` 键。
  - 做到一半改钉为工作簿 `card` 表写一行、只在售价卡选了产品时写、写时模板版本取 `PPT-3`；`7f6473689` 照这一版写。
  - 评审后再钉（现行）：`PPT-3` 的 `card` 表总有 `serviceProducts` 一行，null 或空白即不声明，有值只许售价卡；`PPT-1`、`PPT-2` 没有这一行。`098f3a451` 照这一版改。本票「要做的」里写 `PPT-3` 那一句是改钉前的说法，以本条为准。
  - 方案文档的 `serviceProducts`（字符串数组，不声明即缺席，从不发空数组）仍用在页面读方案 JSON 的几处：价卡目录行、导入预览与草稿的内容概要、详情读到的文档。
- 落点：
  - 新文件 `pages/pricing/service-products.ts`：几句话与纯函数——`servedProductsText`（售价卡列出指名产品、不声明念「其余产品（不指名）」、非售价卡答 null 不出这一格）、`toggleServiceProduct`、`addTypedServiceProducts`（手填按英文分号拆、逐个去首尾空白）、`canonicalServiceProducts`（去重排序）、`serviceProductChoices`（册上的在前，已选却不在册上的接在后面照样勾着）。
  - 表单模型：`BasicsState.serviceProducts`；`basicsWithDirection` 把原在 `StepBasics` 里的方向配用途搬成纯函数，方向离开售价即清空已选产品，切回售价不复原。
  - 工作簿（`card-form/workbook.ts`）：字段集与模板版本都照规格口，页面不替服务端定版本。规格口的 `card` 表列了 `serviceProducts` 就照「字段列全」总写这一行：售价卡选了产品写去重排序的分号串，没选或不是售价卡写 null；规格口没列它（`PPT-1`、`PPT-2`）就不写。`specTakesServiceProducts` 判规格口收不收这一格。这一行出的问题标回「适用服务产品」。
  - 编辑器认得的规格版本加上 `PPT-3`，后端规格口升到 `PPT-3` 后编辑器照常打开。
  - 还原与展示：`workbookToForm` 带回产品；展示用文档（`card-form/document.ts`）有产品才带 `serviceProducts` 键；`workbookServiceProducts` 给编辑器预览念适用产品——它就是送去预览、服务端读过的那一份，不靠预览答复回显。
  - 价卡详情解析（`price-card-content.ts`）认得 `serviceProducts` 顶层键，不再当「未识别的内容」降级成一行。
  - 编辑器基本信息一步：售价方向且规格口收这一格时出「适用服务产品」多选，从服务产品册勾，候选复用参与方页已有的 `listServiceProducts` 与 `serviceProductObjectOptions`，没有新加读口；规格口不收时只说「当前模板版本不收适用服务产品……」。读册时只说在读，读不到才退回按标识添加并说明原因。复选框组带 `role="group"` 与指向标签的 `aria-labelledby`（`FieldBox` 加了可选的 `labelId`）。
  - 编辑器预览通过后的概要、价卡列表的适用范围一格下、详情页头、导入面内容概要各多「适用产品」一行，非售价卡不出。
  - 试算页：服务产品一格只在售价方向出现，也只在售价方向发 `serviceProduct`，手填值去首尾空白；不选说明「只按不指名产品的卡试算」，选了说明「先用指名这个产品的卡，没有再用不指名的卡」。
  - `api.ts`：`PriceCardRecord.serviceProducts?`、`PriceCardImportContent.serviceProducts?`、`EstimatePayload.serviceProduct?`。
- 判据：
  - 规格口列了这一格就总写这一行（选了写分号串、没选与非售价卡写 null）、没列就不写，模板版本照规格口：单测（今天的 `PPT-2` 规格与模拟的 `PPT-3` 规格各一遍）✅。
  - 方向切换时丢弃这一格：`basicsWithDirection` 单测（售价→采购、售价→内部、再切回售价不复原，用途配对照旧）✅。
  - 文档映射带与不带产品：有产品文档带 `serviceProducts`，没有即键缺席，非售价卡缺席；解析不把它当未识别的内容 ✅。
  - 错误映射：`card` 表 `serviceProducts` 的 `FIELD_UNKNOWN`、`FIELD_MISSING`、`CELL_INVALID` 标回「适用服务产品」，`TEMPLATE_VERSION_UNSUPPORTED`（表名为空）说不到格子；问题码取自后端 ports 的模板问题码 ✅。
  - 显示：指名、其余、采购三种单测 ✅。手填按分号拆：单测 ✅。
  - 试算载荷：售价方向带与不带产品、手填去空白、非售价方向不发；说明句随之变：单测 ✅。
  - 三道门通过 ✅（见验证）。
- 未做：
  - 浏览器里没走过：这台机器上起不了带后端的页面，事件路径（勾选 → 状态 → 重渲）没有用 `dom-probe` 证。
  - 静态模板文件 `public/templates/price-card-import-template-PPT-2.xlsx` 与下载链接没换：xlsx 由 Go 的 `cmd/parcel-pricing-template` 写出，后端在票 14 加 `PPT-3` 那一格后重新生成，链接跟着换到 `PPT-3`。
  - 编辑器单测用的规格取数 `card-form/testdata/price-card-template-spec-PPT-2.json` 仍是今天那份；测试里按钉住的字段名补出一份模拟的 `PPT-3` 规格。后端规格口落地后重取一份 `PPT-3` 的替换。
  - 导入预览与草稿内容概要里的 `serviceProducts` 假定与目录行同名同形，未经后端证实；后端不回显时，售价卡的概要会念成「其余产品（不指名）」。编辑器预览不受此影响（念自己发的工作簿）。
- 验证（`098f3a451`）：`apps/admin-web` 下 `node node_modules/typescript/bin/tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 1358 过 0 败（基线 1345）；`node_modules/.bin/vite build` 退 0。联调等票 14、15。
- 评审（非作者，2026-11-11）：无阻断；所提意见都在 `098f3a451` 改了——适用服务产品照规格口写、不再自抬 `PPT-3`；规格口列了就总写这一行（null 即不声明）；错误映射单测换成后端真问题码；读册时不摆手填框；手填按分号拆；复选框组加 `role="group"` 与 `aria-labelledby`；试算的服务产品只在售价方向出现与发送、手填去空白；`knownTemplateVersions` 注释不再数别处的内容；本记录补上为何在分支上做。
