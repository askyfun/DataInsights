import { InboxOutlined } from '@ant-design/icons';
import {
  Button,
  Input,
  Modal,
  message,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  Upload,
} from 'antd';
import React, { useEffect, useState } from 'react';
import { useIntl } from 'react-intl';
import type { Dataset, DatasetColumn } from '../../api';
import { datasetsApi } from '../../api';
import ModalFooter from '../ModalFooter';

const { Text } = Typography;

// 与后端 extract.MaxUploadBytes（50 << 20）对齐。客户端只做即时反馈，权威校验仍在后端。
export const MAX_UPLOAD_BYTES = 50 << 20;
// 与后端 extract.MaxUploadRows 对齐（R-85）。行数上限与体积上限是两件事：
// 50MB 的窄表可以是上百万行，摄入时间随行数线性增长，拖住的是首次上传的等待。
export const MAX_UPLOAD_ROWS = 1_000_000;
// 与后端 extract.MaxUploadColumns 对齐。每列每行占一个占位符，宽表会撞驱动上限。
export const MAX_UPLOAD_COLUMNS = 1000;
const ACCEPT = '.csv,.xlsx';

// 推断只会产出标量类型；array/map 不在可选范围。
const REVIEW_TYPES = ['string', 'integer', 'float', 'boolean', 'date', 'datetime'] as const;

function errMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

// 后端的超限文案是英文技术表述（"file has N rows, which exceeds the 1000000
// row limit"）。这里认出它并换成带操作建议的中文提示——用户看到"超了上限"
// 之后需要知道下一步做什么，而不是知道自己违规了。
//
// 刻意只匹配后端稳定的数字与关键词（上限常量在本文件里定义，与后端同源），
// 不匹配整句英文：后端改措辞不该让这里的识别静默失效。
function uploadLimitHint(error: unknown): string | null {
  const msg = error instanceof Error ? error.message : '';
  if (!msg) return null;
  if (msg.includes('row limit')) return 'rows';
  if (msg.includes('column limit')) return 'columns';
  return null;
}

/** 把上传错误翻成可操作提示；识别不出的错误原样透出（不吞信息）。 */
function uploadErrorMessage(
  error: unknown,
  intl: { formatMessage: (d: { id: string }, v?: Record<string, unknown>) => string }
): string {
  const fallback = intl.formatMessage({ id: 'common.error' });
  const kind = uploadLimitHint(error);
  if (kind === 'rows') {
    return intl.formatMessage(
      { id: 'dataset.upload.tooManyRows' },
      { rows: MAX_UPLOAD_ROWS.toLocaleString() }
    );
  }
  if (kind === 'columns') {
    return intl.formatMessage(
      { id: 'dataset.upload.tooManyColumns' },
      { cols: MAX_UPLOAD_COLUMNS }
    );
  }
  return errMessage(error, fallback);
}

function isSupported(file: File): boolean {
  const lower = file.name.toLowerCase();
  return lower.endsWith('.csv') || lower.endsWith('.xlsx');
}

export interface FileDropZoneProps {
  onFile: (file: File) => void;
  disabled?: boolean;
  /** 空态大入口用的加强文案。 */
  titleId?: string;
}

/** 拖拽/点选上传区：就地做类型与大小校验，通过后把文件交给 onFile。 */
export const FileDropZone: React.FC<FileDropZoneProps> = ({ onFile, disabled, titleId }) => {
  const intl = useIntl();
  const [error, setError] = useState<string | null>(null);

  const accept = (file: File): boolean => {
    if (!isSupported(file)) {
      const msg = intl.formatMessage({ id: 'dataset.upload.invalidType' });
      setError(msg);
      message.error(msg);
      return false;
    }
    if (file.size > MAX_UPLOAD_BYTES) {
      const msg = intl.formatMessage(
        { id: 'dataset.upload.tooLarge' },
        { mb: MAX_UPLOAD_BYTES >> 20 }
      );
      setError(msg);
      message.error(msg);
      return false;
    }
    setError(null);
    return true;
  };

  return (
    <Upload.Dragger
      accept={ACCEPT}
      maxCount={1}
      showUploadList={false}
      disabled={disabled}
      beforeUpload={(file) => {
        if (accept(file)) {
          onFile(file);
        }
        // 一律阻止 antd 自带上传：真正的上传走 importFile/replaceFile 手工通道。
        return Upload.LIST_IGNORE;
      }}
    >
      <p className="ant-upload-drag-icon">
        <InboxOutlined />
      </p>
      <p className="ant-upload-text">
        {intl.formatMessage({ id: titleId ?? 'dataset.upload.dragText' })}
      </p>
      <p className="ant-upload-hint">
        {intl.formatMessage(
          { id: 'dataset.upload.hint' },
          {
            mb: MAX_UPLOAD_BYTES >> 20,
            rows: MAX_UPLOAD_ROWS.toLocaleString(),
            cols: MAX_UPLOAD_COLUMNS,
          }
        )}
      </p>
      {error && <Text type="danger">{error}</Text>}
    </Upload.Dragger>
  );
};

