import { createApp } from 'vue'
import { createPinia } from 'pinia'
import LogViewer from '../views/LogViewer.vue'
import '../style.css'

const app = createApp(LogViewer)
app.use(createPinia())
app.mount('#app')
