import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { listPriceCards, type PriceCardListResponseBody } from '../pricing/api';
import {
  listBusinessParties,
  listGroupLegalEntities,
  type BusinessPartyListResponseBody,
  type GroupLegalEntityListResponseBody,
} from './api';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { Field, ReferencePicker, momentPlaceholder } from './PublicationFormFields';
import { planReferenceOf } from './publication-form-shared';
import {
  emptySupplierAgreementDraft,
  supplierAgreementFieldPaths,
  supplierAgreementPayloadOf,
  withShellCopiedIntoAgreement,
  type SupplierAgreementDraft,
} from './supplier-agreement-form';

/**
 * 供应商协议版本的逐字段表单（票 admin-write-faces/11；ADR-0101 决定八逐册裁形）。
 *
 * **为什么是逐字段表单**：低频、运营配置员操作、正文六格无子表——决定一判据（登记频次 × 操作者角色 × 载荷结构）
 * 的直接读数；本册正文不是矩阵，模板导入（决定二）只针对上百格的价卡。五步（预览摘要 → 存为待批准 → 批准 → 发布）由公共半边 PublicationDraftFlow 走，
 * 本组件只摆版本壳五格与协议正文六格，草稿 → 载荷在 supplier-agreement-form.ts。
 *
 * **三样从读面选而不是手抄**：供应商与责任法人从主数据读面（业务参与方册、集团法人册）选，采购方案从价卡目录选
 * ——跨上下文只传引用，选出来的仍是引用串（`planId@planVersion`），表单不读方案内容；方案的方向与绑定换算是
 * PRICE_RULE 那册的事，这里不出现、不按方向过滤，目录行上的方向只是显给人看。读面在接入渠道未配置那堵墙前
 * （403）或读不到时退回手填，表单不因此变死（判据同 pricing 的 QuoteBasisField）。
 *
 * **表单不算摘要、不收也不送批准人、不裁任何门**（伞票 07 硬句）：连「必填」都不在本地拦——空字段、区间先后、
 * 引用在不在册一律送上去，预览口逐格 problems 回来挂到对应格旁；本地没有任何一格编不进载荷类型（全是文本），
 * 所以不给 localProblems。
 */
export interface SupplierAgreementPublicationFormProps {
  /** 载体到达「发布」那一步时回调，供应商协议页借它刷同页目录读面。 */
  onPublished?: () => void;
}

