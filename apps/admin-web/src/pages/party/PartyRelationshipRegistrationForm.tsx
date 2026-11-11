import { useEffect, useId, useState } from 'react';
import { ArrowLeftRight } from 'lucide-react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import {
  partyIdentityOutcomeLabels,
  type BusinessPartyListResponseBody,
  type PartyRelationshipRecord,
} from './api';
import { loadCustomerAccountSuggestions } from './reference-inputs';
import { problemNote } from './presentation';
import { Field, Problems, ReferencePickerFor, fieldLabel, selectClass, useLoaded } from './PublicationFormFields';
import { authorizedDeliveringPartyRole } from './authorized-delivery-scope';
import {
  AdvancedGroup,
  FormGroup,
  RevisionField,
  SnapshotJsonDetails,
  WallTimeField,
  businessPartyPickerOptions,
  customerAccountPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
import { advancedGroupOpen } from './registration-form';
import {
  deliveryScopeSpellingOf,
  emptyPartyRelationshipDraft,
  partyRelationshipFormIntro,
  partyRelationshipLocalProblems,
  partyRelationshipPayloadOf,
  partyRoleOptions,
  suggestedPartyRelationshipRevision,
  type PartyRelationshipDraft,
} from './party-relationship-form';
import './party-relationship-registration-form.css';

/**
 * 「建立合作关系」表单（票 admin-web-group-legal-entities/10 第 2 条）。本页几册里它价值最大：格里有封闭词与从册上选的引用，
 * 粘 JSON 时打错一个词只得到一个说不清的 400。壳在 useRegistrationForm，这里只摆本册的格。
 *
 * 格按人话排，头一行就是一句话（票 business-party-redesign/11）：〔担任角色的一方〕对〔另一方〕担任〔角色〕，照着读就是这段
 * 关系；下面是覆盖哪些业务、从什么时候到什么时候、依据哪份单据、批准了没有。关系编号仍要人起（服务端不生成），放在最后一格；
 * 更新序号由表单算好收在「高级」里。
 *
 * 表单开在两种地方，是同一副骨架：某家单位的页上（右栏的「关联」签、对象页的「合作关系」块）给 `self`，句子里它那一方锁死、
 * 可以换方向；合作关系视图里表的上方不给 `self`，两方都选，底下带 JSON 快照的折叠区（`snapshotJson`）。后一处宽得多，各组在
 * 宽处排四列，排法在 party-relationship-registration-form.css。
 *
 * **本组件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：双方在不在册、角色与双方是否匹配、
 * 区间是否倒置、修订连不连续，一律送上去让服务端答，答回来的问题码经 field-problem-text.ts 译成人话；本地只拦编码层（修订号与
 * 各时刻），纯函数在 party-relationship-form.ts。**可缺键缺席而不是零值**——结束时间留空即长期有效、不勾「已批准」即待确认，
 * 理由在那个文件头上。
 *
 * 双方用 ReferencePickerFor 从参与方册选：候选显名称 · 编码 · 状态，不按状态过滤，读面不可用退回手填。候选取的是页面持有的
 * 那份参与方列表答案，不另读——页面在登记册答 REGISTERED 后重取，刚登进去的那一家随新答案立刻出现在候选里（票 10 评审 N5）。
 *
 * 选了授权交付方时，适用范围一格换成「客户账户」「网点（可选）」两个输入：这一角色的范围服务端只认一种拼法，由
 * authorized-delivery-scope.ts 拼（票 ps-external-mark-relations/16）。别的角色照旧手填引用。
 */
export interface PartyRelationshipRegistrationFormProps {
  /** 页面已取回的关系列表（给修订号建议用）；没取到传 null，建议一律为 1。 */
  knownRelationships: readonly PartyRelationshipRecord[] | null;
  /** 页面持有的参与方列表答案（给双方候选用）；首取回来之前为 null。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  /**
   * 登记册答 `REGISTERED` 时回调，页面借它重取关系列表。逐字段那条路把送出的草稿交回，列表页据它选中刚登的那一段
   * （票 business-party-ux/06）；JSON 镜像那条路交 null。开在某家单位页上的调用方不用它也没关系。
   */
  onRegistered: (sent: PartyRelationshipDraft | null) => void;
  /** 草稿有没有没提交的输入；页面据此决定收掉表单之前要不要先确认。卸载时报 false。 */
  onUnsavedChange?: (unsaved: boolean) => void;
  /**
   * 表单开在哪一家单位的页上（右栏的「关联」签、对象页的「合作关系」块；票 business-party-edit-mode/03、business-party-redesign/11）：
   * 句子写成「本单位 对〔对方〕担任〔角色〕」，本单位那一方预填并锁死，只选对方与角色；「换方向」把本单位换到另一侧，登的就是
   * 别人对它担任角色的那一种。只在挂载时读；换一家单位由调用方换 key 重挂。不给即两方都要选（合作关系视图里的那一张）。
   */
  self?: { partyId: string; name: string };
  /**
   * 双方与角色三格每变一次报一次，卸载时报 null：页面拿去在关系图上先画出正在填的这一段。要给稳定的引用（状态的 set 函数），
   * 每次渲染换一个新函数会让它报个不停。
   */
  onPartiesChange?: (parties: { holder: string; counterparty: string; role: string } | null) => void;
  /** 「取消」。有没提交的输入时要不要先问一句由调用方定：表单收掉之后页面去哪，只有它知道。 */
  onCancel: () => void;
  /**
   * 表单底下带不带「粘贴登记快照 JSON」的折叠区。合作关系视图里的那一张带——这一册的 JSON 镜像只有这一个入口；开在某家单位
   * 页上的不带。
   */
  snapshotJson?: boolean;
  /**
   * 开头那段说明（party-relationship-form.ts 的 partyRelationshipFormIntro）调用方已经摆在别处时给——合作关系视图把它摆在
   * 段头上，表单省下一行的高。
   */
  introHidden?: boolean;
}

const info = moduleInfoById['business-parties'];
const kind = 'party-relationship';
const roleOptions = partyRoleOptions();
const advancedPaths = ['relationships[0].revision'] as const;

export function PartyRelationshipRegistrationForm({
  knownRelationships,
  parties,
  onRegistered,
  onUnsavedChange,
  self,
  onPartiesChange,
  onCancel,
  snapshotJson = false,
  introHidden = false,
}: PartyRelationshipRegistrationFormProps) {
  // 本单位站在这段关系的哪一侧：缺省是它担任角色（持有方）；「换方向」之后是别人对它担任（相对方）。
  const [selfSide, setSelfSide] = useState<'HOLDER' | 'COUNTERPARTY'>('HOLDER');
  const selfFields = (side: 'HOLDER' | 'COUNTERPARTY'): Partial<PartyRelationshipDraft> =>
    self === undefined ? {} : side === 'HOLDER' ? { holder: self.partyId } : { counterparty: self.partyId };
  const form = useRegistrationForm<PartyRelationshipDraft>({
    kind,
    // 空白草稿随方向走：只点了「换方向」、别的什么都没填，不算有没提交的输入。
    empty: () => ({ ...emptyPartyRelationshipDraft(), ...selfFields(selfSide) }),
    suggestion: (draft) => suggestedPartyRelationshipRevision(knownRelationships, draft.relationshipId),
    localProblems: partyRelationshipLocalProblems,
    payloadOf: partyRelationshipPayloadOf,
    landedOutcome: 'REGISTERED',
    onLanded: onRegistered,
  });
  const { draft, patch, problems, locked, timeZone, unsaved } = form;
  useEffect(() => {
    onUnsavedChange?.(unsaved);
  }, [onUnsavedChange, unsaved]);
  useEffect(() => () => onUnsavedChange?.(false), [onUnsavedChange]);
  useEffect(() => {
    onPartiesChange?.({ holder: draft.holder, counterparty: draft.counterparty, role: draft.role });
  }, [onPartiesChange, draft.holder, draft.counterparty, draft.role]);
  useEffect(() => () => onPartiesChange?.(null), [onPartiesChange]);
  // 一句话那一行里「对方」那一格落在草稿的哪一格上，随方向换。
  const otherParty = selfSide === 'HOLDER' ? draft.counterparty : draft.holder;
  const swapDirection = () => {
    if (self === undefined) return;
    const next = selfSide === 'HOLDER' ? 'COUNTERPARTY' : 'HOLDER';
    setSelfSide(next);
    patch(next === 'HOLDER' ? { holder: self.partyId, counterparty: otherParty } : { holder: otherParty, counterparty: self.partyId });
  };

  const known = knownRelationships?.find((row) => row.relationshipId === draft.relationshipId);
  const deliveryScope = draft.role === authorizedDeliveringPartyRole ? deliveryScopeSpellingOf(draft) : null;
  const accounts = useLoaded(loadCustomerAccountSuggestions);
  const accountOptions = accounts?.kind === 'outcome' ? customerAccountPickerOptions(accounts.body) : [];
  const accountListId = useId();
  // 高级组：null 即操作者还没点过，按 advancedGroupOpen 的缺省走（编号已在册时展开——这一笔是更新，让人看见序号）。
  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);
  const advancedOpen = advancedGroupOpen(advancedChoice, problems, advancedPaths, known !== undefined);
  const advancedSummary = form.revisionField.edited
    ? `更新序号改为 ${form.revisionField.value}`
    : known
      ? `这是关系 ${known.relationshipId} 的第 ${form.revisionField.suggestion} 次更新，序号已算好`
      : '首次登记，序号由系统填';

  // 一句话那一行的几件。这一行里各件一律与输入框同高（h-7）：句子里的字夹在控件之间，控件高矮不一，字就对不齐——原生下拉
  // 自己的上下留白比输入框高一截，所以在这里压到同高。
  const roleSelect = (
    <select
      className={`${selectClass} !h-7 !py-0`}
      value={draft.role}
      disabled={locked}
      aria-label="角色"
      onChange={(event) => patch({ role: event.target.value })}
    >
      <option value="">请选择</option>
      {roleOptions.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  );
  const selfBox = self ? (
    <div className="min-w-0">
      <span className={fieldLabel}>本单位</span>
      <div className="flex h-7 items-center rounded-md border border-idpxyz-border bg-idpxyz-editor px-3 type-value text-idpxyz-text" title={self.name}>
        <span className="truncate">{self.name}</span>
      </div>
    </div>
  ) : null;
  // 句子里的两格各落在草稿的哪一格上是定的：前一格是担任角色的一方（holder），后一格是它面对的那一方（counterparty）。
  // 给了 `self` 时它占其中一格、锁死，另一格的标签写「对方」；没给就两格都选，标签是「哪一方」「对哪一方」。
  const partyPicker = (side: 'holder' | 'counterparty') => (
    <ReferencePickerFor<BusinessPartyListResponseBody>
      label={self ? '对方 *' : side === 'holder' ? '哪一方 *' : '对哪一方 *'}
      path={`relationships[0].${side}`}
      problems={problems}
      value={draft[side]}
      locked={locked}
      onChange={(value) => patch(side === 'holder' ? { holder: value } : { counterparty: value })}
      answer={parties}
      optionsOf={businessPartyPickerOptions}
      emptyNote={`还没有登记任何往来单位；先在「新增往来单位」里登记${self ? '对方' : '双方'}，或直接填编码。`}
      readFace="往来单位"
    />
  );
  const selfHolds = selfSide === 'HOLDER';
  const partyCell = (side: 'holder' | 'counterparty') => (self && (side === 'holder') === selfHolds ? selfBox : partyPicker(side));
  const sentenceHint = !self
    ? '前一格是担任角色的单位，例如客户、承运商；后一格是它面对的那一方，通常是我们自己的公司主体，授权交付时是客户所属的单位。'
    : selfHolds
      ? '担任角色的是本单位。要登记别的单位对它担任角色，点「换方向」。'
      : '担任角色的是对方，本单位是它面对的那一方。要登记本单位担任角色，点「换方向」。';
  const sentenceWord = (word: string) => (
    <div className="shrink-0">
      <span className={fieldLabel} aria-hidden="true">
        &nbsp;
      </span>
      <span className="flex h-7 items-center type-value text-idpxyz-text">{word}</span>
    </div>
  );

  const body = (
    <>
          {introHidden ? null : <p className="type-body text-idpxyz-breadcrumb">{partyRelationshipFormIntro}</p>}

          {/* 两段：先说这段关系是谁对谁、什么角色、管哪些业务，再说从何时到何时、依据什么（票 business-party-ux/06）。 */}
          <FormGroup title="双方与角色">
            {/* 头一行是一句话，照着读就是这段关系——〔担任角色的一方〕对〔另一方〕担任〔角色〕。各格上面留同样高的一行标签位，
                句子里的字才与控件在同一条线上。两列的组里它占整行，四列时占一半（另一半是覆盖范围）。 */}
            <div className="flex flex-col gap-2 md:col-span-2">
              <div className="flex flex-wrap items-start gap-x-2.5 gap-y-3">
                <div className="min-w-[180px] flex-[1_1_180px]">{partyCell('holder')}</div>
                {sentenceWord('对')}
                <div className="min-w-[180px] flex-[1_1_180px]">{partyCell('counterparty')}</div>
                {sentenceWord('担任')}
                <div className="min-w-[150px] flex-[0_1_210px]">
                  <Field label="角色 *" path="relationships[0].role" problems={problems}>
                    {roleSelect}
                  </Field>
                </div>
                {self ? (
                  <div className="shrink-0">
                    <span className={fieldLabel} aria-hidden="true">
                      &nbsp;
                    </span>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 type-body text-idpxyz-breadcrumb"
                      disabled={locked}
                      onClick={swapDirection}
                      title={selfHolds ? '换成别的单位对本单位担任角色' : '换成本单位对别的单位担任角色'}
                    >
                      <ArrowLeftRight className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                      换方向
                    </Button>
                  </div>
                ) : null}
              </div>
              {/* 本单位那一格是锁死的，登记册在它那一格上报的问题没有控件可挂，显在句子下面。 */}
              {self ? <Problems lines={problems[selfHolds ? 'relationships[0].holder' : 'relationships[0].counterparty']} /> : null}
              <p className="type-body leading-5 text-idpxyz-breadcrumb">{sentenceHint}</p>
            </div>

            {/* 覆盖范围这一格带一个钩子类：宽处四列时它占两列（party-relationship-registration-form.css）。 */}
            <div className="party-relationship-scope min-w-0">
              {deliveryScope ? (
                <Field label="覆盖范围 *" path="relationships[0].scope" problems={problems} as="div">
                  <div className="grid grid-cols-2 gap-2">
                    <label className="block">
                      <span className={fieldLabel}>客户账户 *</span>
                      {/* 账户册读口分页：能直接输入、带第一页候选作提示，不做成只列得出第一页的选单（票 party-commercial-uiux/13）。 */}
                      <Input
                        value={draft.scopeCustomerAccount}
                        readOnly={locked}
                        list={accountOptions.length > 0 ? accountListId : undefined}
                        className="font-mono type-value"
                        placeholder="可从提示里选，也可直接输入账户编码"
                        onChange={(event) => patch({ scopeCustomerAccount: event.target.value })}
                      />
                      {accountOptions.length > 0 ? (
                        <datalist id={accountListId}>
                          {accountOptions.map((option) => (
                            <option key={option.value} value={option.value} label={option.label} />
                          ))}
                        </datalist>
                      ) : null}
                    </label>
                    <label className="block">
                      <span className={fieldLabel}>网点（可选）</span>
                      <Input
                        value={draft.scopeNode}
                        readOnly={locked}
                        className="font-mono type-value"
                        placeholder="留空即不限网点"
                        onChange={(event) => patch({ scopeNode: event.target.value })}
                      />
                    </label>
                  </div>
                  <span className="block type-body text-idpxyz-breadcrumb mt-1">
                    这家单位替哪个客户账户交付，可以再限定到一个网点。
                  </span>
                </Field>
              ) : (
                <Field label="覆盖范围 *" path="relationships[0].scope" problems={problems}>
                  <Input
                    value={draft.scope}
                    readOnly={locked}
                    className="font-mono type-value"
                    placeholder="如 全部业务，或某条线路、某个客户账户"
                    onChange={(event) => patch({ scope: event.target.value })}
                  />
                  <span className="block type-body text-idpxyz-breadcrumb mt-1">这段关系管哪些业务；按公司约定的写法填。</span>
                </Field>
              )}
            </div>
          </FormGroup>

          <FormGroup title="有效期与依据">
            <WallTimeField
              label="开始时间 *"
              path="relationships[0].effectiveStartsAt"
              problems={problems}
              value={draft.effectiveStartsAt}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveStartsAt) => patch({ effectiveStartsAt })}
            />

            <WallTimeField
              label="结束时间"
              path="relationships[0].effectiveEndsAt"
              problems={problems}
              value={draft.effectiveEndsAt}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveEndsAt) => patch({ effectiveEndsAt })}
              note="留空即长期有效，直到撤销或到期。"
            />

            <Field label="来源单据 *" path="relationships[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono type-value"
                placeholder="合同编号、邮件或审批单号"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>

            <Field label="关系编号 *" path="relationships[0].relationshipId" problems={problems}>
              <Input
                value={draft.relationshipId}
                readOnly={locked}
                className="font-mono type-value"
                placeholder="如 REL-SHIPPER-01-CUSTOMER"
                onChange={(event) => patch({ relationshipId: event.target.value })}
              />
              <span className="block type-body text-idpxyz-breadcrumb mt-1">
                {known
                  ? `这个编号已登记；这次保存会更新那段关系。`
                  : '给这段关系起一个内部编号，登记后不能改。'}
              </span>
            </Field>
          </FormGroup>

          {/* 批准与「高级」两块各自收着时都只有一行：窄处上下排，宽处并排（party-relationship-registration-form.css）。 */}
          <div className="party-relationship-extras flex flex-col gap-4">
            {/* 批准是一件事实而不是几格文本：勾选框才是「有没有批准」的声明，不勾时批准那几格不进载荷、残字也不进。 */}
            <div className="party-relationship-approval flex flex-col gap-3 rounded border border-idpxyz-border p-3">
              <label className="flex items-center gap-2 type-value text-idpxyz-text">
                <input
                  type="checkbox"
                  checked={draft.approved}
                  disabled={locked}
                  onChange={(event) => patch({ approved: event.target.checked })}
                />
                已批准
                <span className="type-body text-idpxyz-breadcrumb">不勾则先作为待确认关系，批准后再保存一次</span>
              </label>
              {draft.approved ? (
                <div className="grid grid-cols-2 gap-3">
                  <Field label="批准单据 *" path="relationships[0].approval.reference" problems={problems}>
                    <Input
                      value={draft.approvalReference}
                      readOnly={locked}
                      className="font-mono type-value"
                      placeholder="审批单号或批准邮件"
                      onChange={(event) => patch({ approvalReference: event.target.value })}
                    />
                  </Field>
                  <WallTimeField
                    label="批准时间 *"
                    path="relationships[0].approval.approvedAt"
                    problems={problems}
                    value={draft.approvedAt}
                    locked={locked}
                    timeZone={timeZone}
                    onChange={(approvedAt) => patch({ approvedAt })}
                  />
                </div>
              ) : null}
            </div>

            <AdvancedGroup open={advancedOpen} onToggle={() => setAdvancedChoice(!advancedOpen)} summary={advancedSummary}>
              <RevisionField
                label="更新序号"
                path="relationships[0].revision"
                problems={problems}
                {...form.revisionField}
                note="系统按已登记的记录自动算好，一般不用改。保存时提示序号对不上，说明列表已过时，刷新后再试。"
              />
            </AdvancedGroup>
          </div>

    </>
  );
  // 动作行左边说话、右边「取消 / 保存」，主动作靠右，与对象页编辑态的页脚同一排法。JSON 快照的折叠区排在动作行之后：它自己带
  // 提交钮，是另一条路，不归上面那枚「保存」管。
  return (
    <div className="party-relationship-form flex flex-col gap-4">
      {body}
      <div className="flex flex-wrap items-start justify-between gap-3 border-t border-idpxyz-border pt-3">
        <div className="min-w-0 flex-1 type-body">
          <RegistrationAnswerNote state={form.state} owner={info.owner} outcomeLabels={partyIdentityOutcomeLabels} problemNote={problemNote} />
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button variant="outline" size="sm" className="type-body" disabled={locked} onClick={onCancel}>
            取消
          </Button>
          <Button variant="default" size="sm" className={`${primaryActionClass} type-body`} onClick={form.send} disabled={locked || !form.canSend}>
            {locked ? '保存中…' : '保存'}
          </Button>
        </div>
      </div>
      {snapshotJson ? <SnapshotJsonDetails kind={kind} submit={form.submit} /> : null}
    </div>
  );
}
