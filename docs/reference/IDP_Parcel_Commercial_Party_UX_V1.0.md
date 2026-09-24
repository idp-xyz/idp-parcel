可以。**IDP Parcel 的 Commercial Party 不应该做成“客户/地址簿管理”这么简单，而应该升级为整个 Parcel 平台的统一交易主体中心（Party Master / Party Hub）**。

这也是后续 Shipment、Quote、Rate、Customs、Commercial Invoice、Billing、Pickup、Return、Track & Trace 都会依赖的基础模块。

DHL 目前的 MyDHL API 已经直接支持 `Shipper / Pickup / BookingRequestor / Buyer / Recipient / Exporter / Importer / Seller / Payer / Manufacturer / UltimateConsignee / Broker` 等多个主体角色，而且 Registration Number 会根据**国家 + 主体角色 + 号码类型**变化。IDP Parcel 因此不能把 Party 写死成几个固定客户类型，而要做成 **Party Master + Dynamic Role Binding**。([DHL API Developer Portal][1])

---

# IDP Parcel — Commercial Party Management UX/UI V1.0

## 一、先确定产品模型

我建议系统内部统一叫：

> **Commercial Parties**

中文：

> **商业主体**

菜单位置：

```text
Master Data
├─ Commercial Parties
├─ Products / Commodities
├─ Locations
├─ Carrier Accounts
├─ Service Mapping
├─ Zones
├─ Rate Cards
└─ Surcharges
```

Commercial Party 不等于 Customer。

一个 Party 可以同时是：

```text
ACME Trading Ltd.

✓ Shipper
✓ Exporter
✓ Seller
✓ Payer

某一次 Shipment：
Shipper = ACME Trading Ltd.
Exporter = ACME Trading Ltd.
Seller = ACME Trading Ltd.

另一次 Shipment：
Shipper = Shanghai Warehouse
Seller = ACME Trading Ltd.
Exporter = ACME Trading Ltd.
Payer = IDP Logistics
```

所以：

> **Party 是主体，Role 是主体在具体业务交易中的身份。**

这是整个 UX 的核心。

---

# 二、完整页面体系

Commercial Party 模块至少需要下面这些页面：

```text
Commercial Parties
│
├─ 01 Party List
├─ 02 Create Party
├─ 03 Party Overview
├─ 04 Addresses
├─ 05 Contacts
├─ 06 Roles
├─ 07 Tax & Registration
├─ 08 Billing & Payment
├─ 09 Carrier Accounts
├─ 10 Relationships
├─ 11 Compliance & Documents
├─ 12 Shipment Usage
├─ 13 Audit History
├─ 14 Duplicate Detection
├─ 15 Merge Party
├─ 16 Bulk Import
└─ 17 Party Settings
```

这才是我认为真正完整的 Commercial Party。

---

# 三、01｜Commercial Party List

这是主入口。

页面：

```text
Commercial Parties                              + Add Party

Manage organizations and individuals used across
shipments, customs, billing and carrier operations.

[ Search parties...                       ]  Filters  Import  Export
```

顶部不是堆很多没意义的 KPI，而放 4 个真正有业务价值的状态：

```text
All Parties             1,284
Active                   1,217
Needs Attention             18
Expiring Registrations      11
```

### 主表格

| Party            | Party ID  | Roles                     | Country | Primary Address | Registration | Status | Last Used |
| ---------------- | --------- | ------------------------- | ------- | --------------- | ------------ | ------ | --------- |
| ACME Trading Ltd | PTY-10283 | Seller · Exporter · Payer | CN      | Shanghai        | VAT · USCC   | Active | 2h ago    |
| Amazon EU Sarl   | PTY-10892 | Buyer · Importer          | LU      | Luxembourg      | VAT · EORI   | Active | Yesterday |
| John Smith       | PTY-11982 | Recipient                 | US      | California      | —            | Active | Sep 14    |

