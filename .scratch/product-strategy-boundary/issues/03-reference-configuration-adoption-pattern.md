# 03 参考配置的存放、版本与显式采用路径，在注册号类型目录上立样板

Category: enhancement
Status: ready-for-agent
Blocked by: 无
地盘：参考配置的存放目录（本票定）；party-commercial 注册号类型目录的采用路径（`parcel-commercial` CLI 与登记用例）；演示种子里对应一行。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定三、越权风险点 4、5；[ADR-0145](../../../docs/adr/0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 越权风险点 2。

## 做什么

1. 定参考配置的形态：随产品版本发布的存放位置、标识与版本号的写法、校验方式（与登记册同一套领域构造门）。
2. 定采用路径：采用就是一次普通登记，依据格写「参考配置 标识@版本」；没采用照旧答`未配置`。
3. 在注册号类型目录上立样板：CN（统一社会信用代码）与 SG（UEN）两份公开格式作为参考配置；演示租户经采用路径登记它们，替换今天的合成格式条目。
4. 各登记册依据格能否容纳这种引用，列出与本样板不同的册，交各上下文 owner 复核。

## 不做

- 不定首版随附哪些国家与地区、是否带校验位算法——归 PC owner 与用户，本票只把 CN、SG 作样板并在完成记录里标「待定发布范围」。
- 不给任何真实租户登记。

## 完成判据

- 一份参考配置能经采用路径进入演示租户的注册号类型目录，未采用的租户照旧答`未登记`；采用记录的依据指向参考配置版本。
