import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone } from '../moment';
import { controlFailureDispositionLabels, controlKindLabels, jointPassConditionLabels } from './presentation';
import { listCommercialPolicies } from './api';
import { ExistingVersionsNote } from './ExistingVersionsNote';
import { policyKnownVersions } from './known-versions';
import { fetchPublicationVocabulary, type PublicationVocabularyResponseBody } from './publication-draft-api';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import type { NewVersionStart } from './new-version';
import { Field, ProblemText, VocabularySelect, useLoaded } from './PublicationFormFields';
import { WallTimeField } from './party-registration-fields';
import { fieldProblemText } from './field-problem-text';
import {
  controlPolicyCodesOf,
  controlPolicyFieldPaths,
  controlPolicyLocalProblems,
  controlPolicyPayloadOf,
  controlRowHints,
  emptyControlPolicyDraft,
  withControlRowAdded,
  withControlRowPatched,
  withControlRowRemoved,
  type ControlItemDraft,
  type ControlPolicyDraft,
} from './pre-acceptance-financial-control-policy-form';

const loadKnownVersions = () => listCommercialPolicies('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY');

/**
 * 接受前财务控制策略版本的逐字段表单（票 admin-write-faces/13；ADR-0101 决定八逐册裁形）。
 *
 * **为什么是逐字段表单、控制项可加行**：低频、运营配置员操作、正文是一格 + 一张几行的表——决定一判据（登记频次 ×
 * 操作者角色 × 载荷结构）的直接读数；本册正文不是矩阵，模板导入（决定二）只针对上百格的价卡。五步（预览摘要 → 存为待批准 → 批准 → 发布）由公共半边
 * PublicationDraftFlow 走，本组件只摆版本壳五格与策略正文，草稿 → 载荷在 pre-acceptance-financial-control-policy-form.ts。
 *
 * **三个封闭集只从词表选，两样手填，一样整数**：共同通过条件、控制种类、失败处置的下拉只吃服务端词表读口（票 20，
 * `kind=PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 的同名三集）答的码 × 本页中文词表，表单不内置任何一格、不预选——
 * 首发共同通过条件只有一值也不预选，那是租户说出来的不是产品默认的（ADR-0115 Decision 三）；**控制种类下拉里没有
 * 「无控制」**，那一格属客户合同的声明（ADR-0115 Decision 一），词表里没有它，表单也长不出它。费用范围与责任是开放
 * 引用串手填（0024 头注：收串不校验存在性）；判断顺序是整数格。词表在未配置那堵墙前（403）或读不到时下拉显占位、
 * 不自造码，送预览会由服务端点名那一格。
 *
 * **表单不算摘要、不收也不送批准人、不裁任何门**（伞票 07 硬句）：空字段、集外的码、引用在不在册一律送上去，预览口
 * 逐格 problems 回来挂到对应格旁；至少一项、顺序唯一、（种类 × 范围）唯一是跨行的门，由构造门在预览上答成成因。本地
 * 只拦编不进 JSON 整数的顺序文本与换不成时刻的时刻格（localProblems）；重复的行只在表下**提示**一句，不挡预览——票面
 * 允许提示，拒绝由服务端说。
 */
export interface PreAcceptanceFinancialControlPolicyPublicationFormProps {
  /** 载体到达「发布」那一步时回调，政策页借它切到接受前财务控制策略册并重读。 */
  onPublished?: () => void;
  /** 基于目录里某一版预填（票 party-commercial-uiux/14）；换一次预填由页面换 key 重建本表单。 */
  start?: NewVersionStart<ControlPolicyDraft>;
}

const bodyPath = 'preAcceptanceFinancialControlPolicy';
const kind = 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY';

