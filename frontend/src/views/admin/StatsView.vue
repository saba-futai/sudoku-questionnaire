<script setup lang="ts">
/**
 * StatsView — aggregate view of the decoded answers.
 *
 * The server decodes every response in order to aggregate it; nothing decoded is
 * ever written back. The charts here are the only place the admin sees the whole
 * population at once, so each one carries its numbers in text as well: a chart
 * that can only be read by looking at a picture is not readable by everyone.
 */
import { BarChart, LineChart, PieChart } from 'echarts/charts';
import { AriaComponent, GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import type { EChartsOption } from 'echarts';
import { BarChart3, Image as ImageIcon, Quote, TriangleAlert } from 'lucide-vue-next';
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import VChart from 'vue-echarts';

import { api, getToken } from '@/api/client';
import type { QuestionStat, StatsView } from '@/api/types';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import AppSkeleton from '@/components/ui/AppSkeleton.vue';
import EmptyState from '@/components/ui/EmptyState.vue';
import { formatDate, formatDuration, formatNumber, typeLabel } from '@/lib/format';
import { theme } from '@/lib/theme';
import { errorMessage } from '@/lib/toast';

use([
  BarChart,
  LineChart,
  PieChart,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  // Generates a screen-reader description from the series itself, so a chart is
  // never the only way to read the numbers.
  AriaComponent,
  CanvasRenderer,
]);

const route = useRoute();
const router = useRouter();
const id = computed(() => Number(typeof route.params.id === 'string' ? route.params.id : '0'));

const stats = ref<StatsView | null>(null);
const loading = ref(true);
const loadError = ref('');

async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    stats.value = await api<StatsView>(`/api/admin/questionnaires/${id.value}/stats`);
  } catch (err) {
    loadError.value = errorMessage(err);
  } finally {
    loading.value = false;
  }
}

onMounted(load);

// --------------------------------------------------------------- theming ---

/** Reads a semantic token so charts follow the theme instead of hard-coding hex. */
function cssVar(name: string, fallback: string): string {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return value || fallback;
}

/**
 * Chart colours, recomputed whenever the theme flips. `theme.value` is read only
 * to register the dependency: the actual values come from the CSS variables the
 * theme class swaps out.
 */
const ink = computed(() => {
  void theme.value;
  return {
    text: cssVar('--text', '#16161e'),
    muted: cssVar('--text-muted', '#57576b'),
    subtle: cssVar('--text-subtle', '#8b8b9e'),
    line: cssVar('--border', '#e7e7ee'),
    surface: cssVar('--surface-raised', '#ffffff'),
  };
});

/** The brand ramp, darkest to lightest — used as a magnitude ramp, not identity. */
const RAMP = ['#4338ca', '#4f46e5', '#6366f1', '#818cf8', '#a5b4fc', '#c7d2fe'];
const BRAND = '#6366f1';
/** The pencil-mark amber. Reserved for rating, the one amber thing in the app. */
const ACCENT = '#fbbf24';

function base(): EChartsOption {
  const t = ink.value;
  return {
    textStyle: { fontFamily: 'inherit', color: t.text },
    aria: { enabled: true, decal: { show: false } },
    tooltip: {
      backgroundColor: t.surface,
      borderColor: t.line,
      borderWidth: 1,
      padding: [8, 10],
      textStyle: { color: t.text, fontSize: 12 },
      extraCssText: 'border-radius:10px;box-shadow:0 10px 30px -14px rgba(0,0,0,.45);',
    },
  };
}

/** Recessive axes and thin dashed gridlines: the data should be the only ink. */
function axisBase() {
  const t = ink.value;
  return {
    axisLine: { lineStyle: { color: t.line } },
    axisTick: { show: false },
    axisLabel: { color: t.subtle, fontSize: 11 },
    splitLine: { lineStyle: { color: t.line, type: 'dashed' as const } },
  };
}

// -------------------------------------------------------------- timeline ---

