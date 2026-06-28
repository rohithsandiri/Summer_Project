// load-testing/high_load.js
//
// k6 load test simulating heavy load (approaching limit threshold).

import http from 'k6/http';
import { check, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8000';

export const options = {
  stages: [
    { duration: '20s', target: 25 }, // Heavy ramp-up
    { duration: '1m', target: 50 },  // High steady state
    { duration: '20s', target: 0 },  // Ramp-down
  ],
  thresholds: {
    http_req_failed: ['rate<0.05'], // Allow minor rate limits or timeouts
    http_req_duration: ['p(95)<1200'], // Under 1.2s under high load
  },
};

export default function () {
  const headers = {
    'Content-Type': 'application/json',
    'X-Client-IP': `192.168.1.${Math.floor(Math.random() * 200)}`,
  };

  let res = http.get(`${GATEWAY_URL}/api/inventory/item_1`, { headers });
  check(res, {
    'inventory query returns 200 or 429': (r) => [200, 429].includes(r.status),
  });

  sleep(0.2);

  const orderPayload = JSON.stringify({
    item_id: 'item_1',
    quantity: 1,
    idempotency_key: uuidv4(),
  });

  res = http.post(`${GATEWAY_URL}/api/orders`, orderPayload, { headers });
  check(res, {
    'create order returns 201 or 429': (r) => [201, 429].includes(r.status),
  });

  sleep(0.5);
}

import { htmlReport } from 'https://raw.githubusercontent.com/benc-uk/k6-reporter/main/dist/bundle.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';

export function handleSummary(data) {
  return {
    'summary_high.html': htmlReport(data),
    stdout: textSummary(data, { indent: ' ', enableColours: true }),
  };
}