这里 **Roles 使用 Tag**：

```text
Seller
Exporter
Payer
+2
```

不建议全部展开。

---

# 四、Filter UX

点击 Filters 后从右侧弹出：

```text
Filters

Party Type
○ Organization
○ Individual

Status
☑ Active
□ Draft
□ Suspended
□ Inactive

Country / Region
[ Search country ]

Roles
□ Shipper
□ Recipient
□ Seller
□ Buyer
□ Exporter
□ Importer
□ Payer
□ Manufacturer
□ Broker
...

Registration
□ VAT
□ EORI
□ IOSS
□ GST
...

Validation
□ Address issue
□ Missing tax ID
□ Expiring registration
□ Compliance review

[Reset]                         [Apply]
```

必须支持：

```text
Saved Views
```

例如：

```text
EU Importers
US Consignees
China Exporters
Missing Tax IDs
Compliance Issues
```

---

# 五、Party Quick View

点击 Party 不立即跳页面。

先右侧 Drawer：

```text
┌──────────────────────────────────┐
│ ACME Trading Ltd.        Active  │
│ PTY-10283                        │
│                                  │
│ Shanghai, China                  │
│                                  │
│ Seller   Exporter   Payer        │
│                                  │
│ PROFILE COMPLETENESS             │
│ █████████████████░ 92%           │
│                                  │
│ Primary Contact                  │
│ Jack Zhang                       │
│ jack@acme.com                    │
│ +86 21 xxxx xxxx                 │
│                                  │
│ Registrations                    │
│ CN Unified Social Credit ✓       │
│ VAT                        ✓      │
│                                  │
│ Last Used                        │
│ SHP-260916-1289 · 2h ago         │
│                                  │
│ [Open Party]                     │
│ [Use in Shipment]                │
└──────────────────────────────────┘
```

这个 UX 会比“点击一行直接进入详情页”舒服很多。

---

# 六、02｜Add Commercial Party

不要做一个 60 个字段的长表单。

采用：

## Step Wizard

```text
1 Identity
2 Roles
3 Addresses
4 Contacts
5 Tax & Registration
6 Billing
7 Review
```

同时允许：

> Save Draft

---

# 七、Step 1 — Identity

```text
Create Commercial Party

● Identity
○ Roles
○ Addresses
○ Contacts
○ Registration
○ Billing
○ Review
```

字段：

```text
Party Type *

[ Organization ▼ ]

Legal Name *
[ ACME Trading Limited                    ]

Display Name
[ ACME Trading                            ]

Party ID
[ Auto: PTY-012483                        ]

Country of Registration *
[ China ▼ ]

Company Registration Number
[ 9131XXXXXXXXXX ]

Business Type
[ Corporation ▼ ]

Preferred Language
[ English ▼ ]

Default Currency
[ USD ▼ ]

Timezone
[ Asia/Shanghai ▼ ]

External Reference
[ ERP-CUS-92831 ]

Tags
[ VIP ] [ Marketplace ] [+]
```

### UX重点

输入：

```text
ACME Trading
```

系统即时做 Duplicate Detection：

```text
Possible duplicates found

ACME Trading Ltd.
Shanghai, China
92% match

[View] [Use existing]
```

避免：

```text
ACME
ACME LTD
ACME Trading
ACME Trading Limited
```

变成四个主体。

---

# 八、Step 2 — Roles

页面名称不要写：

> Party Type

而应该写：

> **Available Business Roles**

```text
Select how this party may participate in logistics transactions.

Shipping
☑ Shipper
□ Pickup Party
□ Recipient / Consignee
□ Return Party

Commercial
☑ Seller
□ Buyer

Customs
☑ Exporter
□ Importer
□ Manufacturer
□ Ultimate Consignee
□ Broker

Financial
☑ Payer
□ Bill-To
```

注意：

### 这里定义的是

> **Allowed / Available Role**

而不是说这个主体永远就是这个身份。

