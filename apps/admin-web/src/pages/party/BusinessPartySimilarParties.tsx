import { useEffect, useState } from 'react';
import { Search } from 'lucide-react';
import { StatusPill } from '../../components/StatusPill';
import { useSettledKeyword } from '../use-catalogue-cursor';
import { listBusinessParties } from './api';
import { businessPartyObjectHash } from './business-parties-address';
import type { PartyTileKind } from './business-parties-workbench';
import { similarKeywordMinChars, similarPartiesOf, similarPartiesShown, similarPartyKeywords, type SimilarParties } from './business-party-similar';
import { PartyTile } from './party-role-visuals';

/**
 * 「新增往来单位」表单旁的一栏：随名称与编码的输入列出名录里含这几个字的单位（票 business-party-redesign/11）。只是提醒，
 * 不拦保存——什么时候问、怎么并在 business-party-similar.ts。
 *
 * 问的是目录读口自己，不在页面手上那一页里找：页面持有的只是不带条件的第一页，册一多，要找的那一家多半不在里面。输入停稳了
 * 才问（useSettledKeyword），答复回来之前照旧摆着上一份，只标成「正在查」——每敲一个字就清空再填回来，这一栏会跟着闪。
 *
 * 列出来的每一家是一个去它对象页的链接。对象页在壳层是另一张标签，这张表单留在原来的标签里、填的内容不动；有没提交的输入的
 * 标签换回来也不重挂（unsaved-guard.ts 的 useUnsavedGuard）。
 */
export function BusinessPartySimilarParties({
  name,
  partyId,
  tileKindOf,
  className = '',
}: {
  /** 表单里此刻的名称与编码，原样传进来。 */
  name: string;
  partyId: string;
  /** 一家单位的图标块，与名录的行同一个算法；不给就都按「没有角色」画。 */
  tileKindOf?: (partyId: string) => PartyTileKind;
  className?: string;
}) {
  const keywords = similarPartyKeywords(useSettledKeyword(name), useSettledKeyword(partyId));
  // 检索词的签名：几个词连同次序。作副作用的依赖用它而不用数组本身——数组每次渲染都是新的。
  const asking = JSON.stringify(keywords);
  const [answered, setAnswered] = useState<{ asked: string; result: SimilarParties } | null>(null);
  useEffect(() => {
    const words = JSON.parse(asking) as string[];
    if (words.length === 0) return;
    let cancelled = false;
    void Promise.all(words.map((q) => listBusinessParties({ q }))).then((answers) => {
      if (cancelled) return;
      setAnswered({
        asked: asking,
        result: similarPartiesOf(
          answers.map((answer) => (answer.kind === 'outcome' ? { parties: answer.body.parties, hasNext: answer.body.page.next !== null } : null)),
        ),
      });
    });
    return () => {
      cancelled = true;
    };
  }, [asking]);

  const typed = keywords.length > 0;
  // 没在问（两格都不够长）时上一份答复不作数；在问而答复还是上一次的，照摆、标成正在查。
  const result = typed ? (answered?.result ?? null) : null;
  const pending = typed && answered?.asked !== asking;
  const note = 'px-4 py-5 type-body leading-5 text-idpxyz-breadcrumb';

  return (
    <aside className={`rounded-lg border border-idpxyz-border bg-idpxyz-bg ${className}`} aria-label="名录里相近的单位">
      <header className="flex items-center gap-2 border-b border-idpxyz-border px-4 py-2.5">
        <Search className="h-4 w-4 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
        <h2 className="type-section leading-5 text-idpxyz-textBright">名录里相近的单位</h2>
        {pending && result !== null ? <span className="ml-auto type-body text-idpxyz-breadcrumb">正在查…</span> : null}
      </header>
      <div aria-live="polite" aria-busy={pending}>
        {!typed ? (
          <p className={note}>
            名称或编码输到 {similarKeywordMinChars} 个字，这里会列出名录里含这几个字的单位，免得同一家登两次。只是提醒，不拦保存。
          </p>
        ) : result === null ? (
          <p className={note}>正在查…</p>
        ) : result.shown.length === 0 ? (
          <p className={note}>
            {result.unread
              ? '名录这会儿没读到，相近的单位列不出来；不影响保存。'
              : '没有查到名称或编码含这几个字的单位。全称与已登的名称差几个字时查不到，拿不准就换简称再试。'}
          </p>
        ) : (
          <>
            <ul className={pending ? 'opacity-60' : undefined}>
              {result.shown.map((row) => (
                <li key={row.partyId} className="border-b border-idpxyz-border last:border-b-0">
                  <a
                    href={businessPartyObjectHash(row.partyId)}
                    title="打开这家单位看看是不是同一家；这张表单留着，填的内容不动"
                    className="flex items-center gap-2.5 px-4 py-2 hover:bg-idpxyz-hover focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-idpxyz-accent"
                  >
                    <PartyTile small kind={tileKindOf?.(row.partyId) ?? 'NONE'} />
                    {/* 名称与编码放不下就折行，不截断：这一栏就是给人认「是不是同一家」的，截掉的那半截可能正是分得出两家的地方。 */}
                    <span className="min-w-0 flex-1">
                      <span className="block break-words type-value text-idpxyz-text">{row.partyName}</span>
                      <span className="block break-all font-mono type-body text-idpxyz-breadcrumb">{row.partyId}</span>
                    </span>
                    <StatusPill code={row.status} />
                  </a>
                </li>
              ))}
            </ul>
            <p className="border-t border-idpxyz-border px-4 py-2.5 type-body leading-5 text-idpxyz-breadcrumb">
              已经有了就不要再登一家：点开看看是不是同一家。
              {result.more ? `只列出前 ${similarPartiesShown} 家，其余的到名录里搜。` : ''}
              {result.unread ? '有一部分没读到，列出来的可能不全。' : ''}
            </p>
          </>
        )}
      </div>
    </aside>
  );
}
