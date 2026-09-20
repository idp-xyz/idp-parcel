import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import {
  commercialRegistrationEndpoints,
  listBusinessParties,
  listCustomerAccounts,
  listGroupLegalEntities,
  partyIdentityOutcomeLabels,
  type BusinessPartyListResponseBody,
  type CustomerAccountListResponseBody,
  type GroupLegalEntityListResponseBody,
} from './api';
import { problemNote, registrationTitles, type IdentityKind } from './presentation';
import {
  Field,
  ReferencePickerFor,
  selectClass,
  type PickerOption,
  type ReferencePickerFaceProps,
} from './PublicationFormFields';
import {
  RevisionField,
  SnapshotJsonDetails,
  WallTimeField,
  businessPartyPickerOptions,
  customerAccountPickerOptions,
  legalEntityPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
import {
  emptyIdentityDeactivationDraft,
  identityDeactivationLocalProblems,
  identityDeactivationPayloadOf,
  identityKindOptions,
  identityTargetOf,
  isIdentityKind,
  suggestedDeactivationRevision,
  type IdentityDeactivationDraft,
  type RevisionedIdentity,
} from './identity-deactivation-form';

/**
 * 身份停用的逐字段表单（票 admin-web-group-legal-entities/10 第 3 条）。停用口一个命令带种类，法人与客户账户的停用
 * 也走本签（页面上登记签那段头注说明为什么它摆在本页）；结果分别显示在集团与法人页、客户与合同页的
 * 「客户账户」签，本页读得见的是参与方身份那一册。壳在 useRegistrationForm，这里只摆本册的格。
 *
 * **本组件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：身份在不在册、已否停用、修订是否
 * 错位，一律送上去让服务端答；本地只拦编码层（修订号、停用时刻），纯函数在 identity-deactivation-form.ts。
 *
 * 标识按种类从对应的册上选，候选与修订建议取的是**同一份答案**（票 13 第 5 条）：参与方册用页面持有的那份，不另读；
 * 法人册与客户账户册在选中那一种类时才读、读一次，Picker 收这份答案（ReferencePickerFor）而不自己再读——此前一挂
 * 即读两册、Picker 又各自读一遍，同一册在一张表单里被读两三遍。种类 → 读口 / 候选 / 读面名 / 投影合成一张按
 * IdentityKind 键的表（第 2 条），三分支 JSX 与 switch 收掉；词表多一格，表编不过。答案与取出它的那一册结成对走
 * （票 14 第 4 条，KindRegister 头注）。
 */
export interface IdentityDeactivationFormProps {
  /** 页面持有的参与方列表答案（种类为业务参与方时给候选与修订号建议用）；首取回来之前为 null。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  /** 登记册答 `DEACTIVATED` 时回调，页面借它重取参与方列表让「已停用」+ 时点 + 依据立刻可见。 */
  onDeactivated: () => void;
}

const info = moduleInfoById['business-parties'];
const kind = 'identity-deactivation';
const endpoint = `POST ${commercialRegistrationEndpoints[kind]}`;
const kindOptions = identityKindOptions();

const pickerNotes = {
  emptyNote: '对应册今天为空；先登记身份，或手填标识由服务端判。',
  optionsNote: '候选不按状态过滤：已停用的再停一次由服务端答，表单不拦。',
} as const;

/** 种类 → 该册答案体的形状。三册体形各异（parties / entities / accounts），一个种类只在这里说一次它的体是什么。 */
interface RegisterBodyOf {
  BUSINESS_PARTY: BusinessPartyListResponseBody;
  LEGAL_ENTITY: GroupLegalEntityListResponseBody;
  CUSTOMER_ACCOUNT: CustomerAccountListResponseBody;
}

/**
 * 一种身份对应的那一册：读口、候选转写、读面名、投成「标识 + 修订」，按种类 K 收窄，体形从 RegisterBodyOf 取。
 *
 * 答案不单独在表外流动：读回来或从页面接过来的答案，一律由**取出它的那一册**结成 RegisterAnswer 一起走（票 14 第 4 条）。
 * 此前 Body 在表外擦成 unknown、答案与册各按 selectedKind 取——配对没人守，而且真的错过：换种类那一帧，`patch({ kind })`
 * 触发的渲染先于 effect 的 `setLoaded(null)`，loaded 还是上一册的答案，配到新册的投影上会在 `body.accounts.map` 一类
 * 取键处抛。结成对之后种类对不上就当没取到，类型与那一帧一起守住。
 */
interface KindRegister<K extends IdentityKind> {
  kind: K;
  readFace: string;
  optionsOf(body: RegisterBodyOf[K]): PickerOption[];
  targetsOf(body: RegisterBodyOf[K]): readonly RevisionedIdentity[];
  /** 读这一册，答案连同本册交回。 */
  load(): Promise<RegisterAnswer<K>>;
  /** 答案已在手上（参与方册由页面持有）或还没有（null）时，与本册结成对。 */
  withAnswer(answer: ApiResult<RegisterBodyOf[K]> | null): RegisterAnswer<K>;
}

/** 一册与它的答案，体形同一个 K；只由 KindRegister 自己结成——拆开各自传，就回到擦型那条路。 */
interface RegisterAnswer<K extends IdentityKind> {
  register: KindRegister<K>;
  answer: ApiResult<RegisterBodyOf[K]> | null;
}

/** 任一册的配对：状态里放的是它，哪一册由 register.kind 说。 */
type AnyRegisterAnswer = { [K in IdentityKind]: RegisterAnswer<K> }[IdentityKind];

function kindRegister<K extends IdentityKind>(
  kind: K,
  entry: {
    readFace: string;
    load(): Promise<ApiResult<RegisterBodyOf[K]>>;
    optionsOf(body: RegisterBodyOf[K]): PickerOption[];
    targetsOf(body: RegisterBodyOf[K]): readonly RevisionedIdentity[];
  },
): KindRegister<K> {
  const self: KindRegister<K> = {
    kind,
    readFace: entry.readFace,
    optionsOf: entry.optionsOf,
    targetsOf: entry.targetsOf,
    load: () => entry.load().then((answer) => self.withAnswer(answer)),
    withAnswer: (answer) => ({ register: self, answer }),
  };
  return self;
}

/** 参与方册那一行的 load 不会被调到：页面已持有那份答案传进来（props.parties），再读一次就是第 5 条要收的那次。 */
const kindRegisters: { [K in IdentityKind]: KindRegister<K> } = {
  BUSINESS_PARTY: kindRegister('BUSINESS_PARTY', {
    readFace: '参与方册',
    load: listBusinessParties,
    optionsOf: businessPartyPickerOptions,
    targetsOf: (body) => body.parties.map(identityTargetOf.BUSINESS_PARTY),
  }),
  LEGAL_ENTITY: kindRegister('LEGAL_ENTITY', {
    readFace: '法人册',
    load: listGroupLegalEntities,
    optionsOf: legalEntityPickerOptions,
    targetsOf: (body) => body.entities.map(identityTargetOf.LEGAL_ENTITY),
  }),
  CUSTOMER_ACCOUNT: kindRegister('CUSTOMER_ACCOUNT', {
    readFace: '客户账户册',
    load: listCustomerAccounts,
    optionsOf: customerAccountPickerOptions,
    targetsOf: (body) => body.accounts.map(identityTargetOf.CUSTOMER_ACCOUNT),
  }),
};

/** 配对里的答案投成「标识 + 修订」：没取到 / 不是业务答案 → null，建议退回 1。 */
function pairedTargetsOf<K extends IdentityKind>({ register, answer }: RegisterAnswer<K>): readonly RevisionedIdentity[] | null {
  return answer?.kind === 'outcome' ? register.targetsOf(answer.body) : null;
}

/**
 * 配对里那一册的候选：候选转写与读面名都取自配对里的册，不再按种类另查一遍。面上那几格（标签、路径、问题表、措辞）
 * 与体形无关，Omit 掉带 Body 的 optionsOf 之后 ReferencePickerFaceProps 的类型参数填什么都一样。
 */
function RegisterPicker<K extends IdentityKind>({
  paired,
  ...face
}: { paired: RegisterAnswer<K> } & Omit<ReferencePickerFaceProps<unknown>, 'optionsOf' | 'readFace'>) {
  return (
    <ReferencePickerFor<RegisterBodyOf[K]>
      {...face}
      answer={paired.answer}
      optionsOf={paired.register.optionsOf}
      readFace={paired.register.readFace}
    />
  );
}

export function IdentityDeactivationForm({ parties, onDeactivated }: IdentityDeactivationFormProps) {
  // 本表单自读那一册的答案，连同是哪一册（参与方册除外——它从页面来）。换种类即换册，上一册的答案不留：与「换种类连
  // 标识一起清」同一条纪律，上一册的候选留在下拉里会被送到另一册去查。
  const [loaded, setLoaded] = useState<AnyRegisterAnswer | null>(null);
  // 本表单自读那一册的重取序号（票 13 第 7 条）：停用法人 / 客户账户落册后按种类重读对应册，同册再停一个时候选与建议
  // 按新册面算，不按停用前那份。参与方册的重取仍由页面做（onDeactivated）。
  const [reloadKey, setReloadKey] = useState(0);
  // 选中种类那一册与它的答案。换种类之后、effect 重读回来之前那一帧 loaded 还是上一册的：种类对不上就当这一册还没取到，
  // 不拿上一册的答案配这一册的投影（KindRegister 头注说的那次抛就在这一帧）。
  const selectedRegisterFor = (kind: string): AnyRegisterAnswer | null => {
    if (!isIdentityKind(kind)) return null;
    if (kind === 'BUSINESS_PARTY') return kindRegisters.BUSINESS_PARTY.withAnswer(parties);
    return loaded?.register.kind === kind ? loaded : kindRegisters[kind].withAnswer(null);
  };
  const targetsFor = (kind: string): readonly RevisionedIdentity[] | null => {
    const paired = selectedRegisterFor(kind);
    return paired === null ? null : pairedTargetsOf(paired);
  };

  const form = useRegistrationForm<IdentityDeactivationDraft>({
    kind,
    empty: emptyIdentityDeactivationDraft,
    suggestion: (draft) => suggestedDeactivationRevision(targetsFor(draft.kind), draft.id),
    localProblems: identityDeactivationLocalProblems,
    payloadOf: identityDeactivationPayloadOf,
    // 答 `DEACTIVATED`（PartyRegistryOutcome 里 PartyIdentityDeactivated 的线上名）才重取；`册上没有这一个身份`与
    // 修订错位都没写进去。按送出的种类重读对应册（第 7 条）：参与方册由页面重取，法人册 / 客户账户册本表单自己重读。
    // JSON 镜像那条路交回 null（快照是未译的 JSON，壳不解它，分不出种类）——两边都重取：各是一个 GET，为分种类去解一份
    // unknown 不值。
    landedOutcome: 'DEACTIVATED',
    onLanded: (sent) => {
      if (sent === null || sent.kind === 'BUSINESS_PARTY') onDeactivated();
      if (sent === null || sent.kind !== 'BUSINESS_PARTY') setReloadKey((value) => value + 1);
    },
  });
  const { draft, patch, problems, locked, timeZone } = form;
  const selectedKind = isIdentityKind(draft.kind) ? draft.kind : null;

  useEffect(() => {
    // 参与方册不在这里读：页面持有的那份答案就是它。其余两册在选中时读一次、落册后重读一次；未回的旧请求按 cancelled 丢。
    setLoaded(null);
    if (selectedKind === null || selectedKind === 'BUSINESS_PARTY') return;
    let cancelled = false;
    const loading: Promise<AnyRegisterAnswer> = kindRegisters[selectedKind].load();
    void loading.then((next) => {
      if (!cancelled) setLoaded(next);
    });
    return () => {
      cancelled = true;
    };
  }, [selectedKind, reloadKey]);

  const selected = selectedRegisterFor(draft.kind);
  const known = targetsFor(draft.kind)?.find((row) => row.id === draft.id);
  const pickerProps = {
    label: '身份标识 *',
    path: 'deactivations[0].id',
    problems,
    value: draft.id,
    locked,
    onChange: (id: string) => patch({ id }),
    ...pickerNotes,
  };

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-xs text-idpxyz-textMuted">
            停用形成修订链上新的一笔，原修订不被改写；自停用时点起该身份不再支持新的商业决定。修订号是你声明自己看到的
            册面（册上最新 + 1），错位说明册面已被并发推进或意图已陈旧，由服务端拒而不是替你猜。关系不在这里停：关系的
            终止走撤销 / 到期 / 替代。提交打到 <span className="font-mono">{endpoint}</span>；租户不在表单上。
          </p>

          <div className="grid grid-cols-2 gap-3">
            <Field label="身份种类 *" path="deactivations[0].kind" problems={problems}>
              <select
                className={selectClass}
                value={draft.kind}
                disabled={locked}
                // 换种类连标识一起清：上一册的标识留在格里，会被送到另一册去查。
                onChange={(event) => patch({ kind: event.target.value, id: '' })}
              >
                <option value="">未选</option>
                {kindOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">
                法人与客户账户的停用也走本签；结果分别显示在集团与法人页、客户与合同页的「客户账户」签。
              </span>
            </Field>

            {selected !== null ? (
              // key 带种类：换种类即换 Picker，上一册的候选与「不在读面上」那一项不留在下拉里。
              <RegisterPicker key={selected.register.kind} paired={selected} {...pickerProps} />
            ) : (
              <Field label="身份标识 *" path="deactivations[0].id" problems={problems}>
                <Input
                  value={draft.id}
                  readOnly={locked}
                  className="font-mono text-[13px]"
                  placeholder="先选身份种类，再从对应册上选"
                  onChange={(event) => patch({ id: event.target.value })}
                />
              </Field>
            )}

            <RevisionField
              path="deactivations[0].revision"
              problems={problems}
              {...form.revisionField}
              note={
                // 落点显的就是建议值本身（同一份 rows、同一个标识算出来的），不在这里再加一次——「最新修订加一」只在
                // suggestedNextRevision 一处。
                known
                  ? `对应册里该身份最新修订 r${known.revision}，建议停用落点 r${form.revisionField.suggestion}；册面可能已陈旧，错位由服务端判。`
                  : '对应册里没有这一标识（或册没取到），建议为 1；停用一个不在册的身份会被服务端答「册上没有这一个身份」。'
              }
            />

            <Field label="停用依据 *" path="deactivations[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="停用依据引用"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>

            <WallTimeField
              label="停用时点 *"
              path="deactivations[0].at"
              problems={problems}
              value={draft.at}
              locked={locked}
              timeZone={timeZone}
              onChange={(at) => patch({ at })}
            />
          </div>

          <div className="flex items-start gap-3">
            <Button onClick={form.send} disabled={locked || !form.canSend}>
              {locked ? '提交中…' : '提交停用'}
            </Button>
            <RegistrationAnswerNote
              state={form.state}
              owner={info.owner}
              outcomeLabels={partyIdentityOutcomeLabels}
              problemNote={problemNote}
            />
          </div>
        </CardContent>
      </Card>

      <SnapshotJsonDetails kind={kind} submit={form.submit} />
    </div>
  );
}
