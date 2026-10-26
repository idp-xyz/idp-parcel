import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { accessRefusalTitle } from '../../auth/access-refusal';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone } from '../moment';
import { cancellationPartyLabels } from './presentation';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { fetchPublicationVocabulary, type PublicationVocabularyResponseBody } from './publication-draft-api';
import { Field, Problems, RowFrame, selectClass, useLoaded } from './PublicationFormFields';
import { WallTimeField } from './party-registration-fields';
import {
  authorizationRuleFieldPaths,
  authorizationRuleLocalProblems,
  emptyAuthorizationRuleDraft,
  emptyCancellationRowDraft,
  partyOptionsOf,
  payloadOf,
  type AuthorizationRuleDraft,
  type CancellationRowDraft,
  type PartyOptionsState,
} from './authorization-rule-form';

/**
 * 授权规则版本的逐字段发布表单（票 admin-write-faces/17；ADR-0101 决定八自裁为逐字段表单 + 取消授权按请求方可加行）。
 * 五步「表单 → 预览摘要 → 存为待批准 → 批准 → 发布」由 PublicationDraftFlow 走，本组件只摆本册的几格并把草稿组成
 * 载荷（纯函数在 authorization-rule-form.ts）。
 *
 * 版本壳之外归本册的正文只有 0013 的取消授权目录：一张两列几行的表（请求方 × 规则引用）。授权授予册
 * （AuthorizedAction 一族）不经这条发布路，不在这里。
 *
 * **表单不算摘要、不裁任何门、不代判**（伞票 07 硬句）：没选请求方、空规则、同一请求方两行、零行都照样送上去，
 * 答回来的是构造门的拒绝（逐格问题落在行上，跨行的判在预览上答`未受理`带成因）。**请求方下拉不内置枚举**：
 * 码从服务端词表读口取（票 20），中文用本页词表；读口在未配置那堵墙前时下拉显占位，不拿任何内置码顶替，
 * 也不预选任何一项。
 */
export interface AuthorizationRulePublicationFormProps {
  /** 载体到达「发布」那一步时回调，页面借它刷同页目录读面（取消授权按请求方逐格上列）。 */
  onPublished?: () => void;
}

const sectionTitle = 'text-[13px] font-medium text-idpxyz-text';

export function AuthorizationRulePublicationForm({ onPublished }: AuthorizationRulePublicationFormProps) {
  const [draft, setDraft] = useState<AuthorizationRuleDraft>(emptyAuthorizationRuleDraft());
  const parties = usePartyOptions();

  const patch = (change: Partial<AuthorizationRuleDraft>) => setDraft((current) => ({ ...current, ...change }));
  const patchRow = (index: number, change: Partial<CancellationRowDraft>) =>
    patch({ rows: draft.rows.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  // 时刻按操作者的显示时区填、送出 UTC（票 party-commercial-uiux/10），与登记表单同一条换算。
  const timeZone = currentDisplayTimeZone();

  return (
    <PublicationDraftFlow
      kind="AUTHORIZATION_RULE"
      title="发布授权规则版本"
      assemblePayload={() => payloadOf(draft, timeZone)}
      localProblems={authorizationRuleLocalProblems(draft, timeZone)}
      fieldPaths={authorizationRuleFieldPaths(draft)}
      onPublished={onPublished}
    >
      {(form) => (
        <div className="flex flex-col gap-5">
          <ShellFields draft={draft} patch={patch} form={form} timeZone={timeZone} />
          <CancellationAuthorityFields draft={draft} patch={patch} patchRow={patchRow} form={form} parties={parties} />
        </div>
      )}
    </PublicationDraftFlow>
  );
}

// ——版本壳：身份三元（对象标识 / 版本号 / 适用范围）与有效区间。租户不在这里：它从操作者信封来。

function ShellFields({
  draft,
  patch,
  form,
  timeZone,
}: {
  draft: AuthorizationRuleDraft;
  patch: (change: Partial<AuthorizationRuleDraft>) => void;
  form: PublicationFormContext;
  timeZone: string;
}) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className={sectionTitle}>版本信息</h3>
      <div className="grid grid-cols-2 gap-3">
        <Field label="授权规则对象标识 *" path="objectId" problems={form.problems}>
          <Input
            value={draft.objectId}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            onChange={(event) => patch({ objectId: event.target.value })}
          />
        </Field>
        <Field label="版本号 *" path="version" problems={form.problems}>
          <Input
            value={draft.version}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            onChange={(event) => patch({ version: event.target.value })}
          />
        </Field>
        <Field label="适用范围 *" path="scope" problems={form.problems}>
          <Input
            value={draft.scope}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="商业范围引用"
            onChange={(event) => patch({ scope: event.target.value })}
          />
        </Field>
        <div />
        <WallTimeField
          label="生效起点 *"
          path="effectiveStartsAt"
          problems={form.problems}
          value={draft.effectiveStartsAt}
          locked={form.locked}
          timeZone={timeZone}
          onChange={(value) => patch({ effectiveStartsAt: value })}
        />
        <WallTimeField
          label="生效止点"
          path="effectiveEndsAt"
          problems={form.problems}
          value={draft.effectiveEndsAt}
          locked={form.locked}
          timeZone={timeZone}
          note="留空即持续有效。"
          onChange={(value) => patch({ effectiveEndsAt: value })}
        />
      </div>
      <p className="text-[11px] text-idpxyz-textMuted">
        起点晚于现在时，发布稿照样可以保存和批准；发布时会提示还没到生效起点，到了起点再来发布即可，这不是失败。
      </p>
    </section>
  );
}

