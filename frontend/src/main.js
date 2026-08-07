import 'element-plus/dist/index.css'
// Element Plus 的深色变量也挂在 html.dark 上，与本项目的主题开关天然对齐。
// main.css 放在最后引入，保证我们的变量覆盖 Element Plus 的默认值。
import 'element-plus/theme-chalk/dark/css-vars.css'
import './assets/main.css'

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/dist/locale/zh-cn.mjs'

import App from './App.vue'
import router from './router'
import { initTheme } from './composables/useTheme'

initTheme()

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })

app.mount('#app')
