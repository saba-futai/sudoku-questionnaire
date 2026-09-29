<script setup lang="ts">
/**
 * Administrator sign-in. Deliberately the only screen in the admin area that
 * renders without a session, so it carries its own background treatment rather
 * than the shell.
 */
import { ArrowLeft, Eye, EyeOff, Lock, ShieldCheck, User } from 'lucide-vue-next';
import { computed, ref } from 'vue';
import { RouterLink, useRoute, useRouter } from 'vue-router';

import AppButton from '@/components/ui/AppButton.vue';
import AppInput from '@/components/ui/AppInput.vue';
import SudokuMark from '@/components/ui/SudokuMark.vue';
import { errorMessage } from '@/lib/toast';
import { useAuthStore } from '@/stores/auth';

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

const username = ref('');
const password = ref('');
const revealed = ref(false);
const error = ref('');
const submitting = ref(false);

// Set by the router guard when a token is rejected mid-session. It is an
// explanation, not a failure, so it must not read like an error.
const expired = computed(() => Boolean(route.query.expired));

async function submit(): Promise<void> {
  if (submitting.value) return;
  error.value = '';

  if (!username.value.trim()) {
    error.value = '请输入用户名';
    return;
  }
  if (!password.value) {
    error.value = '请输入密码';
    return;
  }

  submitting.value = true;
  try {
    await auth.login(username.value.trim(), password.value);
    const target = route.query.redirect;
    // Only ever follow an in-app path, so a crafted ?redirect= cannot bounce
    // the admin off-site.
    const next = typeof target === 'string' && target.startsWith('/') ? target : '/admin';
    await router.replace(next);
  } catch (err) {
    error.value = errorMessage(err);
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div class="relative min-h-screen overflow-hidden bg-surface">
    <!-- Grid paper plus two soft light sources: the surface a puzzle is solved on. -->
    <div class="grid-paper pointer-events-none absolute inset-0 opacity-60" aria-hidden="true" />
    <div
      class="pointer-events-none absolute -top-32 left-1/2 size-[560px] -translate-x-1/2 rounded-full bg-brand-500/10 blur-3xl"
      aria-hidden="true"
    />
    <div
      class="pointer-events-none absolute -bottom-40 -right-24 size-[420px] rounded-full bg-pencil-400/10 blur-3xl"
      aria-hidden="true"
    />

    <div class="relative flex min-h-screen flex-col items-center justify-center px-4 py-10">
      <div class="animate-fade-up w-full max-w-[420px]">
        <div class="mb-7 flex flex-col items-center text-center">
          <SudokuMark size="md" />
          <p class="mt-4 text-lg font-semibold tracking-tight text-ink">QRforQA</p>
          <p class="mt-0.5 text-xs text-subtle">数独编码问卷系统</p>
        </div>

        <div class="card relative overflow-hidden p-6 sm:p-8">
          <!-- A watermark of the mark, echoing the encoding that runs underneath. -->
          <SudokuMark
            size="lg"
            :animate="false"
            pencil
            class="pointer-events-none absolute -right-6 -top-6 opacity-[0.06]"
          />

          <div class="relative">
            <h1 class="text-xl font-semibold tracking-tight text-ink">管理员登录</h1>
            <p class="mt-1 text-sm text-muted">登录后可查看解码后的答卷明文与统计。</p>

            <p
              v-if="expired"
              class="mt-5 flex items-start gap-2 rounded-[var(--radius-control)] border border-amber-200 bg-amber-50 px-3 py-2.5 text-xs text-amber-800 dark:border-amber-400/25 dark:bg-amber-500/10 dark:text-amber-200"
            >
              <ShieldCheck class="mt-px size-4 shrink-0" />
              登录已过期,请重新登录
            </p>

            <form class="mt-6 space-y-4" novalidate @submit.prevent="submit">
              <AppInput
                v-model="username"
                label="用户名"
                placeholder="请输入管理员用户名"
                autocomplete="username"
                :disabled="submitting"
              />

              <!-- AppInput has no trailing slot, so the reveal toggle sits at the
                   end of the label row rather than over the field, where it
                   would cover a long password. -->
              <div class="relative">
                <AppInput
                  v-model="password"
                  label="密码"
                  :type="revealed ? 'text' : 'password'"
                  placeholder="请输入密码"
                  autocomplete="current-password"
                  :disabled="submitting"
                />
                <button
                  type="button"
                  class="absolute right-0 top-0 grid size-6 place-items-center rounded text-subtle transition hover:text-ink"
                  :aria-label="revealed ? '隐藏密码' : '显示密码'"
                  :aria-pressed="revealed"
                  @click="revealed = !revealed"
                >
                  <component :is="revealed ? EyeOff : Eye" class="size-4" />
                </button>
              </div>

              <p
                v-if="error"
                role="alert"
                class="animate-fade-in flex items-start gap-2 rounded-[var(--radius-control)] border border-rose-200 bg-rose-50 px-3 py-2.5 text-xs text-rose-700 dark:border-rose-400/25 dark:bg-rose-500/10 dark:text-rose-300"
              >
                <Lock class="mt-px size-4 shrink-0" />
                {{ error }}
              </p>

              <AppButton type="submit" size="lg" block :loading="submitting">
                {{ submitting ? '登录中…' : '登录' }}
              </AppButton>
            </form>

            <p class="mt-5 flex items-center justify-center gap-1.5 text-xs text-subtle">
              <User class="size-3.5" />
              仅限授权管理员访问
            </p>
          </div>
        </div>

        <div class="mt-6 text-center">
          <RouterLink
            to="/"
            class="inline-flex items-center gap-1.5 rounded text-xs text-muted transition hover:text-ink"
          >
            <ArrowLeft class="size-3.5" />
            返回问卷列表
          </RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>
