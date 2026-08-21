interface DonutChartProps {
  percent: number; // 0-100
  size?: number;
  strokeWidth?: number;
  color?: string;
  trackColor?: string;
  label?: string;
  subLabel?: string;
}

export default function DonutChart({
  percent,
  size = 120,
  strokeWidth = 10,
  color = '#1D4ED8',
  trackColor = '#E5E7EB',
  label,
  subLabel,
}: DonutChartProps) {
  const radius = (size - strokeWidth) / 2;
  const circumference = 2 * Math.PI * radius;
  const clamped = Math.max(0, Math.min(100, percent));
  const dashOffset = circumference * (1 - clamped / 100);

  return (
    <div style={{ position: 'relative', width: size, height: size, display: 'inline-block' }}>
      <svg width={size} height={size} style={{ transform: 'rotate(-90deg)' }}>
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={trackColor}
          strokeWidth={strokeWidth}
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeDasharray={circumference}
          strokeDashoffset={dashOffset}
          strokeLinecap="round"
          style={{ transition: 'stroke-dashoffset 0.8s ease' }}
        />
      </svg>
      <div
        style={{
          position: 'absolute',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <span style={{ fontSize: size * 0.22, fontWeight: 700, color: 'var(--text-primary)', lineHeight: 1.2 }}>
          {label ?? `${clamped.toFixed(0)}%`}
        </span>
        {subLabel && (
          <span style={{ fontSize: size * 0.1, color: 'var(--text-secondary)' }}>{subLabel}</span>
        )}
      </div>
    </div>
  );
}
