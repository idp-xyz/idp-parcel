import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import { creditRatioBaseLabels } from './presentation';
import { fetchPublicationVocabulary, type PublicationVocabularyResponseBody } from './publication-draft-api';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { Field, Problems, VocabularySelect, fieldLabel, useLoaded } from './PublicationFormFields';
import {
  creditPolicyFieldPaths,
  creditPolicyLocalProblems,
  creditPolicyPayloadOf,
  emptyCreditPolicyDraft,
  ratioBaseCodesOf,
  type CreditPolicyDraft,
} from './credit-policy-form';

/**
 * 信用政策版本的逐字段表单（票 admin-write-faces/16；ADR-0101 决定八本册选形）。五步由 PublicationDraftFlow
 * 走，本组件只摆壳五格与正文几格，并把服务端逐格问题挂到对应格旁。
 *
 * **额度是一个「金额 / 比例」二选一控件，恰一由服务端裁**：两格并排、都可填；两格都填或都空照样送预览，
 * 答回来的拒绝挂在「额度」组上（`creditPolicy.limit`）。零金额是「授予零信用」，照发 0。比例额度的基数是第三格
 * （ADR-0129）：封闭集下拉，码只从词表读口取，不内置、不预选；比例在场而基数缺席、金额在场而基数在场都照发，
 * 由服务端点名 `creditPolicy.ratioBase`。表单不算摘要、不收也不送批准人、不裁任何门；本表单只管发布，
 * 基数怎么折成额度是 SA 的事，不在这里替它算（票面「硬句」）。
 */
export function CreditPolicyPublicationForm({ onPublished }: { onPublished?: () => void }) {
  const [draft, setDraft] = useState<CreditPolicyDraft>(emptyCreditPolicyDraft());
  const patch = (change: Partial<CreditPolicyDraft>) => setDraft((current) => ({ ...current, ...change }));
  // 基数的封闭集只从服务端词表读口取（票 20，`kind=CREDIT_POLICY` 的 `ratioBase` 一集）；本表单不内置 POSTED_BALANCE /
  // PRIOR_PERIOD_CONFIRMED_CHARGES，内置一份就是同一封闭集的第二份写法（票 20 立票理由）。
  const vocabulary = useLoaded(loadCreditVocabulary);
  const ratioBaseCodes = vocabulary?.kind === 'outcome' ? ratioBaseCodesOf(vocabulary.body.sets) : null;

  return (
    <PublicationDraftFlow
      kind="CREDIT_POLICY"
      title="发布信用政策版本"
      assemblePayload={() => creditPolicyPayloadOf(draft)}
      localProblems={creditPolicyLocalProblems(draft)}
      fieldPaths={creditPolicyFieldPaths}
      onPublished={onPublished}
    >
      {(form) => (
        <CreditPolicyFields draft={draft} patch={patch} form={form} vocabulary={vocabulary} ratioBaseCodes={ratioBaseCodes} />
      )}
    </PublicationDraftFlow>
  );
}

const groupTitle = 'text-[12px] font-medium text-idpxyz-text';

function loadCreditVocabulary(): Promise<ApiResult<PublicationVocabularyResponseBody>> {
  return fetchPublicationVocabulary('CREDIT_POLICY');
}

