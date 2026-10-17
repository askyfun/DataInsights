// 本地文件上传（issue #138）前端组件契约：
//   - 拖拽/点选入口就地校验类型与大小（超限/类型不符即拒绝，不发请求）；
//   - 上传成功后展示后端推断的列，允许改类型/角色后「开始分析」；
//   - 未改动则不多发一次 updateColumns（避免无意义写入）。
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { message as antdMessage } from 'antd';
import { IntlProvider } from 'react-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Dataset, DatasetColumn } from '../../api';
import DatasetUploadModal, {
  DatasetReplaceModal,
  MAX_UPLOAD_BYTES,
  MAX_UPLOAD_COLUMNS,
  MAX_UPLOAD_ROWS,
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

// R-85：上限口径与错误可区分性。
//
// 错误提示断言的是 message.error 的入参而不是 DOM：antd 静态 message 在
// tests/setup.ts 里被统一短路成空函数（它渲染在 testing-library 的树外 root，
// 且 rc-motion 的真实定时器会在收尾后触发伪影），断言 DOM 只会永远失败。
describe('上传上限（R-85）', () => {
  // 收集本次用例里 message.error 收到的文案。
  const errorTexts = (): string[] =>
    (antdMessage.error as unknown as ReturnType<typeof vi.fn>).mock.calls.map((c) => String(c[0]));

  it('拖拽区同时写明体积、行数与列数上限', async () => {
    wrap(<DatasetUploadModal open onCancel={vi.fn()} onCreated={vi.fn()} />);

    // 三项上限必须同时出现在提示里：只写体积会让用户以为大文件总是能传。
    const hint = await screen.findByText(/≤\s*50MB/);
    expect(hint.textContent).toContain(MAX_UPLOAD_ROWS.toLocaleString());
    expect(hint.textContent).toContain(String(MAX_UPLOAD_COLUMNS));
  });

  it('后端报行数超限时给出可区分的中文提示（不是通用失败）', async () => {
    instance.post.mockImplementation((url: string) => {
      if (url === '/api/datasets/import') {
        return Promise.reject(
          new Error('file has 1200000 rows, which exceeds the 1000000 row limit')
        );
      }
      return Promise.reject(new Error(`unexpected POST ${url}`));
    });
    wrap(<DatasetUploadModal open onCancel={vi.fn()} onCreated={vi.fn()} />);

    selectFile(makeFile('big.csv'));
    fireEvent.click(screen.getByRole('button', { name: '上传并解析' }));

    // 提示必须包含可操作的下一步（拆分/筛选），而不是把后端英文甩给用户。
    await waitFor(() => expect(errorTexts().join('\n')).toMatch(/1,000,000\s*行/));
    expect(errorTexts().join('\n')).not.toMatch(/row limit/);
  });

  it('后端报列数超限时给出可区分的中文提示', async () => {
    instance.post.mockImplementation((url: string) => {
      if (url === '/api/datasets/import') {
        return Promise.reject(
          new Error('file has 1500 columns, which exceeds the 1000 column limit')
        );
      }
      return Promise.reject(new Error(`unexpected POST ${url}`));
    });
    wrap(<DatasetUploadModal open onCancel={vi.fn()} onCreated={vi.fn()} />);

    selectFile(makeFile('wide.csv'));
    fireEvent.click(screen.getByRole('button', { name: '上传并解析' }));

    await waitFor(() => expect(errorTexts().join('\n')).toMatch(/1000\s*列/));
  });

  it('无法识别的错误原样透出，不被吞成通用失败', async () => {
    instance.post.mockImplementation((url: string) => {
      if (url === '/api/datasets/import') {
        return Promise.reject(new Error('relation "di_extract_9" does not exist'));
      }
      return Promise.reject(new Error(`unexpected POST ${url}`));
    });
    wrap(<DatasetUploadModal open onCancel={vi.fn()} onCreated={vi.fn()} />);

    selectFile(makeFile('data.csv'));
    fireEvent.click(screen.getByRole('button', { name: '上传并解析' }));

    // 吞掉原始错误等于把排查线索也一起吞掉。
    await waitFor(() => expect(errorTexts().join('\n')).toMatch(/does not exist/));
  });
});
