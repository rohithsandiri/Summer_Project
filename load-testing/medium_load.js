// load-testing/medium_load.js
//
// k6 load test simulating normal operational load (medium traffic).

import http from 'k6/http';
import { check, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8000';

export const options = {
  stages: [
    { duration: '15s', target: 10 }, // Ramp-up
    { duration: '45s', target: 20 }, // Medium steady state
    { duration: '15s', target: 0 },  // Ramp-down
  ],
  thresholds: {
    http_req_failed: ['rate<0.02'],
    http_req_duration: ['p(95)<500'], // 95% of requests under 500ms
  },
};

export default function () {
  const headers = {
    'Content-Type': 'application/json',
    'X-Client-IP': `192.168.1.${Math.floor(Math.random() * 200)}`,
  };

  // Run a mix of read and write workflows
  let res = http.get(`${GATEWAY_URL}/api/inventory/item_1`, { headers });
  check(res, {
    'inventory query returns 200': (r) => r.status === 200,
  });

  sleep(0.5);

  const orderPayload = JSON.stringify({
    item_id: 'item_1',
    quantity: 1,
    idempotency_key: uuidv4(),
  });

  res = http.post(`${GATEWAY_URL}/api/orders`, orderPayload, { headers });
  check(res, {
    'create order returns 201 or 429': (r) => [201, 429].includes(r.status),
  });

  sleep(1);
}

import { htmlReport } from 'https://raw.githubusercontent.com/benc-uk/k6-reporter/main/dist/bundle.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';

export function handleSummary(data) {
  return {
    'summary_medium.html': htmlReport(data),
    stdout: textSummary(data, { indent: ' ', enableColours: true }),
  };
}