export function PreAcceptanceFinancialControlPolicyPublicationForm({
  onPublished,
  start,
}: PreAcceptanceFinancialControlPolicyPublicationFormProps) {
  const [draft, setDraft] = useState<ControlPolicyDraft>(start?.draft ?? emptyControlPolicyDraft());
  const patch = (change: Partial<ControlPolicyDraft>) => setDraft((current) => ({ ...current, ...change }));
  const patchRow = (index: number, change: Partial<ControlItemDraft>) =>
    setDraft((current) => withControlRowPatched(current, index, change));
  // 词表读一次、只读一次：三个集合同一口答，表单打开时取一份，行再多也不重取。
  const vocabulary = useLoaded(loadControlPolicyVocabulary);
  const sets = vocabulary?.kind === 'outcome' ? vocabulary.body.sets : null;
  const codesOf = (name: string) => (sets === null ? null : controlPolicyCodesOf(sets, name));
  const hints = controlRowHints(draft);
  // 时刻按操作者的显示时区填、送出 UTC（票 party-commercial-uiux/10），与登记表单同一条换算。
  const timeZone = currentDisplayTimeZone();
  // 这个对象已有哪些版本（票 party-commercial-uiux/15）：读不到就不提示，不报错。
  const knownAnswer = useLoaded(loadKnownVersions);
  const known = knownAnswer?.kind === 'outcome' ? policyKnownVersions(knownAnswer.body) : null;

  return (
    <PublicationDraftFlow
      kind="PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY"
      title="发布接受前财务控制策略版本"
      assemblePayload={() => controlPolicyPayloadOf(draft, timeZone)}
      localProblems={controlPolicyLocalProblems(draft, timeZone)}
      fieldPaths={controlPolicyFieldPaths(draft)}
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
            <p className="text-[11px] text-idpxyz-textMuted">
              版本适用范围与生效区间说的是<strong>这一版</strong>
              本身。策略正文没有自己的范围与区间：它只说控制怎么做，用在哪个费用范围由客户合同按范围指定。
            </p>
            <div className="grid grid-cols-2 gap-3">
              <Field label="策略对象标识 *" path="objectId" problems={problems}>
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
            <h3 className="text-[13px] font-medium text-idpxyz-text">策略正文（这一版的条款内容）</h3>
            <p className="text-[11px] text-idpxyz-textMuted">
              先选共同通过条件，再填控制项表：每行说在哪个费用范围上做哪一种控制、判断顺序排第几、失败了怎么处置、失败或补偿的责任归谁。至少要有一项；判断顺序从 1 起，同一版里不能重复；同一范围上同一种控制只能有一行。
              <strong>「明确不做控制」不在这里设</strong>：那是客户合同按范围的声明（要写明不适用的依据），控制种类里没有这一项。
            </p>
            <div className="grid grid-cols-2 gap-3">
              <VocabularySelect
                label="共同通过条件 *"
                path={`${bodyPath}.jointPassCondition`}
                setName="jointPassCondition"
                kind={kind}
                codes={codesOf('jointPassCondition')}
                vocabulary={vocabulary}
                labels={jointPassConditionLabels}
                problems={problems}
                value={draft.jointPassCondition}
                locked={locked}
                onChange={(jointPassCondition) => patch({ jointPassCondition })}
              />
            </div>

            <div className="flex items-center justify-between">
              <h4 className="text-[12px] font-medium text-idpxyz-text">控制项（{draft.controls.length} 行）</h4>
              <Button variant="outline" disabled={locked} onClick={() => setDraft(withControlRowAdded)}>
                加一行
              </Button>
            </div>
            {draft.controls.length === 0 ? (
              <p className="text-[11px] text-idpxyz-textMuted">
                还没有控制项。一版策略至少要有一项控制；「明确不做控制」由客户合同声明，不在这里。
              </p>
            ) : null}
            {draft.controls.map((row, index) => (
              <ControlRow
                key={index}
                index={index}
                row={row}
                problems={problems}
                locked={locked}
                controlCodes={codesOf('control')}
                onFailureCodes={codesOf('onFailure')}
                vocabulary={vocabulary}
                onChange={(change) => patchRow(index, change)}
                onRemove={() => setDraft((current) => withControlRowRemoved(current, index))}
              />
            ))}
            {hints.length > 0 ? (
              <ul className="text-[11px] text-idpxyz-textMuted list-disc ml-4">
                {hints.map((hint) => (
                  <li key={hint}>提示：{hint}</li>
                ))}
              </ul>
            ) : null}
          </section>
        </div>
      )}
    </PublicationDraftFlow>
  );
}

// 一行控制项：三个下拉 + 两个引用串 + 一个整数格，外加删行；行本身（controls[i]）被点名时显在行下。
function ControlRow({
  index,
  row,
  problems,
  locked,
  controlCodes,
  onFailureCodes,
  vocabulary,
  onChange,
  onRemove,
}: {
  index: number;
  row: ControlItemDraft;
  problems: Record<string, string[]>;
  locked: boolean;
  controlCodes: string[] | null;
  onFailureCodes: string[] | null;
  vocabulary: ApiResult<PublicationVocabularyResponseBody> | null;
  onChange: (change: Partial<ControlItemDraft>) => void;
  onRemove: () => void;
}) {
  const rowPath = `${bodyPath}.controls[${index}]`;
  const rowLines = problems[rowPath] ?? [];
  return (
    <div className="rounded border border-idpxyz-border p-3 flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <span className="text-[12px] text-idpxyz-textMuted">第 {index + 1} 行</span>
        <Button variant="outline" disabled={locked} onClick={onRemove}>
          删这一行
        </Button>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <VocabularySelect
          label="控制种类 *"
          path={`${rowPath}.control`}
          setName="control"
          kind={kind}
          codes={controlCodes}
          vocabulary={vocabulary}
          labels={controlKindLabels}
          problems={problems}
          value={row.control}
          locked={locked}
          onChange={(control) => onChange({ control })}
        />
        <Field label="费用范围 *" path={`${rowPath}.chargeScope`} problems={problems}>
          <Input
            value={row.chargeScope}
            disabled={locked}
            className="font-mono text-[13px]"
            placeholder="费用范围（系统不核对是否已登记）"
            onChange={(event) => onChange({ chargeScope: event.target.value })}
          />
        </Field>
        <Field label="判断顺序 *" path={`${rowPath}.order`} problems={problems}>
          <Input
            value={row.order}
            disabled={locked}
            className="font-mono text-[13px]"
            placeholder="从 1 起的整数，同一版里不重复"
            onChange={(event) => onChange({ order: event.target.value })}
          />
        </Field>
        <VocabularySelect
          label="失败处置 *"
          path={`${rowPath}.onFailure`}
          setName="onFailure"
          kind={kind}
          codes={onFailureCodes}
          vocabulary={vocabulary}
          labels={controlFailureDispositionLabels}
          problems={problems}
          value={row.onFailure}
          locked={locked}
          onChange={(onFailure) => onChange({ onFailure })}
        />
        <Field label="失败或补偿责任方 *" path={`${rowPath}.responsibility`} problems={problems}>
          <Input
            value={row.responsibility}
            disabled={locked}
            className="font-mono text-[13px]"
            placeholder="参与方、条款或角色"
            onChange={(event) => onChange({ responsibility: event.target.value })}
          />
        </Field>
      </div>
      {rowLines.map((line) => (
        <span key={line} className="block text-[11px] text-idpxyz-danger">
          <ProblemText problem={fieldProblemText(line)} />
        </span>
      ))}
    </div>
  );
}

// 稳定的函数引用：useLoaded 以它为依赖，写成内联箭头会每次渲染重取。
function loadControlPolicyVocabulary(): Promise<ApiResult<PublicationVocabularyResponseBody>> {
  return fetchPublicationVocabulary('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY');
}