真正角色在 Shipment 创建时动态绑定。

DHL 当前接口也是围绕多个独立 Party Role 构造 shipment，并且这些角色使用相似的主体信息结构。([DHL API Developer Portal][1])

---

# 九、Step 3 — Addresses

一个 Party 必须支持多个地址。

绝对不能：

```text
party.address
```

一个字段搞定。

应该：

```text
Party
 └── Addresses[]
```

UI：

```text
Addresses

┌─────────────────────────────────────┐
│ ★ Shanghai HQ                       │
│                                     │
│ Registered · Billing                │
│ 1288 XX Road                        │
│ Shanghai 200000                     │
│ China                               │
│                                     │
│ Address verified ✓                  │
│                                     │
│ Edit            ⋮                   │
└─────────────────────────────────────┘

┌─────────────────────────────────────┐
│ Shenzhen Warehouse                  │
│                                     │
│ Pickup · Shipping · Return          │
│ ...                                 │
└─────────────────────────────────────┘

+ Add Address
```

---

# 十、Add Address

地址用途：

```text
Address Usage

□ Registered
□ Shipping
□ Pickup
□ Delivery
□ Billing
□ Customs
□ Return
```

地址：

```text
Address Name
Shanghai Warehouse

Country / Region *
China

Address Line 1 *
...

Address Line 2
...

City *
Shanghai

State / Province *
Shanghai

Postal Code *
200000

Residential
○ No
○ Yes

Default Address
☑ Make primary address
```

然后自动：

```text
Address Validation

✓ Country recognized
✓ Postal code valid
✓ City/postcode match

Suggested address:
...

[Use Suggested]
```

---

# 十一、Addresses 必须有 Validation 状态

状态体系：

```text
Verified
Validated
Warning
Invalid
Not Validated
```

不能只有：

```text
✓
```

否则运营根本不知道：

> 谁验证的？

因此 hover：

```text
Validated

Provider:
Google Address / Carrier / Manual

Validated:
2026-09-16 14:31

Confidence:
High
```

---

# 十二、Step 4 — Contacts

不能把：

```text
name
phone
email
```

直接塞 Party 表。

必须多联系人。

```text
Contacts

PRIMARY
Jack Zhang
Logistics Manager

jack@acme.com
+86 138 xxxx xxxx

Shipment Notification ✓
Customs Contact ✓

────────────────

Mary Chen
Finance Manager

mary@acme.com

Billing Contact ✓

+ Add Contact
```

联系人职责：

```text
□ Primary Contact
□ Shipping
□ Customs
□ Billing
□ Pickup
□ Return
□ Compliance
□ Notification
```

---

# 十三、Step 5 — Tax & Registration

这是 Commercial Party 最重要页面之一。

我建议名称：

> **Tax & Regulatory IDs**

而不是 Tax ID。

因为跨境实际涉及：

```text
VAT
EORI
IOSS
GST
ABN
EIN
CNPJ
CPF
USCC
RFC
OSS
Foreign Importer Identifier
Broker License
Food Safety Registration
...
```

DHL 甚至在 2026 年仍持续新增 Registration Number Type，例如 Foreign Importer Identifier、Broker License，以及 2026 年 9 月加入马来西亚 Food Safety Registration。这说明 **Registration Type 绝不能写死在前端**，必须由 Reference Data / Rule Engine 驱动。([DHL API Developer Portal][2])

---

# 十四、Registration UI

```text
Tax & Regulatory IDs                           + Add ID

China Unified Social Credit Code
913100XXXXXXXXXX

Country
China

Applicable Roles
Exporter · Seller

Status
✓ Verified

────────────────────────────────────

EU VAT
DE123456789

Country
Germany

Applicable Roles
Importer · Buyer

Valid From
01 Jan 2025

Valid Until
—

Status
✓ Verified
```

新增时：

