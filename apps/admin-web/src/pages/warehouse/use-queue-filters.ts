import { useState } from 'react';

/**
 * 一张队列式列表的筛选态：检索词、状态签、货主、仓库、种类与排序（null 即按页面自己的缺省排）。
 * 种类这一维各册读口叫法不一（指令种类、冻结对象、库内作业种类），词表也各是各的；筛选态只存选了哪个词，空串即不按这一维筛，
 * 哪个词对应读口的哪个筛选维由各列表自己决定。
 */
export interface QueueFilters {
  keyword: string;
  status: string;
  owner: string;
  warehouse: string;
  kind: string;
  sort: string | null;
  setKeyword: (value: string) => void;
  setStatus: (value: string) => void;
  setOwner: (value: string) => void;
  setWarehouse: (value: string) => void;
  setKind: (value: string) => void;
  setSort: (value: string | null) => void;
}

/**
 * 筛选态由页签的外面持有：一个模块页有几个页签时，切走的那一张列表会卸载，状态放在列表里就丢了，换回来又得重选。
 * 单据页（别的标签）与这张列表标签无关，不经过这里。
 */
export function useQueueFilters(initialStatus: string): QueueFilters {
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState(initialStatus);
  const [owner, setOwner] = useState('');
  const [warehouse, setWarehouse] = useState('');
  const [kind, setKind] = useState('');
  const [sort, setSort] = useState<string | null>(null);
  return { keyword, status, owner, warehouse, kind, sort, setKeyword, setStatus, setOwner, setWarehouse, setKind, setSort };
}
