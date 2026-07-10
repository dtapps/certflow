import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { NConfigProvider, NMessageProvider } from 'naive-ui'
import LogViewer from '../views/LogViewer.vue'
import { useThemeStore } from '../stores/theme'
import '../style.css'

const app = createApp({
  setup() {
    const themeStore = useThemeStore()
    return () =>
      h(
        NConfigProvider,
        { theme: themeStore.naiveTheme, themeOverrides: themeStore.naiveThemeOverrides },
        () => h(NMessageProvider, null, () => h(LogViewer)),
      )
  },
})
app.use(createPinia())
app.mount('#app')
