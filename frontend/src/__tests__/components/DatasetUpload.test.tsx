// 本地文件上传（issue #138）前端组件契约：
//   - 拖拽/点选入口就地校验类型与大小（超限/类型不符即拒绝，不发请求）；
//   - 上传成功后展示后端推断的列，允许改类型/角色后「开始分析」；
//   - 未改动则不多发一次 updateColumns（避免无意义写入）。
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { IntlProvider } from 'react-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Dataset, DatasetColumn } from '../../api';
import DatasetUploadModal, {
  DatasetReplaceModal,
  MAX_UPLOAD_BYTES,
} from '../../components/DatasetUpload';
import { messages } from '../../i18n/useLocale';

const { instance } = vi.hoisted(() => {
  const inst = {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
    interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
  };
  return { instance: inst };
});

vi.mock('axios', () => ({
  default: { create: () => instance },
}));

class FakeResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', FakeResizeObserver);

const envelope = (data: unknown) => ({ data: { code: 20000, msg: 'success', trace: '', data } });

const dataset = {
  id: 7,
  name: 'demo',
  datasource_id: 3,
  table_name: 'di_extract_7',
  query_sql: null,
  query_type: 'table',
  mode: 'extract',
  description: null,
  tags: '[]',
  quality_rules: '[]',
  columns: '[]',
  shard_enabled: false,
  shard_keys: '[]',
  created_at: '',
  updated_at: '',
} as unknown as Dataset;

const inferred: DatasetColumn[] = [
  { id: 'c1', name: 'amount', expr: 'amount', type: 'float', role: 'metric', comment: '' },
  { id: 'c2', name: 'city', expr: 'city', type: 'string', role: 'dimension', comment: '' },
];

const makeFile = (name: string, size = 128): File => {
  const file = new File(['x'], name);
  Object.defineProperty(file, 'size', { value: size });
  return file;
};

const selectFile = (file: File) => {
  const input = document.querySelector('input[type="file"]');
  if (!input) throw new Error('file input not found');
  fireEvent.change(input, { target: { files: [file] } });
};

const wrap = (ui: React.ReactElement) =>
  render(
    <IntlProvider locale="zh-CN" messages={messages['zh-CN']}>
      {ui}
    </IntlProvider>
  );

const postedUrls = () => instance.post.mock.calls.map((call) => call[0] as string);

beforeEach(() => {
  vi.clearAllMocks();
  instance.post.mockImplementation((url: string) => {
    if (url === '/api/datasets/import') return Promise.resolve(envelope(dataset));
    if (url === '/api/datasets/7/replace') return Promise.resolve(envelope(dataset));
    if (url === '/api/datasets/7/columns') return Promise.resolve(envelope(dataset));
    return Promise.reject(new Error(`unexpected POST ${url}`));
  });
  instance.get.mockImplementation((url: string) => {
    if (url === '/api/datasets/7/columns') return Promise.resolve(envelope(inferred));
    return Promise.reject(new Error(`unexpected GET ${url}`));
  });
});

describe('DatasetUploadModal', () => {
  it('拒绝非 csv/xlsx 文件，且不发起上传', async () => {
    wrap(<DatasetUploadModal open onCancel={vi.fn()} onCreated={vi.fn()} />);

    selectFile(makeFile('notes.txt'));

    await screen.findByText('仅支持 .csv 或 .xlsx 文件');
    expect(instance.post).not.toHaveBeenCalled();
  });

  it('拒绝超过 50MB 的文件', async () => {
    wrap(<DatasetUploadModal open onCancel={vi.fn()} onCreated={vi.fn()} />);

    selectFile(makeFile('big.csv', MAX_UPLOAD_BYTES + 1));

    await screen.findByText('文件超过 50MB 上限');
    expect(instance.post).not.toHaveBeenCalled();
  });

  it('上传成功后展示推断列，改动角色后开始分析会持久化并回调', async () => {
    const onCreated = vi.fn();
    wrap(<DatasetUploadModal open onCancel={vi.fn()} onCreated={onCreated} />);

    selectFile(makeFile('data.csv'));
    fireEvent.click(screen.getByRole('button', { name: '上传并解析' }));

    await screen.findByText('amount');
    await screen.findByText('city');

    // 改第一列角色（metric → dimension）后再开始分析。
    fireEvent.click(screen.getAllByRole('switch')[0]);
    fireEvent.click(screen.getByRole('button', { name: /开始分析/ }));

    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(dataset));
    expect(postedUrls()).toContain('/api/datasets/import');
    expect(postedUrls()).toContain('/api/datasets/7/columns');
  });

  it('未改动字段时不重复调用 updateColumns', async () => {
    const onCreated = vi.fn();
    wrap(<DatasetUploadModal open onCancel={vi.fn()} onCreated={onCreated} />);

    selectFile(makeFile('data.csv'));
    fireEvent.click(screen.getByRole('button', { name: '上传并解析' }));
    await screen.findByText('amount');

    fireEvent.click(screen.getByRole('button', { name: /开始分析/ }));

    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(dataset));
    expect(postedUrls()).not.toContain('/api/datasets/7/columns');
  });
});

describe('DatasetReplaceModal', () => {
  it('替换文件调用 replace 端点并回调', async () => {
    const onReplaced = vi.fn();
    wrap(<DatasetReplaceModal open dataset={dataset} onCancel={vi.fn()} onReplaced={onReplaced} />);

    selectFile(makeFile('next.csv'));
    fireEvent.click(screen.getByRole('button', { name: /替\s*换/ }));

    await waitFor(() => expect(onReplaced).toHaveBeenCalledWith(dataset));
    expect(postedUrls()).toContain('/api/datasets/7/replace');
  });
});
