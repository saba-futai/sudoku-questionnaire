import { createPinia } from 'pinia';
import { createApp } from 'vue';

import '@fontsource/inter/400.css';
import '@fontsource/inter/500.css';
import '@fontsource/inter/600.css';
import '@fontsource/inter/700.css';
import '@fontsource/jetbrains-mono/400.css';
import '@fontsource/jetbrains-mono/500.css';
import '@fontsource/jetbrains-mono/600.css';
import './styles/main.css';

import App from './App.vue';
import { router } from './router';
import { setUnauthorizedHandler } from './api/client';
import { applyTheme, theme, watchSystemTheme } from './lib/theme';

// Not persisted: on a first visit the theme came from the OS, and writing it
// down here would quietly turn "follow system" into a fixed choice.
applyTheme(theme.value, false);
watchSystemTheme();

const app = createApp(App);
app.use(createPinia());
app.use(router);

// A rejected token means the session expired mid-use; bounce to login rather
// than leaving the admin staring at failed requests.
setUnauthorizedHandler(() => {
  if (router.currentRoute.value.name !== 'admin-login') {
    void router.replace({
      name: 'admin-login',
      query: { redirect: router.currentRoute.value.fullPath, expired: '1' },
    });
  }
});

app.mount('#app');
