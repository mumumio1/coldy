import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const orderLatency = new Trend('order_latency');

export const options = {
  stages: [
    { duration: '2m', target: 300 },  // Ramp up to 300 RPS
    { duration: '5m', target: 300 },  // Stay at 300 RPS
    { duration: '2m', target: 800 },  // Ramp up to 800 RPS
    { duration: '5m', target: 800 },  // Stay at 800 RPS
    { duration: '2m', target: 0 },    // Ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(95)<120', 'p(99)<200'], // SLO: p95 < 120ms
    'http_req_failed': ['rate<0.01'],                 // SLO: error rate < 1%
    'errors': ['rate<0.008'],                         // SLO: error rate < 0.8%
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://api.coldy.example.com';

export default function () {
  // Create order (idempotent)
  const idempotencyKey = `order-${Date.now()}-${__VU}-${__ITER}`;
  const orderPayload = JSON.stringify({
    idempotency_key: idempotencyKey,
    user_id: `user-${__VU}`,
    items: [
      {
        product_id: 'product-1',
        quantity: 2,
      },
    ],
    shipping_address: {
      street: '123 Main St',
      city: 'San Francisco',
      state: 'CA',
      postal_code: '94102',
      country: 'US',
    },
  });

  const orderStart = Date.now();
  const orderRes = http.post(`${BASE_URL}/v1/orders`, orderPayload, {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${__ENV.AUTH_TOKEN}`,
      'Idempotency-Key': idempotencyKey,
    },
  });

  orderLatency.add(Date.now() - orderStart);

  const orderSuccess = check(orderRes, {
    'order created': (r) => r.status === 200 || r.status === 201,
    'order latency < 120ms': (r) => r.timings.duration < 120,
  });

  if (!orderSuccess) {
    errorRate.add(1);
  } else {
    errorRate.add(0);

    // Get order details
    const orderId = JSON.parse(orderRes.body).order.id;
    const getRes = http.get(`${BASE_URL}/v1/orders/${orderId}`, {
      headers: {
        'Authorization': `Bearer ${__ENV.AUTH_TOKEN}`,
      },
    });

    check(getRes, {
      'get order success': (r) => r.status === 200,
    });
  }

  sleep(1);
}

export function handleSummary(data) {
  return {
    'summary.json': JSON.stringify(data),
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
  };
}

function textSummary(data, options) {
  const indent = options.indent || '';
  const colors = options.enableColors || false;

  let summary = '\n';
  summary += `${indent}Checks...............: ${data.metrics.checks.values.passes / data.metrics.checks.values.count * 100}% passed\n`;
  summary += `${indent}Requests.............: ${data.metrics.http_reqs.values.count} total\n`;
  summary += `${indent}Request duration.....: avg=${data.metrics.http_req_duration.values.avg.toFixed(2)}ms p95=${data.metrics.http_req_duration.values['p(95)'].toFixed(2)}ms\n`;
  summary += `${indent}Request failed.......: ${(data.metrics.http_req_failed.values.rate * 100).toFixed(2)}%\n`;

  return summary;
}