const timelineOption = computed<EChartsOption>(() => {
  const points = stats.value?.timeline ?? [];
  const t = ink.value;
  // A single day is one point. A line through one point draws nothing, so the
  // shape switches to a bar rather than showing an empty plot.
  const single = points.length === 1;
  return {
    ...base(),
    grid: { left: 4, right: 12, top: 16, bottom: 4, containLabel: true },
    xAxis: {
      type: 'category',
      data: points.map((p) => p.date.slice(5)),
      boundaryGap: single,
      ...axisBase(),
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      ...axisBase(),
    },
    series: single
      ? [
          {
            name: '提交量',
            type: 'bar',
            barMaxWidth: 48,
            itemStyle: { color: BRAND, borderRadius: [4, 4, 0, 0] },
            data: points.map((p) => p.count),
          },
        ]
      : [
          {
            name: '提交量',
            type: 'line',
            smooth: 0.25,
            symbol: 'circle',
            symbolSize: 8,
            showSymbol: points.length <= 2,
            lineStyle: { width: 2, color: BRAND },
            itemStyle: { color: BRAND, borderColor: t.surface, borderWidth: 2 },
            areaStyle: {
              color: {
                type: 'linear',
                x: 0,
                y: 0,
                x2: 0,
                y2: 1,
                colorStops: [
                  { offset: 0, color: 'rgba(99,102,241,0.28)' },
                  { offset: 1, color: 'rgba(99,102,241,0.02)' },
                ],
              },
            },
            data: points.map((p) => p.count),
          },
        ],
  };
});

const timelineSummary = computed(() => {
  const points = stats.value?.timeline ?? [];
  return points.map((p) => `${p.date} 共 ${p.count} 份`).join('、');
});

// ----------------------------------------------------- per-question charts ---

function fmtPct(n: number): string {
  return `${n % 1 === 0 ? n.toFixed(0) : n.toFixed(1)}%`;
}

/**
 * For multi-choice, `answered` counts selections rather than respondents, so
 * the server's skipped figure can go negative. Zero floors it, and the card
 * labels the count as selections so the two are not conflated.
 */
function skippedCount(q: QuestionStat): number {
  return Math.max(0, q.type === 'multi' ? 0 : q.skipped);
}

/** Multi counts selections; every other type counts respondents. */
function answeredLabel(q: QuestionStat): string {
  return q.type === 'multi' ? `共 ${q.answered} 次选择` : `作答 ${q.answered}`;
}

/** Horizontal bars, one hue: the measure is a count, not an identity. */
function barOption(
  rows: { label: string; count: number; suffix?: string }[],
  hue: string = BRAND,
): EChartsOption {
  const t = ink.value;
  return {
    ...base(),
    grid: { left: 4, right: 72, top: 8, bottom: 4, containLabel: true },
    xAxis: { type: 'value', minInterval: 1, ...axisBase() },
    yAxis: {
      type: 'category',
      inverse: true,
      data: rows.map((r) => r.label),
      ...axisBase(),
      splitLine: { show: false },
      axisLabel: { color: t.muted, fontSize: 12, width: 120, overflow: 'truncate' },
    },
    series: [
      {
        type: 'bar',
        barMaxWidth: 18,
        itemStyle: { color: hue, borderRadius: [0, 4, 4, 0] },
        label: {
          show: true,
          position: 'right',
          color: t.muted,
          fontSize: 11,
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          formatter: (p: any) => rows[p.dataIndex]?.suffix ?? '',
        },
        data: rows.map((r) => r.count),
      },
    ],
  };
}

/** A donut of one measure across options, shaded light→dark by magnitude. */
function donutOption(q: QuestionStat): EChartsOption {
  const t = ink.value;
  const options = q.options ?? [];
  return {
    ...base(),
    color: RAMP,
    tooltip: {
      trigger: 'item',
      backgroundColor: t.surface,
      borderColor: t.line,
      borderWidth: 1,
      padding: [8, 10],
      textStyle: { color: t.text, fontSize: 12 },
      extraCssText: 'border-radius:10px;box-shadow:0 10px 30px -14px rgba(0,0,0,.45);',
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      formatter: (p: any) => `${p.name}<br/>${p.value} 份 · ${p.percent}%`,
    },
    legend: {
      bottom: 0,
      icon: 'circle',
      itemWidth: 8,
      itemHeight: 8,
      textStyle: { color: t.muted, fontSize: 11 },
      formatter: (name: string) => {
        const o = options.find((x) => x.label === name);
        return o ? `${name}  ${o.count}(${fmtPct(o.percent)})` : name;
      },
    },
    series: [
      {
        type: 'pie',
        radius: ['52%', '78%'],
        center: ['50%', '44%'],
        avoidLabelOverlap: true,
        // The 2px surface ring keeps adjacent segments from bleeding together.
        itemStyle: { borderColor: t.surface, borderWidth: 2, borderRadius: 4 },
        label: {
          show: true,
          color: t.muted,
          fontSize: 11,
          formatter: '{d}%',
        },
        labelLine: { length: 8, length2: 8, lineStyle: { color: t.line } },
        // An option nobody picked keeps its legend entry but gets no slice,
        // no callout and no stray "0%" floating over the ring.
        data: options.map((o) => ({
          name: o.label,
          value: o.count,
          label: { show: o.count > 0 },
          labelLine: { show: o.count > 0 },
        })),
      },
    ],
  };
}

