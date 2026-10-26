import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone } from '../moment';
import { creditRatioBaseLabels } from './presentation';
import { ExistingVersionsNote } from './ExistingVersionsNote';
import { policyKnownVersions } from './known-versions';
import { fetchPublicationVocabulary, type PublicationVocabularyResponseBody } from './publication-draft-api';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { listGroupLegalEntities, listCommercialPolicies } from './api';
import type { NewVersionStart } from './new-version';
import { Field, Problems, ReferencePicker, VocabularySelect, fieldLabel, useLoaded } from './PublicationFormFields';
import { WallTimeField, legalEntityPickerOptions } from './party-registration-fields';
import {
  creditPolicyFieldPaths,
  creditPolicyLocalProblems,
  creditPolicyPayloadOf,
  emptyCreditPolicyDraft,
  ratioBaseCodesOf,
  type CreditPolicyDraft,
} from './credit-policy-form';

const loadKnownVersions = () => listCommercialPolicies('CREDIT_POLICY');

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
export function CreditPolicyPublicationForm({
  onPublished,
  start,
}: {
  onPublished?: () => void;
  /** 基于目录里某一版预填（票 party-commercial-uiux/14）；换一次预填由页面换 key 重建本表单。 */
  start?: NewVersionStart<CreditPolicyDraft>;
}) {
  const [draft, setDraft] = useState<CreditPolicyDraft>(start?.draft ?? emptyCreditPolicyDraft());
  const patch = (change: Partial<CreditPolicyDraft>) => setDraft((current) => ({ ...current, ...change }));
  // 基数的封闭集只从服务端词表读口取（票 20，`kind=CREDIT_POLICY` 的 `ratioBase` 一集）；本表单不内置 POSTED_BALANCE /
  // PRIOR_PERIOD_CONFIRMED_CHARGES，内置一份就是同一封闭集的第二份写法（票 20 立票理由）。
  const vocabulary = useLoaded(loadCreditVocabulary);
  const ratioBaseCodes = vocabulary?.kind === 'outcome' ? ratioBaseCodesOf(vocabulary.body.sets) : null;
  // 时刻按操作者的显示时区填、送出 UTC（票 party-commercial-uiux/10），与登记表单同一条换算。
  const timeZone = currentDisplayTimeZone();
  // 这个对象已有哪些版本（票 party-commercial-uiux/15）：读不到就不提示，不报错。
  const knownAnswer = useLoaded(loadKnownVersions);
  const known = knownAnswer?.kind === 'outcome' ? policyKnownVersions(knownAnswer.body) : null;

  return (
    <PublicationDraftFlow
      kind="CREDIT_POLICY"
      title="发布信用政策版本"
      assemblePayload={() => creditPolicyPayloadOf(draft, timeZone)}
      localProblems={creditPolicyLocalProblems(draft, timeZone)}
      fieldPaths={creditPolicyFieldPaths}
      onPublished={onPublished}
      prefilledFrom={start?.from}
      prefilledCaution={start?.caution}
      versionNote={
        <ExistingVersionsNote
          known={known}
          objectId={draft.objectId}
          version={draft.version}
          timeZone={timeZone}
        />
      }
    >
      {(form) => (
        <CreditPolicyFields
          draft={draft}
          patch={patch}
          form={form}
          vocabulary={vocabulary}
          ratioBaseCodes={ratioBaseCodes}
          timeZone={timeZone}
        />
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
  timeZone,
}: {
  draft: CreditPolicyDraft;
  patch: (change: Partial<CreditPolicyDraft>) => void;
  form: PublicationFormContext;
  vocabulary: ApiResult<PublicationVocabularyResponseBody> | null;
  ratioBaseCodes: string[] | null;
  timeZone: string;
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
  const moment = (path: string, label: string, key: keyof CreditPolicyDraft, note?: string) => (
    <WallTimeField
      label={label}
      path={path}
      problems={problems}
      value={draft[key]}
      locked={locked}
      timeZone={timeZone}
      note={note}
      onChange={(value) => patch({ [key]: value } as Partial<CreditPolicyDraft>)}
    />
  );

  return (
    <div className="flex flex-col gap-4">
      <section className="flex flex-col gap-2">
        <p className={groupTitle}>版本信息</p>
        <Problems lines={problems['kind']} />
        <div className="grid grid-cols-2 gap-3">
          {field('objectId', '政策对象标识 *', 'objectId')}
          {field('version', '版本号 *', 'version')}
          {field('scope', '适用范围 *', 'scope')}
          <div />
          {moment('effectiveStartsAt', '版本有效区间起点 *', 'effectiveStartsAt')}
        </div>
        {form.versionNote}
      </section>

      <section className="flex flex-col gap-2">
        <p className={groupTitle}>正文（这一版的条款内容）</p>
        <div className="grid grid-cols-3 gap-3">
          {/* 从集团法人册选，不手敲标识（票 party-commercial-uiux/13）；选出来的只是引用，在不在册由服务端判。 */}
          <ReferencePicker
            label="责任法人 *"
            path="creditPolicy.legalEntity"
            problems={problems}
            value={draft.legalEntity}
            locked={locked}
            onChange={(legalEntity) => patch({ legalEntity })}
            load={listGroupLegalEntities}
            optionsOf={legalEntityPickerOptions}
            emptyNote="还没有登记任何责任法人；先在集团与法人页登记，再发布信用政策。"
            readFace="集团法人册"
          />
          {/* 授权层级与费用类型是开放引用集：没有词表可选，操作者照原词手填。 */}
          {field('creditPolicy.authorityLevel', '授权层级 *', 'authorityLevel', '按原词填写')}
          {field('creditPolicy.chargeType', '费用类型 *', 'chargeType', '按原词填写')}
        </div>

        <div>
          <span className={fieldLabel}>额度（金额或比例，只填一项）*</span>
          <div className="grid grid-cols-3 gap-3">
            {field('creditPolicy.limitMinor', '金额（最小货币单位）', 'limitMinor', '整数；0 是「授予零信用」', 'numeric')}
            {field('creditPolicy.limitRatioBasisPoints', '比例（基点，1% = 100）', 'limitRatioBasisPoints', '整数', 'numeric')}
            <VocabularySelect
              label="比例的基数（填了比例时必填）"
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
          {/* 只填一项、比例带基数、金额不带基数都由服务端构造门判，表单不替它挑；基数怎么折成额度归结算侧，这里不算。 */}
          <p className="text-[11px] text-idpxyz-textMuted mt-1">
            金额与比例只填一项。填比例时要选基数（按什么算比例），填金额时不选基数。是否填对由服务端在预览时检查。
          </p>
        </div>

        <div className="grid grid-cols-2 gap-3 items-end">
          {moment('creditPolicy.effectiveStartsAt', '正文有效区间起点 *', 'bodyEffectiveStartsAt')}
          {moment('creditPolicy.effectiveEndsAt', '正文有效区间止点', 'bodyEffectiveEndsAt', '留空即持续有效。')}
        </div>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            disabled={locked}
            onClick={() => patch({ bodyEffectiveStartsAt: draft.effectiveStartsAt, bodyEffectiveEndsAt: draft.effectiveEndsAt })}
          >
            有效区间从版本信息带入
          </Button>
          {/* 正文区间是 0020 正文自己的列，与版本区间各自送；带入只是抄一次文本，不是默认值。 */}
          <span className="text-[11px] text-idpxyz-textMuted">
            正文的有效区间属于这条信用政策本身，和版本的有效区间分开填；带入只是省一次手填。
          </span>
        </div>
      </section>
    </div>
  );
}