function CreditPolicyFields({
  draft,
  patch,
  form,
  vocabulary,
  ratioBaseCodes,
}: {
  draft: CreditPolicyDraft;
  patch: (change: Partial<CreditPolicyDraft>) => void;
  form: PublicationFormContext;
  vocabulary: ApiResult<PublicationVocabularyResponseBody> | null;
  ratioBaseCodes: string[] | null;
}) {
  const { problems, locked } = form;
  // 本册的格全是文本框：一格 = 共享 Field + Input。被点名的格连输入框边框一起变红是本册此前就有的提示，抬共享层时照留
  // ——共享 Field 只管标签与问题行，框怎么显归各册自己的控件。
  const field = (
    path: string,
    label: string,
    key: keyof CreditPolicyDraft,
    placeholder?: string,
    inputMode?: 'numeric',
  ) => (
    <Field label={label} path={path} problems={problems}>
      <Input
        value={draft[key]}
        readOnly={locked}
        inputMode={inputMode}
        className={`font-mono text-[13px] ${(problems[path] ?? []).length > 0 ? 'border-idpxyz-danger' : ''}`}
        placeholder={placeholder}
        onChange={(event) => patch({ [key]: event.target.value } as Partial<CreditPolicyDraft>)}
      />
    </Field>
  );

  return (
    <div className="flex flex-col gap-4">
      <section className="flex flex-col gap-2">
        <p className={groupTitle}>
          版本壳 <span className="font-mono text-idpxyz-textMuted">kind = CREDIT_POLICY</span>
        </p>
        <Problems lines={problems['kind']} />
        <div className="grid grid-cols-2 gap-3">
          {field('objectId', '政策对象标识 *', 'objectId')}
          {field('version', '版本号 *', 'version')}
          {field('scope', '适用范围 *', 'scope')}
          <div />
          {field('effectiveStartsAt', '壳有效区间起点 *', 'effectiveStartsAt', 'RFC 3339 或 YYYY-MM-DD（只到天补成当天零点 UTC）')}
          {field('effectiveEndsAt', '壳有效区间止点', 'effectiveEndsAt', '留空即无上界')}
        </div>
      </section>

      <section className="flex flex-col gap-2">
        <p className={groupTitle}>正文（信用政策册，0020）</p>
        <div className="grid grid-cols-3 gap-3">
          {field('creditPolicy.legalEntity', '责任法人 *', 'legalEntity')}
          {field('creditPolicy.authorityLevel', '授权层级 *', 'authorityLevel', '开放引用集，按原词填')}
          {field('creditPolicy.chargeType', '费用类型 *', 'chargeType', '开放引用集，按原词填')}
        </div>

        <div>
          <span className={fieldLabel}>额度（金额 / 比例，恰一）*</span>
          <div className="grid grid-cols-3 gap-3">
            {field('creditPolicy.limitMinor', '金额（最小货币单位）', 'limitMinor', '整数；0 是「授予零信用」', 'numeric')}
            {field('creditPolicy.limitRatioBasisPoints', '比例（基点，1% = 100）', 'limitRatioBasisPoints', '整数', 'numeric')}
            <VocabularySelect
              label="比例的基数（比例在场时必填）"
              path="creditPolicy.ratioBase"
              setName="ratioBase"
              kind="CREDIT_POLICY"
              codes={ratioBaseCodes}
              vocabulary={vocabulary}
              labels={creditRatioBaseLabels}
              problems={problems}
              value={draft.ratioBase}
              locked={locked}
              onChange={(ratioBase) => patch({ ratioBase })}
            />
          </div>
          <Problems lines={problems['creditPolicy.limit']} />
          <p className="text-[11px] text-idpxyz-textMuted mt-1">
            金额 / 比例恰一填写。两格都填或都空照样送预览，由服务端构造门裁、答在这一组上；表单不替它挑。比例要带基数
            （相对于什么的比例），缺了或金额带了基数由服务端点名基数那一格；基数怎么折成额度归结算侧，这里不算。
          </p>
        </div>

        <div className="grid grid-cols-2 gap-3 items-end">
          {field('creditPolicy.effectiveStartsAt', '正文有效区间起点 *', 'bodyEffectiveStartsAt', 'RFC 3339 或 YYYY-MM-DD')}
          {field('creditPolicy.effectiveEndsAt', '正文有效区间止点', 'bodyEffectiveEndsAt', '留空即无上界')}
        </div>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            disabled={locked}
            onClick={() => patch({ bodyEffectiveStartsAt: draft.effectiveStartsAt, bodyEffectiveEndsAt: draft.effectiveEndsAt })}
          >
            正文区间抄壳区间
          </Button>
          <span className="text-[11px] text-idpxyz-textMuted">
            正文区间是信用政策自己的（0020 正文列），与壳区间各自送；抄过来是省一次手填，不是默认。
          </span>
        </div>
      </section>
    </div>
  );
}