```text
Registration Type *
[ EORI ▼ ]

Issuing Country *
[ Germany ▼ ]

Registration Number *
[ DE123456789012345 ]

Applicable Roles
☑ Importer
☑ Exporter

Valid From
[ ]

Valid Until
[ ]

Verification Source
[ Manual ▼ ]

Notes
[ ]
```

---

# 十五、最重要的 UX：Country-aware

如果：

```text
Country = Germany
Role = Importer
```

UI 自动推荐：

```text
Recommended IDs

EORI
VAT
```

如果：

```text
Country = Brazil
```

显示该市场相关 registration requirements。

而不是给用户一个：

```text
Registration Type
↓
186 种类型
```

自己慢慢找。

---

# 十六、Party Detail Page

这是最终核心页面。

Header：

```text
← Commercial Parties

ACME Trading Ltd.                         Active ●

PTY-10283

Shanghai, China

Seller   Exporter   Shipper   Payer

[Create Shipment]  [Edit]  [⋯]
```

下面：

```text
Overview
Addresses
Contacts
Roles
Tax & IDs
Billing
Carrier Accounts
Relationships
Compliance
Usage
Audit
```

---

# 十七、Overview

我建议做成 Bento Dashboard：

```text
┌──────────────────────┐ ┌──────────────────────┐
│ PARTY INFORMATION    │ │ PROFILE HEALTH       │
│                      │ │                      │
│ Organization         │ │ 92%                  │
│ China                │ │ ██████████████░      │
│ PTY-10283            │ │                      │
│                      │ │ 2 items need action  │
└──────────────────────┘ └──────────────────────┘

┌──────────────────────┐ ┌──────────────────────┐
│ PRIMARY ADDRESS      │ │ PRIMARY CONTACT      │
│                      │ │                      │
│ Shanghai Warehouse   │ │ Jack Zhang           │
│ ...                  │ │ Logistics Manager    │
│ Verified ✓           │ │                      │
└──────────────────────┘ └──────────────────────┘

┌───────────────────────────────────────────────┐
│ BUSINESS ROLES                                │
│                                               │
│ Shipper   Seller   Exporter   Payer            │
└───────────────────────────────────────────────┘

┌───────────────────────────────────────────────┐
│ REGISTRATIONS                                 │
│                                               │
│ CN USCC                       Verified          │
│ VAT                           Verified          │
│ EORI                          Expiring soon     │
└───────────────────────────────────────────────┘
```

---

# 十八、Role Profile

我建议增加一个非常重要的设计：

# Role Profiles

因为：

```text
ACME
```

作为：

```text
Shipper
```

和作为：

```text
Importer
```

实际使用的信息可能不同。

比如：

```text
Exporter Profile

Default Address
Shanghai HQ

Default Contact
Jack Zhang

Registration
CN USCC

Default Incoterm
DAP
```

而：

```text
Payer Profile

Billing Address
Shanghai HQ

Finance Contact
Mary Chen

Currency
USD

Payment Terms
NET 30
```

所以：

```text
Party
 └─ Role Profile
     ├─ Shipper
     ├─ Exporter
     └─ Payer
```

这个设计非常有价值。

---

# 十九、Billing & Payment

页面：

```text
Billing

Billing Status
Active

Default Currency
USD

Payment Terms
Net 30

Credit Limit
USD 50,000

Tax Inclusive
No

Billing Address
Shanghai HQ

Billing Contact
Mary Chen
```

Payment responsibility 必须区别：

```text
Transportation Charges

Shipper
Recipient
Third Party

────────────

Duties & Taxes

Shipper
Recipient
Third Party
```

不要把两个东西合成：

> Payer

---

# 二十、Carrier Accounts

Commercial Party 可以关联 Carrier Account。

```text
Carrier Accounts

DHL Express
Account ••••2891
Billing Party: ACME Trading
Status: Active

UPS
Account ••••9282
Status: Active

FedEx
Account ••••7192
Status: Active

+ Link Carrier Account
```