export interface DatasetUploadModalProps {
  open: boolean;
  onCancel: () => void;
  onCreated: (dataset: Dataset) => void;
  /** 从空态拖入的文件：打开弹窗时预置，免去二次选择。 */
  initialFile?: File | null;
}

/**
 * 本地文件上传建数据集（issue #138）：
 * 选择文件 → 后端解析并推断列类型 → 复核（类型/角色可改）→ 开始分析。
 */
const DatasetUploadModal: React.FC<DatasetUploadModalProps> = ({
  open,
  onCancel,
  onCreated,
  initialFile,
}) => {
  const intl = useIntl();
  const [file, setFile] = useState<File | null>(null);
  const [name, setName] = useState('');
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [step, setStep] = useState(0);
  const [created, setCreated] = useState<Dataset | null>(null);
  const [columns, setColumns] = useState<DatasetColumn[]>([]);
  // 后端推断出的原始列：与编辑后的 columns 比对，未改就不发多余的写入。
  const [inferred, setInferred] = useState<DatasetColumn[]>([]);

  const reset = () => {
    setFile(null);
    setName('');
    setStep(0);
    setCreated(null);
    setColumns([]);
    setInferred([]);
  };

  useEffect(() => {
    if (open && initialFile) {
      setFile(initialFile);
    }
  }, [open, initialFile]);

  const handleCancel = () => {
    reset();
    onCancel();
  };

  const handleUpload = async () => {
    if (!file) return;
    setUploading(true);
    try {
      const resp = await datasetsApi.importFile(file, name.trim() || undefined);
      const dataset = resp.data.data;
      const colsResp = await datasetsApi.getColumns(dataset.id);
      const cols = colsResp.data.data ?? [];
      setCreated(dataset);
      setColumns(cols);
      setInferred(cols);
      setStep(1);
    } catch (error) {
      message.error(uploadErrorMessage(error, intl));
    } finally {
      setUploading(false);
    }
  };

  const handleStartAnalysis = async () => {
    if (!created) return;
    setSaving(true);
    try {
      if (JSON.stringify(columns) !== JSON.stringify(inferred)) {
        await datasetsApi.updateColumns(created.id, columns);
      }
      onCreated(created);
      reset();
    } catch (error) {
      message.error(errMessage(error, intl.formatMessage({ id: 'common.error' })));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title={intl.formatMessage({ id: 'dataset.upload.title' })}
      open={open}
      onCancel={handleCancel}
      footer={null}
      width={720}
      destroyOnHidden
    >
      {step === 0 ? (
        <div>
          <FileDropZone onFile={setFile} disabled={uploading} />
          {file && (
            <Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
              {file.name}
            </Text>
          )}
          <Input
            style={{ marginTop: 12 }}
            placeholder={intl.formatMessage({ id: 'dataset.upload.namePlaceholder' })}
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={uploading}
          />
          <ModalFooter>
            <Button onClick={handleCancel} disabled={uploading}>
              {intl.formatMessage({ id: 'common.cancel' })}
            </Button>
            <Button type="primary" onClick={handleUpload} loading={uploading} disabled={!file}>
              {intl.formatMessage({ id: 'dataset.upload.submit' })}
            </Button>
          </ModalFooter>
        </div>
      ) : (
        <div>
          <Text type="secondary" style={{ display: 'block', marginBottom: 12 }}>
            {intl.formatMessage({ id: 'dataset.upload.reviewHint' })}
          </Text>
          <Table
            dataSource={columns}
            rowKey="id"
            size="small"
            pagination={false}
            columns={[
              {
                title: intl.formatMessage({ id: 'field.name' }),
                dataIndex: 'name',
                key: 'name',
                render: (value: string) => <Text strong>{value}</Text>,
              },
              {
                title: intl.formatMessage({ id: 'field.type' }),
                dataIndex: 'type',
                key: 'type',
                width: 160,
                render: (value: string, record: DatasetColumn) => (
                  <Select
                    value={value}
                    size="small"
                    style={{ width: 130 }}
                    onChange={(next) =>
                      setColumns((prev) =>
                        prev.map((col) =>
                          col === record ? { ...col, type: next as DatasetColumn['type'] } : col
                        )
                      )
                    }
                    options={REVIEW_TYPES.map((t) => ({
                      value: t,
                      label: intl.formatMessage({ id: `dataType.${t}` }),
                    }))}
                  />
                ),
              },
              {
                title: intl.formatMessage({ id: 'field.role' }),
                dataIndex: 'role',
                key: 'role',
                width: 120,
                render: (value: string, record: DatasetColumn) => (
                  <Switch
                    checked={value === 'metric'}
                    checkedChildren={intl.formatMessage({ id: 'field.metric' })}
                    unCheckedChildren={intl.formatMessage({ id: 'field.dimension' })}
                    size="small"
                    onChange={(checked) =>
                      setColumns((prev) =>
                        prev.map((col) =>
                          col === record ? { ...col, role: checked ? 'metric' : 'dimension' } : col
                        )
                      )
                    }
                    style={{
                      backgroundColor: value === 'metric' ? 'var(--dr-metric)' : undefined,
                    }}
                  />
                ),
              },
            ]}
          />
          <Space style={{ marginTop: 12 }}>{created && <Tag>{created.name}</Tag>}</Space>
          <ModalFooter>
            <Button onClick={() => setStep(0)} disabled={saving}>
              {intl.formatMessage({ id: 'dataset.upload.back' })}
            </Button>
            <Button type="primary" onClick={handleStartAnalysis} loading={saving}>
              {intl.formatMessage({ id: 'dataset.upload.startAnalysis' })}
            </Button>
          </ModalFooter>
        </div>
      )}
    </Modal>
  );
};

export interface DatasetReplaceModalProps {
  open: boolean;
  dataset: Dataset | null;
  onCancel: () => void;
  onReplaced: (dataset: Dataset) => void;
}

/** 覆盖已上传数据集的文件；展示名未变的字段保留原列 ID（图表不断链）。 */
export const DatasetReplaceModal: React.FC<DatasetReplaceModalProps> = ({
  open,
  dataset,
  onCancel,
  onReplaced,
}) => {
  const intl = useIntl();
  const [file, setFile] = useState<File | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open) {
      setFile(null);
    }
  }, [open]);

  const handleReplace = async () => {
    if (!dataset || !file) return;
    setLoading(true);
    try {
      const resp = await datasetsApi.replaceFile(dataset.id, file);
      message.success(intl.formatMessage({ id: 'dataset.replace.success' }));
      onReplaced(resp.data.data);
      setFile(null);
    } catch (error) {
      message.error(uploadErrorMessage(error, intl));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      title={intl.formatMessage({ id: 'dataset.replace.title' })}
      open={open}
      onCancel={onCancel}
      footer={null}
      width={640}
      destroyOnHidden
    >
      <Text type="secondary" style={{ display: 'block', marginBottom: 12 }}>
        {intl.formatMessage({ id: 'dataset.replace.hint' })}
      </Text>
      <FileDropZone onFile={setFile} disabled={loading} />
      {file && (
        <Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
          {file.name}
        </Text>
      )}
      <ModalFooter>
        <Button onClick={onCancel} disabled={loading}>
          {intl.formatMessage({ id: 'common.cancel' })}
        </Button>
        <Button type="primary" onClick={handleReplace} loading={loading} disabled={!file}>
          {intl.formatMessage({ id: 'dataset.replace.submit' })}
        </Button>
      </ModalFooter>
    </Modal>
  );
};

export default DatasetUploadModal;
