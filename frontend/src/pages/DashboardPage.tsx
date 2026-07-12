import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, Col, Row, Spin, Typography } from 'antd';
import { getBusinessApps } from '../services/api';

const { Title } = Typography;

const appNameMap: Record<string, string> = {
  finance: '财务中心',
};

const appDescriptionMap: Record<string, string> = {
  finance: '上传财务数据，运行智能体分析流程，生成运营报告并完成审批归档。',
};

interface BusinessApp {
  code: string;
  name: string;
  description: string;
  icon: string;
  status: string;
}

export default function DashboardPage() {
  const [apps, setApps] = useState<BusinessApp[]>([]);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  useEffect(() => {
    getBusinessApps()
      .then(({ data }) => setApps(data.data))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;

  return (
    <div>
      <Title level={4}>业务应用</Title>
      <Row gutter={[16, 16]}>
        {apps.map((app) => (
          <Col key={app.code} xs={24} sm={12} lg={8}>
            <Card hoverable title={appNameMap[app.code] || app.name} onClick={() => navigate(`/${app.code}`)}>
              {appDescriptionMap[app.code] || app.description}
            </Card>
          </Col>
        ))}
      </Row>
    </div>
  );
}
