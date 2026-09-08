import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import {
  creditPolicyFieldPaths,
  creditPolicyLocalProblems,
  creditPolicyPayloadOf,
  emptyCreditPolicyDraft,
  type CreditPolicyDraft,
} from './credit-policy-form';

/**
 * 信用政策版本的逐字段表单（票 admin-write-faces/16；ADR-0101 决定八本册选形）。五步由 PublicationDraftFlow
 * 走，本组件只摆壳五格与正文几格，并把服务端逐格问题挂到对应格旁。
 *
 * **额度是一个「金额 / 比例」二选一控件，恰一由服务端裁**：两格并排、都可填；两格都填或都空照样送预览，
 * 答回来的拒绝挂在「额度」组上（`creditPolicy.limit`）。零金额是「授予零信用」，照发 0。表单不算摘要、
 * 不收也不送批准人、不裁任何门；`CreditBasis` 的消费缝今天不存在，本表单只管发布，不替 SA 接（票面「硬句」）。
 */
export function CreditPolicyPublicationForm({ onPublished }: { onPublished?: () => void }) {
  const [draft, setDraft] = useState<CreditPolicyDraft>(emptyCreditPolicyDraft());
  const patch = (change: Partial<CreditPolicyDraft>) => setDraft((current) => ({ ...current, ...change }));

  return (
    <PublicationDraftFlow
      kind="CREDIT_POLICY"
      title="发布信用政策版本"
      assemblePayload={() => creditPolicyPayloadOf(draft)}
      localProblems={creditPolicyLocalProblems(draft)}
      fieldPaths={creditPolicyFieldPaths}
      onPublished={onPublished}
    >
      {(form) => <CreditPolicyFields draft={draft} patch={patch} form={form} />}
    </PublicationDraftFlow>
  );
}

const fieldLabel = 'block text-[12px] text-idpxyz-textMuted mb-1';
const groupTitle = 'text-[12px] font-medium text-idpxyz-text';

function CreditPolicyFields({
  draft,
  patch,
  form,
}: {
  draft: CreditPolicyDraft;
  patch: (change: Partial<CreditPolicyDraft>) => void;
  form: PublicationFormContext;
}) {
  const { problems, locked } = form;
  const field = (path: string, label: string, key: keyof CreditPolicyDraft, placeholder?: string) => (
    <Field
      label={label}
      value={draft[key]}
      problems={problems[path]}
      locked={locked}
      placeholder={placeholder}
      onChange={(value) => patch({ [key]: value } as Partial<CreditPolicyDraft>)}
    />
  );

  return (
    <div className="flex flex-col gap-4">
      <section className="flex flex-col gap-2">
        <p className={groupTitle}>
          版本壳 <span className="font-mono text-idpxyz-textMuted">kind = CREDIT_POLICY</span>
        </p>
        <ProblemLines lines={problems['kind']} />
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
          <div className="grid grid-cols-2 gap-3">
            <Field
              label="金额（最小货币单位）"
              value={draft.limitMinor}
              problems={problems['creditPolicy.limitMinor']}
              locked={locked}
              placeholder="整数；0 是「授予零信用」"
              inputMode="numeric"
              onChange={(value) => patch({ limitMinor: value })}
            />
            <Field
              label="比例（基点，1% = 100）"
              value={draft.limitRatioBasisPoints}
              problems={problems['creditPolicy.limitRatioBasisPoints']}
              locked={locked}
              placeholder="整数"
              inputMode="numeric"
              onChange={(value) => patch({ limitRatioBasisPoints: value })}
            />
          </div>
          <ProblemLines lines={problems['creditPolicy.limit']} />
          <p className="text-[11px] text-idpxyz-textMuted mt-1">
            恰一填写。两格都填或都空照样送预览，由服务端构造门裁、答在这一组上；表单不替它挑。
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

function Field({
  label,
  value,
  problems,
  locked,
  placeholder,
  inputMode,
  onChange,
}: {
  label: string;
  value: string;
  problems?: string[];
  locked: boolean;
  placeholder?: string;
  inputMode?: 'numeric';
  onChange: (value: string) => void;
}) {
  return (
    <label className="block">
      <span className={fieldLabel}>{label}</span>
      <Input
        value={value}
        readOnly={locked}
        inputMode={inputMode}
        className={`font-mono text-[13px] ${problems && problems.length > 0 ? 'border-idpxyz-danger' : ''}`}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
      <ProblemLines lines={problems} />
    </label>
  );
}

// 服务端（或本地编不进类型）点名的那一格的问题，逐条挂在格下；原话原样显示，不改写。
function ProblemLines({ lines }: { lines?: string[] }) {
  if (!lines || lines.length === 0) return null;
  return (
    <ul className="text-[11px] text-idpxyz-danger list-disc ml-4 mt-1">
      {lines.map((line) => (
        <li key={line}>{line}</li>
      ))}
    </ul>
  );
}
