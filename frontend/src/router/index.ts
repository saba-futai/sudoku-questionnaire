import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router';

import { getToken } from '@/api/client';

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: () => import('@/views/public/HomeView.vue'),
    meta: { layout: 'public', title: '问卷列表' },
  },
  {
    path: '/q/:slug',
    name: 'fill',
    component: () => import('@/views/public/FillView.vue'),
    meta: { layout: 'public', title: '填写问卷' },
  },
  {
    path: '/q/:slug/done',
    name: 'submitted',
    component: () => import('@/views/public/DoneView.vue'),
    meta: { layout: 'public', title: '提交成功' },
  },
  {
    path: '/admin/login',
    name: 'admin-login',
    component: () => import('@/views/admin/LoginView.vue'),
    meta: { layout: 'bare', title: '管理员登录', public: true },
  },
  {
    path: '/admin',
    component: () => import('@/views/admin/AdminLayout.vue'),
    meta: { layout: 'bare', requiresAuth: true },
    children: [
      {
        path: '',
        name: 'admin-dashboard',
        component: () => import('@/views/admin/DashboardView.vue'),
        meta: { title: '概览' },
      },
      {
        path: 'questionnaires',
        name: 'admin-questionnaires',
        component: () => import('@/views/admin/QuestionnairesView.vue'),
        meta: { title: '问卷管理' },
      },
      {
        path: 'questionnaires/new',
        name: 'admin-questionnaire-new',
        component: () => import('@/views/admin/BuilderView.vue'),
        meta: { title: '新建问卷' },
      },
      {
        path: 'questionnaires/:id/edit',
        name: 'admin-questionnaire-edit',
        component: () => import('@/views/admin/BuilderView.vue'),
        meta: { title: '编辑问卷' },
      },
      {
        path: 'questionnaires/:id/responses',
        name: 'admin-responses',
        component: () => import('@/views/admin/ResponsesView.vue'),
        meta: { title: '答卷数据' },
      },
      {
        path: 'questionnaires/:id/stats',
        name: 'admin-stats',
        component: () => import('@/views/admin/StatsView.vue'),
        meta: { title: '统计分析' },
      },
      {
        path: 'codec',
        name: 'admin-codec',
        component: () => import('@/views/admin/CodecView.vue'),
        meta: { title: '数独编解码检视器' },
      },
      {
        path: 'audit',
        name: 'admin-audit',
        component: () => import('@/views/admin/AuditView.vue'),
        meta: { title: '操作日志' },
      },
      {
        path: 'settings',
        name: 'admin-settings',
        component: () => import('@/views/admin/SettingsView.vue'),
        meta: { title: '设置' },
      },
    ],
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/NotFoundView.vue'),
    meta: { layout: 'public', title: '页面不存在' },
  },
];

export const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(to, _from, saved) {
    if (saved) return saved;
    if (to.hash) return { el: to.hash, behavior: 'smooth' };
    return { top: 0 };
  },
});

router.beforeEach((to) => {
  if (to.meta.requiresAuth && !getToken()) {
    return { name: 'admin-login', query: { redirect: to.fullPath } };
  }
  if (to.name === 'admin-login' && getToken()) {
    return { name: 'admin-dashboard' };
  }
  return true;
});

router.afterEach((to) => {
  const title = (to.meta.title as string | undefined) ?? '';
  document.title = title ? `${title} · QRforQA` : 'QRforQA · 数独问卷';
});
