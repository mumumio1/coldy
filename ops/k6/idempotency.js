import http from 'k6/http';
import { check } from 'k6';
import { SharedArray } from 'k6/data';

export const options = {
  scenarios: {
    concurrent_requests: {
      executor: 'shared-iterations',
      vus: 10,
      iterations: 100,
      maxDuration: '30s',
    },
  },
  thresholds: {
    'checks': ['rate==1.0'], // All checks must pass for idempotency
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://api.coldy.example.com';

// Use same idempotency key across VUs
const IDEMPOTENCY_KEY = `test-${Date.now()}`;

export default function () {
  const payload = JSON.stringify({
    idempotency_key: IDEMPOTENCY_KEY,
    user_id: 'test-user',
    items: [{ product_id: 'product-1', quantity: 1 }],
    shipping_address: {
      street: '123 Test St',
      city: 'Test City',
      state: 'TS',
      postal_code: '12345',
      country: 'US',
    },
  });

  const res = http.post(`${BASE_URL}/v1/orders`, payload, {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${__ENV.AUTH_TOKEN}`,
      'Idempotency-Key': IDEMPOTENCY_KEY,
    },
  });

  // All responses should be identical (same order ID)
  check(res, {
    'status is 200 or 201': (r) => r.status === 200 || r.status === 201,
    'has order id': (r) => JSON.parse(r.body).order.id !== undefined,
  });
}

export function handleSummary(data) {
  const orderIds = new Set();
  
  console.log(`\nIdempotency Test Results:`);
  console.log(`Total requests: ${data.metrics.http_reqs.values.count}`);
  console.log(`Checks passed: ${data.metrics.checks.values.passes}`);
  console.log(`All requests should return the same order ID`);
  
  return {
    'stdout': JSON.stringify(data, null, 2),
  };
}