function optionSummary(q: QuestionStat): string {
  const parts = (q.options ?? [])
    .filter((o) => o.count > 0)
    .map((o) => `${o.label} ${o.count} 份(${fmtPct(o.percent)})`);
  return parts.length ? parts.join('、') : '暂无数据';
}

/** Multi-choice bars read as penetration, so the share is of all respondents. */
function multiRows(q: QuestionStat) {
  const total = stats.value?.total ?? 0;
  return (q.options ?? []).map((o) => ({
    label: o.label,
    count: o.count,
    suffix: `${o.count} 人${total ? ` · ${((o.count / total) * 100).toFixed(0)}%` : ''}`,
  }));
}

// ---------------------------------------------------------------- derived ---

const timelinePoints = computed(() => stats.value?.timeline ?? []);

/**
 * Thumbnails come from an authenticated endpoint, so the token rides in the
 * query string — a plain <img> cannot carry an Authorization header.
 */
function imageUrl(imageId: number): string {
  const token = getToken();
  return `/api/admin/images/${imageId}${token ? `?token=${encodeURIComponent(token)}` : ''}`;
}

function textSummary(q: QuestionStat): string {
  const parts = [`作答 ${q.answered} 份`, `跳过 ${q.skipped} 份`];
  if (q.longestChars) parts.push(`最长 ${q.longestChars} 字`);
  return parts.join(' · ');
}

function ratingSummary(q: QuestionStat): string {
  const dist = (q.options ?? [])
    .filter((o) => o.count > 0)
    .map((o) => `${o.label} 分 ${o.count} 人`)
    .join('、');
  return `平均 ${(q.average ?? 0).toFixed(2)} 分 · ${dist || '暂无数据'}`;
}

function imageSummary(q: QuestionStat): string {
  return `${q.answered} 份答卷上传了图片,共 ${q.images?.length ?? 0} 张`;
}
</script>

