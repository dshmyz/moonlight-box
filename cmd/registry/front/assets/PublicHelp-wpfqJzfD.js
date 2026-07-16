import{C as H}from"./CodeBlock-f_eVi84Y.js";import{B as I,L as c,P as e,M as o,O as a,a as g,ag as i,H as n,Y as u,X as b,Z as _,F as x,ab as O,Q as R,R as D,I as S,f as m}from"./vendor-bKowFFQW.js";import{_ as U}from"./index-C7N-KONC.js";import"./elementPlus-qIFyr5Rn.js";import"./clipboard-CWwTdFKP.js";import"./mermaid-cnjGtJ55.js";const E={class:"public-help-page"},X={class:"help-content"},Q={class:"tab-content"},Z={class:"steps-container"},j={key:0,class:"step-card"},J={class:"step-actions"},K={key:1,class:"step-card"},W={class:"manager-selector"},ee={key:0,class:"config-card"},se={class:"config-title"},le=["innerHTML"],oe={class:"step-actions"},ae={key:2,class:"step-card success-card"},ie={class:"verify-command"},te={class:"success-result"},ne={class:"tab-content"},de={class:"guide-cards"},ce={class:"guide-info"},re={class:"guide-desc"},pe={class:"guide-actions"},ue={class:"tab-content"},me={class:"search-wrapper"},ve={class:"faq-content"},fe=["innerHTML"],ge=I({__name:"PublicHelp",setup(ye){const w=g("quickstart"),d=g(0),r=g("npm"),y=g(""),C=g(""),V=m(()=>({npm:"NPM",maven:"Maven",pypi:"PyPI",go:"Go",yum:"Yum/APT"})[r.value]),p=m(()=>window.location.origin),G=m(()=>window.location.host),$=m(()=>({npm:`
      <div class="code-block-wrapper">
        <pre><code>registry=${p.value}/repository/npm-virtual/</code></pre>
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
  &lt;url&gt;${p.value}/repository/maven-virtual/&lt;/url&gt;
&lt;/mirror&gt;</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>在 settings.xml 的 servers 节点中配置用户名密码</span>
      </div>
    `,pypi:`
      <div class="code-block-wrapper">
        <pre><code>[global]
index-url = ${p.value}/repository/pypi-virtual/simple/
trusted-host = ${G.value}</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>使用 pip config 设置，或在安装时使用 --index-url 参数</span>
      </div>
    `,go:`
      <div class="code-block-wrapper">
        <pre><code>export GOPROXY=${p.value}/go,https://proxy.golang.org,direct
export GOSUMDB=off</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>Go 模块代理无需额外认证</span>
      </div>
    `,yum:`
      <div class="code-block-wrapper">
        <pre><code># CentOS/RHEL (Yum)
cat > /etc/yum.repos.d/moonlight.repo << EOF
[moonlight]
name=Moonlight Repository
baseurl=${p.value}/yum/
enabled=1
gpgcheck=0
EOF

# Debian/Ubuntu (APT)
echo "deb [trusted=yes] ${p.value}/apt/ /" > /etc/apt/sources.list.d/moonlight.list
apt-get update</code></pre>
      </div>
      <div class="config-note">
        <i class="fa-solid fa-info-circle"></i>
        <span>根据您的操作系统选择对应的配置方式</span>
      </div>
    `})[r.value]),h=m(()=>({npm:"npm config list",maven:"mvn help:effective-settings",pypi:"pip config list",go:"go env GOPROXY",yum:"yum repolist | grep moonlight"})[r.value]),B=[{name:"npm",title:"NPM 配置",description:"配置 npm 使用私有仓库",file:".npmrc",icon:"fa-solid fa-box",color:"linear-gradient(135deg, #CB3837 0%, #911F27 100%)"},{name:"maven",title:"Maven 配置",description:"配置 Maven 镜像和认证",file:"settings.xml",icon:"fa-solid fa-file-xml",color:"linear-gradient(135deg, #C71A36 0%, #8B0000 100%)"},{name:"pypi",title:"PyPI 配置",description:"配置 pip 使用私有索引",file:"pip.conf",icon:"fa-solid fa-code",color:"linear-gradient(135deg, #3776AB 0%, #2E68A6 100%)"},{name:"go",title:"Go 配置",description:"配置 GOPROXY 环境变量",file:"go-env.sh",icon:"fa-solid fa-file-code",color:"linear-gradient(135deg, #00ADD8 0%, #007D9C 100%)"},{name:"yum",title:"Yum/APT 配置",description:"配置 Yum 或 APT 使用私有仓库",file:"repo.conf",icon:"fa-solid fa-server",color:"linear-gradient(135deg, #FB8C00 0%, #E65100 100%)"}],M=[{name:"auth",title:"如何进行认证配置？",content:`
      <p>当前版本使用用户名密码进行认证：</p>
      <ul>
        <li><strong>NPM/PyPI：</strong>使用您的账号密码进行认证</li>
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
          <code>npm publish --registry=http://your-registry/repository/npm-local/</code>
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
    `}],T=m(()=>{if(!y.value)return M;const t=y.value.toLowerCase();return M.filter(s=>s.title.toLowerCase().includes(t)||s.content.toLowerCase().includes(t))}),q=t=>{window.open(`/docs/templates/${t}`,"_blank")};return(t,s)=>{const k=i("el-step"),Y=i("el-steps"),v=i("el-button"),f=i("el-radio-button"),L=i("el-radio-group"),P=i("el-tab-pane"),N=i("el-input"),A=i("el-collapse-item"),z=i("el-collapse"),F=i("el-tabs");return n(),c("div",E,[s[32]||(s[32]=e("div",{class:"help-hero"},[e("div",{class:"hero-content"},[e("div",{class:"hero-icon"},[e("i",{class:"fa-solid fa-book-open"})]),e("h1",null,"帮助中心"),e("p",null,"快速配置您的客户端，开始使用 Moonlight Box")])],-1)),e("div",X,[o(F,{modelValue:w.value,"onUpdate:modelValue":s[7]||(s[7]=l=>w.value=l),type:"card",class:"help-tabs"},{default:a(()=>[o(P,{name:"quickstart"},{label:a(()=>[...s[8]||(s[8]=[e("span",{class:"tab-label"},[e("i",{class:"fa-solid fa-rocket"}),e("span",null,"快速开始")],-1)])]),default:a(()=>[e("div",Q,[e("div",Z,[o(Y,{active:d.value,"finish-status":"success","align-center":"",class:"steps-wrapper"},{default:a(()=>[o(k,{title:"配置认证",description:"设置用户名密码"}),o(k,{title:"配置客户端",description:"选择包管理器并配置"}),o(k,{title:"开始使用",description:"验证配置并使用"})]),_:1},8,["active"])]),d.value===0?(n(),c("div",j,[s[10]||(s[10]=e("div",{class:"step-header"},[e("div",{class:"step-number"},"01"),e("div",null,[e("h3",null,"配置认证信息"),e("p",{class:"step-desc"},"根据您使用的包管理器，配置相应的认证方式")])],-1)),s[11]||(s[11]=e("div",{class:"auth-timeline"},[e("div",{class:"timeline-item"},[e("div",{class:"timeline-icon"},[e("i",{class:"fa-solid fa-key"})]),e("div",{class:"timeline-content"},[e("strong",null,"NPM/PyPI"),e("p",null,"使用您的账号密码进行认证")])]),e("div",{class:"timeline-item"},[e("div",{class:"timeline-icon"},[e("i",{class:"fa-solid fa-file-code"})]),e("div",{class:"timeline-content"},[e("strong",null,"Maven"),e("p",null,"在 settings.xml 中配置 server 信息")])]),e("div",{class:"timeline-item"},[e("div",{class:"timeline-icon"},[e("i",{class:"fa-solid fa-globe"})]),e("div",{class:"timeline-content"},[e("strong",null,"Go"),e("p",null,"通过 GOPROXY 配置，无需额外认证")])])],-1)),s[12]||(s[12]=e("div",{class:"info-alert"},[e("i",{class:"fa-solid fa-info-circle"}),e("span",null,"访问令牌功能即将推出，当前版本请使用用户名密码进行认证")],-1)),e("div",J,[o(v,{type:"primary",size:"large",onClick:s[0]||(s[0]=l=>d.value=1)},{default:a(()=>[...s[9]||(s[9]=[e("i",{class:"fa-solid fa-arrow-right"},null,-1),u(" 下一步 ",-1)])]),_:1})])])):b("",!0),d.value===1?(n(),c("div",K,[s[21]||(s[21]=e("div",{class:"step-header"},[e("div",{class:"step-number"},"02"),e("div",null,[e("h3",null,"配置客户端"),e("p",{class:"step-desc"},"选择您的包管理器并配置")])],-1)),e("div",W,[o(L,{modelValue:r.value,"onUpdate:modelValue":s[1]||(s[1]=l=>r.value=l),class:"manager-group"},{default:a(()=>[o(f,{value:"npm",class:"manager-option"},{default:a(()=>[...s[13]||(s[13]=[e("i",{class:"fa-solid fa-box"},null,-1),e("span",null,"NPM",-1)])]),_:1}),o(f,{value:"maven",class:"manager-option"},{default:a(()=>[...s[14]||(s[14]=[e("i",{class:"fa-solid fa-box"},null,-1),e("span",null,"Maven",-1)])]),_:1}),o(f,{value:"pypi",class:"manager-option"},{default:a(()=>[...s[15]||(s[15]=[e("i",{class:"fa-solid fa-code"},null,-1),e("span",null,"PyPI",-1)])]),_:1}),o(f,{value:"go",class:"manager-option"},{default:a(()=>[...s[16]||(s[16]=[e("i",{class:"fa-brands fa-golang"},null,-1),e("span",null,"Go",-1)])]),_:1}),o(f,{value:"yum",class:"manager-option"},{default:a(()=>[...s[17]||(s[17]=[e("i",{class:"fa-solid fa-server"},null,-1),e("span",null,"Yum/APT",-1)])]),_:1})]),_:1},8,["modelValue"])]),r.value?(n(),c("div",ee,[e("h4",se,[s[18]||(s[18]=e("i",{class:"fa-solid fa-code"},null,-1)),u(" "+_(V.value)+" 配置示例 ",1)]),e("div",{class:"config-content",innerHTML:$.value},null,8,le)])):b("",!0),e("div",oe,[o(v,{size:"large",onClick:s[2]||(s[2]=l=>d.value=0)},{default:a(()=>[...s[19]||(s[19]=[e("i",{class:"fa-solid fa-arrow-left"},null,-1),u(" 上一步 ",-1)])]),_:1}),o(v,{type:"primary",size:"large",onClick:s[3]||(s[3]=l=>d.value=2)},{default:a(()=>[...s[20]||(s[20]=[e("i",{class:"fa-solid fa-arrow-right"},null,-1),u(" 下一步 ",-1)])]),_:1})])])):b("",!0),d.value===2?(n(),c("div",ae,[s[26]||(s[26]=e("div",{class:"step-header"},[e("div",{class:"step-number success"},"03"),e("div",null,[e("h3",null,"验证配置"),e("p",{class:"step-desc"},"运行以下命令验证您的配置")])],-1)),e("div",ie,[o(H,{code:h.value},null,8,["code"])]),e("div",te,[s[23]||(s[23]=e("div",{class:"success-icon"},[e("i",{class:"fa-solid fa-check-circle"})],-1)),s[24]||(s[24]=e("h4",null,"配置完成！",-1)),s[25]||(s[25]=e("p",null,"您现在可以开始使用仓库了",-1)),o(v,{type:"primary",size:"large",onClick:s[4]||(s[4]=l=>t.$router.push("/"))},{default:a(()=>[...s[22]||(s[22]=[e("i",{class:"fa-solid fa-rocket"},null,-1),u(" 浏览仓库 ",-1)])]),_:1})])])):b("",!0)])]),_:1}),o(P,{name:"guide"},{label:a(()=>[...s[27]||(s[27]=[e("span",{class:"tab-label"},[e("i",{class:"fa-solid fa-book"}),e("span",null,"配置指南")],-1)])]),default:a(()=>[e("div",ne,[s[29]||(s[29]=e("div",{class:"guide-intro"},[e("i",{class:"fa-solid fa-file-text"}),e("div",null,[e("h3",null,"详细配置说明"),e("p",null,"根据您使用的包管理器，查看对应的配置指南")])],-1)),e("div",de,[(n(),c(x,null,O(B,l=>e("div",{class:"guide-card",key:l.name},[e("div",{class:"guide-icon",style:R({background:l.color})},[e("i",{class:D(l.icon)},null,2)],4),e("div",ce,[e("h4",null,_(l.title),1),e("p",re,_(l.description),1),e("code",null,_(l.file),1)]),e("div",pe,[o(v,{size:"small",type:"primary",onClick:be=>q(l.file)},{default:a(()=>[...s[28]||(s[28]=[e("i",{class:"fa-solid fa-download"},null,-1),u(" 下载模板 ",-1)])]),_:1},8,["onClick"])])])),64))])])]),_:1}),o(P,{name:"faq"},{label:a(()=>[...s[30]||(s[30]=[e("span",{class:"tab-label"},[e("i",{class:"fa-solid fa-circle-question"}),e("span",null,"常见问题")],-1)])]),default:a(()=>[e("div",ue,[e("div",me,[o(N,{modelValue:y.value,"onUpdate:modelValue":s[5]||(s[5]=l=>y.value=l),placeholder:"搜索问题...","prefix-icon":"Search",clearable:"",size:"large",class:"faq-search"},null,8,["modelValue"])]),o(z,{modelValue:C.value,"onUpdate:modelValue":s[6]||(s[6]=l=>C.value=l),accordion:"",class:"faq-collapse"},{default:a(()=>[(n(!0),c(x,null,O(T.value,l=>(n(),S(A,{key:l.name,title:l.title,name:l.name,class:"faq-item"},{default:a(()=>[e("div",ve,[e("div",{innerHTML:l.content},null,8,fe)])]),_:2},1032,["title","name"]))),128))]),_:1},8,["modelValue"]),s[31]||(s[31]=e("div",{class:"contact-section"},[e("i",{class:"fa-solid fa-message-circle"}),e("div",null,[e("p",null,"没有找到答案？"),e("a",{href:"mailto:admin@company.com",class:"contact-link"}," 联系管理员：admin@company.com ")])],-1))])]),_:1})]),_:1},8,["modelValue"])])])}}}),xe=U(ge,[["__scopeId","data-v-a4195205"]]);export{xe as default};
