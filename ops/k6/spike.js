import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '10s', target: 100 },   // Normal load
    { duration: '1m', target: 1000 },   // Spike to 10x
    { duration: '10s', target: 100 },   // Back to normal
    { duration: '10s', target: 0 },     // Ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(95)<500'],  // Degraded but acceptable
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://api.coldy.example.com';

export default function () {
  const res = http.get(`${BASE_URL}/v1/catalog/products`);
  
  check(res, {
    'status is 200': (r) => r.status === 200,
    'response time ok': (r) => r.timings.duration < 500,
  });

  sleep(0.5);
}

