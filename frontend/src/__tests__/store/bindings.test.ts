import { describe, expect, it } from 'vitest';
import { type BindingInstance, nextBindingId, reconcileGroupBindings } from '@/store';

/**
 * bindingId 生成与 QueryPanel Select diff 的纯函数测试。
 *
 * nextBindingId：全局唯一、顺序递增，删除中间 binding 后不复用旧号。
 * reconcileGroupBindings：QueryPanel 多选 Select 变更时，保留仍选中列名的现有
 * bindingId（避免按 bindingId 存的 aggregation/alias 丢失），只为新增列名生成新号，
 * 移除取消选中的 binding，输出顺序跟随 Select 的 values。
 */

const b = (bindingId: string, field: string): BindingInstance => ({ bindingId, field });

describe('nextBindingId', () => {
  it('空视野返回 b-0', () => {
    expect(nextBindingId([])).toBe('b-0');
    expect(nextBindingId([[], []])).toBe('b-0');
  });

  it('返回组内最大序号 +1', () => {
    expect(nextBindingId([[b('b-0', 'a'), b('b-1', 'b')]])).toBe('b-2');
  });

  it('跨多个组扫描全局最大号', () => {
    expect(nextBindingId([[b('b-0', 'a')], [b('b-5', 'b')], [b('b-2', 'c')]])).toBe('b-6');
  });

  it('删除中间 binding 后新增不复用被删的号', () => {
    // 曾有 b-0/b-1/b-2，删掉 b-1 后剩 b-0/b-2 → 下一个是 b-3（不复用 b-1）
    expect(nextBindingId([[b('b-0', 'a'), b('b-2', 'c')]])).toBe('b-3');
  });

  it('忽略不符合 b-N 形式的 id（健壮性）', () => {
    expect(nextBindingId([[b('weird', 'a'), b('b-1', 'b')]])).toBe('b-2');
  });
});

describe('reconcileGroupBindings', () => {
  it('保留仍被选中列名的现有 bindingId', () => {
    const group = [b('b-0', 'a'), b('b-1', 'b')];
    expect(reconcileGroupBindings(group, ['a', 'b'], [group])).toEqual([
      b('b-0', 'a'),
      b('b-1', 'b'),
    ]);
  });

  it('只为新增列名生成新号，且不与其它组已占用的号冲突', () => {
    const group = [b('b-0', 'a')];
    const other = [b('b-1', 'x')]; // 另一组已占用 b-1
    expect(reconcileGroupBindings(group, ['a', 'c'], [group, other])).toEqual([
      b('b-0', 'a'),
      b('b-2', 'c'),
    ]);
  });

  it('移除被取消选中的列名对应 binding', () => {
    const group = [b('b-0', 'a'), b('b-1', 'b')];
    expect(reconcileGroupBindings(group, ['b'], [group])).toEqual([b('b-1', 'b')]);
  });

  it('输出顺序跟随 selectedFields（重排保留各自 bindingId）', () => {
    const group = [b('b-0', 'a'), b('b-1', 'b')];
    expect(reconcileGroupBindings(group, ['b', 'a'], [group])).toEqual([
      b('b-1', 'b'),
      b('b-0', 'a'),
    ]);
  });

  it('一次新增多个列名时号互不冲突', () => {
    const group: BindingInstance[] = [];
    expect(reconcileGroupBindings(group, ['a', 'b', 'c'], [group])).toEqual([
      b('b-0', 'a'),
      b('b-1', 'b'),
      b('b-2', 'c'),
    ]);
  });
});
