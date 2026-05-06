import{d as g,o as m,c as f,b as t,a as o,w as l,f as a,y as M,t as q,j as E,r as C,A as k,i as r,_ as b,h as x,F as V,n as R,k as U}from"./index-BVNCA_Wb.js";import{C as $}from"./CodeBlock-zuRO5avo.js";const Y={class:"quick-start"},D={key:0,class:"step-content-wrapper"},A={class:"step-card"},S={class:"step-actions"},z={key:1,class:"step-content-wrapper"},H={class:"step-card"},L={key:0,class:"manager-config"},B=["innerHTML"],h={class:"step-actions"},j={key:2,class:"step-content-wrapper"},Q={class:"step-card final-step"},F={class:"action-buttons"},K=g({__name:"QuickStart",setup(y){const d=C(0),p=C("npm"),c=k(()=>({npm:"NPM",maven:"Maven",pypi:"PyPI",go:"Go",nuget:"NuGet"})[p.value]),u=k(()=>{const e=window.location.origin;return{npm:`
      <p>创建或编辑 <code>~/.npmrc</code> 文件：</p>
      <pre><code>registry=${e}/repo/npm-virtual/
//${window.location.host}/repo/npm-virtual/:_authToken=YOUR_TOKEN_HERE</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('npmrc')">
          下载 .npmrc 模板
        </el-button>
      </p>
    `,maven:`
      <p>编辑 <code>~/.m2/settings.xml</code> 文件：</p>
      <pre><code>&lt;settings&gt;
  &lt;servers&gt;
    &lt;server&gt;
      &lt;id&gt;moonlight&lt;/id&gt;
      &lt;username&gt;YOUR_USERNAME&lt;/username&gt;
      &lt;password&gt;YOUR_PASSWORD&lt;/password&gt;
    &lt;/server&gt;
  &lt;/servers&gt;
  &lt;mirrors&gt;
    &lt;mirror&gt;
      &lt;id&gt;moonlight&lt;/id&gt;
      &lt;mirrorOf&gt;central&lt;/mirrorOf&gt;
      &lt;url&gt;${e}/repo/maven-virtual/&lt;/url&gt;
    &lt;/mirror&gt;
  &lt;/mirrors&gt;
&lt;/settings&gt;</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('settings.xml')">
          下载 settings.xml 模板
        </el-button>
      </p>
    `,pypi:`
      <p>创建 <code>~/.pip/pip.conf</code> 文件：</p>
      <pre><code>[global]
index-url = ${e}/repo/pypi-virtual/simple/
trusted-host = ${window.location.host}</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('pip.conf')">
          下载 pip.conf 模板
        </el-button>
      </p>
    `,go:`
      <p>设置环境变量：</p>
      <pre><code>export GOPROXY=${e}/go,https://proxy.golang.org,direct
export GOPRIVATE=${window.location.host}
export GOSUMDB=off</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('go-env.sh')">
          下载环境变量脚本
        </el-button>
      </p>
    `,nuget:`
      <p>运行以下命令：</p>
      <pre><code>nuget sources add -name moonlight -source ${e}/nuget/v3/index.json
nuget sources update -name moonlight -username YOUR_USERNAME -password YOUR_PASSWORD</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('NuGet.Config')">
          下载 NuGet.Config 模板
        </el-button>
      </p>
    `}[p.value]}),n=e=>{window.open(`/docs/templates/${e}`,"_blank")},i=()=>{n({npm:".npmrc",maven:"settings.xml",pypi:"pip.conf",go:"go-env.sh",nuget:"NuGet.Config"}[p.value])};return(e,s)=>{const v=r("el-step"),w=r("el-steps"),_=r("el-timeline-item"),P=r("el-timeline"),O=r("el-alert"),N=r("el-button"),T=r("el-radio-button"),I=r("el-radio-group");return m(),f("div",Y,[s[29]||(s[29]=t("div",{class:"section-header"},[t("h2",null,"🚀 快速开始"),t("p",{class:"section-desc"},"按照以下步骤配置您的客户端，开始使用 Moonlight Registry")],-1)),o(w,{active:d.value,"finish-status":"success","align-center":"",class:"steps-container"},{default:l(()=>[o(v,{title:"配置认证",description:"设置用户名密码"}),o(v,{title:"选择包管理器",description:"配置客户端工具"}),o(v,{title:"开始使用",description:"验证配置并使用"})]),_:1},8,["active"]),d.value===0?(m(),f("div",D,[t("div",A,[s[14]||(s[14]=t("div",{class:"step-icon"},"🔐",-1)),s[15]||(s[15]=t("h3",null,"配置认证信息",-1)),s[16]||(s[16]=t("p",null,"根据您使用的包管理器，配置相应的认证方式：",-1)),o(P,null,{default:l(()=>[o(_,null,{dot:l(()=>[...s[5]||(s[5]=[t("i",{class:"fa-solid fa-key"},null,-1)])]),default:l(()=>[s[6]||(s[6]=t("p",null,[t("strong",null,"NPM/PyPI/NuGet："),a("使用您的账号密码进行认证")],-1))]),_:1}),o(_,null,{dot:l(()=>[...s[7]||(s[7]=[t("i",{class:"fa-solid fa-file-code"},null,-1)])]),default:l(()=>[s[8]||(s[8]=t("p",null,[t("strong",null,"Maven："),a("在 settings.xml 中配置 server 信息")],-1))]),_:1}),o(_,null,{dot:l(()=>[...s[9]||(s[9]=[t("i",{class:"fa-solid fa-globe"},null,-1)])]),default:l(()=>[s[10]||(s[10]=t("p",null,[t("strong",null,"Go："),a("通过 GOPROXY 配置，无需额外认证")],-1))]),_:1})]),_:1}),o(O,{type:"info",closable:!1,style:{"margin-top":"20px"}},{title:l(()=>[...s[11]||(s[11]=[a("提示",-1)])]),default:l(()=>[s[12]||(s[12]=t("p",null,"访问令牌功能即将推出，当前版本请使用用户名密码进行认证",-1))]),_:1}),t("div",S,[o(N,{type:"primary",size:"large",onClick:s[0]||(s[0]=G=>d.value=1)},{default:l(()=>[...s[13]||(s[13]=[a(" 下一步 ",-1),t("i",{class:"fa-solid fa-arrow-right"},null,-1)])]),_:1})])])])):M("",!0),d.value===1?(m(),f("div",z,[t("div",H,[s[24]||(s[24]=t("div",{class:"step-icon"},"📦",-1)),s[25]||(s[25]=t("h3",null,"选择您的包管理器",-1)),o(I,{modelValue:p.value,"onUpdate:modelValue":s[1]||(s[1]=G=>p.value=G),size:"large",class:"manager-selector"},{default:l(()=>[o(T,{label:"npm"},{default:l(()=>[...s[17]||(s[17]=[t("i",{class:"fa-brands fa-npm"},null,-1),t("span",null,"NPM",-1)])]),_:1}),o(T,{label:"maven"},{default:l(()=>[...s[18]||(s[18]=[t("i",{class:"fa-brands fa-java"},null,-1),t("span",null,"Maven",-1)])]),_:1}),o(T,{label:"pypi"},{default:l(()=>[...s[19]||(s[19]=[t("i",{class:"fa-brands fa-python"},null,-1),t("span",null,"PyPI",-1)])]),_:1}),o(T,{label:"go"},{default:l(()=>[...s[20]||(s[20]=[t("i",{class:"fa-brands fa-golang"},null,-1),t("span",null,"Go",-1)])]),_:1}),o(T,{label:"nuget"},{default:l(()=>[...s[21]||(s[21]=[t("i",{class:"fa-solid fa-box"},null,-1),t("span",null,"NuGet",-1)])]),_:1})]),_:1},8,["modelValue"]),p.value?(m(),f("div",L,[t("h4",null,q(c.value)+" 配置",1),t("div",{innerHTML:u.value},null,8,B)])):M("",!0),t("div",h,[o(N,{size:"large",onClick:s[2]||(s[2]=G=>d.value=0)},{default:l(()=>[...s[22]||(s[22]=[t("i",{class:"fa-solid fa-arrow-left"},null,-1),a(" 上一步 ",-1)])]),_:1}),o(N,{type:"primary",size:"large",onClick:s[3]||(s[3]=G=>d.value=2)},{default:l(()=>[...s[23]||(s[23]=[a(" 下一步 ",-1),t("i",{class:"fa-solid fa-arrow-right"},null,-1)])]),_:1})])])])):M("",!0),d.value===2?(m(),f("div",j,[t("div",Q,[s[28]||(s[28]=E('<div class="success-icon" data-v-7e4956b8>🎉</div><h3 data-v-7e4956b8>配置完成！</h3><p class="success-desc" data-v-7e4956b8>您现在可以开始使用 Moonlight Registry 仓库了</p><div class="next-steps" data-v-7e4956b8><h4 data-v-7e4956b8>下一步操作</h4><ul data-v-7e4956b8><li data-v-7e4956b8><i class="fa-solid fa-arrow-right" data-v-7e4956b8></i> 浏览仓库查找可用的软件包</li><li data-v-7e4956b8><i class="fa-solid fa-arrow-right" data-v-7e4956b8></i> 发布您的第一个包</li><li data-v-7e4956b8><i class="fa-solid fa-arrow-right" data-v-7e4956b8></i> 配置 CI/CD 集成</li></ul></div>',4)),t("div",F,[o(N,{type:"primary",size:"large",onClick:s[4]||(s[4]=G=>e.$router.push("/"))},{default:l(()=>[...s[26]||(s[26]=[t("i",{class:"fa-solid fa-search"},null,-1),a(" 浏览仓库 ",-1)])]),_:1}),o(N,{size:"large",onClick:i},{default:l(()=>[...s[27]||(s[27]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载配置文件 ",-1)])]),_:1})])])])):M("",!0)])}}}),J=b(K,[["__scopeId","data-v-7e4956b8"]]),X={class:"npm-config"},W={class:"config-methods"},Z={class:"method-card"},tt={class:"method-steps"},et={class:"step-item"},st={class:"step-content"},ot={class:"step-item"},nt={class:"step-content"},lt={class:"step-item"},it={class:"step-content"},at={class:"method-card"},dt={class:"method-steps"},rt={class:"step-item"},pt={class:"step-content"},ut={class:"step-item"},ct={class:"step-content"},mt={class:"step-item"},ft={class:"step-content"},_t={class:"method-card"},vt={class:"method-steps"},gt={class:"step-item"},bt={class:"step-content"},yt={class:"step-item"},wt={class:"step-content"},$t={class:"troubleshooting"},kt=g({__name:"NPMConfig",setup(y){const d=k(()=>`${window.location.origin}/repo/npm-virtual/`),p=k(()=>window.location.host),c=k(()=>`# NPM 配置文件
registry=${d.value}

# 认证信息
//${p.value}/repo/npm-virtual/:_authToken=YOUR_TOKEN_HERE

# 作用域包配置（可选）
# @mycompany:registry=${window.location.origin}/repo/npm-local/`),u=k(()=>JSON.stringify({publishConfig:{registry:d.value}},null,2)),n=i=>{window.open(`/docs/templates/${i}`,"_blank")};return(i,e)=>{const s=r("el-alert"),v=r("el-button"),w=r("el-collapse-item"),_=r("el-collapse");return m(),f("div",X,[o(s,{type:"info",closable:!1},{default:l(()=>[...e[1]||(e[1]=[a(" NPM 是 Node.js 的包管理器，用于安装、发布和管理 JavaScript 包 ",-1)])]),_:1}),t("div",W,[t("div",Z,[e[11]||(e[11]=t("div",{class:"method-header"},[t("div",{class:"method-badge"},"推荐"),t("h4",null,"方式一：配置文件")],-1)),e[12]||(e[12]=t("p",{class:"method-desc"},[a("通过编辑 "),t("code",null,"~/.npmrc"),a(" 文件进行配置，适合长期使用")],-1)),t("div",tt,[t("div",et,[e[4]||(e[4]=t("div",{class:"step-number"},"1",-1)),t("div",st,[e[3]||(e[3]=t("h5",null,"下载配置模板",-1)),o(v,{type:"primary",onClick:e[0]||(e[0]=P=>n(".npmrc"))},{default:l(()=>[...e[2]||(e[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 .npmrc 模板 ",-1)])]),_:1})])]),t("div",ot,[e[7]||(e[7]=t("div",{class:"step-number"},"2",-1)),t("div",nt,[e[5]||(e[5]=t("h5",null,"放置配置文件",-1)),e[6]||(e[6]=t("p",null,[a("将下载的文件放到 "),t("code",null,"~/.npmrc"),a(" 或项目根目录")],-1)),o($,{code:c.value,title:"~/.npmrc"},null,8,["code"])])]),t("div",lt,[e[10]||(e[10]=t("div",{class:"step-number"},"3",-1)),t("div",it,[e[8]||(e[8]=t("h5",null,"替换认证信息",-1)),e[9]||(e[9]=t("p",null,[a("将 "),t("code",null,"YOUR_TOKEN_HERE"),a(" 替换为您的访问令牌")],-1)),o($,{code:"sed -i 's/YOUR_TOKEN_HERE/your-actual-token/g' ~/.npmrc"})])])])]),t("div",at,[e[19]||(e[19]=t("div",{class:"method-header"},[t("h4",null,"方式二：命令行配置")],-1)),e[20]||(e[20]=t("p",{class:"method-desc"},"通过命令行快速配置，适合临时使用",-1)),t("div",dt,[t("div",rt,[e[14]||(e[14]=t("div",{class:"step-number"},"1",-1)),t("div",pt,[e[13]||(e[13]=t("h5",null,"设置仓库地址",-1)),o($,{code:`npm config set registry ${d.value}`},null,8,["code"])])]),t("div",ut,[e[16]||(e[16]=t("div",{class:"step-number"},"2",-1)),t("div",ct,[e[15]||(e[15]=t("h5",null,"设置认证信息",-1)),o($,{code:`npm config set //${p.value}/repo/npm-virtual/:_authToken YOUR_TOKEN_HERE`},null,8,["code"])])]),t("div",mt,[e[18]||(e[18]=t("div",{class:"step-number"},"3",-1)),t("div",ft,[e[17]||(e[17]=t("h5",null,"验证配置",-1)),o($,{code:"npm config list"})])])])]),t("div",_t,[e[25]||(e[25]=t("div",{class:"method-header"},[t("h4",null,"发布包")],-1)),t("div",vt,[t("div",gt,[e[22]||(e[22]=t("div",{class:"step-number"},"1",-1)),t("div",bt,[e[21]||(e[21]=t("h5",null,"发布到本地仓库",-1)),o($,{code:`npm publish --registry=${d.value}`},null,8,["code"])])]),t("div",yt,[e[24]||(e[24]=t("div",{class:"step-number"},"2",-1)),t("div",wt,[e[23]||(e[23]=t("h5",null,"或在 package.json 中配置",-1)),o($,{code:u.value,title:"package.json"},null,8,["code"])])])])])]),t("div",$t,[e[29]||(e[29]=t("h4",null,"常见问题",-1)),o(_,null,{default:l(()=>[o(w,{title:"npm install 报错 404 Not Found",name:"404"},{default:l(()=>[...e[26]||(e[26]=[t("p",null,[t("strong",null,"可能的原因：")],-1),t("ol",null,[t("li",null,[a("仓库地址错误 - 检查 "),t("code",null,"npm config get registry")]),t("li",null,"包不存在 - 确认包已发布到仓库"),t("li",null,"认证失败 - 检查令牌是否正确")],-1)])]),_:1}),o(w,{title:"npm adduser 不工作",name:"adduser"},{default:l(()=>[...e[27]||(e[27]=[t("p",null,[a("当前版本暂不支持 "),t("code",null,"npm adduser"),a(" 命令。")],-1),t("p",null,[t("strong",null,"替代方案：")],-1),t("ol",null,[t("li",null,"通过 Web UI 获取令牌"),t("li",null,"手动配置 .npmrc 文件"),t("li",null,"联系管理员获取预配置文件")],-1)])]),_:1}),o(w,{title:"如何删除已发布的包",name:"unpublish"},{default:l(()=>[o($,{code:"npm unpublish package-name@1.0.0 --registry=http://your-registry/repo/npm-local/"}),o(s,{type:"warning",closable:!1},{default:l(()=>[...e[28]||(e[28]=[a(" 需要相应权限才能删除包 ",-1)])]),_:1})]),_:1})]),_:1})])])}}}),Ct=b(kt,[["__scopeId","data-v-f6405df7"]]),xt={class:"maven-config"},Pt={class:"config-content"},Nt={class:"content-row"},Tt={class:"content-actions"},Gt=g({__name:"MavenConfig",setup(y){const d=x(),p=u=>{window.open(`/docs/templates/${u}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(u,n)=>{const i=r("el-alert"),e=r("el-button");return m(),f("div",xt,[o(i,{type:"info",closable:!1},{default:l(()=>[...n[1]||(n[1]=[a(" Maven 是 Java 项目的构建和依赖管理工具，用于管理项目依赖、构建生命周期和插件配置 ",-1)])]),_:1}),t("div",Pt,[t("div",Nt,[n[4]||(n[4]=t("div",{class:"content-text"},[t("h4",null,"快速开始"),t("p",null,"下载配置模板并查看完整文档，快速完成 Maven 仓库配置")],-1)),t("div",Tt,[o(e,{type:"primary",onClick:n[0]||(n[0]=s=>p("settings.xml"))},{default:l(()=>[...n[2]||(n[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 settings.xml ",-1)])]),_:1}),o(e,{onClick:c},{default:l(()=>[...n[3]||(n[3]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看完整文档 ",-1)])]),_:1})])])])])}}}),Mt=b(Gt,[["__scopeId","data-v-87e57313"]]),Ot={class:"pypi-config"},Vt={class:"config-content"},Rt={class:"content-row"},qt={class:"content-actions"},Et=g({__name:"PyPIConfig",setup(y){const d=x(),p=u=>{window.open(`/docs/templates/${u}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(u,n)=>{const i=r("el-alert"),e=r("el-button");return m(),f("div",Ot,[o(i,{type:"info",closable:!1},{default:l(()=>[...n[1]||(n[1]=[a(" PyPI 是 Python 的包索引和依赖管理工具，用于安装、发布和管理 Python 包 ",-1)])]),_:1}),t("div",Vt,[t("div",Rt,[n[4]||(n[4]=t("div",{class:"content-text"},[t("h4",null,"快速开始"),t("p",null,"下载配置模板并查看完整文档，快速完成 PyPI 仓库配置")],-1)),t("div",qt,[o(e,{type:"primary",onClick:n[0]||(n[0]=s=>p("pip.conf"))},{default:l(()=>[...n[2]||(n[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 pip.conf ",-1)])]),_:1}),o(e,{onClick:c},{default:l(()=>[...n[3]||(n[3]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看完整文档 ",-1)])]),_:1})])])])])}}}),It=b(Et,[["__scopeId","data-v-f22ecf5e"]]),Ut={class:"go-config"},Yt={class:"config-content"},Dt={class:"content-row"},At={class:"content-actions"},St=g({__name:"GoConfig",setup(y){const d=x(),p=u=>{window.open(`/docs/templates/${u}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(u,n)=>{const i=r("el-alert"),e=r("el-button");return m(),f("div",Ut,[o(i,{type:"info",closable:!1},{default:l(()=>[...n[1]||(n[1]=[a(" Go modules 是 Go 语言的依赖管理系统，通过 GOPROXY 环境变量配置代理 ",-1)])]),_:1}),t("div",Yt,[t("div",Dt,[n[4]||(n[4]=t("div",{class:"content-text"},[t("h4",null,"快速开始"),t("p",null,"下载环境变量脚本并查看完整文档，快速完成 Go 模块仓库配置")],-1)),t("div",At,[o(e,{type:"primary",onClick:n[0]||(n[0]=s=>p("go-env.sh"))},{default:l(()=>[...n[2]||(n[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载环境变量脚本 ",-1)])]),_:1}),o(e,{onClick:c},{default:l(()=>[...n[3]||(n[3]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看完整文档 ",-1)])]),_:1})])])])])}}}),zt=b(St,[["__scopeId","data-v-45093aab"]]),Ht={class:"nuget-config"},Lt={class:"config-content"},Bt={class:"content-row"},ht={class:"content-actions"},jt=g({__name:"NuGetConfig",setup(y){const d=x(),p=u=>{window.open(`/docs/templates/${u}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(u,n)=>{const i=r("el-alert"),e=r("el-button");return m(),f("div",Ht,[o(i,{type:"info",closable:!1,style:{"margin-bottom":"20px"}},{default:l(()=>[...n[1]||(n[1]=[a(" NuGet 是 .NET 的包管理器，用于管理 .NET 项目的依赖包和发布 ",-1)])]),_:1}),t("div",Lt,[t("div",Bt,[n[4]||(n[4]=t("div",{class:"content-text"},[t("h4",null,"快速开始"),t("p",null,"下载配置模板并查看完整文档，快速完成 NuGet 仓库配置")],-1)),t("div",ht,[o(e,{type:"primary",onClick:n[0]||(n[0]=s=>p("NuGet.Config"))},{default:l(()=>[...n[2]||(n[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 NuGet.Config ",-1)])]),_:1}),o(e,{onClick:c},{default:l(()=>[...n[3]||(n[3]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看完整文档 ",-1)])]),_:1})])])])])}}}),Qt=b(jt,[["__scopeId","data-v-25a4d472"]]),Ft={class:"yum-apt-config"},Kt={class:"config-content"},Jt={class:"config-section"},Xt={class:"section-row"},Wt={class:"section-actions"},Zt={class:"config-section"},te={class:"section-row"},ee={class:"section-actions"},se=g({__name:"YumAPTConfig",setup(y){const d=x(),p=n=>{window.open(`/docs/templates/${n}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})},u=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(n,i)=>{const e=r("el-alert"),s=r("el-button"),v=r("el-divider");return m(),f("div",Ft,[o(e,{type:"info",closable:!1},{default:l(()=>[...i[2]||(i[2]=[a(" Yum 和 APT 是 Linux 系统的包管理器，用于安装、更新和管理系统软件包 ",-1)])]),_:1}),t("div",Kt,[t("div",Jt,[t("div",Xt,[i[5]||(i[5]=t("div",{class:"section-text"},[t("h4",null,"Yum 配置"),t("p",null,"下载配置文件并查看完整文档，快速完成 Yum 仓库配置")],-1)),t("div",Wt,[o(s,{type:"primary",onClick:i[0]||(i[0]=w=>p("moonlight.repo"))},{default:l(()=>[...i[3]||(i[3]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 Yum 配置 ",-1)])]),_:1}),o(s,{onClick:c},{default:l(()=>[...i[4]||(i[4]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看文档 ",-1)])]),_:1})])])]),o(v),t("div",Zt,[t("div",te,[i[8]||(i[8]=t("div",{class:"section-text"},[t("h4",null,"APT 配置"),t("p",null,"下载配置文件并查看完整文档，快速完成 APT 仓库配置")],-1)),t("div",ee,[o(s,{type:"primary",onClick:i[1]||(i[1]=w=>p("moonlight.list"))},{default:l(()=>[...i[6]||(i[6]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 APT 配置 ",-1)])]),_:1}),o(s,{onClick:u},{default:l(()=>[...i[7]||(i[7]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看文档 ",-1)])]),_:1})])])])])])}}}),oe=b(se,[["__scopeId","data-v-fad342d3"]]),ne={class:"configuration-guide"},le={class:"manager-tabs"},ie=g({__name:"ConfigurationGuide",setup(y){const d=C("npm");return(p,c)=>{const u=r("el-tab-pane"),n=r("el-tabs");return m(),f("div",ne,[c[1]||(c[1]=t("div",{class:"section-header"},[t("h2",null,"配置指南"),t("p",{class:"section-desc"},"选择您的包管理器，查看详细的配置说明和示例")],-1)),t("div",le,[o(n,{modelValue:d.value,"onUpdate:modelValue":c[0]||(c[0]=i=>d.value=i),type:"card",class:"guide-tabs"},{default:l(()=>[o(u,{label:"NPM",name:"npm"},{default:l(()=>[o(Ct)]),_:1}),o(u,{label:"Maven",name:"maven"},{default:l(()=>[o(Mt)]),_:1}),o(u,{label:"PyPI",name:"pypi"},{default:l(()=>[o(It)]),_:1}),o(u,{label:"Go",name:"go"},{default:l(()=>[o(zt)]),_:1}),o(u,{label:"NuGet",name:"nuget"},{default:l(()=>[o(Qt)]),_:1}),o(u,{label:"Yum/APT",name:"yum"},{default:l(()=>[o(oe)]),_:1})]),_:1},8,["modelValue"])])])}}}),ae=b(ie,[["__scopeId","data-v-5a6edf25"]]),de={class:"faq"},re={class:"search-box"},pe={class:"faq-content"},ue={class:"faq-category"},ce={class:"question-wrapper"},me={class:"question"},fe=["innerHTML"],_e=g({__name:"FAQ",setup(y){const d=C(""),p=C(["通用问题"]),c=[{name:"通用问题",items:[{question:"如何进行认证配置？",answer:`
          <p>当前版本使用用户名密码进行认证：</p>
          <ol>
            <li><strong>NPM/PyPI/NuGet：</strong>使用您的账号密码进行认证</li>
            <li><strong>Maven：</strong>在 settings.xml 中配置 server 信息</li>
            <li><strong>Go：</strong>通过 GOPROXY 配置，无需额外认证</li>
          </ol>
          <p style="margin-top: 10px; color: #64748b;">
            <i class="fa-solid fa-info-circle"></i> 访问令牌功能即将推出
          </p>
        `},{question:"本地仓库和代理仓库有什么区别？",answer:`
          <table>
            <tr>
              <th>类型</th>
              <th>说明</th>
              <th>用途</th>
            </tr>
            <tr>
              <td>本地仓库</td>
              <td>存储内部开发的包</td>
              <td>发布和托管内部包</td>
            </tr>
            <tr>
              <td>代理仓库</td>
              <td>代理外部仓库</td>
              <td>缓存外部包，加速下载</td>
            </tr>
            <tr>
              <td>虚拟仓库</td>
              <td>聚合多个仓库</td>
              <td>统一访问入口</td>
            </tr>
          </table>
        `}]},{name:"NPM 相关",items:[{question:"为什么 npm adduser 不工作？",answer:`
          <p>当前版本暂不支持 <code>npm adduser</code> 命令。</p>
          <p><strong>替代方案：</strong></p>
          <ol>
            <li>手动配置 <code>.npmrc</code> 文件，使用用户名密码认证</li>
            <li>联系管理员获取预配置文件</li>
          </ol>
        `},{question:"如何发布作用域包？",answer:`
          <p>配置作用域仓库：</p>
          <pre><code>@mycompany:registry=http://your-registry/repo/npm-local/</code></pre>
          <p>或使用命令行：</p>
          <pre><code>npm publish --registry=http://your-registry/repo/npm-local/</code></pre>
        `}]},{name:"Maven 相关",items:[{question:"Maven 发布包失败，提示 401 Unauthorized？",answer:`
          <p>检查以下配置：</p>
          <ol>
            <li>settings.xml 中的 server 配置</li>
            <li>pom.xml 中的 repository id 是否匹配</li>
            <li>用户名和密码是否正确</li>
          </ol>
        `}]},{name:"PyPI 相关",items:[{question:"使用 twine 上传包失败？",answer:`
          <p>当前上传端点为 <code>/pypi/upload/</code>，请使用：</p>
          <pre><code>twine upload --repository-url http://your-registry/pypi/upload/ dist/*</code></pre>
        `}]},{name:"Go 相关",items:[{question:'go get 报错 "checksum mismatch"？',answer:`
          <p>当前版本不支持校验和数据库，请禁用：</p>
          <pre><code>export GOSUMDB=off</code></pre>
        `}]}],u=k(()=>d.value?c.map(n=>({...n,items:n.items.filter(i=>i.question.toLowerCase().includes(d.value.toLowerCase())||i.answer.toLowerCase().includes(d.value.toLowerCase()))})).filter(n=>n.items.length>0):c);return(n,i)=>{const e=r("el-input"),s=r("el-collapse-item"),v=r("el-collapse"),w=r("el-alert");return m(),f("div",de,[i[5]||(i[5]=t("div",{class:"section-header"},[t("h2",null,"❓ 常见问题"),t("p",{class:"section-desc"},"查找常见问题的解答，快速解决您遇到的问题")],-1)),t("div",re,[o(e,{modelValue:d.value,"onUpdate:modelValue":i[0]||(i[0]=_=>d.value=_),placeholder:"搜索问题...","prefix-icon":"Search",clearable:"",size:"large"},null,8,["modelValue"])]),t("div",pe,[o(v,{modelValue:p.value,"onUpdate:modelValue":i[1]||(i[1]=_=>p.value=_),accordion:"",class:"faq-collapse"},{default:l(()=>[(m(!0),f(V,null,R(u.value,_=>(m(),U(s,{key:_.name,title:_.name,name:_.name},{default:l(()=>[t("div",ue,[(m(!0),f(V,null,R(_.items,(P,O)=>(m(),f("div",{key:O,class:"faq-item"},[t("div",ce,[i[2]||(i[2]=t("i",{class:"fa-solid fa-question-circle question-icon"},null,-1)),t("h4",me,q(P.question),1)]),t("div",{class:"answer",innerHTML:P.answer},null,8,fe)]))),128))])]),_:2},1032,["title","name"]))),128))]),_:1},8,["modelValue"])]),o(w,{type:"info",closable:!1,class:"contact-alert"},{title:l(()=>[...i[3]||(i[3]=[t("i",{class:"fa-solid fa-message-circle"},null,-1),a(" 没有找到答案？ ",-1)])]),default:l(()=>[i[4]||(i[4]=t("p",null,[a("联系管理员："),t("a",{href:"mailto:admin@company.com"},"admin@company.com")],-1))]),_:1})])}}}),ve=b(_e,[["__scopeId","data-v-b75c3e7f"]]),ge={class:"help-center"},be={class:"page-header"},ye={class:"content-panel"},we=g({__name:"HelpCenter",setup(y){const d=x(),p=C("quickstart"),c=()=>{d.back()};return(u,n)=>{const i=r("el-button"),e=r("el-tab-pane"),s=r("el-tabs");return m(),f("div",ge,[t("header",be,[n[2]||(n[2]=E('<div class="header-content" data-v-e5615caf><div class="header-icon" data-v-e5615caf><i class="fa-solid fa-question-circle" data-v-e5615caf></i></div><div class="header-text" data-v-e5615caf><h2 data-v-e5615caf>帮助中心</h2><p class="header-subtitle" data-v-e5615caf>查找使用文档和常见问题解答</p></div></div>',1)),o(i,{class:"back-btn",onClick:c},{default:l(()=>[...n[1]||(n[1]=[t("i",{class:"fa-solid fa-arrow-left"},null,-1),t("span",null,"返回",-1)])]),_:1})]),t("div",ye,[o(s,{modelValue:p.value,"onUpdate:modelValue":n[0]||(n[0]=v=>p.value=v),class:"help-tabs"},{default:l(()=>[o(e,{label:"快速开始",name:"quickstart"},{default:l(()=>[o(J)]),_:1}),o(e,{label:"配置指南",name:"configuration"},{default:l(()=>[o(ae)]),_:1}),o(e,{label:"常见问题",name:"faq"},{default:l(()=>[o(ve)]),_:1})]),_:1},8,["modelValue"])])])}}}),Ce=b(we,[["__scopeId","data-v-e5615caf"]]);export{Ce as default};
