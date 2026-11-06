# 11 party-commercial 其余十八册迁到 ADR-0144

Category: enhancement
Status: in-progress——分支 `cursor/crpr-11-party`，基 `d48c5b130`；迁移编号预留 party-commercial `0047`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/partycommercial/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试（客户账户已迁，不在内）；`cmd/parcel-api` 里这些读口的占位；
`migrations/party_commercial` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/commercial-business-parties` | `PartyIdentityCatalogueRead.ListBusinessParties` | `recorded_at DESC, party_id`（折叠到最新修订后） |
| `/commercial-group-legal-entities` | `PartyIdentityCatalogueRead.ListGroupLegalEntities` | `recorded_at DESC, legal_entity_id` |
| `/commercial-party-relationships` | `PartyIdentityCatalogueRead.ListPartyRelationships` | `recorded_at DESC, relationship_id` |
| `/commercial-product-channel-mappings` | `ProductChannelMappingCatalogueRead.ListProductChannelMappings` | `recorded_at DESC, mapping_id` |
| `/commercial-registration-number-types` | `RegistrationNumberTypeCatalogueRead.ListRegistrationNumberTypes` | `recorded_at DESC, country_code, type_code` |
| `/commercial-service-products` | `ServiceProductCatalogueRead.ListServiceProducts` | `published_at DESC, object_id, version_label` |
| `/commercial-customer-contracts` | `CommercialRelationCatalogueRead.ListCustomerContracts` | `published_at DESC, object_id, version_label` |
| `/commercial-supplier-agreements` | `CommercialRelationCatalogueRead.ListSupplierAgreements` | `published_at DESC, object_id, version_label` |
| `/commercial-policies`（九种 `kind`） | `CommercialPolicyCatalogueRead` 的九个方法 | 版本壳按 `published_at` / `declared_at` / `registered_at` 倒序加 `object_id, version_label` |
| `/commercial-open-publication-drafts` | `PublicationDraftRead.ListOpenDrafts` | `submitted_at DESC, object_kind, object_id, version_label` |

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：

- `kind` 保持册子选择器原义（决定四），九种各是一册。
- 身份三册与客户账户同一份登记册、同一个生命周期导出，状态维的口径与客户账户一字不差。
- 修订链三口（法人、业务参与方、产品渠道映射的 `…/revisions`）有意不设上限，不在内。
