import { useEffect, useState } from 'react';
import { listDisposalRules, type DisposalRuleRecord } from './api';

// 一个租户的仓不会多到几十页；多到这里还没读全就当没读全，不拿读了一半的当作全部。
const maxPages = 10;

/**
 * 各仓当前的待认领处置规则，按仓编码。读不到（没授予、服务端没形成答案）或页数多到没读全时答 null：
 * 调用方据此只说「不知道」，不说哪个仓没登记——没读全的时候，缺的那个仓可能只是在后面的页里。
 */
async function loadDisposalRules(): Promise<ReadonlyMap<string, DisposalRuleRecord> | null> {
  const rules = new Map<string, DisposalRuleRecord>();
  let after: string | null = null;
  for (let page = 0; page < maxPages; page += 1) {
    const answer = await listDisposalRules({}, after);
    if (answer.kind !== 'outcome') return null;
    for (const rule of answer.body.disposalRules) rules.set(rule.warehouseCode, rule);
    if (answer.body.page.next === null) return rules;
    after = answer.body.page.next;
  }
  return null;
}

/** 待认领的「下一步」与认领期限要对着仓的处置规则判；`refresh` 变一次重读一遍。 */
export function useDisposalRules(refresh: number): ReadonlyMap<string, DisposalRuleRecord> | null {
  const [rules, setRules] = useState<ReadonlyMap<string, DisposalRuleRecord> | null>(null);
  useEffect(() => {
    let cancelled = false;
    void loadDisposalRules().then((loaded) => {
      if (!cancelled) setRules(loaded);
    });
    return () => {
      cancelled = true;
    };
  }, [refresh]);
  return rules;
}
