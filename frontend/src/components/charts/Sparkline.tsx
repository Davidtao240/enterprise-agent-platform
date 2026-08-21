interface SparklineProps {
  data: number[];
  width?: number;
  height?: number;
  stroke?: string;
  strokeWidth?: number;
}

export default function Sparkline({
  data,
  width = 120,
  height = 40,
  stroke = '#1D4ED8',
  strokeWidth = 2,
}: SparklineProps) {
  if (data.length < 2) {
    return <svg width={width} height={height} />;
  }

  const min = Math.min(...data);
  const max = Math.max(...data);
  const range = max - min || 1;
  const padding = 4;

  const points = data.map((v, i) => {
    const x = padding + (i / (data.length - 1)) * (width - padding * 2);
    const y = height - padding - ((v - min) / range) * (height - padding * 2);
    return `${x},${y}`;
  });

  const linePath = points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p}`).join(' ');
  const areaPath = `${linePath} L${width - padding},${height} L${padding},${height} Z`;
  const lastPoint = points[points.length - 1].split(',');

  return (
    <svg width={width} height={height} style={{ display: 'block' }}>
      <defs>
        <linearGradient id={`spark-${stroke.replace('#', '')}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={stroke} stopOpacity={0.25} />
          <stop offset="100%" stopColor={stroke} stopOpacity={0} />
        </linearGradient>
      </defs>
      <path d={areaPath} fill={`url(#spark-${stroke.replace('#', '')})`} />
      <path d={linePath} fill="none" stroke={stroke} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
      <circle cx={Number(lastPoint[0])} cy={Number(lastPoint[1])} r={3} fill={stroke} />
    </svg>
  );
}
