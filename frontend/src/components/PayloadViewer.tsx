import { Typography } from 'antd';

// JSON 载荷查看器:后端 JSON 字段多为字符串,自动尝试解析后格式化展示。
export default function PayloadViewer({ value }: { value?: string | null }) {
  if (value === undefined || value === null || value === '') {
    return <Typography.Text type="secondary">—</Typography.Text>;
  }
  let parsed: unknown = value;
  try {
    parsed = JSON.parse(value);
  } catch {
    // 保留原始文本
  }
  return (
    <Typography.Text>
      <pre style={{ margin: 0, maxWidth: 640, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
        {typeof parsed === 'string' ? parsed : JSON.stringify(parsed, null, 2)}
      </pre>
    </Typography.Text>
  );
}
