import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

/**
 * End-to-end tests driving a real browser against a built server.
 *
 * The point of interest is the CLIENT-SIDE codec: the browser fetches the
 * validity bitmap, derives its own table from the per-questionnaire key,
 * verifies its fingerprint against the server's, encodes the answers, and only
 * then submits. The Python smoke test cannot cover that path because it encodes
 * with the Go CLI instead.
 *
 * Requires a running server with the frontend bundle embedded:
 *   make web && (cd backend && go build -o bin/qrforqa ./cmd/server && ./bin/qrforqa serve)
 */

const ADMIN_USER = process.env.QRFORQA_ADMIN_USER ?? 'admin';
const ADMIN_PASS = process.env.QRFORQA_ADMIN_PASSWORD ?? '';

test.skip(!ADMIN_PASS, 'QRFORQA_ADMIN_PASSWORD must be set to run the browser tests');

/** Creates a questionnaire through the API so tests do not depend on the builder UI. */
async function seedQuestionnaire(request: APIRequestContext, token: string, title: string) {
  const response = await request.post('/api/admin/questionnaires', {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      status: 'published',
      definition: {
        title,
        description: 'Playwright 端到端测试问卷',
        steps: [
          {
            id: 's1',
            title: '第一步',
            questions: [
              { key: 'name', type: 'text', label: '你的称呼', required: true, maxLength: 40 },
              {
                key: 'role',
                type: 'single',
                label: '你的角色',
                required: true,
                options: [
                  { value: 'dev', label: '开发' },
                  { value: 'ops', label: '运维' },
                ],
              },
            ],
          },
          {
            id: 's2',
            title: '第二步',
            questions: [
              {
                key: 'topics',
                type: 'multi',
                label: '关注方向',
                required: false,
                options: [
                  { value: 'perf', label: '性能' },
                  { value: 'ux', label: '体验' },
                ],
              },
              { key: 'score', type: 'rating', label: '总体评分', required: true, min: 1, max: 5 },
              { key: 'comment', type: 'textarea', label: '补充说明', required: false, maxLength: 300 },
            ],
          },
        ],
      },
    },
  });
  expect(response.ok(), `seed failed: ${response.status()} ${await response.text()}`).toBeTruthy();
  return (await response.json()) as { id: number; slug: string };
}

/**
 * One token for the whole run. Re-authenticating in every test would hammer the
 * login limiter, and the limiter exists on purpose — the test should not be the
 * reason it gets relaxed. Exactly one test exercises the login form itself.
 */
let cachedToken: string | null = null;

async function login(request: APIRequestContext): Promise<string> {
  if (cachedToken) return cachedToken;
  const response = await request.post('/api/admin/login', {
    data: { username: ADMIN_USER, password: ADMIN_PASS },
  });
  expect(response.ok(), `admin login failed: ${response.status()}`).toBeTruthy();
  cachedToken = ((await response.json()) as { token: string }).token;
  return cachedToken;
}

/** Seeds the session directly, so admin tests do not depend on the login UI. */
async function signIn(page: Page, request: APIRequestContext) {
  const token = await login(request);
  await page.addInitScript(
    ([t, u]) => {
      localStorage.setItem('qrforqa:token', t);
      localStorage.setItem('qrforqa:username', u);
    },
    [token, ADMIN_USER],
  );
  await page.goto('/admin');
  await expect(page).toHaveURL(/\/admin($|\/)/, { timeout: 20_000 });
}