<template>
  <div class="mx-auto w-full max-w-7xl">
    <!-- --------------------------------------------------- load states -->
    <div v-if="loading" class="space-y-4">
      <AppSkeleton variant="card" :lines="2" />
      <AppSkeleton variant="card" :lines="5" />
      <AppSkeleton variant="card" :lines="5" />
    </div>

    <AppCard v-else-if="loadError" padding="lg">
      <EmptyState :icon="TriangleAlert" title="无法加载统计" :description="loadError">
        <AppButton variant="secondary" @click="load">重试</AppButton>
      </EmptyState>
    </AppCard>

    <template v-else-if="stats">
      <!-- ------------------------------------------------------ header -->
      <header class="mb-5 flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <h1 class="text-xl font-semibold text-ink">{{ stats.title }}</h1>
          <p class="mt-1 text-sm text-muted">统计基于已提交的全部答卷,数据在读取时解码。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <AppButton
            variant="secondary"
            @click="router.push({ name: 'admin-responses', params: { id } })"
          >
            答卷数据
          </AppButton>
          <AppButton variant="ghost" @click="load">刷新</AppButton>
        </div>
      </header>

      <div class="mb-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <AppCard padding="sm">
          <p class="text-xs text-subtle">总答卷数</p>
          <p class="mt-1 text-2xl font-semibold text-ink tabular-nums">
            {{ formatNumber(stats.total) }}
          </p>
        </AppCard>
        <AppCard padding="sm">
          <p class="text-xs text-subtle">平均用时</p>
          <p class="mt-1 text-2xl font-semibold text-ink tabular-nums">
            {{ formatDuration(stats.avgDurationMs) }}
          </p>
        </AppCard>
        <AppCard padding="sm">
          <p class="text-xs text-subtle">首次提交</p>
          <p class="mt-1 text-lg font-medium text-ink">{{ formatDate(stats.firstAt) }}</p>
        </AppCard>
        <AppCard padding="sm">
          <p class="text-xs text-subtle">最近提交</p>
          <p class="mt-1 text-lg font-medium text-ink">{{ formatDate(stats.lastAt) }}</p>
        </AppCard>
        <AppCard padding="sm">
          <p class="text-xs text-subtle">图片总数</p>
          <p class="mt-1 text-2xl font-semibold text-ink tabular-nums">
            {{ formatNumber(stats.totalImages) }}
          </p>
        </AppCard>
      </div>

      <AppCard v-if="stats.total === 0" padding="none" class="mb-5">
        <EmptyState
          title="还没有答卷"
          description="发布问卷并分享链接后,这里会出现提交趋势与各题统计。"
        >
          <AppButton variant="secondary" @click="router.push({ name: 'admin-questionnaire-edit', params: { id } })">
            编辑问卷
          </AppButton>
        </EmptyState>
      </AppCard>

      <template v-else>
        <!-- ------------------------------------------------- timeline -->
        <AppCard class="mb-5">
          <template #title>
            <h2 class="text-base font-semibold text-ink">提交趋势</h2>
            <p class="mt-0.5 text-xs text-subtle">
              按天统计,包含没有提交的日期,共 {{ timelinePoints.length }} 天
            </p>
          </template>

          <div v-if="timelinePoints.length === 0" class="py-8">
            <EmptyState title="没有可绘制的时间范围" description="提交时间缺失,无法生成趋势图。" />
          </div>

          <template v-else>
            <!-- The height goes inline: vue-echarts puts `height: 100%` on its
                 own root, which would beat a class and collapse the canvas. -->
            <VChart :option="timelineOption" autoresize class="w-full" :style="{ height: '15rem' }" />
            <p class="mt-2 text-xs text-subtle tabular-nums">
              <span class="sr-only">数据:</span>
              {{ timelineSummary }}
            </p>
          </template>
        </AppCard>

        <!-- ------------------------------------------------ questions -->
        <div class="stagger space-y-4">
          <AppCard v-for="(q, qi) in stats.questions" :key="q.key" :style="{ '--i': qi }">
            <template #title>
              <h2 class="text-base font-semibold text-ink">{{ q.label }}</h2>
              <p class="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-subtle">
                <span class="font-mono">{{ q.key }}</span>
                <span class="rounded-full bg-sunken px-2 py-0.5">{{ typeLabel(q.type) }}</span>
                <span class="tabular-nums">
                  {{ answeredLabel(q) }}
                  <template v-if="skippedCount(q) > 0"> · 跳过 {{ skippedCount(q) }}</template>
                </span>
              </p>
            </template>

            <!-- single / select: a donut over the options -->
            <template v-if="q.type === 'single' || q.type === 'select'">
              <div v-if="!q.options?.length" class="py-6">
                <EmptyState title="没有选项数据" description="这道题还没有人作答。" />
              </div>
              <template v-else>
                <VChart :option="donutOption(q)" autoresize class="w-full" :style="{ height: '18rem' }" />
                <p class="mt-2 text-xs text-subtle tabular-nums">{{ optionSummary(q) }}</p>
              </template>
            </template>

            <!-- multi: share of respondents, so the bars read as penetration -->
            <template v-else-if="q.type === 'multi'">
              <VChart
                :option="barOption(multiRows(q))"
                :style="{ height: `${Math.max(180, (q.options?.length ?? 0) * 38 + 60)}px` }"
                autoresize
                class="w-full"
              />
              <p class="mt-2 text-xs text-subtle tabular-nums">{{ optionSummary(q) }}</p>
            </template>

            <!-- rating: the average is the headline, the distribution is the shape -->
            <template v-else-if="q.type === 'rating'">
              <div class="mb-4 flex flex-wrap items-end gap-6">
                <div>
                  <p class="text-xs text-subtle">平均分</p>
                  <p class="text-4xl font-semibold text-ink tabular-nums">
                    {{ (q.average ?? 0).toFixed(2) }}
                  </p>
                </div>
                <div class="text-sm text-muted tabular-nums">
                  <p>总分 {{ q.sum ?? 0 }}</p>
                  <p>{{ q.answered }} 人评分</p>
                </div>
              </div>
              <VChart
                :option="
                  barOption(
                    (q.options ?? []).map((o) => ({
                      label: `${o.label} 分`,
                      count: o.count,
                      suffix: `${o.count} 人 · ${fmtPct(o.percent)}`,
                    })),
                    ACCENT,
                  )
                "
                :style="{ height: `${Math.max(180, (q.options?.length ?? 0) * 38 + 60)}px` }"
                autoresize
                class="w-full"
              />
              <p class="mt-2 text-xs text-subtle tabular-nums">{{ ratingSummary(q) }}</p>
            </template>

            <!-- date: the answers are dates, so a bar per value -->
            <template v-else-if="q.type === 'date'">
              <div v-if="!q.options?.length" class="py-6">
                <EmptyState title="没有日期数据" description="这道题还没有人作答。" />
              </div>
              <template v-else>
                <VChart
                  :option="
                    barOption(
                      q.options.slice(0, 20).map((o) => ({
                        label: o.label,
                        count: o.count,
                        suffix: `${o.count} 人`,
                      })),
                    )
                  "
                  :style="{ height: `${Math.max(180, Math.min(q.options.length, 20) * 34 + 60)}px` }"
                  autoresize
                  class="w-full"
                />
                <p v-if="q.options.length > 20" class="mt-2 text-xs text-subtle">
                  共 {{ q.options.length }} 个不同日期,图中显示前 20 个。
                </p>
                <p class="mt-2 text-xs text-subtle tabular-nums">{{ optionSummary(q) }}</p>
              </template>
            </template>

            <!-- text: the words themselves are the data -->
            <template v-else-if="q.type === 'text' || q.type === 'textarea'">
              <div class="mb-3 flex items-center gap-2 text-xs text-subtle">
                <Quote class="size-3.5" />
                <span class="tabular-nums">{{ textSummary(q) }}</span>
              </div>
              <div v-if="!q.samples?.length" class="py-6">
                <EmptyState title="没有文本答案" description="这道题还没有人填写。" />
              </div>
              <div v-else class="grid gap-3 sm:grid-cols-2">
                <blockquote
                  v-for="(sample, si) in q.samples"
                  :key="si"
                  class="rounded-[var(--radius-control)] border border-line bg-sunken/50 p-3 text-sm whitespace-pre-wrap text-ink"
                >
                  {{ sample }}
                </blockquote>
              </div>
              <p
                v-if="q.sampleTotal && q.samples && q.sampleTotal > q.samples.length"
                class="mt-3 text-xs text-subtle tabular-nums"
              >
                显示前 {{ q.samples.length }} 条,共 {{ q.sampleTotal }} 条。完整内容请导出 CSV / Excel。
              </p>
            </template>

            <!-- image: the uploads themselves -->
            <template v-else-if="q.type === 'image'">
              <div class="mb-3 flex items-center gap-2 text-xs text-subtle">
                <ImageIcon class="size-3.5" />
                <span class="tabular-nums">{{ imageSummary(q) }}</span>
              </div>
              <div v-if="!q.images?.length" class="py-6">
                <EmptyState title="没有图片" description="这道题还没有人上传。" />
              </div>
              <div v-else class="grid grid-cols-3 gap-2 sm:grid-cols-5 lg:grid-cols-6">
                <img
                  v-for="img in q.images"
                  :key="img.id"
                  :src="imageUrl(img.id)"
                  :alt="q.label"
                  loading="lazy"
                  decoding="async"
                  class="aspect-square w-full rounded-lg border border-line object-cover"
                />
              </div>
            </template>
          </AppCard>
        </div>

        <p v-if="stats.questions.length === 0" class="mt-5 text-center text-sm text-subtle">
          <BarChart3 class="mr-1 inline size-4" />
          问卷还没有题目。
        </p>
      </template>
    </template>
  </div>
</template>
