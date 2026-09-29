<script setup lang="ts">
/** Catch-all 404. Deliberately friendly: most arrivals are stale share links. */
import { ArrowLeft, Compass } from 'lucide-vue-next';
import { RouterLink, useRoute, useRouter } from 'vue-router';

import AppButton from '@/components/ui/AppButton.vue';
import SudokuMark from '@/components/ui/SudokuMark.vue';

const route = useRoute();
const router = useRouter();
</script>

<template>
  <div class="flex min-h-full flex-col">
    <div class="grid-paper flex flex-1 flex-col items-center justify-center px-6 py-20 text-center">
      <SudokuMark size="lg" pencil />

      <p class="mt-7 font-mono text-xs tracking-[0.45em] text-subtle">404</p>
      <h1 class="mt-3 text-2xl font-semibold tracking-tight text-ink">这一格是空的</h1>
      <p class="mt-3 max-w-sm text-sm leading-6 text-muted text-balance">
        没有找到这个地址对应的页面。可能是链接过期了,或者问卷已经被发布者关闭。
      </p>

      <code
        class="cipher mt-5 max-w-full truncate rounded-[var(--radius-control)] border border-line bg-sunken px-3 py-1.5 text-subtle"
      >
        {{ route.fullPath }}
      </code>

      <div class="mt-8 flex flex-wrap items-center justify-center gap-3">
        <AppButton size="lg" @click="router.push('/')">
          <ArrowLeft class="size-4" />
          返回首页
        </AppButton>
        <RouterLink
          to="/"
          class="flex h-12 items-center gap-1.5 rounded-xl px-4 text-sm text-muted transition-colors hover:bg-sunken hover:text-ink"
        >
          <Compass class="size-4" />
          看看有哪些问卷
        </RouterLink>
      </div>
    </div>
  </div>
</template>