---

# 二十一、Relationships

这是很多 Parcel 产品会漏掉的。

例如：

```text
ACME US Inc.
        │
 Parent │
        ↓
ACME Trading Ltd.
        │
 Warehouse
        ↓
ACME Shanghai Warehouse
```

以及：

```text
Customer
↕
3PL

Seller
↕
Marketplace

Importer
↕
Customs Broker
```

UI：

```text
Relationships

ACME Holdings
Parent Organization

Shanghai Warehouse
Operating Location

ABC Customs Ltd.
Preferred Customs Broker

IDP Logistics
Billing Partner
```

---

# 二十二、Compliance & Documents

```text
Compliance

KYB
Verified ✓

Sanctions Screening
Clear ✓

Restricted Party Screening
Clear ✓

Last Screened
Sep 16, 2026

─────────────────

Documents

Business License
business-license.pdf
Verified

EORI Certificate
eori.pdf

Power of Attorney
poa.pdf
Expires Dec 31, 2026
```

---

# 二十三、Documents 到期提醒

例如：

```text
⚠ EORI Certificate expires in 21 days

[Review]
```

Commercial Party List 同时出现：

```text
Needs Attention
```

---

# 二十四、Usage

非常关键。

Party 详情必须回答：

> 这个 Party 到底在哪些 Shipment 用过？

```text
Usage

Last 30 days

Shipments              128
Quotes                   52
Commercial Invoices     121
Returns                    6
```

下面：

```text
Recent Shipments

SHP-260916-1028
Shanghai → Los Angeles

Role
Exporter · Shipper

Carrier
FedEx

Sep 16
```

---

# 二十五、Audit

企业级系统必须：

```text
Audit History

Sep 16 14:32
Jack Chen

Changed EORI
DE123456
→
DE987654

──────────────────

Sep 15 09:21
System

Address validation
Pending → Verified
```

涉及：

```text
Who
When
What
Old Value
New Value
Source
```

---

# 二十六、Duplicate Management

系统发现：

```text
ACME Ltd
ACME Trading Limited
```

显示：

```text
Potential Duplicate

92% Match
```

比较：

| Field      | Party A  | Party B              |
| ---------- | -------- | -------------------- |
| Legal Name | ACME Ltd | ACME Trading Limited |
| Country    | CN       | CN                   |
| VAT        | CN12345  | CN12345              |
| Address    | Shanghai | Shanghai             |
| Phone      | +86...   | +86...               |

操作：

```text
Not Duplicate

Merge
```

---

# 二十七、Merge Party

绝对不能简单：

> Delete A → Keep B

而要：

```text
Merge Parties

MASTER RECORD

● ACME Trading Ltd.
○ ACME Ltd.

──────────────────────

Legal Name

● ACME Trading Ltd.
○ ACME Ltd.

Address

☑ Shanghai HQ
☑ Shenzhen Warehouse

Contacts

☑ Jack Zhang
☑ Mary Chen

Registrations

☑ VAT
☑ EORI
```

最后：

```text
Affected Records

182 Shipments
34 Quotes
129 Invoices

All references will be redirected to:

PTY-10283

[Merge Parties]
```

---

# 二十八、Shipment 中怎么使用 Party

这实际上是整个设计最关键的一环。

Create Shipment：

```text
PARTIES

Shipper
[ Search Commercial Party... ]

ACME Trading Ltd.
Shanghai Warehouse
✓ Verified

────────────────────

Consignee
[ Search Commercial Party... ]

John Smith
California

────────────────────

Payer
[ Same as Shipper ▼ ]

────────────────────

Customs Parties

Importer
[ Same as Consignee ▼ ]

Exporter
[ Same as Shipper ▼ ]

Advanced Parties  >
```

---

# 二十九、Advanced Parties

默认折叠：

```text
Advanced Parties

Seller
Buyer
Importer
Exporter
Manufacturer
Broker
Ultimate Consignee
Pickup Party
Return Party
```

