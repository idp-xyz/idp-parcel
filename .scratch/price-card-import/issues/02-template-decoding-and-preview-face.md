# 02 模板解码与预览口

Category: enhancement
Status: resolved——2026-09-25 通道 3；完成记录见文末
Blocked by: 01
地盘：
- 01 定下的读取件位置；
- `internal/parcelpricing/`：模板翻译适配器、预览编排及其 HTTP 端点；
- `cmd/parcel-api/`：装配与端点表一行；
- `docs/product/MECHANISM-INVENTORY.md` 重生成。
出处：[spec](../spec.md)；ADR-0101 决定三、四；ADR-0126 决定四（预览与录入同一段解码）。

## 要做的

1. 按 01 的规范把上传的文件解成领域值：逐格走构造门，**问题收齐再答**，不撞第一格就停。
2. 预览编排：先按原始字节算源文件身份（名与 SHA-256），再解码、立方案、算规范化摘要。
   - 答复分格：已校验（带摘要与方案概要）、有问题（带逐格问题）、未受理、未决。
   - 不写库。
3. 端点 `POST /pricing-price-card-previews`，挂字面量 `UnconfiguredIntake{}`。
   - 租户取自信封，载荷里没有身份格。
   - 隔离读放行装不进它（同 PP 序列预览口）。
4. 03 的录入口要复用同一段解码。本票就把解码写成录入口也能直接调的形状，不在 HTTP 层各写一遍。

## 验收

- 用 01 的示例模板（合成 `SYN-` 值）走通：合格文件答已校验，摘要与把同一方案经领域构造算出的逐字节相等。
- 坏文件的每类问题各有测试：
  - 缺表、缺列、多余列；
  - 格式错的金额；
  - 浮点尾数的数值格；
  - 公式格；
  - 构造门拒收的区间或币种。
- 端点表测试含新行；机制清点重生成。

## 形态

碰 Go 与端点表，走并行会话那条路。

## 完成记录（2026-09-25，通道 3）

进 main 的提交（在 `db6ddee8` 之上重放，分支 `mcp3-pci02` 作封存出处）：
- `88c3e3bd` 读写件 `internal/platform/spreadsheet`；
- `1ed877a5` 模板读口 `ports.PriceCardTemplateReader`、实现 `adapters/pricecardtemplate`、生成命令 `cmd/parcel-pricing-template` 与入库模板 `apps/admin-web/public/templates/price-card-import-template-PPT-1.xlsx`；
- `9c8cabdc` 预览编排 `PreviewPriceCardImportHandler`、端点 `POST /pricing-price-card-previews`、上传解码 `DecodePriceCardUpload` 与装配；
- `aed95f82` 预览口改挂操作者渠道的登记册 Intake（见下「超出票面的」）；
- `3993c998` 机制清点在重放 tip 上重生成。

验收逐条：
- **合成样例走通。** 两张种子卡（`SYN-PLAN-CN-SG-COST-01`、`SYN-PLAN-CN-SG-01`）经模板导入答已校验，内容摘要与入库种子快照经登记门重建出的逐字节相等。另有一张用到模板每一张表的卡，与领域构造函数直接立出的同一张方案同摘要。两处断言各做过一次变异（改一个金额），确认会红。
- **坏文件各类问题有测试**，逐条断言表、行、列坐标与问题码：
  - 整份不收四格，各只答一条；
  - 缺表、未知表、缺列、未知列、缺字段；
  - 格式错的金额、不带时区的时刻、未知代码、重复的列表元素、三格缺一；
  - 浮点尾数的数值格、公式格、错误值、日期类型的格；
  - 构造门拒收的区间重叠、方向与目的不配；
  - 引用解析不到、指到非根、声明了没人用，条件树与计算树的各种坏形；
  - 问题排序稳定。
- **端点表测试含新行**；机制清点重生成。

超出票面的，照实记：
- **预览口没有先挂 `UnconfiguredIntake{}`**，直接挂了 operator-channel/04 落地的 `OperatorRegistryIntake`。本批 spec 自决第 4 格原写「先挂未配置、换真归 operator-channel/04」；重放时发现通道 4 的第四批已把计价登记册 Intake 落地，并在归类表写明「价卡的在线导入属 price-card-import 那一批」，换真的责任因此落在本批，spec 已加更正。预览因此真能用：带有效令牌且持登记册配置写的授予即可调用。
- **入库模板放在管理台静态资源目录**，由 `cmd/parcel-pricing-template` 生成、同包测试逐字节防漂（设计文档第七节），地盘越出了票面所列。
- 设计文档随手更正两处：封闭代码与领域逐字相同（工件种类是小写连字符）；写明模板入库目录。

验证：
- 推送前在代码 tip `3993c998` 上带 `IDP_PARCEL_POSTGRES_DSN` 跑全量 `go test ./... -p 1 -count=1`：退出 0，132 个包 `ok`，没有 FAIL。其后一笔只改本票面与 spec。
- **跨工具**：用 openpyxl 打开入库的空白模板，填采购成本卡另存，金额与重量故意混用数值格。读口答已校验，摘要等于种子。写出件的产物也经 openpyxl 独立读过：文本格式、加粗列头、冻结首行都对。
- 没有覆盖到的：Excel 与 WPS 本体没有在本机实测，本机没有这两个软件。浮点尾数的拒收只由手写字面量的单测覆盖，openpyxl 写数值用最短往返表示，造不出那种字面量。

给 03 的交接：录入口复用同一个 `ports.PriceCardTemplateReader`、应用层同包的 `sourceFileOf`，以及 HTTP 层的 `DecodePriceCardUpload`。操作者渠道照 `IntakePriceCardPreview` 的写法在 `OperatorRegistryIntake` 上加录入口的方法。录入口要的录入者，取认证出的 `OperatorIdentity.Operator`。

评审：作者自审，没有非作者评审。
