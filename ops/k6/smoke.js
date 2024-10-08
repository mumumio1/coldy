import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 1,
  duration: '1m',
  thresholds: {
    'http_req_failed': ['rate<0.01'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
  // Health check
  const healthRes = http.get(`${BASE_URL}/health`);
  check(healthRes, {
    'health check ok': (r) => r.status === 200,
  });

  // Get products
  const catalogRes = http.get(`${BASE_URL}/v1/catalog/products`);
  check(catalogRes, {
    'catalog available': (r) => r.status === 200,
  });

  sleep(1);
}

