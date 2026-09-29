<script setup lang="ts">
import { Download, QrCode as QrCodeIcon } from 'lucide-vue-next';
import QRCode from 'qrcode';
import { computed, ref, watch } from 'vue';

const props = withDefaults(
  defineProps<{
    value?: string;
    size?: number;
    label?: string;
    downloadable?: boolean;
  }>(),
  { value: '', size: 200, downloadable: true },
);

const dataUrl = ref('');

// Deliberately dark-on-white in both themes: an inverted QR is only safe when
// the light modules also stay light, and a white panel is the one background we
// can guarantee under a scanner. Theming the code itself would risk the very
// thing it exists for.
async function render() {
  if (!props.value) {
    dataUrl.value = '';
    return;
  }
  try {
    dataUrl.value = await QRCode.toDataURL(props.value, {
      margin: 1,
      width: Math.max(64, Math.round(props.size)),
      errorCorrectionLevel: 'M',
      color: { dark: '#16161eff', light: '#ffffffff' },
    });
  } catch {
    dataUrl.value = '';
  }
}

watch(() => [props.value, props.size], render, { immediate: true });

const filename = computed(() => {
  const tail = props.value.split(/[?#]/)[0].split('/').filter(Boolean).pop() ?? 'qrcode';
  return `${tail.replace(/[^\w.-]+/g, '-').slice(0, 40) || 'qrcode'}.png`;
});
</script>

<template>
  <figure class="inline-flex flex-col items-center gap-2">
    <!-- The panel is a fixed square so the code never shifts as it renders, and
         so the quiet zone the encoder reserves is never cropped. -->
    <div
      class="grid place-items-center rounded-[var(--radius-card)] bg-white p-3 ring-1 ring-black/5"
      :style="{ width: size + 24, height: size + 24 }"
    >
      <img
        v-if="dataUrl"
        :src="dataUrl"
        :width="size"
        :height="size"
        :alt="label ?? value"
        class="block size-full"
        role="img"
      />
      <div v-else class="grid size-full place-items-center bg-sunken text-subtle" aria-hidden="true">
        <QrCodeIcon class="size-8" />
      </div>
    </div>

    <figcaption v-if="label" class="max-w-[24ch] text-center text-xs text-subtle text-balance">
      {{ label }}
    </figcaption>

    <a
      v-if="downloadable && dataUrl"
      :href="dataUrl"
      :download="filename"
      class="focus-ring mt-0.5 inline-flex h-11 items-center gap-1.5 rounded-[var(--radius-control)] border border-line bg-raised px-3.5 text-[13px] font-medium text-ink transition-colors duration-200 hover:border-line-strong hover:bg-sunken sm:h-10"
    >
      <Download class="size-4" aria-hidden="true" />
      下载二维码
    </a>
  </figure>
</template>
