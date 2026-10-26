import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { currentDisplayTimeZone } from '../moment';
import { listPriceCards, type PriceCardListResponseBody } from '../pricing/api';
import { listBusinessParties, listGroupLegalEntities, listSupplierAgreements } from './api';
import { ExistingVersionsNote } from './ExistingVersionsNote';
import { agreementKnownVersions } from './known-versions';
import type { NewVersionStart } from './new-version';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { Field, ReferencePicker, useLoaded } from './PublicationFormFields';
import { WallTimeField, businessPartyPickerOptions, legalEntityPickerOptions } from './party-registration-fields';
import { planReferenceOf } from './publication-form-shared';
import {
  emptySupplierAgreementDraft,
  supplierAgreementFieldPaths,
  supplierAgreementLocalProblems,
  supplierAgreementPayloadOf,
  withShellCopiedIntoAgreement,
  type SupplierAgreementDraft,
} from './supplier-agreement-form';

const loadKnownVersions = () => listSupplierAgreements();

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
 * 引用在不在册一律送上去，预览口逐格 problems 回来挂到对应格旁；本地只拦换不成时刻的时刻格
 * （supplierAgreementLocalProblems），那是组不出载荷，不是校验。
 */
export interface SupplierAgreementPublicationFormProps {
  /** 载体到达「发布」那一步时回调，供应商协议页借它刷同页目录读面。 */
  onPublished?: () => void;
  /** 基于目录里某一版预填（票 party-commercial-uiux/14）；换一次预填由页面换 key 重建本表单。 */
  start?: NewVersionStart<SupplierAgreementDraft>;
}

export function SupplierAgreementPublicationForm({ onPublished, start }: SupplierAgreementPublicationFormProps) {
  const [draft, setDraft] = useState<SupplierAgreementDraft>(start?.draft ?? emptySupplierAgreementDraft());
  const patch = (change: Partial<SupplierAgreementDraft>) => setDraft((current) => ({ ...current, ...change }));
  // 时刻按操作者的显示时区填、送出 UTC（票 party-commercial-uiux/10），与登记表单同一条换算。
  const timeZone = currentDisplayTimeZone();
  // 这个对象已有哪些版本（票 party-commercial-uiux/15）：读不到就不提示，不报错。
  const knownAnswer = useLoaded(loadKnownVersions);
  const known = knownAnswer?.kind === 'outcome' ? agreementKnownVersions(knownAnswer.body) : null;

  return (
    <PublicationDraftFlow
      kind="SUPPLIER_AGREEMENT"
      title="发布供应商协议版本"
      assemblePayload={() => supplierAgreementPayloadOf(draft, timeZone)}
      localProblems={supplierAgreementLocalProblems(draft, timeZone)}
      fieldPaths={supplierAgreementFieldPaths}
      onPublished={onPublished}
      prefilledFrom={start?.from}
      prefilledCaution={start?.caution}
      versionNote={
        <ExistingVersionsNote
          known={known}
          objectId={draft.objectId}
          version={draft.version}
          startsAt={draft.effectiveStartsAt}
          endsAt={draft.effectiveEndsAt}
          timeZone={timeZone}
        />
      }
    >
      {({ problems, locked, versionNote }: PublicationFormContext) => (
        <div className="flex flex-col gap-5">
          <section className="flex flex-col gap-3">
            <h3 className="text-[13px] font-medium text-idpxyz-text">版本信息</h3>
            {/* 版本上的范围与区间是登记册逐列比对的项，与正文自己的范围与区间是两样；版本上不带指名引用，引用全在正文里。 */}
            <p className="text-[11px] text-idpxyz-textMuted">
              这里的适用范围与有效区间属于<strong>版本</strong>
              ，和下面协议正文自己的范围与区间分开填。供应商、责任法人与采购方案都在正文里填。
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
              <WallTimeField
                label="版本生效起点 *"
                path="effectiveStartsAt"
                problems={problems}
                value={draft.effectiveStartsAt}
                locked={locked}
                timeZone={timeZone}
                onChange={(effectiveStartsAt) => patch({ effectiveStartsAt })}
              />
              <WallTimeField
                label="版本生效止点"
                path="effectiveEndsAt"
                problems={problems}
                value={draft.effectiveEndsAt}
                locked={locked}
                timeZone={timeZone}
                note="留空即持续有效。"
                onChange={(effectiveEndsAt) => patch({ effectiveEndsAt })}
              />
            </div>
            {versionNote}
          </section>

          <section className="flex flex-col gap-3">
            <div className="flex items-center justify-between">
              <h3 className="text-[13px] font-medium text-idpxyz-text">协议正文（这一版的条款内容）</h3>
              <Button variant="outline" disabled={locked} onClick={() => setDraft(withShellCopiedIntoAgreement)}>
                范围与区间从版本信息带入
              </Button>
            </div>
            {/* 方向由领域钉死为 BUY，正文没有方向格；采购方案只传方案版本引用串，表单不读方案内容，方向与绑定换算归价格政策那册。 */}
            <p className="text-[11px] text-idpxyz-textMuted">
              供应商协议约定的是<strong>采购</strong>
              ，方向固定为买价，不用另填。采购方案从价卡目录选；方案的方向与换算在商业价格政策里约定，不在这里。
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
                optionsOf={businessPartyPickerOptions}
                emptyNote="还没有登记任何业务参与方；先在业务参与方页登记供应商，再发布协议。"
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
                // 选项文字与全区选单同一种写法：名称 · 标识 · 中文状态（票 party-commercial-uiux/13）。
                optionsOf={legalEntityPickerOptions}
                emptyNote="还没有登记任何责任法人；先在集团与法人页登记。"
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
              <WallTimeField
                label="协议生效起点 *"
                path="supplierAgreement.effectiveStartsAt"
                problems={problems}
                value={draft.agreementEffectiveStartsAt}
                locked={locked}
                timeZone={timeZone}
                onChange={(agreementEffectiveStartsAt) => patch({ agreementEffectiveStartsAt })}
              />
              <WallTimeField
                label="协议生效止点"
                path="supplierAgreement.effectiveEndsAt"
                problems={problems}
                value={draft.agreementEffectiveEndsAt}
                locked={locked}
                timeZone={timeZone}
                note="留空即持续有效。"
                onChange={(agreementEffectiveEndsAt) => patch({ agreementEffectiveEndsAt })}
              />
            </div>
          </section>
        </div>
      )}
    </PublicationDraftFlow>
  );
}
