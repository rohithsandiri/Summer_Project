// load-testing/low_load.js
//
// k6 load test simulating low background traffic to baseline performance.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8000';

export const options = {
  stages: [
    { duration: '15s', target: 2 },  // Warm-up
    { duration: '30s', target: 5 },  // Low traffic steady state
    { duration: '15s', target: 0 },  // Ramp-down
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<300'], // 95% of requests under 300ms
  },
};

export default function () {
  const headers = {
    'Content-Type': 'application/json',
  };

  // Simple read inventory endpoint
  const res = http.get(`${GATEWAY_URL}/api/inventory/item_1`, { headers });
  check(res, {
    'inventory query returns 200': (r) => r.status === 200,
  });

  sleep(1);
}

import { htmlReport } from 'https://raw.githubusercontent.com/benc-uk/k6-reporter/main/dist/bundle.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';

export function handleSummary(data) {
  return {
    'summary_low.html': htmlReport(data),
    stdout: textSummary(data, { indent: ' ', enableColours: true }),
  };
}