这样普通小包用户不会被复杂性淹没。

专业跨境用户又拥有全部能力。

这是非常重要的 **Progressive Disclosure**。

---

# 三十、Same As UX

例如：

```text
Importer

● Same as Consignee
○ Select different party
```

而不是重新填写一遍。

同理：

```text
Exporter
Same as Shipper
```

```text
Seller
Same as Shipper
```

大量减少输入。

---

# 三十一、Shipment Party Snapshot

这里我强烈建议 IDP Parcel 做一个专业设计：

## Master ≠ Shipment Snapshot

例如今天：

```text
ACME Address
1288 XX Road
```

创建 Shipment：

```text
SHP-10028
```

两个月以后用户修改 Party：

```text
ACME Address
1888 XX Road
```

**历史 Shipment 绝不能跟着变化。**

应该：

```text
Party Master
      ↓
Shipment Creation
      ↓
Party Snapshot
```

Shipment 保存：

```text
partyId
partyVersion
snapshot
```

UI 显示：

```text
ACME Trading Ltd.

Source
Commercial Party PTY-10283

Snapshot created
Sep 16, 2026 14:32
```

这是审计、面单、报关、Commercial Invoice 都非常需要的。

---

# 三十二、Master 更新后的 UX

如果 Party 已经变化：

```text
⚠ Commercial Party has changed

Shipment snapshot:
1288 XX Road

Current party:
1888 XX Road

[Keep Shipment Data]
[Update from Party]
```

而不是静默覆盖。

---

# 三十三、错误处理

不要：

```text
Invalid party.
```

应该：

```text
Importer information incomplete

Germany-bound shipment requires additional
registration information for this carrier/service.

Missing:
• EORI

ACME Trading Ltd.
PTY-10283

[Add EORI]
```

点：

```text
Add EORI
```

右侧 drawer 完成后直接返回 Shipment。

不要把用户踢出业务流程。

---

# 三十四、Validation Severity

统一三层：

```text
ERROR
Shipment cannot proceed.

WARNING
Shipment may proceed but carrier/customs
may reject the data.

INFO
Recommended improvement.
```

例如：

```text
🔴 EORI required
🟠 Address has not been validated
🔵 Billing contact recommended
```

---

# 三十五、Carrier Compatibility

这是 IDP Parcel 相比一般地址簿可以明显专业很多的地方。

Party 页加入：

```text
Carrier Compatibility

DHL Express                 Ready ✓
FedEx                       Ready ✓
UPS                         Attention

UPS:
Importer phone missing
```

本质：

```text
Canonical Party Model
       ↓
Carrier Adapter
       ↓
DHL Party
FedEx Party
UPS Party
Landmark Party
...
```

Carrier 字段变化不污染 IDP 的 Party Master。

---

# 三十六、Bulk Import

必须支持：

```text
CSV
XLSX
API
ERP Sync
```

流程：

```text
Upload
   ↓
Map Columns
   ↓
Validate
   ↓
Duplicate Check
   ↓
Review
   ↓
Import
```

例如：

```text
Your column             IDP Parcel

Company Name            → Legal Name
VAT No                  → Registration / VAT
Country                 → Country
Street                  → Address Line 1
```

---

# 三十七、批量导入结果

不要简单：

```text
Import complete.
```

应该：

```text
Import Complete

328 records processed

301 Created
 18 Updated
  6 Need Review
  3 Failed

[View Issues]
[Download Error Report]
```

---

# 三十八、状态设计

Party Status：

```text
Draft
Active
Suspended
Inactive
Archived
```

Registration Status：

```text
Unverified
Verified
Expiring
Expired
Invalid
```

Address Status：

```text
Not Validated
Validated
Warning
Invalid
```

Compliance：

```text
Not Checked
Clear
Review
Blocked
```

这些状态不要混在一起。

---

# 三十九、权限模型

至少：

