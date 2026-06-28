// load-testing/soak.js
//
// k6 load test simulating steady moderate load over a longer period.
// Aims to uncover memory leaks, cache growth problems, or connection leaks.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8000';

export const options = {
  stages: [
    { duration: '30s', target: 15 }, // ramp-up
    { duration: '3m', target: 15 },  // steady soak run
    { duration: '30s', target: 0 },  // ramp-down
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'],   // error rate must be < 1%
    http_req_duration: ['p(95)<300'], // 95% of requests must complete within 300ms
  },
};

export default function () {
  const headers = {
    'Content-Type': 'application/json',
    'X-Client-IP': `192.168.2.${Math.floor(Math.random() * 100)}`, // simulate IP pooling
  };

  // Run a mix of inventory queries and health checks
  const res = http.get(`${GATEWAY_URL}/api/inventory/item_2`, { headers });
  check(res, {
    'inventory check is successful': (r) => r.status === 200,
  });

  sleep(0.5);
}

// Generate HTML Report at the end of the test execution
import { htmlReport } from 'https://raw.githubusercontent.com/benc-uk/k6-reporter/main/dist/bundle.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';

export function handleSummary(data) {
  return {
    'summary_soak.html': htmlReport(data),
    stdout: textSummary(data, { indent: ' ', enableColours: true }),
  };
}
