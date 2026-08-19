import { useState } from 'react';
import { Card, Form, Input, Select, InputNumber, DatePicker, Checkbox, Button, Space, message } from 'antd';
import type { ClarificationSchema } from '../../services/conversation';

const { TextArea } = Input;

interface ClarificationCardProps {
  schema: ClarificationSchema;
  onSubmit: (answer: Record<string, unknown>) => Promise<void> | void;
  onCancel: () => void;
}

interface FieldDef {
  key: string;
  label: string;
  type: string;
  required: boolean;
  options?: Array<{ label: string; value: string | number }>;
  min?: number;
  max?: number;
  minLength?: number;
  maxLength?: number;
  format?: string;
  placeholder?: string;
  defaultVal?: unknown;
}

function buildFields(schema: ClarificationSchema): FieldDef[] {
  const fields: FieldDef[] = [];
  const required = new Set(schema.required || []);

  for (const [key, prop] of Object.entries(schema.properties)) {
    let fieldType: string = prop.type;
    if (prop.format === 'textarea') fieldType = 'textarea';
    else if (prop.format === 'date') fieldType = 'date';
    else if (prop.format === 'checkbox') fieldType = 'checkbox';
    else if (prop.format === 'attachment') fieldType = 'attachment';
    else if (prop.enum && prop.enum.length > 0) fieldType = 'select';

    fields.push({
      key,
      label: prop.title || key,
      type: fieldType,
      required: required.has(key),
      options: prop.enum?.map((v) => ({ label: String(v), value: v })),
      min: prop.minimum,
      max: prop.maximum,
      minLength: prop.minLength,
      maxLength: prop.maxLength,
      format: prop.format,
      placeholder: prop.description,
      defaultVal: prop.default,
    });
  }

  return fields;
}

function renderField(field: FieldDef, value: unknown, onChange: (v: unknown) => void) {
  switch (field.type) {
    case 'textarea':
      return (
        <TextArea
          value={value as string}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder || `请输入${field.label}`}
          rows={3}
          minLength={field.minLength}
          maxLength={field.maxLength}
        />
      );
    case 'select':
      return (
        <Select
          value={value as string | number}
          onChange={onChange}
          placeholder={field.placeholder || `请选择${field.label}`}
          options={field.options}
        />
      );
    case 'number':
      return (
        <InputNumber
          value={value as number}
          onChange={onChange}
          placeholder={field.placeholder || `请输入${field.label}`}
          min={field.min}
          max={field.max}
          style={{ width: '100%' }}
        />
      );
    case 'date':
      return (
        <DatePicker
          value={value as import('dayjs').Dayjs}
          onChange={onChange}
          placeholder={field.placeholder || `请选择${field.label}`}
          style={{ width: '100%' }}
        />
      );
    case 'checkbox':
      return (
        <Checkbox
          checked={value as boolean}
          onChange={(e) => onChange(e.target.checked)}
        >
          {field.placeholder || field.label}
        </Checkbox>
      );
    case 'attachment':
      return (
        <Input
          value={value as string}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder || '附件占位（MVP 阶段仅展示路径输入）'}
        />
      );
    case 'string':
    default:
      return (
        <Input
          value={value as string}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder || `请输入${field.label}`}
          minLength={field.minLength}
          maxLength={field.maxLength}
        />
      );
  }
}

export default function ClarificationCard({ schema, onSubmit, onCancel }: ClarificationCardProps) {
  const fields = buildFields(schema);
  const [values, setValues] = useState<Record<string, unknown>>(() => {
    const initial: Record<string, unknown> = {};
    fields.forEach((f) => {
      initial[f.key] = f.defaultVal ?? (f.type === 'checkbox' ? false : '');
    });
    return initial;
  });
  const [submitting, setSubmitting] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {};
    fields.forEach((f) => {
      const v = values[f.key];
      if (f.required) {
        if (v === undefined || v === null || v === '' || v === false) {
          newErrors[f.key] = `${f.label}为必填项`;
          return;
        }
      }
      if (v !== undefined && v !== null && v !== '' && v !== false) {
        if (f.minLength != null && typeof v === 'string' && v.length < f.minLength) {
          newErrors[f.key] = `${f.label}至少${f.minLength}个字符`;
        }
        if (f.maxLength != null && typeof v === 'string' && v.length > f.maxLength) {
          newErrors[f.key] = `${f.label}最多${f.maxLength}个字符`;
        }
        if (f.min != null && typeof v === 'number' && v < f.min) {
          newErrors[f.key] = `${f.label}最小值为${f.min}`;
        }
        if (f.max != null && typeof v === 'number' && v > f.max) {
          newErrors[f.key] = `${f.label}最大值为${f.max}`;
        }
        if (f.type === 'select' && f.options && !f.options.some((o) => o.value === v)) {
          newErrors[f.key] = `${f.label}选项无效`;
        }
      }
    });
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleChange = (key: string, v: unknown) => {
    setValues((prev) => ({ ...prev, [key]: v }));
    if (errors[key]) {
      setErrors((prev) => {
        const next = { ...prev };
        delete next[key];
        return next;
      });
    }
  };

  const handleSubmit = async () => {
    if (!validate()) return;
    setSubmitting(true);
    try {
      await onSubmit(values);
      message.success('已提交回复');
    } catch (e) {
      message.error('提交失败，请重试');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Card
      size="small"
      title={schema.properties ? '需要补充信息' : '请补充以下信息'}
      style={{ marginBottom: 12, borderColor: '#faad14' }}
      styles={{ header: { background: '#fffbe6' } }}
    >
      <Form layout="vertical" size="small">
        {fields.map((f) => (
          <Form.Item
            key={f.key}
            label={
              <span>
                {f.label}
                {f.required && <span style={{ color: '#ff4d4f', marginLeft: 4 }}>*</span>}
              </span>
            }
            validateStatus={errors[f.key] ? 'error' : ''}
            help={errors[f.key]}
          >
            {renderField(f, values[f.key], (v) => handleChange(f.key, v))}
          </Form.Item>
        ))}
      </Form>

      <Space style={{ marginTop: 8 }}>
        <Button type="primary" size="small" loading={submitting} onClick={handleSubmit}>
          提交回复
        </Button>
        <Button size="small" onClick={onCancel} disabled={submitting}>
          取消
        </Button>
      </Space>
    </Card>
  );
}