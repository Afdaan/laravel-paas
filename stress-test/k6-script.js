import http from 'k6/http';
import { check, sleep } from 'k6';

// -------------------------------------------------------------------------
// CONFIGURATION
// -------------------------------------------------------------------------
// Gunakan URL backend API atau URL project Laravel yang ingin ditest.
const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8080';

export const options = {
    // Tahapan pengetesan (Ramping up)
    stages: [
        { duration: '30s', target: 20 },  // Naik ke 20 virtual users (VUs) dalam 30 detik
        { duration: '1m', target: 20 },   // Bertahan di 20 VUs selama 1 menit (Warm up)
        { duration: '30s', target: 100 }, // Naik ke 100 VUs (Stress test mulai)
        { duration: '1m', target: 100 },  // Bertahan di 100 VUs
        { duration: '30s', target: 200 }, // Naik ke 200 VUs (Mencari breaking point)
        { duration: '1m', target: 200 },  // Bertahan di 200 VUs
        { duration: '30s', target: 0 },   // Turun kembali ke 0
    ],
    thresholds: {
        http_req_failed: ['rate<0.01'], // Gagal jika error rate di atas 1%
        http_req_duration: ['p(95)<500'], // Gagal jika 95% request lebih lambat dari 500ms
    },
};

export default function () {
    // 1. Hit Landing Page atau Health Check
    const res = http.get(`${BASE_URL}/`);

    check(res, {
        'status is 200': (r) => r.status === 200,
        'response time < 200ms': (r) => r.timings.duration < 200,
    });

    // Simulasi user "mikir" atau browsing
    sleep(1);

    // 2. Jika ini testing API, bisa coba hit endpoint spesifik
    // const payload = JSON.stringify({ email: 'admin@localhost', password: 'admin123' });
    // const params = { headers: { 'Content-Type': 'application/json' } };
    // http.post(`${BASE_URL}/api/auth/login`, payload, params);
}

