import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { NConfigProvider, NMessageProvider } from 'naive-ui'
import LogViewer from '../views/LogViewer.vue'
import { useThemeStore } from '../stores/theme'
import { useNaiveLocale } from '../utils/naive-locale'
import '../style.css'

const app = createApp({
  setup() {
    const themeStore = useThemeStore()
    const { naiveLocale, naiveDateLocale } = useNaiveLocale()
    return () =>
      h(
        NConfigProvider,
        {
          theme: themeStore.naiveTheme,
          themeOverrides: themeStore.naiveThemeOverrides,
          locale: naiveLocale.value,
          dateLocale: naiveDateLocale.value,
        },
        () => h(NMessageProvider, null, () => h(LogViewer)),
      )
  },
})
app.use(createPinia())
app.mount('#app')
