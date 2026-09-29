<script setup lang="ts">
/**
 * Submission receipt.
 *
 * Everything shown here is optional: the byte counts arrive in the query string
 * from FillView (so they survive a refresh without another round trip), and if
 * they are missing the page simply shows the confirmation alone rather than an
 * empty panel or a zero.
 */
import { Check, ChevronDown, Hash } from 'lucide-vue-next';
import { computed, ref } from 'vue';
import { RouterLink, useRoute } from 'vue-router';

import AppCard from '@/components/ui/AppCard.vue';
import SudokuMark from '@/components/ui/SudokuMark.vue';
import { formatDuration } from '@/lib/format';

const route = useRoute();

/** Reads a non-negative integer from the query, tolerating junk. */
function numeric(name: string): number | null {
  const raw = route.query[name];
  const value = Number(Array.isArray(raw) ? raw[0] : raw);
  return Number.isFinite(value) && value > 0 ? value : null;
}

const plaintextBytes = computed(() => numeric('bytes'));
const cipherSymbols = computed(() => numeric('cipher'));
const durationMs = computed(() => numeric('ms'));

/** Each plaintext byte becomes 4 clue symbols, plus a 4-symbol CRC trailer. */
const expansion = computed(() =>
  plaintextBytes.value ? (cipherSymbols.value ?? 0) / plaintextBytes.value : 0,
);

const hasDetail = computed(() => plaintextBytes.value !== null && cipherSymbols.value !== null);
const expanded = ref(false);
</script>

<template>
  <div class="flex min-h-full flex-col">
    <div class="grid-paper flex-1 px-4 py-16 sm:px-6 sm:py-24">
      <div class="mx-auto flex w-full max-w-lg flex-col items-center text-center">
        <div
          class="animate-pulse-ring grid size-20 place-items-center rounded-full bg-brand-600 text-white dark:bg-brand-500"
        >
          <Check class="animate-cell-pop size-10" :stroke-width="2.5" />
        </div>

        <h1 class="animate-fade-up mt-7 text-2xl font-semibold tracking-tight text-ink sm:text-3xl">
          提交成功
        </h1>

        <p class="animate-fade-up mt-4 max-w-md text-sm leading-7 text-muted text-balance">
          你的答案在离开这台设备之前就已经被编码成数独密文。服务器收到的、以及数据库里保存的,
          都只是那一串线索字符。
        </p>

        <div class="animate-fade-up mt-8 flex flex-col items-center gap-4">
          <RouterLink
            to="/"
            class="text-sm text-subtle underline-offset-4 transition-colors hover:text-ink hover:underline"
          >
            再填一份
          </RouterLink>
        </div>

        <div v-if="hasDetail" class="mt-10 w-full">
          <button
            type="button"
            class="focus-ring flex h-11 w-full items-center justify-center gap-2 rounded-[var(--radius-control)] border border-line bg-raised px-4 text-xs text-muted transition-colors hover:text-ink"
            :aria-expanded="expanded"
            @click="expanded = !expanded"
          >
            <Hash class="size-3.5" />
            这次提交被编码成了多少
            <ChevronDown
              class="size-3.5 transition-transform duration-200"
              :class="expanded ? 'rotate-180' : ''"
            />
          </button>

          <AppCard v-if="expanded" padding="md" class="animate-fade-up mt-3 text-left">
            <dl class="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
              <div>
                <dt class="text-xs text-subtle">明文长度</dt>
                <dd class="mt-0.5 font-mono text-ink tabular-nums">{{ plaintextBytes }} 字节</dd>
              </div>
              <div>
                <dt class="text-xs text-subtle">密文长度</dt>
                <dd class="mt-0.5 font-mono text-ink tabular-nums">{{ cipherSymbols }} 个符号</dd>
              </div>
              <div v-if="durationMs">
                <dt class="text-xs text-subtle">填写用时</dt>
                <dd class="mt-0.5 text-ink">{{ formatDuration(durationMs ?? 0) }}</dd>
              </div>
              <div v-if="expansion">
                <dt class="text-xs text-subtle">膨胀比</dt>
                <dd class="mt-0.5 font-mono text-ink tabular-nums">
                  ×{{ expansion.toFixed(1) }}
                </dd>
              </div>
            </dl>
            <p class="mt-4 border-t border-line pt-3 text-xs leading-5 text-subtle">
              每个明文字节编成 4 个线索符号,末尾再加 4 个符号的校验帧。
              密文里没有长度之外的任何结构性信息,所以它既不能被压缩,也不能被反查。
            </p>
          </AppCard>
        </div>

        <SudokuMark size="sm" :animate="false" pencil class="mt-12" />
      </div>
    </div>
  </div>
</template>
