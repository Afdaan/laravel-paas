import http from 'k6/http';
import { check } from 'k6';

/**
 * REDEPLOY STRESS TEST (MULTIPLE PROJECTS)
 * ---------------------------------------
 * Script ini digunakan untuk mengetes sistem antrian (queue) saat menerima
 * banyak permintaan redeploy untuk BERBAGAI project secara bersamaan.
 *
 * CARA JALANIN (via Docker):
 * docker run --rm -i \
 *   -e TARGET_URL=https://hosting.rplmusaba.my.id \
 *   -e TOKEN="JWT_TOKEN_LU" \
 *   -e PROJECT_IDS="ID1,ID2,ID3" \
 *   grafana/k6 run - <stress-test/redeploy-test.js
 */

// CONFIGURATION
const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8080';
const JWT_TOKEN = __ENV.TOKEN;
const PROJECT_IDS_RAW = __ENV.PROJECT_IDS || '';

export const options = {
    vus: 5,         // Simulasi 5 user bersamaan
    iterations: 15, // Total 15 permintaan redeploy
};

export default function () {
    const projectIds = PROJECT_IDS_RAW.split(',').filter(id => id.trim() !== '');

    if (!JWT_TOKEN || projectIds.length === 0) {
        console.error("❌ ERROR: TOKEN dan PROJECT_IDS (comma separated) harus diisi lewat ENV!");
        return;
    }

    // Ambil ID secara bergantian berdasarkan urutan iterasi
    const projectId = projectIds[__ITER % projectIds.length];

    const params = {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${JWT_TOKEN}`,
        },
    };

    const url = `${BASE_URL}/api/projects/${projectId}/redeploy`;
    const res = http.post(url, JSON.stringify({}), params);

    const success = check(res, {
        'status is 200': (r) => r.status === 200,
    });

    if (success) {
        console.log(`✅ [Project ${projectId}] Redeploy Queued!`);
    } else {
        console.log(`❌ [Project ${projectId}] Failed: ${res.status} - ${res.body}`);
    }
}