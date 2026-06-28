// load-testing/stress.js
//
// k6 load test slowly ramping up load to stress-test the system capacity.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8000';

export const options = {
  stages: [
    { duration: '30s', target: 10 },  // ramp-up
    { duration: '1m', target: 30 },   // steady state
    { duration: '30s', target: 80 },  // push past rate limit
    { duration: '1m', target: 80 },   // hold high stress
    { duration: '30s', target: 0 },   // ramp-down
  ],
  thresholds: {
    http_req_duration: ['p(99)<2000'], // 99% of requests must complete within 2s
  },
};

export default function () {
  const headers = {
    'Content-Type': 'application/json',
    'X-Client-IP': `192.168.1.${Math.floor(Math.random() * 200)}`, // simulate IP pooling
  };

  // Hit endpoint
  const res = http.get(`${GATEWAY_URL}/api/inventory/item_1`, { headers });
  check(res, {
    'inventory query returns 200 or 429': (r) => [200, 429].includes(r.status),
  });

  sleep(0.1);
}

// Generate HTML Report at the end of the test execution
import { htmlReport } from 'https://raw.githubusercontent.com/benc-uk/k6-reporter/main/dist/bundle.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';

export function handleSummary(data) {
  return {
    'summary_stress.html': htmlReport(data),
    stdout: textSummary(data, { indent: ' ', enableColours: true }),
  };
}