// ——取消授权目录（0013）：哪种请求方被允许取消、依据哪条规则。缺行是真话（该请求方不许取消），零行是缺件——两者都由服务端说。

function CancellationAuthorityFields({
  draft,
  patch,
  patchRow,
  form,
  parties,
}: {
  draft: AuthorizationRuleDraft;
  patch: (change: Partial<AuthorizationRuleDraft>) => void;
  patchRow: (index: number, change: Partial<CancellationRowDraft>) => void;
  form: PublicationFormContext;
  parties: PartyOptionsState;
}) {
  const rowsBase = 'authorizationRule.cancellationAuthority';
  return (
    <section className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <h3 className={sectionTitle}>取消授权目录：请求方与规则引用</h3>
        <Button variant="outline" disabled={form.locked} onClick={() => patch({ rows: [...draft.rows, emptyCancellationRowDraft()] })}>
          加一行
        </Button>
      </div>
      <p className="text-[11px] text-idpxyz-textMuted">
        每行写一种可以取消的请求方，以及依据哪条规则；没写的请求方不能取消。至少要有一行，同一请求方只能有一行。
      </p>
      {/* 正文根 authorizationRule 认领了就要有处显（票 22 判据 3；票 17 评审点名）：服务端点到正文整体而不是目录时落在这里。 */}
      <Problems lines={form.problems['authorizationRule']} />
      <Problems lines={form.problems[rowsBase]} />
      <div className="flex flex-col gap-2">
        {draft.rows.map((row, index) => {
          const path = `${rowsBase}[${index}]`;
          return (
            <RowFrame
              key={index}
              path={path}
              problems={form.problems}
              locked={form.locked}
              onRemove={() => patch({ rows: draft.rows.filter((_, at) => at !== index) })}
            >
              <Field label="请求方 *" path={`${path}.party`} problems={form.problems}>
                <PartyPicker value={row.party} parties={parties} locked={form.locked} onChange={(party) => patchRow(index, { party })} />
              </Field>
              <Field label="规则引用 *" path={`${path}.rule`} problems={form.problems}>
                <Input
                  value={row.rule}
                  readOnly={form.locked}
                  className="font-mono text-[12px]"
                  placeholder="取消规则引用，如 CANCEL/customer-before-intake"
                  onChange={(event) => patchRow(index, { rule: event.target.value })}
                />
              </Field>
            </RowFrame>
          );
        })}
      </div>
    </section>
  );
}

// ——请求方下拉：选项 = 服务端词表的码 × 本页中文词表（partyOptionsOf）。读不到时只显占位、不内置码回退、不可选——
// 那堵墙前四口也答 403，这张表单本来就提交不了；选单第一项是空的「未选」，不预选任何一格。

// 稳定的函数引用：useLoaded 以它为依赖，写成内联箭头会每次渲染重取。
function loadAuthorizationVocabulary(): Promise<ApiResult<PublicationVocabularyResponseBody>> {
  return fetchPublicationVocabulary('AUTHORIZATION_RULE');
}

function usePartyOptions(): PartyOptionsState {
  return partyOptionsOf(useLoaded(loadAuthorizationVocabulary), cancellationPartyLabels);
}

function PartyPicker({
  value,
  parties,
  locked,
  onChange,
}: {
  value: string;
  parties: PartyOptionsState;
  locked: boolean;
  onChange: (party: string) => void;
}) {
  if (parties.kind === 'options') {
    return (
      <select className={selectClass} value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
        <option value="">未选</option>
        {parties.options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    );
  }
  return (
    <>
      <select className={selectClass} value="" disabled>
        <option value="">
          {parties.kind === 'loading'
            ? '正在读取可选项…'
            : parties.kind === 'unavailable'
              ? '暂时读不到可选项'
              : `暂时读不到可选项（${accessRefusalTitle(parties.kind)}）`}
        </option>
      </select>
      <span className="block text-[11px] text-idpxyz-textMuted mt-1">
        {parties.kind === 'loading'
          ? '正在读取可选的请求方。'
          : parties.kind === 'unconfigured'
            ? '暂时读不到可选的请求方：接入渠道还没有配置。配置好之前这张表单也提交不了。'
            : parties.kind === 'unavailable'
              ? '暂时读不到可选的请求方，请稍后重新打开本页签再试。'
              : `暂时读不到可选的请求方（${accessRefusalTitle(parties.kind)}）。`}
      </span>
    </>
  );
}