export function SupplierAgreementPublicationForm({ onPublished }: SupplierAgreementPublicationFormProps) {
  const [draft, setDraft] = useState<SupplierAgreementDraft>(emptySupplierAgreementDraft());
  const patch = (change: Partial<SupplierAgreementDraft>) => setDraft((current) => ({ ...current, ...change }));

  return (
    <PublicationDraftFlow
      kind="SUPPLIER_AGREEMENT"
      title="发布供应商协议版本"
      assemblePayload={() => supplierAgreementPayloadOf(draft)}
      fieldPaths={supplierAgreementFieldPaths}
      onPublished={onPublished}
    >
      {({ problems, locked }: PublicationFormContext) => (
        <div className="flex flex-col gap-5">
          <section className="flex flex-col gap-3">
            <h3 className="text-[13px] font-medium text-idpxyz-text">版本壳</h3>
            <p className="text-[11px] text-idpxyz-textMuted">
              壳上的适用范围与区间是<strong>版本</strong>的（登记册逐列比对的项），与下面协议正文自己的范围与区间是两样；
              区间上界留空即持续有效。壳上不带指名引用——协议引用的供应商、法人与方案都在正文里。
            </p>
            <div className="grid grid-cols-2 gap-3">
              <Field label="协议对象标识 *" path="objectId" problems={problems}>
                <Input
                  value={draft.objectId}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  onChange={(event) => patch({ objectId: event.target.value })}
                />
              </Field>
              <Field label="版本号 *" path="version" problems={problems}>
                <Input
                  value={draft.version}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  onChange={(event) => patch({ version: event.target.value })}
                />
              </Field>
              <Field label="版本适用范围 *" path="scope" problems={problems}>
                <Input
                  value={draft.scope}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  onChange={(event) => patch({ scope: event.target.value })}
                />
              </Field>
              <div />
              <Field label="版本生效起点 *" path="effectiveStartsAt" problems={problems}>
                <Input
                  value={draft.effectiveStartsAt}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder={momentPlaceholder}
                  onChange={(event) => patch({ effectiveStartsAt: event.target.value })}
                />
              </Field>
              <Field label="版本生效止点（可空）" path="effectiveEndsAt" problems={problems}>
                <Input
                  value={draft.effectiveEndsAt}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder="留空即持续有效"
                  onChange={(event) => patch({ effectiveEndsAt: event.target.value })}
                />
              </Field>
            </div>
          </section>

          <section className="flex flex-col gap-3">
            <div className="flex items-center justify-between">
              <h3 className="text-[13px] font-medium text-idpxyz-text">协议正文（0021）</h3>
              <Button variant="outline" disabled={locked} onClick={() => setDraft(withShellCopiedIntoAgreement)}>
                范围与区间从版本壳带入
              </Button>
            </div>
            <p className="text-[11px] text-idpxyz-textMuted">
              供应商协议约定的是<strong>采购</strong>：方向由领域钉死为 BUY，正文里没有方向格。采购方案只是 parcel-pricing
              方案版本的引用串，本表单不读方案内容——方案的方向与绑定换算归商业价格政策（PRICE_RULE）那册。
            </p>
            <div className="grid grid-cols-2 gap-3">
              <ReferencePicker
                label="供应商（业务参与方）*"
                path="supplierAgreement.supplier"
                problems={problems}
                value={draft.supplier}
                locked={locked}
                onChange={(supplier) => patch({ supplier })}
                load={listBusinessParties}
                optionsOf={(body: BusinessPartyListResponseBody) =>
                  body.parties.map((party) => ({
                    value: party.partyId,
                    label: `${party.partyId} · ${party.partyName} · ${party.status}`,
                  }))
                }
                emptyNote="当前租户尚无业务参与方；先在业务参与方册登记供应商，再发布协议。"
                readFace="业务参与方册"
              />
              <ReferencePicker
                label="责任法人（集团法人）*"
                path="supplierAgreement.legalEntity"
                problems={problems}
                value={draft.legalEntity}
                locked={locked}
                onChange={(legalEntity) => patch({ legalEntity })}
                load={listGroupLegalEntities}
                optionsOf={(body: GroupLegalEntityListResponseBody) =>
                  body.entities.map((entity) => ({
                    value: entity.legalEntityId,
                    label: `${entity.legalEntityId} · ${entity.partyNameKnown ? entity.partyName : entity.partyId} · ${entity.status}`,
                  }))
                }
                emptyNote="当前租户尚无集团法人；先在集团与法人册登记责任法人。"
                readFace="集团法人册"
              />
              <Field label="协议适用范围 *" path="supplierAgreement.scope" problems={problems}>
                <Input
                  value={draft.agreementScope}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  onChange={(event) => patch({ agreementScope: event.target.value })}
                />
              </Field>
              <ReferencePicker
                label="采购定价方案（价卡目录）*"
                path="supplierAgreement.purchasePlan"
                problems={problems}
                value={draft.purchasePlan}
                locked={locked}
                onChange={(purchasePlan) => patch({ purchasePlan })}
                load={listPriceCards}
                optionsOf={(body: PriceCardListResponseBody) =>
                  body.cards.map((card) => ({
                    value: planReferenceOf(card),
                    label: `${planReferenceOf(card)} · ${card.direction} · ${card.purpose} · ${card.scope}`,
                  }))
                }
                emptyNote="价卡目录为空；先登记采购方向的价卡，再发布协议。"
                readFace="价卡目录"
              />
              <Field label="协议生效起点 *" path="supplierAgreement.effectiveStartsAt" problems={problems}>
                <Input
                  value={draft.agreementEffectiveStartsAt}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder={momentPlaceholder}
                  onChange={(event) => patch({ agreementEffectiveStartsAt: event.target.value })}
                />
              </Field>
              <Field label="协议生效止点（可空）" path="supplierAgreement.effectiveEndsAt" problems={problems}>
                <Input
                  value={draft.agreementEffectiveEndsAt}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder="留空即无上界"
                  onChange={(event) => patch({ agreementEffectiveEndsAt: event.target.value })}
                />
              </Field>
            </div>
          </section>
        </div>
      )}
    </PublicationDraftFlow>
  );
}
