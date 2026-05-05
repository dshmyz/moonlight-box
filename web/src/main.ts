import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { library } from '@fortawesome/fontawesome-svg-core'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { faList, faGrip, faSearch } from '@fortawesome/free-solid-svg-icons'

import App from './App.vue'
import router from './router'

library.add(faList, faGrip, faSearch)

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })
app.component('FontAwesomeIcon', FontAwesomeIcon)

app.mount('#app')
