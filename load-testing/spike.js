// load-testing/spike.js
//
// k6 load test simulating a sudden spike in traffic.
// Checks how the rate limiter, gateway circuit breakers, and services respond.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:8000';

export const options = {
  stages: [
    { duration: '10s', target: 5 },   // warm-up
    { duration: '10s', target: 5 },
    { duration: '5s', target: 100 },  // sudden spike
    { duration: '30s', target: 100 }, // hold spike
    { duration: '5s', target: 5 },    // cooldown
    { duration: '10s', target: 5 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.05'], // error rate should be < 5% (allowing some 429 rate limit errors)
    http_req_duration: ['p(95)<1000'], // 95% of requests must complete within 1s
  },
};

export default function () {
  const headers = {
    'Content-Type': 'application/json',
    'X-Client-IP': `192.168.1.${Math.floor(Math.random() * 50)}`, // simulate IP pooling for rate limit
  };

  // 1. Check health
  let res = http.get(`${GATEWAY_URL}/health`, { headers });
  check(res, {
    'health returns 200': (r) => r.status === 200,
  });

  sleep(0.5);

  // 2. Create an order (idempotency key ensures we test Saga execution and cache)
  const orderPayload = JSON.stringify({
    item_id: 'item_1',
    quantity: 1,
    idempotency_key: uuidv4(),
  });

  res = http.post(`${GATEWAY_URL}/api/orders`, orderPayload, { headers });
  check(res, {
    'create order response is 200, 201 or 429': (r) => [200, 201, 429].includes(r.status),
  });

  if (res.status === 201) {
    const body = JSON.parse(res.body);
    const orderID = body.order_id;

    // 3. Query the order back (tests cache read)
    sleep(0.2);
    res = http.get(`${GATEWAY_URL}/api/orders/${orderID}`, { headers });
    check(res, {
      'get order returns 200': (r) => r.status === 200,
    });
  }

  sleep(1);
}

// Generate HTML Report at the end of the test execution
import { htmlReport } from 'https://raw.githubusercontent.com/benc-uk/k6-reporter/main/dist/bundle.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';

export function handleSummary(data) {
  return {
    'summary_spike.html': htmlReport(data),
    stdout: textSummary(data, { indent: ' ', enableColours: true }),
  };
}
