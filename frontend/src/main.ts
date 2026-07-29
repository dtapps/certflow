import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import * as SystemService from '@bindings/cnb.cool/dtapp/certflow/systemservicewrapper'
import './style.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')

SystemService.SetUserAgent(navigator.userAgent)
console.log('UA:', navigator.userAgent)
