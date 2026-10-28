import { useEffect, useState } from 'react';
import { listCustomerAccounts } from '../party/api';
import { customerNameIndexOf, type CustomerNameIndex } from './shipment-request-list';

// 运单列表上的「客户」显名称不显编码：名称在参与方册的客户账户上，这里把那本册读回来建一份编码 → 名称的索引（与参与方页
// 「名称从参与方册转写」同一做法，设计源第八节第一行）。客户账户读口分页（ADR-0144），一页一页走到末页，账户再多也不漏。
//
// 模块级缓存：运单列表与运单详情都要同一份索引，一个标签页里只读一次；读不到（未配置、拒绝、依赖故障）不缓存，下次挂载
// 再问。读不到时页面显编码，不把一次读不到当成「册上没有」。

const emptyIndex: CustomerNameIndex = new Map();
let pending: Promise<CustomerNameIndex> | null = null;

async function loadAllCustomerNames(): Promise<CustomerNameIndex | null> {
  const merged = new Map<string, string>();
  let after: string | null = null;
  do {
    const answer = await listCustomerAccounts({}, after);
    const page = customerNameIndexOf(answer);
    if (page === null) return null;
    for (const [account, name] of page) merged.set(account, name);
    after = answer.kind === 'outcome' ? answer.body.page.next : null;
  } while (after !== null);
  return merged;
}

function loadCustomerNames(): Promise<CustomerNameIndex> {
  if (pending === null) {
    pending = loadAllCustomerNames().then((index) => {
      if (index === null) {
        pending = null;
        return emptyIndex;
      }
      return index;
    });
  }
  return pending;
}

/** 客户账户编码 → 客户名称。读回之前与读不到时是空索引，页面按编码显示。 */
export function useCustomerNames(): CustomerNameIndex {
  const [names, setNames] = useState<CustomerNameIndex>(emptyIndex);
  useEffect(() => {
    let cancelled = false;
    void loadCustomerNames().then((index) => {
      if (!cancelled) setNames(index);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return names;
}
