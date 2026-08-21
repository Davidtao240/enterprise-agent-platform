interface HBarChartProps {
  data: { label: string; value: number; color?: string }[];
  maxValue?: number;
}

export default function HBarChart({ data, maxValue }: HBarChartProps) {
  const max = maxValue ?? Math.max(...data.map((d) => d.value), 1);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      {data.map((item) => (
        <div key={item.label}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              marginBottom: 4,
            }}
          >
            <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{item.label}</span>
            <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary)' }}>
              {item.value.toFixed(1)}
            </span>
          </div>
          <div style={{ height: 8, background: 'var(--neutral-100)', borderRadius: 4, overflow: 'hidden' }}>
            <div
              style={{
                height: '100%',
                width: `${(item.value / max) * 100}%`,
                background: item.color || 'linear-gradient(90deg, #1D4ED8, #3B82F6)',
                borderRadius: 4,
                transition: 'width 0.8s ease',
              }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}
