import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import * as MonitorService from '@bindings/cnb.cool/dtapp/certflow/monitorservicewrapper'
import './style.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')

MonitorService.SetUserAgent(navigator.userAgent)
console.log('UA:', navigator.userAgent)
