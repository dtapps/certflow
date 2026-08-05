import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import * as SystemService from '@bindings/cnb.cool/dtapp/certflow/systemservicewrapper'
import { installErrorReporter, vueErrorHandler } from './utils/errorReporter'
import './style.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
// Vue 渲染/侦听器错误统一上报到后端日志（发布版也能捕获）。
app.config.errorHandler = vueErrorHandler
// 尽早安装全局 window 错误 / Promise 拒绝 拦截，覆盖模块加载期到运行期。
installErrorReporter()
app.mount('#app')

SystemService.SetUserAgent(navigator.userAgent)
console.log('UA:', navigator.userAgent)