test.describe('respondent flow (browser-side codec)', () => {
  test('a respondent fills and submits, and the browser really encodes it', async ({ page, request }) => {
    const token = await login(request);
    const { id, slug } = await seedQuestionnaire(request, token, `浏览器测试 ${Date.now()}`);

    try {
      await page.goto(`/q/${slug}`);

      // The browser must decode the definition itself; if the table derivation
      // failed, the title would never render.
      await expect(page.getByRole('heading', { name: /浏览器测试/ })).toBeVisible({ timeout: 20_000 });

      // ---- step 1
      await page.getByLabel(/你的称呼/).fill('浏览器测试员');
      await page.getByText('开发', { exact: true }).click();
      await page.getByRole('button', { name: /下一步|继续/ }).click();

      // ---- step 2
      await expect(page.getByText('关注方向')).toBeVisible();
      await page.getByText('性能', { exact: true }).click();
      await page.getByText('体验', { exact: true }).click();
      await page.getByRole('radio', { name: /4/ }).click().catch(async () => {
        // Some rating implementations render buttons rather than radios.
        await page.locator('[data-rating-value="4"]').click();
      });
      await page.getByLabel(/补充说明/).fill('来自浏览器的端到端测试 🎯');

      // Capture the submission to prove the payload left the browser as
      // sudoku ciphertext rather than JSON.
      const [submitRequest] = await Promise.all([
        page.waitForRequest(
          (req) => req.url().includes(`/api/public/questionnaires/${slug}/responses`) && req.method() === 'POST',
          { timeout: 30_000 },
        ),
        page.getByRole('button', { name: /提交/ }).click(),
      ]);

      const body = submitRequest.postDataJSON() as { cipher: string; layout: string };
      expect(body.layout).toBe('ascii');

      // 4 wire symbols per plaintext byte plus a 4-symbol trailer.
      const plaintextBytes = new TextEncoder().encode(
        JSON.stringify({ name: '浏览器测试员', role: 'dev', topics: ['perf', 'ux'], score: 4, comment: '来自浏览器的端到端测试 🎯' }),
      ).length;
      expect(body.cipher.length).toBeGreaterThanOrEqual(plaintextBytes * 4);

      // The ASCII layout emits only 0x0A and 0x20..0x7E; seeing any other byte
      // would mean the layout contract was broken.
      const codes = Array.from(body.cipher).map((c) => c.charCodeAt(0));
      const outside = codes.filter((c) => c !== 0x0a && (c < 0x20 || c > 0x7e));
      expect(outside, `ciphertext contains bytes outside the ascii layout: ${outside.slice(0, 8)}`).toEqual([]);

      // The payload must not contain the plaintext it encodes.
      expect(body.cipher).not.toContain('浏览器测试员');
      expect(body.cipher).not.toContain('perf');

      // ---- success
      await expect(page).toHaveURL(new RegExp(`/q/${slug}/done`), { timeout: 20_000 });

      // ---- the administrator sees decoded content
      await signIn(page, request);
      await page.goto(`/admin/questionnaires/${id}/responses`);
      await expect(page.getByText('浏览器测试员')).toBeVisible({ timeout: 20_000 });
      await expect(page.getByText('开发')).toBeVisible();
    } finally {
      await request.delete(`/api/admin/questionnaires/${id}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
    }
  });

  test('required questions block progress, and the error is actionable', async ({ page, request }) => {
    const token = await login(request);
    const { id, slug } = await seedQuestionnaire(request, token, `必填校验 ${Date.now()}`);
    try {
      await page.goto(`/q/${slug}`);
      await expect(page.getByRole('heading', { name: /必填校验/ })).toBeVisible({ timeout: 20_000 });

      // Nothing answered yet: advancing must be refused.
      await page.getByRole('button', { name: /下一步|继续/ }).click();
      await expect(page).toHaveURL(new RegExp(`/q/${slug}$`));
      await expect(page.getByText(/必答|必填|请填写|请选择/).first()).toBeVisible();
    } finally {
      await request.delete(`/api/admin/questionnaires/${id}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
    }
  });
});

test.describe('administrator surfaces', () => {
  test('the codec inspector shows the server and browser agreeing', async ({ page, request }) => {
    const token = await login(request);
    const { id } = await seedQuestionnaire(request, token, `检视器 ${Date.now()}`);
    try {
      await signIn(page, request);
      await page.goto('/admin/codec');

      await expect(page.getByRole('heading', { name: '实时对照' })).toBeVisible({ timeout: 25_000 });
      await expect(page.getByRole('heading', { name: '一致性校验' })).toBeVisible();
      await expect(page.getByRole('heading', { name: '编码表属性' })).toBeVisible();

      // The client-derived fingerprint must equal the server's; the page shows
      // a mismatch banner otherwise, so its absence is the assertion.
      await expect(page.getByText(/不一致/)).toHaveCount(0);
    } finally {
      await request.delete(`/api/admin/questionnaires/${id}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
    }
  });

  test('the login form authenticates and rejects a bad password', async ({ page }) => {
    await page.goto('/admin/login');
    await expect(page.getByRole('heading', { name: '管理员登录' })).toBeVisible();

    // The password field is addressed by type: the reveal toggle carries an
    // aria-label containing 密码, so a label query would be ambiguous.
    const password = page.locator('input[type="password"]');
    await page.getByLabel(/用户名/).fill(ADMIN_USER);
    await password.fill('definitely-not-the-password');
    await page.getByRole('button', { name: /登录/ }).click();
    await expect(page.getByText(/不正确|失败|错误/).first()).toBeVisible({ timeout: 15_000 });
    await expect(page).toHaveURL(/\/admin\/login/);

    await password.fill(ADMIN_PASS);
    await page.getByRole('button', { name: /登录/ }).click();
    await expect(page).toHaveURL(/\/admin($|\/)/, { timeout: 20_000 });
  });

  test('the dashboard renders its statistics', async ({ page, request }) => {
    await signIn(page, request);
    // Named tiles rather than a loose /问卷/ match: on mobile the sidebar
    // (which contains 问卷管理) is hidden, so a generic match would resolve to
    // a display:none element.
    await expect(page.getByText('答卷总数', { exact: false }).filter({ visible: true }).first()).toBeVisible({
      timeout: 20_000,
    });
    await expect(page.getByText('问卷总数', { exact: false }).filter({ visible: true }).first()).toBeVisible();
  });

  test('the questionnaire list is reachable and exports a link', async ({ page, request }) => {
    const token = await login(request);
    const { id, slug } = await seedQuestionnaire(request, token, `列表测试 ${Date.now()}`);
    try {
      await signIn(page, request);
      await page.goto('/admin/questionnaires');
      // The list renders a table on desktop and cards on mobile, and both are
      // in the DOM at all times (one is hidden by CSS). Filter to what the
      // current viewport actually shows.
      await expect(
        page.getByText(/列表测试/).filter({ visible: true }).first(),
      ).toBeVisible({ timeout: 20_000 });
      await expect(page.getByText(slug, { exact: false }).filter({ visible: true }).first()).toBeVisible();
    } finally {
      await request.delete(`/api/admin/questionnaires/${id}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
    }
  });
});

test.describe('layout integrity', () => {
  test('no page overflows horizontally on a phone viewport', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('body')).toBeVisible();

    for (const path of ['/', '/admin/login']) {
      await page.goto(path);
      await page.waitForLoadState('networkidle');
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      );
      expect(overflow, `${path} overflows by ${overflow}px`).toBeLessThanOrEqual(1);
    }
  });
});
