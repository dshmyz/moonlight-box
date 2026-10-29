import{B as y,H as m,L as f,P as t,Y as a,M as e,O as n,X as V,Z as G,az as R,a as C,f as k,ag as r,ay as T,F as I,ab as N,I as E,u as U}from"./vendor-1endFS9I.js";import{_ as b}from"./index-BBev6ung.js";import{C as $}from"./CodeBlock-61jJepOE.js";import"./elementPlus-CliQ8RVu.js";import"./mermaid-Clg-kKwO.js";import"./clipboard-BogYB5YI.js";const D={class:"quick-start"},A={key:0,class:"step-content-wrapper"},z={class:"step-card"},S={class:"step-actions"},H={key:1,class:"step-content-wrapper"},B={class:"step-card"},L={key:0,class:"manager-config"},h=["innerHTML"],Q={class:"step-actions"},F={key:2,class:"step-content-wrapper"},K={class:"step-card final-step"},X={class:"action-buttons"},j=y({__name:"QuickStart",setup(w){const d=C(0),p=C("npm"),c=k(()=>({npm:"NPM",maven:"Maven",pypi:"PyPI",go:"Go"})[p.value]),u=k(()=>{const s=window.location.origin;return{npm:`
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
    `}[p.value]}),l=s=>{window.open(`/docs/templates/${s}`,"_blank")},i=()=>{l({npm:".npmrc",maven:"settings.xml",pypi:"pip.conf",go:"go-env.sh"}[p.value])};return(s,o)=>{const _=r("el-step"),g=r("el-steps"),v=r("el-timeline-item"),x=r("el-timeline"),q=r("el-alert"),P=r("el-button"),O=r("el-radio-button"),Y=r("el-radio-group");return m(),f("div",D,[o[28]||(o[28]=t("div",{class:"section-header"},[t("h2",null,[t("i",{class:"fa-solid fa-rocket"}),a(" 快速开始")]),t("p",{class:"section-desc"},"按照以下步骤配置您的客户端，开始使用 Moonlight Box")],-1)),e(g,{active:d.value,"finish-status":"success","align-center":"",class:"steps-container"},{default:n(()=>[e(_,{title:"配置认证",description:"设置用户名密码"}),e(_,{title:"选择包管理器",description:"配置客户端工具"}),e(_,{title:"开始使用",description:"验证配置并使用"})]),_:1},8,["active"]),d.value===0?(m(),f("div",A,[t("div",z,[o[14]||(o[14]=t("div",{class:"step-icon"},[t("i",{class:"fa-solid fa-lock"})],-1)),o[15]||(o[15]=t("h3",null,"配置认证信息",-1)),o[16]||(o[16]=t("p",null,"根据您使用的包管理器，配置相应的认证方式：",-1)),e(x,null,{default:n(()=>[e(v,null,{dot:n(()=>[...o[5]||(o[5]=[t("i",{class:"fa-solid fa-key"},null,-1)])]),default:n(()=>[o[6]||(o[6]=t("p",null,[t("strong",null,"NPM/PyPI："),a("使用您的账号密码进行认证")],-1))]),_:1}),e(v,null,{dot:n(()=>[...o[7]||(o[7]=[t("i",{class:"fa-solid fa-file-code"},null,-1)])]),default:n(()=>[o[8]||(o[8]=t("p",null,[t("strong",null,"Maven："),a("在 settings.xml 中配置 server 信息")],-1))]),_:1}),e(v,null,{dot:n(()=>[...o[9]||(o[9]=[t("i",{class:"fa-solid fa-globe"},null,-1)])]),default:n(()=>[o[10]||(o[10]=t("p",null,[t("strong",null,"Go："),a("通过 GOPROXY 配置，无需额外认证")],-1))]),_:1})]),_:1}),e(q,{type:"info",closable:!1,style:{"margin-top":"20px"}},{title:n(()=>[...o[11]||(o[11]=[a("提示",-1)])]),default:n(()=>[o[12]||(o[12]=t("p",null,"访问令牌功能即将推出，当前版本请使用用户名密码进行认证",-1))]),_:1}),t("div",S,[e(P,{type:"primary",size:"large",onClick:o[0]||(o[0]=M=>d.value=1)},{default:n(()=>[...o[13]||(o[13]=[a(" 下一步 ",-1),t("i",{class:"fa-solid fa-arrow-right"},null,-1)])]),_:1})])])])):V("",!0),d.value===1?(m(),f("div",H,[t("div",B,[o[23]||(o[23]=t("div",{class:"step-icon"},[t("i",{class:"fa-solid fa-box"})],-1)),o[24]||(o[24]=t("h3",null,"选择您的包管理器",-1)),e(Y,{modelValue:p.value,"onUpdate:modelValue":o[1]||(o[1]=M=>p.value=M),size:"large",class:"manager-selector"},{default:n(()=>[e(O,{value:"npm"},{default:n(()=>[...o[17]||(o[17]=[t("i",{class:"fa-brands fa-npm"},null,-1),t("span",null,"NPM",-1)])]),_:1}),e(O,{value:"maven"},{default:n(()=>[...o[18]||(o[18]=[t("i",{class:"fa-brands fa-java"},null,-1),t("span",null,"Maven",-1)])]),_:1}),e(O,{value:"pypi"},{default:n(()=>[...o[19]||(o[19]=[t("i",{class:"fa-brands fa-python"},null,-1),t("span",null,"PyPI",-1)])]),_:1}),e(O,{value:"go"},{default:n(()=>[...o[20]||(o[20]=[t("i",{class:"fa-brands fa-golang"},null,-1),t("span",null,"Go",-1)])]),_:1})]),_:1},8,["modelValue"]),p.value?(m(),f("div",L,[t("h4",null,G(c.value)+" 配置",1),t("div",{innerHTML:u.value},null,8,h)])):V("",!0),t("div",Q,[e(P,{size:"large",onClick:o[2]||(o[2]=M=>d.value=0)},{default:n(()=>[...o[21]||(o[21]=[t("i",{class:"fa-solid fa-arrow-left"},null,-1),a(" 上一步 ",-1)])]),_:1}),e(P,{type:"primary",size:"large",onClick:o[3]||(o[3]=M=>d.value=2)},{default:n(()=>[...o[22]||(o[22]=[a(" 下一步 ",-1),t("i",{class:"fa-solid fa-arrow-right"},null,-1)])]),_:1})])])])):V("",!0),d.value===2?(m(),f("div",F,[t("div",K,[o[27]||(o[27]=R('<div class="success-icon" data-v-c04e3ef6>🎉</div><h3 data-v-c04e3ef6>配置完成！</h3><p class="success-desc" data-v-c04e3ef6>您现在可以开始使用 Moonlight Box 仓库了</p><div class="next-steps" data-v-c04e3ef6><h4 data-v-c04e3ef6>下一步操作</h4><ul data-v-c04e3ef6><li data-v-c04e3ef6><i class="fa-solid fa-arrow-right" data-v-c04e3ef6></i> 浏览仓库查找可用的软件包</li><li data-v-c04e3ef6><i class="fa-solid fa-arrow-right" data-v-c04e3ef6></i> 发布您的第一个包</li><li data-v-c04e3ef6><i class="fa-solid fa-arrow-right" data-v-c04e3ef6></i> 配置 CI/CD 集成</li></ul></div>',4)),t("div",X,[e(P,{type:"primary",size:"large",onClick:o[4]||(o[4]=M=>s.$router.push("/"))},{default:n(()=>[...o[25]||(o[25]=[t("i",{class:"fa-solid fa-search"},null,-1),a(" 浏览仓库 ",-1)])]),_:1}),e(P,{size:"large",onClick:i},{default:n(()=>[...o[26]||(o[26]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载配置文件 ",-1)])]),_:1})])])])):V("",!0)])}}}),J=b(j,[["__scopeId","data-v-c04e3ef6"]]),W={class:"npm-config"},Z={class:"config-methods"},tt={class:"method-card"},st={class:"method-steps"},ot={class:"step-item"},et={class:"step-content"},lt={class:"step-item"},nt={class:"step-content"},it={class:"step-item"},at={class:"step-content"},dt={class:"method-card"},rt={class:"method-steps"},pt={class:"step-item"},ut={class:"step-content"},ct={class:"step-item"},mt={class:"step-content"},ft={class:"step-item"},vt={class:"step-content"},_t={class:"method-card"},gt={class:"method-steps"},yt={class:"step-item"},bt={class:"step-content"},wt={class:"step-item"},$t={class:"step-content"},kt={class:"troubleshooting"},Ct=y({__name:"NPMConfig",setup(w){const d=k(()=>`${window.location.origin}/repository/npm-virtual/`),p=k(()=>window.location.host),c=k(()=>`# NPM 配置文件
registry=${d.value}

# 认证信息
//${p.value}/repository/npm-virtual/:_authToken=YOUR_TOKEN_HERE

# 作用域包配置（可选）
# @mycompany:registry=${window.location.origin}/repository/npm-local/`),u=k(()=>JSON.stringify({publishConfig:{registry:d.value}},null,2)),l=i=>{window.open(`/docs/templates/${i}`,"_blank")};return(i,s)=>{const o=r("el-alert"),_=r("el-button"),g=r("el-collapse-item"),v=r("el-collapse");return m(),f("div",W,[e(o,{type:"info",closable:!1},{default:n(()=>[...s[1]||(s[1]=[a(" NPM 是 Node.js 的包管理器，用于安装、发布和管理 JavaScript 包 ",-1)])]),_:1}),t("div",Z,[t("div",tt,[s[11]||(s[11]=t("div",{class:"method-header"},[t("div",{class:"method-badge"},"推荐"),t("h4",null,"方式一：配置文件")],-1)),s[12]||(s[12]=t("p",{class:"method-desc"},[a("通过编辑 "),t("code",null,"~/.npmrc"),a(" 文件进行配置，适合长期使用")],-1)),t("div",st,[t("div",ot,[s[4]||(s[4]=t("div",{class:"step-number"},"1",-1)),t("div",et,[s[3]||(s[3]=t("h5",null,"下载配置模板",-1)),e(_,{type:"primary",onClick:s[0]||(s[0]=x=>l(".npmrc"))},{default:n(()=>[...s[2]||(s[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 .npmrc 模板 ",-1)])]),_:1})])]),t("div",lt,[s[7]||(s[7]=t("div",{class:"step-number"},"2",-1)),t("div",nt,[s[5]||(s[5]=t("h5",null,"放置配置文件",-1)),s[6]||(s[6]=t("p",null,[a("将下载的文件放到 "),t("code",null,"~/.npmrc"),a(" 或项目根目录")],-1)),e($,{code:c.value,title:"~/.npmrc"},null,8,["code"])])]),t("div",it,[s[10]||(s[10]=t("div",{class:"step-number"},"3",-1)),t("div",at,[s[8]||(s[8]=t("h5",null,"替换认证信息",-1)),s[9]||(s[9]=t("p",null,[a("将 "),t("code",null,"YOUR_TOKEN_HERE"),a(" 替换为您的访问令牌")],-1)),e($,{code:"sed -i 's/YOUR_TOKEN_HERE/your-actual-token/g' ~/.npmrc"})])])])]),t("div",dt,[s[19]||(s[19]=t("div",{class:"method-header"},[t("h4",null,"方式二：命令行配置")],-1)),s[20]||(s[20]=t("p",{class:"method-desc"},"通过命令行快速配置，适合临时使用",-1)),t("div",rt,[t("div",pt,[s[14]||(s[14]=t("div",{class:"step-number"},"1",-1)),t("div",ut,[s[13]||(s[13]=t("h5",null,"设置仓库地址",-1)),e($,{code:`npm config set registry ${d.value}`},null,8,["code"])])]),t("div",ct,[s[16]||(s[16]=t("div",{class:"step-number"},"2",-1)),t("div",mt,[s[15]||(s[15]=t("h5",null,"设置认证信息",-1)),e($,{code:`npm config set //${p.value}/repository/npm-virtual/:_authToken YOUR_TOKEN_HERE`},null,8,["code"])])]),t("div",ft,[s[18]||(s[18]=t("div",{class:"step-number"},"3",-1)),t("div",vt,[s[17]||(s[17]=t("h5",null,"验证配置",-1)),e($,{code:"npm config list"})])])])]),t("div",_t,[s[25]||(s[25]=t("div",{class:"method-header"},[t("h4",null,"发布包")],-1)),t("div",gt,[t("div",yt,[s[22]||(s[22]=t("div",{class:"step-number"},"1",-1)),t("div",bt,[s[21]||(s[21]=t("h5",null,"发布到本地仓库",-1)),e($,{code:`npm publish --registry=${d.value}`},null,8,["code"])])]),t("div",wt,[s[24]||(s[24]=t("div",{class:"step-number"},"2",-1)),t("div",$t,[s[23]||(s[23]=t("h5",null,"或在 package.json 中配置",-1)),e($,{code:u.value,title:"package.json"},null,8,["code"])])])])])]),t("div",kt,[s[29]||(s[29]=t("h4",null,"常见问题",-1)),e(v,null,{default:n(()=>[e(g,{title:"npm install 报错 404 Not Found",name:"404"},{default:n(()=>[...s[26]||(s[26]=[t("p",null,[t("strong",null,"可能的原因：")],-1),t("ol",null,[t("li",null,[a("仓库地址错误 - 检查 "),t("code",null,"npm config get registry")]),t("li",null,"包不存在 - 确认包已发布到仓库"),t("li",null,"认证失败 - 检查令牌是否正确")],-1)])]),_:1}),e(g,{title:"npm adduser 不工作",name:"adduser"},{default:n(()=>[...s[27]||(s[27]=[t("p",null,[a("当前版本暂不支持 "),t("code",null,"npm adduser"),a(" 命令。")],-1),t("p",null,[t("strong",null,"替代方案：")],-1),t("ol",null,[t("li",null,"通过 Web UI 获取令牌"),t("li",null,"手动配置 .npmrc 文件"),t("li",null,"联系管理员获取预配置文件")],-1)])]),_:1}),e(g,{title:"如何删除已发布的包",name:"unpublish"},{default:n(()=>[e($,{code:"npm unpublish package-name@1.0.0 --registry=http://your-registry/repository/npm-local/"}),e(o,{type:"warning",closable:!1},{default:n(()=>[...s[28]||(s[28]=[a(" 需要相应权限才能删除包 ",-1)])]),_:1})]),_:1})]),_:1})])])}}}),xt=b(Ct,[["__scopeId","data-v-a5206f82"]]),Pt={class:"maven-config"},Mt={class:"config-content"},Tt={class:"content-row"},Ot={class:"content-actions"},Vt=y({__name:"MavenConfig",setup(w){const d=T(),p=u=>{window.open(`/docs/templates/${u}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(u,l)=>{const i=r("el-alert"),s=r("el-button");return m(),f("div",Pt,[e(i,{type:"info",closable:!1},{default:n(()=>[...l[1]||(l[1]=[a(" Maven 是 Java 项目的构建和依赖管理工具，用于管理项目依赖、构建生命周期和插件配置 ",-1)])]),_:1}),t("div",Mt,[t("div",Tt,[l[4]||(l[4]=t("div",{class:"content-text"},[t("h4",null,"快速开始"),t("p",null,"下载配置模板并查看完整文档，快速完成 Maven 仓库配置")],-1)),t("div",Ot,[e(s,{type:"primary",onClick:l[0]||(l[0]=o=>p("settings.xml"))},{default:n(()=>[...l[2]||(l[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 settings.xml ",-1)])]),_:1}),e(s,{onClick:c},{default:n(()=>[...l[3]||(l[3]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看完整文档 ",-1)])]),_:1})])])])])}}}),qt=b(Vt,[["__scopeId","data-v-87e57313"]]),It={class:"pypi-config"},Nt={class:"config-content"},Gt={class:"content-row"},Rt={class:"content-actions"},Yt=y({__name:"PyPIConfig",setup(w){const d=T(),p=u=>{window.open(`/docs/templates/${u}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(u,l)=>{const i=r("el-alert"),s=r("el-button");return m(),f("div",It,[e(i,{type:"info",closable:!1},{default:n(()=>[...l[1]||(l[1]=[a(" PyPI 是 Python 的包索引和依赖管理工具，用于安装、发布和管理 Python 包 ",-1)])]),_:1}),t("div",Nt,[t("div",Gt,[l[4]||(l[4]=t("div",{class:"content-text"},[t("h4",null,"快速开始"),t("p",null,"下载配置模板并查看完整文档，快速完成 PyPI 仓库配置")],-1)),t("div",Rt,[e(s,{type:"primary",onClick:l[0]||(l[0]=o=>p("pip.conf"))},{default:n(()=>[...l[2]||(l[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 pip.conf ",-1)])]),_:1}),e(s,{onClick:c},{default:n(()=>[...l[3]||(l[3]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看完整文档 ",-1)])]),_:1})])])])])}}}),Et=b(Yt,[["__scopeId","data-v-f22ecf5e"]]),Ut={class:"go-config"},Dt={class:"config-content"},At={class:"content-row"},zt={class:"content-actions"},St=y({__name:"GoConfig",setup(w){const d=T(),p=u=>{window.open(`/docs/templates/${u}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(u,l)=>{const i=r("el-alert"),s=r("el-button");return m(),f("div",Ut,[e(i,{type:"info",closable:!1},{default:n(()=>[...l[1]||(l[1]=[a(" Go modules 是 Go 语言的依赖管理系统，通过 GOPROXY 环境变量配置代理 ",-1)])]),_:1}),t("div",Dt,[t("div",At,[l[4]||(l[4]=t("div",{class:"content-text"},[t("h4",null,"快速开始"),t("p",null,"下载环境变量脚本并查看完整文档，快速完成 Go 模块仓库配置")],-1)),t("div",zt,[e(s,{type:"primary",onClick:l[0]||(l[0]=o=>p("go-env.sh"))},{default:n(()=>[...l[2]||(l[2]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载环境变量脚本 ",-1)])]),_:1}),e(s,{onClick:c},{default:n(()=>[...l[3]||(l[3]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看完整文档 ",-1)])]),_:1})])])])])}}}),Ht=b(St,[["__scopeId","data-v-45093aab"]]),Bt={class:"yum-apt-config"},Lt={class:"config-content"},ht={class:"config-section"},Qt={class:"section-row"},Ft={class:"section-actions"},Kt={class:"config-section"},Xt={class:"section-row"},jt={class:"section-actions"},Jt=y({__name:"YumAPTConfig",setup(w){const d=T(),p=l=>{window.open(`/docs/templates/${l}`,"_blank")},c=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})},u=()=>{d.push({name:"DocsViewer",params:{doc:"client-configuration.md"}})};return(l,i)=>{const s=r("el-alert"),o=r("el-button"),_=r("el-divider");return m(),f("div",Bt,[e(s,{type:"info",closable:!1},{default:n(()=>[...i[2]||(i[2]=[a(" Yum 和 APT 是 Linux 系统的包管理器，用于安装、更新和管理系统软件包 ",-1)])]),_:1}),t("div",Lt,[t("div",ht,[t("div",Qt,[i[5]||(i[5]=t("div",{class:"section-text"},[t("h4",null,"Yum 配置"),t("p",null,"下载配置文件并查看完整文档，快速完成 Yum 仓库配置")],-1)),t("div",Ft,[e(o,{type:"primary",onClick:i[0]||(i[0]=g=>p("moonlight.repo"))},{default:n(()=>[...i[3]||(i[3]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 Yum 配置 ",-1)])]),_:1}),e(o,{onClick:c},{default:n(()=>[...i[4]||(i[4]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看文档 ",-1)])]),_:1})])])]),e(_),t("div",Kt,[t("div",Xt,[i[8]||(i[8]=t("div",{class:"section-text"},[t("h4",null,"APT 配置"),t("p",null,"下载配置文件并查看完整文档，快速完成 APT 仓库配置")],-1)),t("div",jt,[e(o,{type:"primary",onClick:i[1]||(i[1]=g=>p("moonlight.list"))},{default:n(()=>[...i[6]||(i[6]=[t("i",{class:"fa-solid fa-download"},null,-1),a(" 下载 APT 配置 ",-1)])]),_:1}),e(o,{onClick:u},{default:n(()=>[...i[7]||(i[7]=[t("i",{class:"fa-solid fa-book"},null,-1),a(" 查看文档 ",-1)])]),_:1})])])])])])}}}),Wt=b(Jt,[["__scopeId","data-v-fad342d3"]]),Zt={class:"configuration-guide"},ts={class:"manager-tabs"},ss=y({__name:"ConfigurationGuide",setup(w){const d=C("npm");return(p,c)=>{const u=r("el-tab-pane"),l=r("el-tabs");return m(),f("div",Zt,[c[1]||(c[1]=t("div",{class:"section-header"},[t("h2",null,"配置指南"),t("p",{class:"section-desc"},"选择您的包管理器，查看详细的配置说明和示例")],-1)),t("div",ts,[e(l,{modelValue:d.value,"onUpdate:modelValue":c[0]||(c[0]=i=>d.value=i),type:"card",class:"guide-tabs"},{default:n(()=>[e(u,{label:"NPM",name:"npm"},{default:n(()=>[e(xt)]),_:1}),e(u,{label:"Maven",name:"maven"},{default:n(()=>[e(qt)]),_:1}),e(u,{label:"PyPI",name:"pypi"},{default:n(()=>[e(Et)]),_:1}),e(u,{label:"Go",name:"go"},{default:n(()=>[e(Ht)]),_:1}),e(u,{label:"Yum/APT",name:"yum"},{default:n(()=>[e(Wt)]),_:1})]),_:1},8,["modelValue"])])])}}}),os=b(ss,[["__scopeId","data-v-0fe2db68"]]),es={class:"faq"},ls={class:"search-box"},ns={class:"faq-content"},is={class:"faq-category"},as={class:"question-wrapper"},ds={class:"question"},rs=["innerHTML"],ps=y({__name:"FAQ",setup(w){const d=C(""),p=C(["通用问题"]),c=[{name:"通用问题",items:[{question:"如何进行认证配置？",answer:`
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
        `}]}],u=k(()=>d.value?c.map(l=>({...l,items:l.items.filter(i=>i.question.toLowerCase().includes(d.value.toLowerCase())||i.answer.toLowerCase().includes(d.value.toLowerCase()))})).filter(l=>l.items.length>0):c);return(l,i)=>{const s=r("el-input"),o=r("el-collapse-item"),_=r("el-collapse"),g=r("el-alert");return m(),f("div",es,[i[5]||(i[5]=t("div",{class:"section-header"},[t("h2",null,[t("i",{class:"fa-solid fa-circle-question"}),a(" 常见问题")]),t("p",{class:"section-desc"},"查找常见问题的解答，快速解决您遇到的问题")],-1)),t("div",ls,[e(s,{modelValue:d.value,"onUpdate:modelValue":i[0]||(i[0]=v=>d.value=v),placeholder:"搜索问题...","prefix-icon":"Search",clearable:"",size:"large"},null,8,["modelValue"])]),t("div",ns,[e(_,{modelValue:p.value,"onUpdate:modelValue":i[1]||(i[1]=v=>p.value=v),accordion:"",class:"faq-collapse"},{default:n(()=>[(m(!0),f(I,null,N(u.value,v=>(m(),E(o,{key:v.name,title:v.name,name:v.name},{default:n(()=>[t("div",is,[(m(!0),f(I,null,N(v.items,(x,q)=>(m(),f("div",{key:q,class:"faq-item"},[t("div",as,[i[2]||(i[2]=t("i",{class:"fa-solid fa-question-circle question-icon"},null,-1)),t("h4",ds,G(x.question),1)]),t("div",{class:"answer",innerHTML:x.answer},null,8,rs)]))),128))])]),_:2},1032,["title","name"]))),128))]),_:1},8,["modelValue"])]),e(g,{type:"info",closable:!1,class:"contact-alert"},{title:n(()=>[...i[3]||(i[3]=[t("i",{class:"fa-solid fa-message-circle"},null,-1),a(" 没有找到答案？ ",-1)])]),default:n(()=>[i[4]||(i[4]=t("p",null,[a("联系管理员："),t("a",{href:"mailto:admin@company.com"},"admin@company.com")],-1))]),_:1})])}}}),us=b(ps,[["__scopeId","data-v-9293d66d"]]),cs={class:"help-center"},ms={class:"page-header"},fs={class:"header-actions"},vs={class:"content-panel"},_s=y({__name:"HelpCenter",setup(w){const d=T(),p=C("quickstart"),c=()=>{d.back()};return(u,l)=>{const i=r("el-button"),s=r("el-tab-pane"),o=r("el-tabs");return m(),f("div",cs,[t("header",ms,[l[4]||(l[4]=R('<div class="header-content" data-v-95233f9c><div class="header-icon" data-v-95233f9c><i class="fa-solid fa-question-circle" data-v-95233f9c></i></div><div class="header-text" data-v-95233f9c><h2 data-v-95233f9c>帮助中心</h2><p class="header-subtitle" data-v-95233f9c>查找使用文档和常见问题解答</p></div></div>',1)),t("div",fs,[e(i,{class:"docs-btn",onClick:l[0]||(l[0]=_=>U(d).push("/admin/api-docs"))},{default:n(()=>[...l[2]||(l[2]=[t("i",{class:"fa-solid fa-book"},null,-1),t("span",null,"API 文档",-1)])]),_:1}),e(i,{class:"back-btn",onClick:c},{default:n(()=>[...l[3]||(l[3]=[t("i",{class:"fa-solid fa-arrow-left"},null,-1),t("span",null,"返回",-1)])]),_:1})])]),t("div",vs,[e(o,{modelValue:p.value,"onUpdate:modelValue":l[1]||(l[1]=_=>p.value=_),class:"help-tabs"},{default:n(()=>[e(s,{label:"快速开始",name:"quickstart"},{default:n(()=>[e(J)]),_:1}),e(s,{label:"配置指南",name:"configuration"},{default:n(()=>[e(os)]),_:1}),e(s,{label:"常见问题",name:"faq"},{default:n(()=>[e(us)]),_:1})]),_:1},8,["modelValue"])])])}}}),Cs=b(_s,[["__scopeId","data-v-95233f9c"]]);export{Cs as default};
