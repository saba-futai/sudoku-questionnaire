<script setup lang="ts">
/**
 * Public landing page.
 *
 * The hero runs the real codec on a throwaway table key so the privacy claim is
 * something a visitor can watch happen, not a sentence they have to take on
 * faith. Everything below it is the published questionnaire list, which is the
 * only part of the page that depends on the network.
 */
import {
  AlertTriangle,
  ArrowRight,
  ChevronRight,
  FileText,
  ListChecks,
  RefreshCw,
  ShieldCheck,
} from 'lucide-vue-next';
import { computed, onMounted, ref } from 'vue';
import { RouterLink } from 'vue-router';

import { api } from '@/api/client';
import type { PublicQuestionnaireSummary } from '@/api/types';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import AppSkeleton from '@/components/ui/AppSkeleton.vue';
import EmptyState from '@/components/ui/EmptyState.vue';
import SudokuMark from '@/components/ui/SudokuMark.vue';
import ThemeToggle from '@/components/ui/ThemeToggle.vue';
import { utf8Encode } from '@/codec';
import { errorMessage } from '@/lib/toast';
import { useCodecStore } from '@/stores/codec';

const codec = useCodecStore();

/**
 * A fixed, throwaway table key. The demo must not need a real questionnaire —
 * and it must not look like it is encoding anything the visitor typed, because
 * nothing here ever leaves the page.
 */
const DEMO_KEY = 'demo|q:0';
const DEMO_PLAINTEXT = '{"mood":"good","n":3}';
const DEMO_BYTES = utf8Encode(DEMO_PLAINTEXT).length;

const plaintext = ref('');
const ciphertext = ref('');
const encoding = ref(false);
const codecError = ref('');

/** 1 byte becomes 4 clue symbols, plus the 4-symbol CRC trailer (spec §9). */
const symbolCount = computed(() => ciphertext.value.length);

async function runDemo(): Promise<void> {
  encoding.value = true;
  codecError.value = '';
  try {
    plaintext.value = DEMO_PLAINTEXT;
    ciphertext.value = await codec.encodeBytes(utf8Encode(DEMO_PLAINTEXT), DEMO_KEY, 'ascii');
  } catch (err) {
    codecError.value = errorMessage(err);
    ciphertext.value = '';
  } finally {
    encoding.value = false;
  }
}

const items = ref<PublicQuestionnaireSummary[]>([]);
const loading = ref(true);
const listError = ref('');

async function load(): Promise<void> {
  loading.value = true;
  listError.value = '';
  try {
    // Unauthenticated: the landing page must work for someone who has never
    // logged in, so the token is deliberately not attached.
    const res = await api<{ items: PublicQuestionnaireSummary[] }>('/api/public/questionnaires', {
      auth: false,
    });
    items.value = res.items ?? [];
  } catch (err) {
    listError.value = errorMessage(err);
    items.value = [];
  } finally {
    loading.value = false;
  }
}

