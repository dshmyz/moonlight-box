import{B as y,H as m,L as f,P as t,Y as a,M as o,O as l,X as V,Z as G,az as R,a as C,f as k,ag as r,ay as T,F as N,ab as I,I as E}from"./vendor-4dUTcw0r.js";import{_ as b}from"./index-Cyo34ujV.js";import{C as $}from"./CodeBlock-C0Xwow75.js";import"./elementPlus-CdvGPpRh.js";import"./mermaid-asGaN1PN.js";import"./clipboard-BmZ27CFa.js";const U={class:"quick-start"},D={key:0,class:"step-content-wrapper"},z={class:"step-card"},A={class:"step-actions"},S={key:1,class:"step-content-wrapper"},H={class:"step-card"},B={key:0,class:"manager-config"},L=["innerHTML"],h={class:"step-actions"},Q={key:2,class:"step-content-wrapper"},F={class:"step-card final-step"},K={class:"action-buttons"},X=y({__name:"QuickStart",setup(w){const d=C(0),p=C("npm"),c=k(()=>({npm:"NPM",maven:"Maven",pypi:"PyPI",go:"Go"})[p.value]),u=k(()=>{const s=window.location.origin;return{npm:`
      <p>创建或编辑 <code>~/.npmrc</code> 文件：</p>
      <pre><code>registry=${s}/repository/npm-virtual/
//${window.location.host}/repository/npm-virtual/:_authToken=YOUR_TOKEN_HERE</code></pre>
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
      &lt;url&gt;${s}/repository/maven-virtual/&lt;/url&gt;
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
index-url = ${s}/repository/pypi-virtual/simple/
trusted-host = ${window.location.host}</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('pip.conf')">
          下载 pip.conf 模板
        </el-button>
      </p>
    `,go:`
      <p>设置环境变量：</p>
      <pre><code>export GOPROXY=${s}/go,https://proxy.golang.org,direct
export GOPRIVATE=${window.location.host}
export GOSUMDB=off</code></pre>
      <p style="margin-top: 10px">
        <el-button size="small" @click="downloadTemplate('go-env.sh')">
          下载环境变量脚本
        </el-button>
      </p>
    `}[p.value]}),n=s=>{window.open(`/docs/templates/${s}`,"_blank")},i=()=>{n({npm:".npmrc",maven:"settings.xml",pypi:"pip.conf",go:"go-env.sh"}[p.value])};return(s,e)=>{const _=r("el-step"),g=r("el-steps"),v=r("el-timeline-item"),x=r("el-timeline"),q=r("el-alert"),P=r("el-button"),O=r("el-radio-button"),Y=r("el-radio-group");return m(),f("div",U,[e[28]||(e[28]=t("div",{class:"section-header"},[t("h2",null,[t("i",{class:"fa-solid fa-rocket"}),a(" 快速开始")]),t("p",{class:"section-desc"},"按照以下步骤配置您的客户端，开始使用 Moonlight Box")],-1)),o(g,{active:d.value,"finish-status":"success","align-center":"",class:"steps-container"},{default:l(()=>[o(_,{title:"配置认证",description:"设置用户名密码"}),o(_,{title:"选择包管理器",description:"配置客户端工具"}),o(_,{title:"开始使用",description:"验证配置并使用"})]),_:1},8,["active"]),d.value===0?(m(),f("div",D,[t("div",z,[e[14]||(e[14]=t("div",{class:"step-icon"},[t("i",{class:"fa-solid fa-lock"})],-1)),e[15]||(e[15]=t("h3",null,"配置认证信息",-1)),e[16]||(e[16]=t("p",null,"根据您使用的包管理器，配置相应的认证方式：",-1)),o(x,null,{default:l(()=>[o(v,null,{dot:l(()=>[...e[5]||(e[5]=[t("i",{class:"fa-solid fa-key"},null,-1)])]),default:l(()=>[e[6]||(e[6]=t("p",null,[t("strong",null,"NPM/PyPI："),a("使用您的账号密码进行认证")],-1))]),_:1}),o(v,null,{dot:l(()=>[...e[7]||(e[7]=[t("i",{class:"fa-solid fa-file-code"},null,-1)])]),default:l(()=>[e[8]||(e[8]=t("p",null,[t("strong",null,"Maven："),a("在 settings.xml 中配置 server 信息")],-1))]),_:1}),o(v,null,{dot:l(()=>[...e[9]||(e[9]=[t("i",{class:"fa-solid fa-globe"},null,-1)])]),default:l(()=>[e[10]||(e[10]=t("p",null,[t("strong",null,"Go："),a("通过 GOPROXY 配置，无需额外认证")],-1))]),_:1})]),_:1}),o(q,{type:"info",closable:!1,style:{"margin-top":"20px"}},{title:l(()=>[...e[11]||(e[11]=[a("提示",-1)])]),default:l(()=>[e[12]||(e[12]=t("p",null,"访问令牌功能即将推出，当前版本请使用用户名密码进行认证",-1))]),_:1}),t("div",A,[o(P,{type:"primary",size:"large",onClick:e[0]||(e[0]=M=>d.value=1)},{default:l(()=>[...e[13]||(e[13]=[a(" 下一步 ",-1),t("i",{class:"fa-solid fa-arrow-right"},null,-1)])]),_:1})])])])):V("",!0),d.value===1?(m(),f("div",S,[t("div",H,[e[23]||(e[23]=t("div",{class:"step-icon"},[t("i",{class:"fa-solid fa-box"})],-1)),e[24]||(e[24]=t("h3",null,"选择您的包管理器",-1)),o(Y,{modelValue:p.value,"onUpdate:modelValue":e[1]||(e[1]=M=>p.value=M),size:"large",class:"manager-selector"},{default:l(()=>[o(O,{value:"npm"},{default:l(()=>[...e[17]||(e[17]=[t("i",{class:"fa-brands fa-npm"},null,-1),t("span",null,"NPM",-1)])]),_:1}),o(O,{value:"maven"},{default:l(()=>[...e[18]||(e[18]=[t("i",{class:"fa-brands fa-java"},null,-1),t("span",null,"Maven",-1)])]),_:1}),o(O,{value:"pypi"},{default:l(()=>[...e[19]||(e[19]=[t("i",{class:"fa-brands fa-python"},null,-1),t("span",null,"PyPI",-1)])]),_:1}),o(O,{value:"go"},{default:l(()=>[...e[20]||(e[20]=[t("i",{class:"fa-brands fa-golang"},null,-1),t("span",null,"Go",-1)])]),_:1})]),_:1},8,["modelValue"]),p.value?(m(),f("div",B,[t("h4",null,G(c.value)+" 配置",1),t("div",{innerHTML:u.value},null,8,L)])):V("",!0),t("div",h,[o(P,{size:"large",onClick:e[2]||(e[2]=M=>d.value=0)},{default:l(()=>[...e[21]||(e[21]=[t("i",{class:"fa-solid fa-arrow-left"},null,-1),a(" 上一步 ",-1)])]),_:1}),o(P,{type:"primary",size:"large",onClick:e[3]||(e[3]=M=>d.value=2)},{default:l(()=>[...e[22]||(e[22]=[a(" 下一步 ",-1),t("i",{class:"fa-solid fa-arrow-right"},null,-1)])]),_:1})])])])):V("",!0),d.value===2?(m(),f("div",Q,[t("div",F,[e[27]||(e[27]=R('<div class="success-icon" data-v-c04e3ef6>🎉</div><h3 data-v-c04e3ef6>配置完成！</h3><p class="success-desc" data-v-c04e3ef6>您现在可以开始使用 Moonlight Box 仓库了</p><div class="next-steps" data-v-c04e3ef6><h4 data-v-c04e3ef6>下一步操作</h4><ul data-v-c04e3ef6><li data-v-c04e3ef6><i class="fa-solid fa-arrow-right" data-v-c04e3ef6></i> 浏览仓库查找可用的软件包</li><li data-v-c04e3ef6><i class="fa-solid fa-arrow-right" data-v-c04e3ef6></i> 发布您的第一个包</li><li data-v-c04e3ef6><i class="fa-solid fa-arrow-right" data-v-c04e3ef6></i> 配置 CI/CD 集成</li></ul></div>',4)),t("div",K,[o(P,{type:"primary",size:"large",onClick:e[4]||(e[4]=M=>s.$router.push("/"))},{default:l(()=>[...e[25]||(e[25]=[t("i",{class:"fa-solid fa-search"},null,-1),a(" 浏览仓库 ",-1)])]),_:1}),o(P,{size:"large",onClick:i},{default:l(()=>[...e[26]||(e[26]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载配置文件 ",-1)])]),_:1})])])])):V("",!0)])}}}),j=b(X,[["__scopeId","data-v-c04e3ef6"]]),J={class:"npm-config"},W={class:"config-methods"},Z={class:"method-card"},tt={class:"method-steps"},st={class:"step-item"},et={class:"step-content"},ot={class:"step-item"},lt={class:"step-content"},nt={class:"step-item"},it={class:"step-content"},at={class:"method-card"},dt={class:"method-steps"},rt={class:"step-item"},pt={class:"step-content"},ut={class:"step-item"},ct={class:"step-content"},mt={class:"step-item"},ft={class:"step-content"},vt={class:"method-card"},_t={class:"method-steps"},gt={class:"step-item"},yt={class:"step-content"},bt={class:"step-item"},wt={class:"step-content"},$t={class:"troubleshooting"},kt=y({__name:"NPMConfig",setup(w){const d=k(()=>`${window.location.origin}/repository/npm-virtual/`),p=k(()=>window.location.host),c=k(()=>`# NPM 配置文件
registry=${d.value}

# 认证信息
//${p.value}/repository/npm-virtual/:_authToken=YOUR_TOKEN_HERE

# 作用域包配置（可选）
# @mycompany:registry=${window.location.origin}/repository/npm-local/`),u=k(()=>JSON.stringify({publishConfig:{registry:d.value}},null,2)),n=i=>{window.open(`/docs/templates/${i}`,"_blank")};return(i,s)=>{const e=r("el-alert"),_=r("el-button"),g=r("el-collapse-item"),v=r("el-collapse");return m(),f("div",J,[o(e,{type:"info",closable:!1},{default:l(()=>[...s[1]||(s[1]=[a(" NPM 是 Node.js 的包管理器，用于安装、发布和管理 JavaScript 包 ",-1)])]),_:1}),t("div",W,[t("div",Z,[s[11]||(s[11]=t("div",{class:"method-header"},[t("div",{class:"method-badge"},"推荐"),t("h4",null,"方式一：配置文件")],-1)),s[12]||(s[12]=t("p",{class:"method-desc"},[a("通过编辑 "),t("code",null,"~/.npmrc"),a(" 文件进行配置，适合长期使用")],-1)),t("div",tt,[t("div",st,[s[4]||(s[4]=t("div",{class:"step-number"},"1",-1)),t("div",et,[s[3]||(s[3]=t("h5",null,"下载配置模板",-1)),o(_,{type:"primary",onClick:s[0]||(s[0]=x=>n(".npmrc"))},{default:l(()=>[...s[2]||(s[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 .npmrc 模板 ",-1)])]),_:1})])]),t("div",ot,[s[7]||(s[7]=t("div",{class:"step-number"},"2",-1)),t("div",lt,[s[5]||(s[5]=t("h5",null,"放置配置文件",-1)),s[6]||(s[6]=t("p",null,[a("将下载的文件放到 "),t("code",null,"~/.npmrc"),a(" 或项目根目录")],-1)),o($,{code:c.value,title:"~/.npmrc"},null,8,["code"])])]),t("div",nt,[s[10]||(s[10]=t("div",{class:"step-number"},"3",-1)),t("div",it,[s[8]||(s[8]=t("h5",null,"替换认证信息",-1)),s[9]||(s[9]=t("p",null,[a("将 "),t("code",null,"YOUR_TOKEN_HERE"),a(" 替换为您的访问令牌")],-1)),o($,{code:"sed -i 's/YOUR_TOKEN_HERE/your-actual-token/g' ~/.npmrc"})])])])]),t("div",at,[s[19]||(s[19]=t("div",{class:"method-header"},[t("h4",null,"方式二：命令行配置")],-1)),s[20]||(s[20]=t("p",{class:"method-desc"},"通过命令行快速配置，适合临时使用",-1)),t("div",dt,[t("div",rt,[s[14]||(s[14]=t("div",{class:"step-number"},"1",-1)),t("div",pt,[s[13]||(s[13]=t("h5",null,"设置仓库地址",-1)),o($,{code:`npm config set registry ${d.value}`},null,8,["code"])])]),t("div",ut,[s[16]||(s[16]=t("div",{class:"step-number"},"2",-1)),t("div",ct,[s[15]||(s[15]=t("h5",null,"设置认证信息",-1)),o($,{code:`npm config set //${p.value}/repository/npm-virtual/:_authToken YOUR_TOKEN_HERE`},null,8,["code"])])]),t("div",mt,[s[18]||(s[18]=t("div",{class:"step-number"},"3",-1)),t("div",ft,[s[17]||(s[17]=t("h5",null,"验证配置",-1)),o($,{code:"npm config list"})])])])]),t("div",vt,[s[25]||(s[25]=t("div",{class:"method-header"},[t("h4",null,"发布包")],-1)),t("div",_t,[t("div",gt,[s[22]||(s[22]=t("div",{class:"step-number"},"1",-1)),t("div",yt,[s[21]||(s[21]=t("h5",null,"发布到本地仓库",-1)),o($,{code:`npm publish --registry=${d.value}`},null,8,["code"])])]),t("div",bt,[s[24]||(s[24]=t("div",{class:"step-number"},"2",-1)),t("div",wt,[s[23]||(s[23]=t("h5",null,"或在 package.json 中配置",-1)),o($,{code:u.value,title:"package.json"},null,8,["code"])])])])])]),t("div",$t,[s[29]||(s[29]=t("h4",null,"常见问题",-1)),o(v,null,{default:l(()=>[o(g,{title:"npm install 报错 404 Not Found",name:"404"},{default:l(()=>[...s[26]||(s[26]=[t("p",null,[t("strong",null,"可能的原因：")],-1),t("ol",null,[t("li",null,[a("仓库地址错误 - 检查 "),t("code",null,"npm config get registry")]),t("li",null,"包不存在 - 确认包已发布到仓库"),t("li",null,"认证失败 - 检查令牌是否正确")],-1)])]),_:1}),o(g,{title:"npm adduser 不工作",name:"adduser"},{default:l(()=>[...s[27]||(s[27]=[t("p",null,[a("当前版本暂不支持 "),t("code",null,"npm adduser"),a(" 命令。")],-1),t("p",null,[t("strong",null,"替代方案：")],-1),t("ol",null,[t("li",null,"通过 Web UI 获取令牌"),t("li",null,"手动配置 .npmrc 文件"),t("li",null,"联系管理员获取预配置文件")],-1)])]),_:1}),o(g,{title:"如何删除已发布的包",name:"unpublish"},{default:l(()=>[o($,{code:"npm unpublish package-name@1.0.0 --registry=http://your-registry/repository/npm-local/"}),o(e,{type:"warning",closable:!1},{default:l(()=>[...s[28]||(s[28]=[a(" 需要相应权限才能删除包 ",-1)])]),_:1})]),_:1})]),_:1})])])}}}),Ct=b(kt,[["__scopeId","data-v-a5206f82"]]),xt={class:"maven-config"},Pt={class:"config-content"},Mt={class:"content-row"},Tt={class:"content-actions"},Ot=y({__name:"MavenConfig",setup(w){const d=T(),p=u=>{window.open(`/docs/templates/${u}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(u,n)=>{const i=r("el-alert"),s=r("el-button");return m(),f("div",xt,[o(i,{type:"info",closable:!1},{default:l(()=>[...n[1]||(n[1]=[a(" Maven 是 Java 项目的构建和依赖管理工具，用于管理项目依赖、构建生命周期和插件配置 ",-1)])]),_:1}),t("div",Pt,[t("div",Mt,[n[4]||(n[4]=t("div",{class:"content-text"},[t("h4",null,"快速开始"),t("p",null,"下载配置模板并查看完整文档，快速完成 Maven 仓库配置")],-1)),t("div",Tt,[o(s,{type:"primary",onClick:n[0]||(n[0]=e=>p("settings.xml"))},{default:l(()=>[...n[2]||(n[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 settings.xml ",-1)])]),_:1}),o(s,{onClick:c},{default:l(()=>[...n[3]||(n[3]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看完整文档 ",-1)])]),_:1})])])])])}}}),Vt=b(Ot,[["__scopeId","data-v-87e57313"]]),qt={class:"pypi-config"},Nt={class:"config-content"},It={class:"content-row"},Gt={class:"content-actions"},Rt=y({__name:"PyPIConfig",setup(w){const d=T(),p=u=>{window.open(`/docs/templates/${u}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(u,n)=>{const i=r("el-alert"),s=r("el-button");return m(),f("div",qt,[o(i,{type:"info",closable:!1},{default:l(()=>[...n[1]||(n[1]=[a(" PyPI 是 Python 的包索引和依赖管理工具，用于安装、发布和管理 Python 包 ",-1)])]),_:1}),t("div",Nt,[t("div",It,[n[4]||(n[4]=t("div",{class:"content-text"},[t("h4",null,"快速开始"),t("p",null,"下载配置模板并查看完整文档，快速完成 PyPI 仓库配置")],-1)),t("div",Gt,[o(s,{type:"primary",onClick:n[0]||(n[0]=e=>p("pip.conf"))},{default:l(()=>[...n[2]||(n[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 pip.conf ",-1)])]),_:1}),o(s,{onClick:c},{default:l(()=>[...n[3]||(n[3]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看完整文档 ",-1)])]),_:1})])])])])}}}),Yt=b(Rt,[["__scopeId","data-v-f22ecf5e"]]),Et={class:"go-config"},Ut={class:"config-content"},Dt={class:"content-row"},zt={class:"content-actions"},At=y({__name:"GoConfig",setup(w){const d=T(),p=u=>{window.open(`/docs/templates/${u}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(u,n)=>{const i=r("el-alert"),s=r("el-button");return m(),f("div",Et,[o(i,{type:"info",closable:!1},{default:l(()=>[...n[1]||(n[1]=[a(" Go modules 是 Go 语言的依赖管理系统，通过 GOPROXY 环境变量配置代理 ",-1)])]),_:1}),t("div",Ut,[t("div",Dt,[n[4]||(n[4]=t("div",{class:"content-text"},[t("h4",null,"快速开始"),t("p",null,"下载环境变量脚本并查看完整文档，快速完成 Go 模块仓库配置")],-1)),t("div",zt,[o(s,{type:"primary",onClick:n[0]||(n[0]=e=>p("go-env.sh"))},{default:l(()=>[...n[2]||(n[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载环境变量脚本 ",-1)])]),_:1}),o(s,{onClick:c},{default:l(()=>[...n[3]||(n[3]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看完整文档 ",-1)])]),_:1})])])])])}}}),St=b(At,[["__scopeId","data-v-45093aab"]]),Ht={class:"yum-apt-config"},Bt={class:"config-content"},Lt={class:"config-section"},ht={class:"section-row"},Qt={class:"section-actions"},Ft={class:"config-section"},Kt={class:"section-row"},Xt={class:"section-actions"},jt=y({__name:"YumAPTConfig",setup(w){const d=T(),p=n=>{window.open(`/docs/templates/${n}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})},u=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(n,i)=>{const s=r("el-alert"),e=r("el-button"),_=r("el-divider");return m(),f("div",Ht,[o(s,{type:"info",closable:!1},{default:l(()=>[...i[2]||(i[2]=[a(" Yum 和 APT 是 Linux 系统的包管理器，用于安装、更新和管理系统软件包 ",-1)])]),_:1}),t("div",Bt,[t("div",Lt,[t("div",ht,[i[5]||(i[5]=t("div",{class:"section-text"},[t("h4",null,"Yum 配置"),t("p",null,"下载配置文件并查看完整文档，快速完成 Yum 仓库配置")],-1)),t("div",Qt,[o(e,{type:"primary",onClick:i[0]||(i[0]=g=>p("moonlight.repo"))},{default:l(()=>[...i[3]||(i[3]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 Yum 配置 ",-1)])]),_:1}),o(e,{onClick:c},{default:l(()=>[...i[4]||(i[4]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看文档 ",-1)])]),_:1})])])]),o(_),t("div",Ft,[t("div",Kt,[i[8]||(i[8]=t("div",{class:"section-text"},[t("h4",null,"APT 配置"),t("p",null,"下载配置文件并查看完整文档，快速完成 APT 仓库配置")],-1)),t("div",Xt,[o(e,{type:"primary",onClick:i[1]||(i[1]=g=>p("moonlight.list"))},{default:l(()=>[...i[6]||(i[6]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 APT 配置 ",-1)])]),_:1}),o(e,{onClick:u},{default:l(()=>[...i[7]||(i[7]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看文档 ",-1)])]),_:1})])])])])])}}}),Jt=b(jt,[["__scopeId","data-v-fad342d3"]]),Wt={class:"configuration-guide"},Zt={class:"manager-tabs"},ts=y({__name:"ConfigurationGuide",setup(w){const d=C("npm");return(p,c)=>{const u=r("el-tab-pane"),n=r("el-tabs");return m(),f("div",Wt,[c[1]||(c[1]=t("div",{class:"section-header"},[t("h2",null,"配置指南"),t("p",{class:"section-desc"},"选择您的包管理器，查看详细的配置说明和示例")],-1)),t("div",Zt,[o(n,{modelValue:d.value,"onUpdate:modelValue":c[0]||(c[0]=i=>d.value=i),type:"card",class:"guide-tabs"},{default:l(()=>[o(u,{label:"NPM",name:"npm"},{default:l(()=>[o(Ct)]),_:1}),o(u,{label:"Maven",name:"maven"},{default:l(()=>[o(Vt)]),_:1}),o(u,{label:"PyPI",name:"pypi"},{default:l(()=>[o(Yt)]),_:1}),o(u,{label:"Go",name:"go"},{default:l(()=>[o(St)]),_:1}),o(u,{label:"Yum/APT",name:"yum"},{default:l(()=>[o(Jt)]),_:1})]),_:1},8,["modelValue"])])])}}}),ss=b(ts,[["__scopeId","data-v-0fe2db68"]]),es={class:"faq"},os={class:"search-box"},ls={class:"faq-content"},ns={class:"faq-category"},is={class:"question-wrapper"},as={class:"question"},ds=["innerHTML"],rs=y({__name:"FAQ",setup(w){const d=C(""),p=C(["通用问题"]),c=[{name:"通用问题",items:[{question:"如何进行认证配置？",answer:`
          <p>当前版本使用用户名密码进行认证：</p>
          <ol>
            <li><strong>NPM/PyPI：</strong>使用您的账号密码进行认证</li>
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
          <pre><code>@mycompany:registry=http://your-registry/repository/npm-local/</code></pre>
          <p>或使用命令行：</p>
          <pre><code>npm publish --registry=http://your-registry/repository/npm-local/</code></pre>
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
        `}]}],u=k(()=>d.value?c.map(n=>({...n,items:n.items.filter(i=>i.question.toLowerCase().includes(d.value.toLowerCase())||i.answer.toLowerCase().includes(d.value.toLowerCase()))})).filter(n=>n.items.length>0):c);return(n,i)=>{const s=r("el-input"),e=r("el-collapse-item"),_=r("el-collapse"),g=r("el-alert");return m(),f("div",es,[i[5]||(i[5]=t("div",{class:"section-header"},[t("h2",null,[t("i",{class:"fa-solid fa-circle-question"}),a(" 常见问题")]),t("p",{class:"section-desc"},"查找常见问题的解答，快速解决您遇到的问题")],-1)),t("div",os,[o(s,{modelValue:d.value,"onUpdate:modelValue":i[0]||(i[0]=v=>d.value=v),placeholder:"搜索问题...","prefix-icon":"Search",clearable:"",size:"large"},null,8,["modelValue"])]),t("div",ls,[o(_,{modelValue:p.value,"onUpdate:modelValue":i[1]||(i[1]=v=>p.value=v),accordion:"",class:"faq-collapse"},{default:l(()=>[(m(!0),f(N,null,I(u.value,v=>(m(),E(e,{key:v.name,title:v.name,name:v.name},{default:l(()=>[t("div",ns,[(m(!0),f(N,null,I(v.items,(x,q)=>(m(),f("div",{key:q,class:"faq-item"},[t("div",is,[i[2]||(i[2]=t("i",{class:"fa-solid fa-question-circle question-icon"},null,-1)),t("h4",as,G(x.question),1)]),t("div",{class:"answer",innerHTML:x.answer},null,8,ds)]))),128))])]),_:2},1032,["title","name"]))),128))]),_:1},8,["modelValue"])]),o(g,{type:"info",closable:!1,class:"contact-alert"},{title:l(()=>[...i[3]||(i[3]=[t("i",{class:"fa-solid fa-message-circle"},null,-1),a(" 没有找到答案？ ",-1)])]),default:l(()=>[i[4]||(i[4]=t("p",null,[a("联系管理员："),t("a",{href:"mailto:admin@company.com"},"admin@company.com")],-1))]),_:1})])}}}),ps=b(rs,[["__scopeId","data-v-9293d66d"]]),us={class:"help-center"},cs={class:"page-header"},ms={class:"content-panel"},fs=y({__name:"HelpCenter",setup(w){const d=T(),p=C("quickstart"),c=()=>{d.back()};return(u,n)=>{const i=r("el-button"),s=r("el-tab-pane"),e=r("el-tabs");return m(),f("div",us,[t("header",cs,[n[2]||(n[2]=R('<div class="header-content" data-v-e5615caf><div class="header-icon" data-v-e5615caf><i class="fa-solid fa-question-circle" data-v-e5615caf></i></div><div class="header-text" data-v-e5615caf><h2 data-v-e5615caf>帮助中心</h2><p class="header-subtitle" data-v-e5615caf>查找使用文档和常见问题解答</p></div></div>',1)),o(i,{class:"back-btn",onClick:c},{default:l(()=>[...n[1]||(n[1]=[t("i",{class:"fa-solid fa-arrow-left"},null,-1),t("span",null,"返回",-1)])]),_:1})]),t("div",ms,[o(e,{modelValue:p.value,"onUpdate:modelValue":n[0]||(n[0]=_=>p.value=_),class:"help-tabs"},{default:l(()=>[o(s,{label:"快速开始",name:"quickstart"},{default:l(()=>[o(j)]),_:1}),o(s,{label:"配置指南",name:"configuration"},{default:l(()=>[o(ss)]),_:1}),o(s,{label:"常见问题",name:"faq"},{default:l(()=>[o(ps)]),_:1})]),_:1},8,["modelValue"])])])}}}),$s=b(fs,[["__scopeId","data-v-e5615caf"]]);export{$s as default};
