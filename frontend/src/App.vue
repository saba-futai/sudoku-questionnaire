<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useRoute } from 'vue-router';

import ToastHost from '@/components/ui/ToastHost.vue';
import { useCodecStore } from '@/stores/codec';

const route = useRoute();
const codec = useCodecStore();

/**
 * Only public pages get a top-level page transition.
 *
 * The admin shell is a multi-root component (mobile bar + sidebar + content),
 * and Vue cannot animate a Fragment — wrapping it produced a warning on every
 * admin route. The admin layout already animates its own content on child
 * navigation, so it is rendered plainly.
 */
const isBare = computed(() => route.meta.layout === 'bare');

onMounted(() => {
  // Warm the codec (bitmap fetch + digest check) while the user is still
  // reading the page, so the first submission is not the one that pays for it.
  codec.load().catch(() => {
    /* the banner below surfaces the failure */
  });
});
</script>

<template>
  <div class="min-h-full flex flex-col">
    <!-- A fingerprint mismatch means this browser and the server disagree on
         the coding table. Showing the payload as corrupt data would be far
         worse than refusing it, so the banner is deliberately loud. -->
    <div
      v-if="codec.mismatches.length"
      role="alert"
      class="bg-rose-600 text-white px-4 py-2.5 text-sm flex items-start gap-3 justify-center"
    >
      <span class="font-semibold shrink-0">编码表不一致</span>
      <span class="opacity-95">{{ codec.mismatches[0] }}</span>
    </div>

    <RouterView v-slot="{ Component }">
      <component :is="Component" v-if="isBare" />
      <Transition v-else name="page" mode="out-in">
        <component :is="Component" />
      </Transition>
    </RouterView>

    <ToastHost />
  </div>
</template>