```text
Commercial Party

party.view
party.create
party.edit
party.deactivate

party.address.manage
party.contact.manage

party.registration.view
party.registration.manage

party.billing.view
party.billing.manage

party.carrier_account.manage

party.compliance.view
party.compliance.manage

party.merge

party.import
party.export

party.audit.view
```

敏感字段可以 Mask：

```text
Tax ID
DE12••••••789

Carrier Account
••••••2891
```

---

# 四十、我建议的最终信息架构

```text
COMMERCIAL PARTY

Identity
├── Legal Identity
├── Organization / Individual
├── External References
└── Status

Roles
├── Shipping
├── Commercial
├── Customs
└── Financial

Locations
├── Registered
├── Shipping
├── Pickup
├── Billing
├── Customs
└── Return

Contacts
├── Primary
├── Logistics
├── Customs
├── Finance
└── Compliance

Registrations
├── Tax
├── Customs
├── Import
├── Export
└── Regulatory

Financial
├── Billing
├── Currency
├── Payment Terms
└── Credit

Carrier
├── Carrier Accounts
├── Carrier Requirements
└── Compatibility

Relationship
├── Parent
├── Subsidiary
├── Warehouse
├── Broker
└── Business Partner

Compliance
├── KYB
├── Screening
├── Documents
└── Expiry

Usage
├── Quote
├── Shipment
├── Customs
├── Invoice
└── Return

Governance
├── Version
├── Audit
├── Merge
└── Duplicate
```

---

# 四十一、视觉 UX 我建议继续保持 IDP Parcel 现有风格

不要把它做成传统 ERP 密密麻麻的后台。

采用：

```text
Light UI
White / neutral surface

IDP Navy / Black
+
Cyan / Teal functional accent

12px radius
1px neutral border
Very subtle shadow

Data dense
but visually quiet
```

信息层级：

```text
Page
→ Section
→ Card
→ Field
```

而不是：

```text
框
套框
套框
再套框
```

列表页偏专业 SaaS：

```text
Linear
Stripe
Shippo
现代物流控制台
```

详情页偏：

```text
Stripe Customer
+
Shopify Organization
+
现代 TMS Master Data
```

---

# 四十二、最重要的 5 个设计原则

IDP Parcel 这个模块我会锁定以下原则：

1. **Party ≠ Role**：主体是主数据，角色在业务交易中动态绑定。
2. **Master ≠ Snapshot**：历史 Shipment 永远保留当时的 Party 数据。
3. **Registration Rule-driven**：税号/监管号根据国家、角色、承运商动态呈现，不能前端写死；DHL 当前仍持续更新这类类型。([DHL API Developer Portal][3])
4. **Simple by default, powerful when needed**：普通用户只看到 Shipper / Consignee / Payer，高级 Customs Party 折叠。
5. **One Party, many contexts**：地址、联系人、Registration、Carrier Account、Billing、Compliance 都归属于统一 Party Master。

按照这个结构，Commercial Party 才能真正成为 **IDP Parcel 的核心 Master Data Domain**，而不是一个附属“地址簿”。它也可以直接支撑你之前已经设计的 **国家/Zone/报价、Surcharge、Shipment、Commercial Invoice、Customs、Track & Trace** 等模块。

[1]: https://developer.dhl.com/sites/default/files/2026-03/DHL%20EXPRESS%20-%20MyDHL%20API%20-%20SOAP%20Developer%20Guide%20-%20v2.41.pdf?utm_source=chatgpt.com "DHL EXPRESS - MyDHL API - SOAP Developer Guide"
[2]: https://developer.dhl.com/api-reference/mydhl-api-soap-dhl-express?utm_source=chatgpt.com "MyDHL API SOAP (DHL Express)"
[3]: https://developer.dhl.com/api-reference/dhl-express-mydhl-api?language_content_entity=ja&utm_source=chatgpt.com "DHL Express - MyDHL API"
