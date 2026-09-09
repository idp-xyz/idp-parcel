import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import { cancellationPartyLabels } from './presentation';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { fetchPublicationVocabulary, type PublicationVocabularyResponseBody } from './publication-draft-api';
import { Field, Problems, RowFrame, selectClass, useLoaded } from './PublicationFormFields';
import {
  authorizationRuleFieldPaths,
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

  return (
    <PublicationDraftFlow
      kind="AUTHORIZATION_RULE"
      title="发布授权规则版本"
      assemblePayload={() => payloadOf(draft)}
      fieldPaths={authorizationRuleFieldPaths(draft)}
      onPublished={onPublished}
    >
      {(form) => (
        <div className="flex flex-col gap-5">
          <ShellFields draft={draft} patch={patch} form={form} />
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
}: {
  draft: AuthorizationRuleDraft;
  patch: (change: Partial<AuthorizationRuleDraft>) => void;
  form: PublicationFormContext;
}) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className={sectionTitle}>版本壳</h3>
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
        <Field label="生效起点 *" path="effectiveStartsAt" problems={form.problems}>
          <Input
            value={draft.effectiveStartsAt}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="RFC 3339，如 2026-10-01T00:00:00Z"
            onChange={(event) => patch({ effectiveStartsAt: event.target.value })}
          />
        </Field>
        <Field label="生效止点（留空即无上界）" path="effectiveEndsAt" problems={form.problems}>
          <Input
            value={draft.effectiveEndsAt}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="RFC 3339；留空即无上界"
            onChange={(event) => patch({ effectiveEndsAt: event.target.value })}
          />
        </Field>
      </div>
      <p className="text-[11px] text-idpxyz-textMuted">
        起点晚于此刻时载体可存、可批，发布会答「等待生效边界」——到界再来发布，不是失败。
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
        <h3 className={sectionTitle}>取消授权目录：请求方 × 规则引用</h3>
        <Button variant="outline" disabled={form.locked} onClick={() => patch({ rows: [...draft.rows, emptyCancellationRowDraft()] })}>
          加一行
        </Button>
      </div>
      <p className="text-[11px] text-idpxyz-textMuted">
        有行即该请求方被允许取消并带规则引用；没写的请求方就是不许取消——那是目录说出的真话。一行都没有是缺件不是
        「谁都不许」，同一请求方第二行是冲突：两条都由服务端在预览上答，表单不代判。请求方的封闭集由服务端词表供，
        表单不内置、不预选。
      </p>
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
          {parties.kind === 'loading' ? '正在读词表…' : parties.kind === 'unconfigured' ? '词表未就绪（接入渠道未配置）' : '词表读不到'}
        </option>
      </select>
      <span className="block text-[11px] text-idpxyz-textMuted mt-1">
        {parties.kind === 'loading'
          ? '请求方的封闭集正在从服务端词表读口读取。'
          : parties.kind === 'unconfigured'
            ? '词表读口在接入渠道未配置那堵墙前（403）；表单不内置任何码顶替，四口同在墙前，此时本表单也提交不了。'
            : '词表读口未形成答案；表单不内置任何码顶替，稍后重开本签再试。'}
      </span>
    </>
  );
}