function scrollToList(): void {
  document.getElementById('questionnaires')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

onMounted(() => {
  void runDemo();
  void load();
});
</script>

<template>
  <div class="flex min-h-full flex-col">
    <header class="glass sticky top-0 z-40 border-b border-line">
      <div class="mx-auto flex h-14 max-w-6xl items-center gap-3 px-3 sm:h-16 sm:px-6">
        <SudokuMark size="sm" :animate="false" pencil />
        <div class="min-w-0">
          <p class="truncate text-sm font-semibold tracking-tight text-ink">QRforQA</p>
          <p class="truncate text-[11px] text-subtle">数独编码的问卷系统</p>
        </div>
        <div class="ml-auto flex items-center gap-1">
          <ThemeToggle />
          <!-- Deliberately quiet: this is for the operator, not the respondent. -->
          <RouterLink
            to="/admin/login"
            class="flex h-11 items-center rounded-lg px-3 text-xs text-subtle transition-colors hover:bg-sunken hover:text-ink"
          >
            管理入口
          </RouterLink>
        </div>
      </div>
    </header>

    <section class="relative overflow-hidden border-b border-line">
      <div class="grid-paper pointer-events-none absolute inset-0 opacity-70" aria-hidden="true" />
      <div
        class="pointer-events-none absolute -top-32 left-1/2 h-72 w-[42rem] -translate-x-1/2 rounded-full bg-brand-500/10 blur-3xl dark:bg-brand-500/15"
        aria-hidden="true"
      />

      <div
        class="relative mx-auto grid max-w-6xl gap-10 px-4 py-14 sm:px-6 sm:py-20 lg:grid-cols-[1.05fr_1fr] lg:items-center lg:gap-14"
      >
        <div class="animate-fade-up">
          <span
            class="inline-flex items-center gap-2 rounded-full border border-line bg-raised px-3 py-1 text-xs text-muted"
          >
            <ShieldCheck class="size-3.5 text-brand-500" />
            端到端数独编码
          </span>

          <h1
            class="mt-5 text-3xl leading-[1.18] font-semibold tracking-tight text-ink text-balance sm:text-4xl lg:text-[2.75rem]"
          >
            你的答案,在离开浏览器之前<br class="hidden sm:block" />
            就已经变成了一道数独
          </h1>

          <p class="mt-5 max-w-xl text-[15px] leading-7 text-muted">
            问卷结构在浏览器本地被解出,填写内容也在浏览器本地被编码成 4×4 数独的线索字符。
            服务器保存的只有密文,它没有办法还原出你填了什么。
          </p>

          <div class="mt-7 flex flex-wrap items-center gap-3">
            <AppButton size="lg" @click="scrollToList">
              开始填写
              <ArrowRight class="size-4" />
            </AppButton>
            <span v-if="!loading && !listError" class="text-xs text-subtle">
              共 {{ items.length }} 份可填写问卷
            </span>
          </div>
        </div>

        <div class="animate-fade-up space-y-5" style="animation-delay: 90ms">
          <div class="flex justify-center">
            <SudokuMark size="lg" />
          </div>

          <AppCard padding="md">
            <div class="mb-3 flex items-center justify-between gap-3">
              <h2 class="text-sm font-semibold text-ink">实时演示</h2>
              <span class="rounded-full bg-sunken px-2 py-0.5 text-[11px] text-subtle">
                演示用固定编码表
              </span>
            </div>

            <p class="mb-1.5 font-mono text-[11px] tracking-wide text-subtle">明文</p>
            <div class="rounded-[var(--radius-control)] border border-line bg-sunken px-3 py-2">
              <code class="cipher text-ink">{{ plaintext || '…' }}</code>
            </div>

            <div class="my-3 flex items-center justify-center">
              <ArrowRight
                class="size-4 text-subtle transition-transform duration-500"
                :class="encoding ? 'translate-x-1 text-brand-500' : ''"
              />
            </div>

            <p class="mb-1.5 font-mono text-[11px] tracking-wide text-subtle">数独密文</p>
            <div
              class="max-h-28 overflow-y-auto rounded-[var(--radius-control)] border border-line bg-sunken px-3 py-2"
            >
              <code v-if="ciphertext" class="cipher text-brand-700 dark:text-brand-300">
                {{ ciphertext }}
              </code>
              <code v-else class="cipher text-subtle">正在编码…</code>
            </div>

            <p v-if="codecError" class="mt-2 text-xs text-rose-600 dark:text-rose-400">
              {{ codecError }}
            </p>

            <div class="mt-3 flex items-center justify-between gap-3">
              <p v-if="ciphertext" class="font-mono text-[11px] text-subtle">
                {{ DEMO_BYTES }} 字节 → {{ symbolCount }} 个符号
              </p>
              <span v-else />
              <AppButton
                variant="ghost"
                size="sm"
                :loading="encoding"
                @click="runDemo"
              >
                <RefreshCw v-if="!encoding" class="size-3.5" />
                再编一次
              </AppButton>
            </div>

            <p class="mt-2 border-t border-line pt-2 text-[11px] leading-5 text-subtle">
              这一段只在本机演示。同一个字节每次会编成不同的线索,这正是它无法被反查的原因。
            </p>
          </AppCard>
        </div>
      </div>
    </section>

    <section id="questionnaires" class="mx-auto w-full max-w-6xl flex-1 px-4 py-12 sm:px-6 sm:py-16">
      <div class="mb-6 flex items-end justify-between gap-4">
        <div>
          <h2 class="text-xl font-semibold tracking-tight text-ink">可填写的问卷</h2>
          <p class="mt-1 text-sm text-muted">选择一个开始,答案只会以密文离开你的设备。</p>
        </div>
        <AppButton
          v-if="!loading && !listError && items.length"
          variant="ghost"
          size="sm"
          @click="load"
        >
          <RefreshCw class="size-3.5" />
          刷新
        </AppButton>
      </div>

      <div v-if="loading" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <AppSkeleton v-for="n in 3" :key="n" variant="card" />
      </div>

      <AppCard v-else-if="listError" padding="lg" class="animate-fade-up">
        <div class="flex flex-col items-center gap-4 py-6 text-center">
          <div class="grid size-12 place-items-center rounded-2xl bg-rose-50 text-rose-600 dark:bg-rose-500/10 dark:text-rose-400">
            <AlertTriangle class="size-5" />
          </div>
          <div>
            <p class="text-sm font-semibold text-ink">问卷列表加载失败</p>
            <p class="mt-1 text-sm text-muted">{{ listError }}</p>
          </div>
          <AppButton variant="secondary" @click="load">
            <RefreshCw class="size-4" />
            重试
          </AppButton>
        </div>
      </AppCard>

      <AppCard v-else-if="!items.length" padding="none" class="animate-fade-up">
        <EmptyState
          title="暂时没有可填写的问卷"
          description="发布之后,问卷会出现在这里。如果你刚刚发布,稍等片刻再刷新。"
          :icon="FileText"
        >
          <AppButton variant="secondary" @click="load">
            <RefreshCw class="size-4" />
            刷新
          </AppButton>
        </EmptyState>
      </AppCard>

      <div v-else class="stagger grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <RouterLink
          v-for="(item, i) in items"
          :key="item.slug"
          :to="`/q/${item.slug}`"
          class="group block rounded-[var(--radius-card)]"
          :style="{ '--i': String(i) }"
        >
          <AppCard hover padding="md" class="flex h-full flex-col">
            <div class="flex items-start justify-between gap-3">
              <h3
                class="text-base leading-6 font-semibold text-ink transition-colors group-hover:text-brand-600 dark:group-hover:text-brand-400"
              >
                {{ item.title || '未命名问卷' }}
              </h3>
              <ChevronRight
                class="mt-0.5 size-4 shrink-0 text-subtle transition-transform duration-200 group-hover:translate-x-0.5 group-hover:text-brand-500"
              />
            </div>

            <p v-if="item.description" class="mt-2 line-clamp-3 text-sm leading-6 text-muted">
              {{ item.description }}
            </p>

            <div class="mt-auto flex flex-wrap items-center gap-x-4 gap-y-1 pt-4 text-xs text-subtle">
              <span class="inline-flex items-center gap-1.5">
                <ListChecks class="size-3.5" />
                {{ item.stepCount }} 步
              </span>
              <span class="inline-flex items-center gap-1.5">
                <FileText class="size-3.5" />
                {{ item.questionCount }} 题
              </span>
            </div>
          </AppCard>
        </RouterLink>
      </div>
    </section>

    <footer class="border-t border-line px-4 py-10 sm:px-6">
      <div class="mx-auto flex max-w-3xl flex-col items-center gap-3 text-center">
        <SudokuMark size="sm" :animate="false" pencil />
        <p class="text-xs leading-6 text-subtle text-balance">
          数据库里只有数独密文。即使整库被拖走,没有对应的编码表,任何一个答案都无法被还原。
        </p>
        <RouterLink to="/admin/login" class="text-xs text-subtle transition-colors hover:text-ink">
          管理员登录
        </RouterLink>
      </div>
    </footer>
  </div>
</template>
