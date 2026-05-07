import{C as R}from"./CodeBlock-B7I6oCdn.js";import{d as D,c as r,b as e,a as o,w as n,r as g,i as t,o as a,f as m,y as b,t as _,F as G,n as M,x as H,v as I,k as S,A as v,_ as U}from"./index-DBxFuNM9.js";const E={class:"public-help-page"},X={class:"help-content"},j={class:"tab-content"},Q={class:"steps-container"},J={key:0,class:"step-card"},K={class:"step-actions"},W={key:1,class:"step-card"},Z={class:"manager-selector"},ee={key:0,class:"config-card"},le={class:"config-title"},se=["innerHTML"],oe={class:"step-actions"},ne={key:2,class:"step-card success-card"},te={class:"verify-command"},ie={class:"success-result"},ae={class:"tab-content"},de={class:"guide-cards"},ce={class:"guide-info"},re={class:"guide-desc"},pe={class:"guide-actions"},ue={class:"tab-content"},me={class:"search-wrapper"},ve={class:"faq-content"},fe=["innerHTML"],ge=D({__name:"PublicHelp",setup(ye){const w=g("quickstart"),d=g(0),p=g("npm"),y=g(""),C=g(""),h=v(()=>({npm:"NPM",maven:"Maven",pypi:"PyPI",go:"Go",nuget:"NuGet",yum:"Yum/APT"})[p.value]),c=v(()=>window.location.origin),N=v(()=>window.location.host),O=v(()=>({npm:`
      <div class="code-block-wrapper">
        <pre><code>registry=${c.value}/repo/npm-virtual/</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>运行 <code>npm login</code> 并输入您的账号密码进行认证</span>
      </div>
    `,maven:`
      <div class="code-block-wrapper">
        <pre><code>&lt;mirror&gt;
  &lt;id&gt;moonlight&lt;/id&gt;
  &lt;mirrorOf&gt;central&lt;/mirrorOf&gt;
  &lt;url&gt;${c.value}/repo/maven-virtual/&lt;/url&gt;
&lt;/mirror&gt;</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>在 settings.xml 的 servers 节点中配置用户名密码</span>
      </div>
    `,pypi:`
      <div class="code-block-wrapper">
        <pre><code>[global]
index-url = ${c.value}/repo/pypi-virtual/simple/
trusted-host = ${N.value}</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>使用 pip config 设置，或在安装时使用 --index-url 参数</span>
      </div>
    `,go:`
      <div class="code-block-wrapper">
        <pre><code>export GOPROXY=${c.value}/go,https://proxy.golang.org,direct
export GOSUMDB=off</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>Go 模块代理无需额外认证</span>
      </div>
    `,nuget:`
      <div class="code-block-wrapper">
        <pre><code>nuget sources add -name moonlight \\
  -source ${c.value}/nuget/v3/index.json</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>使用 nuget setapikey 或在 NuGet.Config 中配置认证</span>
      </div>
    `,yum:`
      <div class="code-block-wrapper">
        <pre><code># CentOS/RHEL (Yum)
cat > /etc/yum.repos.d/moonlight.repo << EOF
[moonlight]
name=Moonlight Repository
baseurl=${c.value}/yum/
enabled=1
gpgcheck=0
EOF

# Debian/Ubuntu (APT)
echo "deb [trusted=yes] ${c.value}/apt/ /" > /etc/apt/sources.list.d/moonlight.list
apt-get update</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>根据您的操作系统选择对应的配置方式</span>
      </div>
    `})[p.value]),V=v(()=>({npm:"npm config list",maven:"mvn help:effective-settings",pypi:"pip config list",go:"go env GOPROXY",nuget:"nuget sources list",yum:"yum repolist | grep moonlight"})[p.value]),$=[{name:"npm",title:"NPM 配置",description:"配置 npm 使用私有仓库",file:".npmrc",icon:"fa-solid fa-box",color:"linear-gradient(135deg, #CB3837 0%, #911F27 100%)"},{name:"maven",title:"Maven 配置",description:"配置 Maven 镜像和认证",file:"settings.xml",icon:"fa-solid fa-file-xml",color:"linear-gradient(135deg, #C71A36 0%, #8B0000 100%)"},{name:"pypi",title:"PyPI 配置",description:"配置 pip 使用私有索引",file:"pip.conf",icon:"fa-solid fa-code",color:"linear-gradient(135deg, #3776AB 0%, #2E68A6 100%)"},{name:"go",title:"Go 配置",description:"配置 GOPROXY 环境变量",file:"go-env.sh",icon:"fa-solid fa-file-code",color:"linear-gradient(135deg, #00ADD8 0%, #007D9C 100%)"},{name:"nuget",title:"NuGet 配置",description:"配置 NuGet 包源",file:"NuGet.Config",icon:"fa-solid fa-cube",color:"linear-gradient(135deg, #004880 0%, #003366 100%)"},{name:"yum",title:"Yum/APT 配置",description:"配置 Yum 或 APT 使用私有仓库",file:"repo.conf",icon:"fa-solid fa-server",color:"linear-gradient(135deg, #FB8C00 0%, #E65100 100%)"}],x=[{name:"auth",title:"如何进行认证配置？",content:`
      <p>当前版本使用用户名密码进行认证：</p>
      <ul>
        <li><strong>NPM/PyPI/NuGet：</strong>使用您的账号密码进行认证</li>
        <li><strong>Maven：</strong>在 settings.xml 中配置 server 信息</li>
        <li><strong>Go：</strong>通过 GOPROXY 配置，无需额外认证</li>
      </ul>
      <div class="info-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>访问令牌功能即将推出</span>
      </div>
    `},{name:"npm-adduser",title:"npm adduser 不工作怎么办？",content:`
      <p>当前版本暂不支持 <code>npm adduser</code> 命令。</p>
      <p>请手动配置 <code>.npmrc</code> 文件，使用用户名密码认证。</p>
    `},{name:"publish",title:"如何发布包？",content:`
      <div class="command-list">
        <div class="command-item">
          <span class="command-label">NPM:</span>
          <code>npm publish --registry=http://your-registry/repo/npm-local/</code>
        </div>
        <div class="command-item">
          <span class="command-label">Maven:</span>
          <code>mvn clean deploy</code>
        </div>
        <div class="command-item">
          <span class="command-label">PyPI:</span>
          <code>twine upload --repository-url http://your-registry/pypi/upload/ dist/*</code>
        </div>
      </div>
    `},{name:"go-checksum",title:"Go get 报错 checksum mismatch？",content:`
      <p>当前版本不支持校验和数据库，请禁用：</p>
      <div class="code-block-wrapper small">
        <pre><code>export GOSUMDB=off</code></pre>
      </div>
    `}],T=v(()=>{if(!y.value)return x;const i=y.value.toLowerCase();return x.filter(l=>l.title.toLowerCase().includes(i)||l.content.toLowerCase().includes(i))}),B=i=>{window.open(`/docs/templates/${i}`,"_blank")};return(i,l)=>{const k=t("el-step"),q=t("el-steps"),f=t("el-button"),u=t("el-radio-button"),A=t("el-radio-group"),P=t("el-tab-pane"),Y=t("el-input"),L=t("el-collapse-item"),z=t("el-collapse"),F=t("el-tabs");return a(),r("div",E,[l[30]||(l[30]=e("div",{class:"help-hero"},[e("div",{class:"hero-content"},[e("div",{class:"hero-icon"},[e("i",{class:"fa-solid fa-book-open"})]),e("h1",null,"帮助中心"),e("p",null,"快速配置您的客户端，开始使用 Moonlight Registry")])],-1)),e("div",X,[o(F,{modelValue:w.value,"onUpdate:modelValue":l[7]||(l[7]=s=>w.value=s),type:"card",class:"help-tabs"},{default:n(()=>[o(P,{label:"🚀 快速开始",name:"quickstart"},{default:n(()=>[e("div",j,[e("div",Q,[o(q,{active:d.value,"finish-status":"success","align-center":"",class:"steps-wrapper"},{default:n(()=>[o(k,{title:"配置认证",description:"设置用户名密码"}),o(k,{title:"配置客户端",description:"选择包管理器并配置"}),o(k,{title:"开始使用",description:"验证配置并使用"})]),_:1},8,["active"])]),d.value===0?(a(),r("div",J,[l[9]||(l[9]=e("div",{class:"step-header"},[e("div",{class:"step-number"},"01"),e("div",null,[e("h3",null,"配置认证信息"),e("p",{class:"step-desc"},"根据您使用的包管理器，配置相应的认证方式")])],-1)),l[10]||(l[10]=e("div",{class:"auth-timeline"},[e("div",{class:"timeline-item"},[e("div",{class:"timeline-icon"},[e("i",{class:"fa-solid fa-key"})]),e("div",{class:"timeline-content"},[e("strong",null,"NPM/PyPI/NuGet"),e("p",null,"使用您的账号密码进行认证")])]),e("div",{class:"timeline-item"},[e("div",{class:"timeline-icon"},[e("i",{class:"fa-solid fa-file-code"})]),e("div",{class:"timeline-content"},[e("strong",null,"Maven"),e("p",null,"在 settings.xml 中配置 server 信息")])]),e("div",{class:"timeline-item"},[e("div",{class:"timeline-icon"},[e("i",{class:"fa-solid fa-globe"})]),e("div",{class:"timeline-content"},[e("strong",null,"Go"),e("p",null,"通过 GOPROXY 配置，无需额外认证")])])],-1)),l[11]||(l[11]=e("div",{class:"info-alert"},[e("i",{class:"fa-solid fa-info-circle"}),e("span",null,"访问令牌功能即将推出，当前版本请使用用户名密码进行认证")],-1)),e("div",K,[o(f,{type:"primary",size:"large",onClick:l[0]||(l[0]=s=>d.value=1)},{default:n(()=>[...l[8]||(l[8]=[e("i",{class:"fa-solid fa-arrow-right"},null,-1),m(" 下一步 ",-1)])]),_:1})])])):b("",!0),d.value===1?(a(),r("div",W,[l[21]||(l[21]=e("div",{class:"step-header"},[e("div",{class:"step-number"},"02"),e("div",null,[e("h3",null,"配置客户端"),e("p",{class:"step-desc"},"选择您的包管理器并配置")])],-1)),e("div",Z,[o(A,{modelValue:p.value,"onUpdate:modelValue":l[1]||(l[1]=s=>p.value=s),class:"manager-group"},{default:n(()=>[o(u,{label:"npm",class:"manager-option"},{default:n(()=>[...l[12]||(l[12]=[e("i",{class:"fa-solid fa-box"},null,-1),e("span",null,"NPM",-1)])]),_:1}),o(u,{label:"maven",class:"manager-option"},{default:n(()=>[...l[13]||(l[13]=[e("i",{class:"fa-solid fa-box"},null,-1),e("span",null,"Maven",-1)])]),_:1}),o(u,{label:"pypi",class:"manager-option"},{default:n(()=>[...l[14]||(l[14]=[e("i",{class:"fa-solid fa-code"},null,-1),e("span",null,"PyPI",-1)])]),_:1}),o(u,{label:"go",class:"manager-option"},{default:n(()=>[...l[15]||(l[15]=[e("i",{class:"fa-brands fa-golang"},null,-1),e("span",null,"Go",-1)])]),_:1}),o(u,{label:"nuget",class:"manager-option"},{default:n(()=>[...l[16]||(l[16]=[e("i",{class:"fa-solid fa-cube"},null,-1),e("span",null,"NuGet",-1)])]),_:1}),o(u,{label:"yum",class:"manager-option"},{default:n(()=>[...l[17]||(l[17]=[e("i",{class:"fa-solid fa-server"},null,-1),e("span",null,"Yum/APT",-1)])]),_:1})]),_:1},8,["modelValue"])]),p.value?(a(),r("div",ee,[e("h4",le,[l[18]||(l[18]=e("i",{class:"fa-solid fa-code"},null,-1)),m(" "+_(h.value)+" 配置示例 ",1)]),e("div",{class:"config-content",innerHTML:O.value},null,8,se)])):b("",!0),e("div",oe,[o(f,{size:"large",onClick:l[2]||(l[2]=s=>d.value=0)},{default:n(()=>[...l[19]||(l[19]=[e("i",{class:"fa-solid fa-arrow-left"},null,-1),m(" 上一步 ",-1)])]),_:1}),o(f,{type:"primary",size:"large",onClick:l[3]||(l[3]=s=>d.value=2)},{default:n(()=>[...l[20]||(l[20]=[e("i",{class:"fa-solid fa-arrow-right"},null,-1),m(" 下一步 ",-1)])]),_:1})])])):b("",!0),d.value===2?(a(),r("div",ne,[l[26]||(l[26]=e("div",{class:"step-header"},[e("div",{class:"step-number success"},"03"),e("div",null,[e("h3",null,"验证配置"),e("p",{class:"step-desc"},"运行以下命令验证您的配置")])],-1)),e("div",te,[o(R,{code:V.value},null,8,["code"])]),e("div",ie,[l[23]||(l[23]=e("div",{class:"success-icon"},[e("i",{class:"fa-solid fa-check-circle"})],-1)),l[24]||(l[24]=e("h4",null,"配置完成！",-1)),l[25]||(l[25]=e("p",null,"您现在可以开始使用仓库了",-1)),o(f,{type:"primary",size:"large",onClick:l[4]||(l[4]=s=>i.$router.push("/"))},{default:n(()=>[...l[22]||(l[22]=[e("i",{class:"fa-solid fa-rocket"},null,-1),m(" 浏览仓库 ",-1)])]),_:1})])])):b("",!0)])]),_:1}),o(P,{label:"📖 配置指南",name:"guide"},{default:n(()=>[e("div",ae,[l[28]||(l[28]=e("div",{class:"guide-intro"},[e("i",{class:"fa-solid fa-file-text"}),e("div",null,[e("h3",null,"详细配置说明"),e("p",null,"根据您使用的包管理器，查看对应的配置指南")])],-1)),e("div",de,[(a(),r(G,null,M($,s=>e("div",{class:"guide-card",key:s.name},[e("div",{class:"guide-icon",style:H({background:s.color})},[e("i",{class:I(s.icon)},null,2)],4),e("div",ce,[e("h4",null,_(s.title),1),e("p",re,_(s.description),1),e("code",null,_(s.file),1)]),e("div",pe,[o(f,{size:"small",type:"primary",onClick:be=>B(s.file)},{default:n(()=>[...l[27]||(l[27]=[e("i",{class:"fa-solid fa-download"},null,-1),m(" 下载模板 ",-1)])]),_:1},8,["onClick"])])])),64))])])]),_:1}),o(P,{label:"❓ 常见问题",name:"faq"},{default:n(()=>[e("div",ue,[e("div",me,[o(Y,{modelValue:y.value,"onUpdate:modelValue":l[5]||(l[5]=s=>y.value=s),placeholder:"搜索问题...","prefix-icon":"Search",clearable:"",size:"large",class:"faq-search"},null,8,["modelValue"])]),o(z,{modelValue:C.value,"onUpdate:modelValue":l[6]||(l[6]=s=>C.value=s),accordion:"",class:"faq-collapse"},{default:n(()=>[(a(!0),r(G,null,M(T.value,s=>(a(),S(L,{key:s.name,title:s.title,name:s.name,class:"faq-item"},{default:n(()=>[e("div",ve,[e("div",{innerHTML:s.content},null,8,fe)])]),_:2},1032,["title","name"]))),128))]),_:1},8,["modelValue"]),l[29]||(l[29]=e("div",{class:"contact-section"},[e("i",{class:"fa-solid fa-message-circle"}),e("div",null,[e("p",null,"没有找到答案？"),e("a",{href:"mailto:admin@company.com",class:"contact-link"}," 联系管理员：admin@company.com ")])],-1))])]),_:1})]),_:1},8,["modelValue"])])])}}}),Pe=U(ge,[["__scopeId","data-v-07ef35ab"]]);export{Pe as default};
