import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, Col, Row, Spin, Typography } from 'antd';
import { getBusinessApps } from '../services/api';

const { Title } = Typography;

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
      <Title level={4}>Business Apps</Title>
      <Row gutter={[16, 16]}>
        {apps.map((app) => (
          <Col key={app.code} xs={24} sm={12} lg={8}>
            <Card hoverable title={app.name} onClick={() => navigate(`/${app.code}`)}>
              {app.description}
            </Card>
          </Col>
        ))}
      </Row>
    </div>
  );
}
