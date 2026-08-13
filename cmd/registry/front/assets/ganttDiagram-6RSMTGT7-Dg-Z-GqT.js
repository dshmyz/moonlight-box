import{g as le,s as ue,t as de,q as fe,a as he,b as me,_ as c,c as rt,d as mt,aB as ke,aC as ye,aD as ge,e as ve,Q as pe,aE as Te,l as et,aF as xe,aG as Rt,aH as Vt,aI as be,aJ as we,aK as _e,aL as De,aM as Ce,aN as Se,aO as Ee,aP as Nt,aQ as Bt,aR as zt,aS as Ht,aT as jt,aU as Ie,k as Me,j as Ae,A as Fe,u as $e}from"./mermaid-BuQoSngf.js";import{q as Xt,s as Kt,d as R,t as Le,w as Oe}from"./elementPlus-BbzhnP1m.js";import"./vendor-bKowFFQW.js";var Jt={exports:{}};(function(t,n){(function(r,i){t.exports=i()})(Xt,function(){var r="day";return function(i,a,k){var T=function(A){return A.add(4-A.isoWeekday(),r)},C=a.prototype;C.isoWeekYear=function(){return T(this).year()},C.isoWeek=function(A){if(!this.$utils().u(A))return this.add(7*(A-this.isoWeek()),r);var D,$,L,O,W=T(this),_=(D=this.isoWeekYear(),$=this.$u,L=($?k.utc:k)().year(D).startOf("year"),O=4-L.isoWeekday(),L.isoWeekday()>4&&(O+=7),L.add(O,r));return W.diff(_,"week")+1},C.isoWeekday=function(A){return this.$utils().u(A)?this.day()||7:this.day(this.day()%7?A:A-7)};var Y=C.startOf;C.startOf=function(A,D){var $=this.$utils(),L=!!$.u(D)||D;return $.p(A)==="isoweek"?L?this.date(this.date()-(this.isoWeekday()-1)).startOf("day"):this.date(this.date()-1-(this.isoWeekday()-1)+7).endOf("day"):Y.bind(this)(A,D)}}})})(Jt);var We=Jt.exports;const Ye=Kt(We);var Qt={exports:{}};(function(t,n){(function(r,i){t.exports=i()})(Xt,function(){var r,i,a=1e3,k=6e4,T=36e5,C=864e5,Y=/\[([^\]]+)]|Y{1,4}|M{1,4}|D{1,2}|d{1,4}|H{1,2}|h{1,2}|a|A|m{1,2}|s{1,2}|Z{1,2}|SSS/g,A=31536e6,D=2628e6,$=/^(-|\+)?P(?:([-+]?[0-9,.]*)Y)?(?:([-+]?[0-9,.]*)M)?(?:([-+]?[0-9,.]*)W)?(?:([-+]?[0-9,.]*)D)?(?:T(?:([-+]?[0-9,.]*)H)?(?:([-+]?[0-9,.]*)M)?(?:([-+]?[0-9,.]*)S)?)?$/,L={years:A,months:D,days:C,hours:T,minutes:k,seconds:a,milliseconds:1,weeks:6048e5},O=function(I){return I instanceof G},W=function(I,p,f){return new G(I,f,p.$l)},_=function(I){return i.p(I)+"s"},J=function(I){return I<0},B=function(I){return J(I)?Math.ceil(I):Math.floor(I)},Z=function(I){return Math.abs(I)},H=function(I,p){return I?J(I)?{negative:!0,format:""+Z(I)+p}:{negative:!1,format:""+I+p}:{negative:!1,format:""}},G=function(){function I(f,u,y){var g=this;if(this.$d={},this.$l=y,f===void 0&&(this.$ms=0,this.parseFromMilliseconds()),u)return W(f*L[_(u)],this);if(typeof f=="number")return this.$ms=f,this.parseFromMilliseconds(),this;if(typeof f=="object")return Object.keys(f).forEach(function(o){g.$d[_(o)]=f[o]}),this.calMilliseconds(),this;if(typeof f=="string"){var v=f.match($);if(v){var m=v.slice(2).map(function(o){return o!=null?Number(o):0});return this.$d.years=m[0],this.$d.months=m[1],this.$d.weeks=m[2],this.$d.days=m[3],this.$d.hours=m[4],this.$d.minutes=m[5],this.$d.seconds=m[6],this.calMilliseconds(),this}}return this}var p=I.prototype;return p.calMilliseconds=function(){var f=this;this.$ms=Object.keys(this.$d).reduce(function(u,y){return u+(f.$d[y]||0)*L[y]},0)},p.parseFromMilliseconds=function(){var f=this.$ms;this.$d.years=B(f/A),f%=A,this.$d.months=B(f/D),f%=D,this.$d.days=B(f/C),f%=C,this.$d.hours=B(f/T),f%=T,this.$d.minutes=B(f/k),f%=k,this.$d.seconds=B(f/a),f%=a,this.$d.milliseconds=f},p.toISOString=function(){var f=H(this.$d.years,"Y"),u=H(this.$d.months,"M"),y=+this.$d.days||0;this.$d.weeks&&(y+=7*this.$d.weeks);var g=H(y,"D"),v=H(this.$d.hours,"H"),m=H(this.$d.minutes,"M"),o=this.$d.seconds||0;this.$d.milliseconds&&(o+=this.$d.milliseconds/1e3,o=Math.round(1e3*o)/1e3);var l=H(o,"S"),h=f.negative||u.negative||g.negative||v.negative||m.negative||l.negative,d=v.format||m.format||l.format?"T":"",x=(h?"-":"")+"P"+f.format+u.format+g.format+d+v.format+m.format+l.format;return x==="P"||x==="-P"?"P0D":x},p.toJSON=function(){return this.toISOString()},p.format=function(f){var u=f||"YYYY-MM-DDTHH:mm:ss",y={Y:this.$d.years,YY:i.s(this.$d.years,2,"0"),YYYY:i.s(this.$d.years,4,"0"),M:this.$d.months,MM:i.s(this.$d.months,2,"0"),D:this.$d.days,DD:i.s(this.$d.days,2,"0"),H:this.$d.hours,HH:i.s(this.$d.hours,2,"0"),m:this.$d.minutes,mm:i.s(this.$d.minutes,2,"0"),s:this.$d.seconds,ss:i.s(this.$d.seconds,2,"0"),SSS:i.s(this.$d.milliseconds,3,"0")};return u.replace(Y,function(g,v){return v||String(y[g])})},p.as=function(f){return this.$ms/L[_(f)]},p.get=function(f){var u=this.$ms,y=_(f);return y==="milliseconds"?u%=1e3:u=y==="weeks"?B(u/L[y]):this.$d[y],u||0},p.add=function(f,u,y){var g;return g=u?f*L[_(u)]:O(f)?f.$ms:W(f,this).$ms,W(this.$ms+g*(y?-1:1),this)},p.subtract=function(f,u){return this.add(f,u,!0)},p.locale=function(f){var u=this.clone();return u.$l=f,u},p.clone=function(){return W(this.$ms,this)},p.humanize=function(f){return r().add(this.$ms,"ms").locale(this.$l).fromNow(!f)},p.valueOf=function(){return this.asMilliseconds()},p.milliseconds=function(){return this.get("milliseconds")},p.asMilliseconds=function(){return this.as("milliseconds")},p.seconds=function(){return this.get("seconds")},p.asSeconds=function(){return this.as("seconds")},p.minutes=function(){return this.get("minutes")},p.asMinutes=function(){return this.as("minutes")},p.hours=function(){return this.get("hours")},p.asHours=function(){return this.as("hours")},p.days=function(){return this.get("days")},p.asDays=function(){return this.as("days")},p.weeks=function(){return this.get("weeks")},p.asWeeks=function(){return this.as("weeks")},p.months=function(){return this.get("months")},p.asMonths=function(){return this.as("months")},p.years=function(){return this.get("years")},p.asYears=function(){return this.as("years")},I}(),Q=function(I,p,f){return I.add(p.years()*f,"y").add(p.months()*f,"M").add(p.days()*f,"d").add(p.hours()*f,"h").add(p.minutes()*f,"m").add(p.seconds()*f,"s").add(p.milliseconds()*f,"ms")};return function(I,p,f){r=f,i=f().$utils(),f.duration=function(g,v){var m=f.locale();return W(g,{$l:m},v)},f.isDuration=O;var u=p.prototype.add,y=p.prototype.subtract;p.prototype.add=function(g,v){return O(g)?Q(this,g,1):u.bind(this)(g,v)},p.prototype.subtract=function(g,v){return O(g)?Q(this,g,-1):y.bind(this)(g,v)}}})})(Qt);var Pe=Qt.exports;const Re=Kt(Pe);var wt=function(){var t=c(function(m,o,l,h){for(l=l||{},h=m.length;h--;l[m[h]]=o);return l},"o"),n=[6,8,10,12,13,14,15,16,17,18,20,21,22,23,24,25,26,27,28,29,30,31,33,35,36,38,40],r=[1,26],i=[1,27],a=[1,28],k=[1,29],T=[1,30],C=[1,31],Y=[1,32],A=[1,33],D=[1,34],$=[1,9],L=[1,10],O=[1,11],W=[1,12],_=[1,13],J=[1,14],B=[1,15],Z=[1,16],H=[1,19],G=[1,20],Q=[1,21],I=[1,22],p=[1,23],f=[1,25],u=[1,35],y={trace:c(function(){},"trace"),yy:{},symbols_:{error:2,start:3,gantt:4,document:5,EOF:6,line:7,SPACE:8,statement:9,NL:10,weekday:11,weekday_monday:12,weekday_tuesday:13,weekday_wednesday:14,weekday_thursday:15,weekday_friday:16,weekday_saturday:17,weekday_sunday:18,weekend:19,weekend_friday:20,weekend_saturday:21,dateFormat:22,inclusiveEndDates:23,topAxis:24,axisFormat:25,tickInterval:26,excludes:27,includes:28,todayMarker:29,title:30,acc_title:31,acc_title_value:32,acc_descr:33,acc_descr_value:34,acc_descr_multiline_value:35,section:36,clickStatement:37,taskTxt:38,taskData:39,click:40,callbackname:41,callbackargs:42,href:43,clickStatementDebug:44,$accept:0,$end:1},terminals_:{2:"error",4:"gantt",6:"EOF",8:"SPACE",10:"NL",12:"weekday_monday",13:"weekday_tuesday",14:"weekday_wednesday",15:"weekday_thursday",16:"weekday_friday",17:"weekday_saturday",18:"weekday_sunday",20:"weekend_friday",21:"weekend_saturday",22:"dateFormat",23:"inclusiveEndDates",24:"topAxis",25:"axisFormat",26:"tickInterval",27:"excludes",28:"includes",29:"todayMarker",30:"title",31:"acc_title",32:"acc_title_value",33:"acc_descr",34:"acc_descr_value",35:"acc_descr_multiline_value",36:"section",38:"taskTxt",39:"taskData",40:"click",41:"callbackname",42:"callbackargs",43:"href"},productions_:[0,[3,3],[5,0],[5,2],[7,2],[7,1],[7,1],[7,1],[11,1],[11,1],[11,1],[11,1],[11,1],[11,1],[11,1],[19,1],[19,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,1],[9,2],[9,2],[9,1],[9,1],[9,1],[9,2],[37,2],[37,3],[37,3],[37,4],[37,3],[37,4],[37,2],[44,2],[44,3],[44,3],[44,4],[44,3],[44,4],[44,2]],performAction:c(function(o,l,h,d,x,s,F){var e=s.length-1;switch(x){case 1:return s[e-1];case 2:this.$=[];break;case 3:s[e-1].push(s[e]),this.$=s[e-1];break;case 4:case 5:this.$=s[e];break;case 6:case 7:this.$=[];break;case 8:d.setWeekday("monday");break;case 9:d.setWeekday("tuesday");break;case 10:d.setWeekday("wednesday");break;case 11:d.setWeekday("thursday");break;case 12:d.setWeekday("friday");break;case 13:d.setWeekday("saturday");break;case 14:d.setWeekday("sunday");break;case 15:d.setWeekend("friday");break;case 16:d.setWeekend("saturday");break;case 17:d.setDateFormat(s[e].substr(11)),this.$=s[e].substr(11);break;case 18:d.enableInclusiveEndDates(),this.$=s[e].substr(18);break;case 19:d.TopAxis(),this.$=s[e].substr(8);break;case 20:d.setAxisFormat(s[e].substr(11)),this.$=s[e].substr(11);break;case 21:d.setTickInterval(s[e].substr(13)),this.$=s[e].substr(13);break;case 22:d.setExcludes(s[e].substr(9)),this.$=s[e].substr(9);break;case 23:d.setIncludes(s[e].substr(9)),this.$=s[e].substr(9);break;case 24:d.setTodayMarker(s[e].substr(12)),this.$=s[e].substr(12);break;case 27:d.setDiagramTitle(s[e].substr(6)),this.$=s[e].substr(6);break;case 28:this.$=s[e].trim(),d.setAccTitle(this.$);break;case 29:case 30:this.$=s[e].trim(),d.setAccDescription(this.$);break;case 31:d.addSection(s[e].substr(8)),this.$=s[e].substr(8);break;case 33:d.addTask(s[e-1],s[e]),this.$="task";break;case 34:this.$=s[e-1],d.setClickEvent(s[e-1],s[e],null);break;case 35:this.$=s[e-2],d.setClickEvent(s[e-2],s[e-1],s[e]);break;case 36:this.$=s[e-2],d.setClickEvent(s[e-2],s[e-1],null),d.setLink(s[e-2],s[e]);break;case 37:this.$=s[e-3],d.setClickEvent(s[e-3],s[e-2],s[e-1]),d.setLink(s[e-3],s[e]);break;case 38:this.$=s[e-2],d.setClickEvent(s[e-2],s[e],null),d.setLink(s[e-2],s[e-1]);break;case 39:this.$=s[e-3],d.setClickEvent(s[e-3],s[e-1],s[e]),d.setLink(s[e-3],s[e-2]);break;case 40:this.$=s[e-1],d.setLink(s[e-1],s[e]);break;case 41:case 47:this.$=s[e-1]+" "+s[e];break;case 42:case 43:case 45:this.$=s[e-2]+" "+s[e-1]+" "+s[e];break;case 44:case 46:this.$=s[e-3]+" "+s[e-2]+" "+s[e-1]+" "+s[e];break}},"anonymous"),table:[{3:1,4:[1,2]},{1:[3]},t(n,[2,2],{5:3}),{6:[1,4],7:5,8:[1,6],9:7,10:[1,8],11:17,12:r,13:i,14:a,15:k,16:T,17:C,18:Y,19:18,20:A,21:D,22:$,23:L,24:O,25:W,26:_,27:J,28:B,29:Z,30:H,31:G,33:Q,35:I,36:p,37:24,38:f,40:u},t(n,[2,7],{1:[2,1]}),t(n,[2,3]),{9:36,11:17,12:r,13:i,14:a,15:k,16:T,17:C,18:Y,19:18,20:A,21:D,22:$,23:L,24:O,25:W,26:_,27:J,28:B,29:Z,30:H,31:G,33:Q,35:I,36:p,37:24,38:f,40:u},t(n,[2,5]),t(n,[2,6]),t(n,[2,17]),t(n,[2,18]),t(n,[2,19]),t(n,[2,20]),t(n,[2,21]),t(n,[2,22]),t(n,[2,23]),t(n,[2,24]),t(n,[2,25]),t(n,[2,26]),t(n,[2,27]),{32:[1,37]},{34:[1,38]},t(n,[2,30]),t(n,[2,31]),t(n,[2,32]),{39:[1,39]},t(n,[2,8]),t(n,[2,9]),t(n,[2,10]),t(n,[2,11]),t(n,[2,12]),t(n,[2,13]),t(n,[2,14]),t(n,[2,15]),t(n,[2,16]),{41:[1,40],43:[1,41]},t(n,[2,4]),t(n,[2,28]),t(n,[2,29]),t(n,[2,33]),t(n,[2,34],{42:[1,42],43:[1,43]}),t(n,[2,40],{41:[1,44]}),t(n,[2,35],{43:[1,45]}),t(n,[2,36]),t(n,[2,38],{42:[1,46]}),t(n,[2,37]),t(n,[2,39])],defaultActions:{},parseError:c(function(o,l){if(l.recoverable)this.trace(o);else{var h=new Error(o);throw h.hash=l,h}},"parseError"),parse:c(function(o){var l=this,h=[0],d=[],x=[null],s=[],F=this.table,e="",b=0,M=0,E=2,S=1,V=s.slice.call(arguments,1),w=Object.create(this.lexer),U={yy:{}};for(var ct in this.yy)Object.prototype.hasOwnProperty.call(this.yy,ct)&&(U.yy[ct]=this.yy[ct]);w.setInput(o,U.yy),U.yy.lexer=w,U.yy.parser=this,typeof w.yylloc>"u"&&(w.yylloc={});var pt=w.yylloc;s.push(pt);var oe=w.options&&w.options.ranges;typeof U.yy.parseError=="function"?this.parseError=U.yy.parseError:this.parseError=Object.getPrototypeOf(this).parseError;function ce(z){h.length=h.length-2*z,x.length=x.length-z,s.length=s.length-z}c(ce,"popStack");function Yt(){var z;return z=d.pop()||w.lex()||S,typeof z!="number"&&(z instanceof Array&&(d=z,z=d.pop()),z=l.symbols_[z]||z),z}c(Yt,"lex");for(var N,tt,j,Tt,it={},ft,X,Pt,ht;;){if(tt=h[h.length-1],this.defaultActions[tt]?j=this.defaultActions[tt]:((N===null||typeof N>"u")&&(N=Yt()),j=F[tt]&&F[tt][N]),typeof j>"u"||!j.length||!j[0]){var xt="";ht=[];for(ft in F[tt])this.terminals_[ft]&&ft>E&&ht.push("'"+this.terminals_[ft]+"'");w.showPosition?xt="Parse error on line "+(b+1)+`:
`+w.showPosition()+`
Expecting `+ht.join(", ")+", got '"+(this.terminals_[N]||N)+"'":xt="Parse error on line "+(b+1)+": Unexpected "+(N==S?"end of input":"'"+(this.terminals_[N]||N)+"'"),this.parseError(xt,{text:w.match,token:this.terminals_[N]||N,line:w.yylineno,loc:pt,expected:ht})}if(j[0]instanceof Array&&j.length>1)throw new Error("Parse Error: multiple actions possible at state: "+tt+", token: "+N);switch(j[0]){case 1:h.push(N),x.push(w.yytext),s.push(w.yylloc),h.push(j[1]),N=null,M=w.yyleng,e=w.yytext,b=w.yylineno,pt=w.yylloc;break;case 2:if(X=this.productions_[j[1]][1],it.$=x[x.length-X],it._$={first_line:s[s.length-(X||1)].first_line,last_line:s[s.length-1].last_line,first_column:s[s.length-(X||1)].first_column,last_column:s[s.length-1].last_column},oe&&(it._$.range=[s[s.length-(X||1)].range[0],s[s.length-1].range[1]]),Tt=this.performAction.apply(it,[e,M,b,U.yy,j[1],x,s].concat(V)),typeof Tt<"u")return Tt;X&&(h=h.slice(0,-1*X*2),x=x.slice(0,-1*X),s=s.slice(0,-1*X)),h.push(this.productions_[j[1]][0]),x.push(it.$),s.push(it._$),Pt=F[h[h.length-2]][h[h.length-1]],h.push(Pt);break;case 3:return!0}}return!0},"parse")},g=function(){var m={EOF:1,parseError:c(function(l,h){if(this.yy.parser)this.yy.parser.parseError(l,h);else throw new Error(l)},"parseError"),setInput:c(function(o,l){return this.yy=l||this.yy||{},this._input=o,this._more=this._backtrack=this.done=!1,this.yylineno=this.yyleng=0,this.yytext=this.matched=this.match="",this.conditionStack=["INITIAL"],this.yylloc={first_line:1,first_column:0,last_line:1,last_column:0},this.options.ranges&&(this.yylloc.range=[0,0]),this.offset=0,this},"setInput"),input:c(function(){var o=this._input[0];this.yytext+=o,this.yyleng++,this.offset++,this.match+=o,this.matched+=o;var l=o.match(/(?:\r\n?|\n).*/g);return l?(this.yylineno++,this.yylloc.last_line++):this.yylloc.last_column++,this.options.ranges&&this.yylloc.range[1]++,this._input=this._input.slice(1),o},"input"),unput:c(function(o){var l=o.length,h=o.split(/(?:\r\n?|\n)/g);this._input=o+this._input,this.yytext=this.yytext.substr(0,this.yytext.length-l),this.offset-=l;var d=this.match.split(/(?:\r\n?|\n)/g);this.match=this.match.substr(0,this.match.length-1),this.matched=this.matched.substr(0,this.matched.length-1),h.length-1&&(this.yylineno-=h.length-1);var x=this.yylloc.range;return this.yylloc={first_line:this.yylloc.first_line,last_line:this.yylineno+1,first_column:this.yylloc.first_column,last_column:h?(h.length===d.length?this.yylloc.first_column:0)+d[d.length-h.length].length-h[0].length:this.yylloc.first_column-l},this.options.ranges&&(this.yylloc.range=[x[0],x[0]+this.yyleng-l]),this.yyleng=this.yytext.length,this},"unput"),more:c(function(){return this._more=!0,this},"more"),reject:c(function(){if(this.options.backtrack_lexer)this._backtrack=!0;else return this.parseError("Lexical error on line "+(this.yylineno+1)+`. You can only invoke reject() in the lexer when the lexer is of the backtracking persuasion (options.backtrack_lexer = true).
`+this.showPosition(),{text:"",token:null,line:this.yylineno});return this},"reject"),less:c(function(o){this.unput(this.match.slice(o))},"less"),pastInput:c(function(){var o=this.matched.substr(0,this.matched.length-this.match.length);return(o.length>20?"...":"")+o.substr(-20).replace(/\n/g,"")},"pastInput"),upcomingInput:c(function(){var o=this.match;return o.length<20&&(o+=this._input.substr(0,20-o.length)),(o.substr(0,20)+(o.length>20?"...":"")).replace(/\n/g,"")},"upcomingInput"),showPosition:c(function(){var o=this.pastInput(),l=new Array(o.length+1).join("-");return o+this.upcomingInput()+`
`+l+"^"},"showPosition"),test_match:c(function(o,l){var h,d,x;if(this.options.backtrack_lexer&&(x={yylineno:this.yylineno,yylloc:{first_line:this.yylloc.first_line,last_line:this.last_line,first_column:this.yylloc.first_column,last_column:this.yylloc.last_column},yytext:this.yytext,match:this.match,matches:this.matches,matched:this.matched,yyleng:this.yyleng,offset:this.offset,_more:this._more,_input:this._input,yy:this.yy,conditionStack:this.conditionStack.slice(0),done:this.done},this.options.ranges&&(x.yylloc.range=this.yylloc.range.slice(0))),d=o[0].match(/(?:\r\n?|\n).*/g),d&&(this.yylineno+=d.length),this.yylloc={first_line:this.yylloc.last_line,last_line:this.yylineno+1,first_column:this.yylloc.last_column,last_column:d?d[d.length-1].length-d[d.length-1].match(/\r?\n?/)[0].length:this.yylloc.last_column+o[0].length},this.yytext+=o[0],this.match+=o[0],this.matches=o,this.yyleng=this.yytext.length,this.options.ranges&&(this.yylloc.range=[this.offset,this.offset+=this.yyleng]),this._more=!1,this._backtrack=!1,this._input=this._input.slice(o[0].length),this.matched+=o[0],h=this.performAction.call(this,this.yy,this,l,this.conditionStack[this.conditionStack.length-1]),this.done&&this._input&&(this.done=!1),h)return h;if(this._backtrack){for(var s in x)this[s]=x[s];return!1}return!1},"test_match"),next:c(function(){if(this.done)return this.EOF;this._input||(this.done=!0);var o,l,h,d;this._more||(this.yytext="",this.match="");for(var x=this._currentRules(),s=0;s<x.length;s++)if(h=this._input.match(this.rules[x[s]]),h&&(!l||h[0].length>l[0].length)){if(l=h,d=s,this.options.backtrack_lexer){if(o=this.test_match(h,x[s]),o!==!1)return o;if(this._backtrack){l=!1;continue}else return!1}else if(!this.options.flex)break}return l?(o=this.test_match(l,x[d]),o!==!1?o:!1):this._input===""?this.EOF:this.parseError("Lexical error on line "+(this.yylineno+1)+`. Unrecognized text.
`+this.showPosition(),{text:"",token:null,line:this.yylineno})},"next"),lex:c(function(){var l=this.next();return l||this.lex()},"lex"),begin:c(function(l){this.conditionStack.push(l)},"begin"),popState:c(function(){var l=this.conditionStack.length-1;return l>0?this.conditionStack.pop():this.conditionStack[0]},"popState"),_currentRules:c(function(){return this.conditionStack.length&&this.conditionStack[this.conditionStack.length-1]?this.conditions[this.conditionStack[this.conditionStack.length-1]].rules:this.conditions.INITIAL.rules},"_currentRules"),topState:c(function(l){return l=this.conditionStack.length-1-Math.abs(l||0),l>=0?this.conditionStack[l]:"INITIAL"},"topState"),pushState:c(function(l){this.begin(l)},"pushState"),stateStackSize:c(function(){return this.conditionStack.length},"stateStackSize"),options:{"case-insensitive":!0},performAction:c(function(l,h,d,x){switch(d){case 0:return this.begin("open_directive"),"open_directive";case 1:return this.begin("acc_title"),31;case 2:return this.popState(),"acc_title_value";case 3:return this.begin("acc_descr"),33;case 4:return this.popState(),"acc_descr_value";case 5:this.begin("acc_descr_multiline");break;case 6:this.popState();break;case 7:return"acc_descr_multiline_value";case 8:break;case 9:break;case 10:break;case 11:return 10;case 12:break;case 13:break;case 14:this.begin("href");break;case 15:this.popState();break;case 16:return 43;case 17:this.begin("callbackname");break;case 18:this.popState();break;case 19:this.popState(),this.begin("callbackargs");break;case 20:return 41;case 21:this.popState();break;case 22:return 42;case 23:this.begin("click");break;case 24:this.popState();break;case 25:return 40;case 26:return 4;case 27:return 22;case 28:return 23;case 29:return 24;case 30:return 25;case 31:return 26;case 32:return 28;case 33:return 27;case 34:return 29;case 35:return 12;case 36:return 13;case 37:return 14;case 38:return 15;case 39:return 16;case 40:return 17;case 41:return 18;case 42:return 20;case 43:return 21;case 44:return"date";case 45:return 30;case 46:return"accDescription";case 47:return 36;case 48:return 38;case 49:return 39;case 50:return":";case 51:return 6;case 52:return"INVALID"}},"anonymous"),rules:[/^(?:%%\{)/i,/^(?:accTitle\s*:\s*)/i,/^(?:(?!\n||)*[^\n]*)/i,/^(?:accDescr\s*:\s*)/i,/^(?:(?!\n||)*[^\n]*)/i,/^(?:accDescr\s*\{\s*)/i,/^(?:[\}])/i,/^(?:[^\}]*)/i,/^(?:%%(?!\{)*[^\n]*)/i,/^(?:[^\}]%%*[^\n]*)/i,/^(?:%%*[^\n]*[\n]*)/i,/^(?:[\n]+)/i,/^(?:\s+)/i,/^(?:%[^\n]*)/i,/^(?:href[\s]+["])/i,/^(?:["])/i,/^(?:[^"]*)/i,/^(?:call[\s]+)/i,/^(?:\([\s]*\))/i,/^(?:\()/i,/^(?:[^(]*)/i,/^(?:\))/i,/^(?:[^)]*)/i,/^(?:click[\s]+)/i,/^(?:[\s\n])/i,/^(?:[^\s\n]*)/i,/^(?:gantt\b)/i,/^(?:dateFormat\s[^#\n;]+)/i,/^(?:inclusiveEndDates\b)/i,/^(?:topAxis\b)/i,/^(?:axisFormat\s[^#\n;]+)/i,/^(?:tickInterval\s[^#\n;]+)/i,/^(?:includes\s[^#\n;]+)/i,/^(?:excludes\s[^#\n;]+)/i,/^(?:todayMarker\s[^\n;]+)/i,/^(?:weekday\s+monday\b)/i,/^(?:weekday\s+tuesday\b)/i,/^(?:weekday\s+wednesday\b)/i,/^(?:weekday\s+thursday\b)/i,/^(?:weekday\s+friday\b)/i,/^(?:weekday\s+saturday\b)/i,/^(?:weekday\s+sunday\b)/i,/^(?:weekend\s+friday\b)/i,/^(?:weekend\s+saturday\b)/i,/^(?:\d\d\d\d-\d\d-\d\d\b)/i,/^(?:title\s[^\n]+)/i,/^(?:accDescription\s[^#\n;]+)/i,/^(?:section\s[^\n]+)/i,/^(?:[^:\n]+)/i,/^(?::[^#\n;]+)/i,/^(?::)/i,/^(?:$)/i,/^(?:.)/i],conditions:{acc_descr_multiline:{rules:[6,7],inclusive:!1},acc_descr:{rules:[4],inclusive:!1},acc_title:{rules:[2],inclusive:!1},callbackargs:{rules:[21,22],inclusive:!1},callbackname:{rules:[18,19,20],inclusive:!1},href:{rules:[15,16],inclusive:!1},click:{rules:[24,25],inclusive:!1},INITIAL:{rules:[0,1,3,5,8,9,10,11,12,13,14,17,23,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,45,46,47,48,49,50,51,52],inclusive:!0}}};return m}();y.lexer=g;function v(){this.yy={}}return c(v,"Parser"),v.prototype=y,y.Parser=v,new v}();wt.parser=wt;var Ve=wt;R.extend(Ye);R.extend(Le);R.extend(Oe);var qt={friday:5,saturday:6},q="",St="",Et=void 0,It="",lt=[],ut=[],Mt=new Map,At=[],gt=[],ot="",Ft="",Zt=["active","done","crit","milestone","vert"],$t=[],nt="",dt=!1,Lt=!1,Ot="sunday",vt="saturday",_t=0,Ne=c(function(){At=[],gt=[],ot="",$t=[],kt=0,Ct=void 0,yt=void 0,P=[],q="",St="",Ft="",Et=void 0,It="",lt=[],ut=[],dt=!1,Lt=!1,_t=0,Mt=new Map,nt="",Fe(),Ot="sunday",vt="saturday"},"clear"),Be=c(function(t){nt=t},"setDiagramId"),ze=c(function(t){St=t},"setAxisFormat"),He=c(function(){return St},"getAxisFormat"),je=c(function(t){Et=t},"setTickInterval"),qe=c(function(){return Et},"getTickInterval"),Ge=c(function(t){It=t},"setTodayMarker"),Ue=c(function(){return It},"getTodayMarker"),Xe=c(function(t){q=t},"setDateFormat"),Ke=c(function(){dt=!0},"enableInclusiveEndDates"),Je=c(function(){return dt},"endDatesAreInclusive"),Qe=c(function(){Lt=!0},"enableTopAxis"),Ze=c(function(){return Lt},"topAxisEnabled"),ts=c(function(t){Ft=t},"setDisplayMode"),es=c(function(){return Ft},"getDisplayMode"),ss=c(function(){return q},"getDateFormat"),is=c(function(t){lt=t.toLowerCase().split(/[\s,]+/)},"setIncludes"),ns=c(function(){return lt},"getIncludes"),rs=c(function(t){ut=t.toLowerCase().split(/[\s,]+/)},"setExcludes"),as=c(function(){return ut},"getExcludes"),os=c(function(){return Mt},"getLinks"),cs=c(function(t){ot=t,At.push(t)},"addSection"),ls=c(function(){return At},"getSections"),us=c(function(){let t=Gt();const n=10;let r=0;for(;!t&&r<n;)t=Gt(),r++;return gt=P,gt},"getTasks"),te=c(function(t,n,r,i){const a=t.format(n.trim()),k=t.format("YYYY-MM-DD");return i.includes(a)||i.includes(k)?!1:r.includes("weekends")&&(t.isoWeekday()===qt[vt]||t.isoWeekday()===qt[vt]+1)||r.includes(t.format("dddd").toLowerCase())?!0:r.includes(a)||r.includes(k)},"isInvalidDate"),ds=c(function(t){Ot=t},"setWeekday"),fs=c(function(){return Ot},"getWeekday"),hs=c(function(t){vt=t},"setWeekend"),ee=c(function(t,n,r,i){if(!r.length||t.manualEndTime)return;let a;t.startTime instanceof Date?a=R(t.startTime):a=R(t.startTime,n,!0),a=a.add(1,"d");let k;t.endTime instanceof Date?k=R(t.endTime):k=R(t.endTime,n,!0);const[T,C]=ms(a,k,n,r,i);t.endTime=T.toDate(),t.renderEndTime=C},"checkTaskDates"),ms=c(function(t,n,r,i,a){let k=!1,T=null;const C=n.add(1e4,"d");for(;t<=n;){if(k||(T=n.toDate()),k=te(t,r,i,a),k&&(n=n.add(1,"d"),n>C))throw new Error("Failed to find a valid date that was not excluded by `excludes` after 10,000 iterations.");t=t.add(1,"d")}return[n,T]},"fixTaskDates"),Dt=c(function(t,n,r){if(r=r.trim(),c(C=>{const Y=C.trim();return Y==="x"||Y==="X"},"isTimestampFormat")(n)&&/^\d+$/.test(r))return new Date(Number(r));const k=/^after\s+(?<ids>[\d\w- ]+)/.exec(r);if(k!==null){let C=null;for(const A of k.groups.ids.split(" ")){let D=st(A);D!==void 0&&(!C||D.endTime>C.endTime)&&(C=D)}if(C)return C.endTime;const Y=new Date;return Y.setHours(0,0,0,0),Y}let T=R(r,n.trim(),!0);if(T.isValid())return T.toDate();{et.debug("Invalid date:"+r),et.debug("With date format:"+n.trim());const C=new Date(r);if(C===void 0||isNaN(C.getTime())||C.getFullYear()<-1e4||C.getFullYear()>1e4)throw new Error("Invalid date:"+r);return C}},"getStartDate"),se=c(function(t){const n=/^(\d+(?:\.\d+)?)([Mdhmswy]|ms)$/.exec(t.trim());return n!==null?[Number.parseFloat(n[1]),n[2]]:[NaN,"ms"]},"parseDuration"),ie=c(function(t,n,r,i=!1){r=r.trim();const k=/^until\s+(?<ids>[\d\w- ]+)/.exec(r);if(k!==null){let D=null;for(const L of k.groups.ids.split(" ")){let O=st(L);O!==void 0&&(!D||O.startTime<D.startTime)&&(D=O)}if(D)return D.startTime;const $=new Date;return $.setHours(0,0,0,0),$}let T=R(r,n.trim(),!0);if(T.isValid())return i&&(T=T.add(1,"d")),T.toDate();let C=R(t);const[Y,A]=se(r);if(!Number.isNaN(Y)){const D=C.add(Y,A);D.isValid()&&(C=D)}return C.toDate()},"getEndDate"),kt=0,at=c(function(t){return t===void 0?(kt=kt+1,"task"+kt):t},"parseId"),ks=c(function(t,n){let r;n.substr(0,1)===":"?r=n.substr(1,n.length):r=n;const i=r.split(","),a={};Wt(i,a,Zt);for(let T=0;T<i.length;T++)i[T]=i[T].trim();let k="";switch(i.length){case 1:a.id=at(),a.startTime=t.endTime,k=i[0];break;case 2:a.id=at(),a.startTime=Dt(void 0,q,i[0]),k=i[1];break;case 3:a.id=at(i[0]),a.startTime=Dt(void 0,q,i[1]),k=i[2];break}return k&&(a.endTime=ie(a.startTime,q,k,dt),a.manualEndTime=R(k,"YYYY-MM-DD",!0).isValid(),ee(a,q,ut,lt)),a},"compileData"),ys=c(function(t,n){let r;n.substr(0,1)===":"?r=n.substr(1,n.length):r=n;const i=r.split(","),a={};Wt(i,a,Zt);for(let k=0;k<i.length;k++)i[k]=i[k].trim();switch(i.length){case 1:a.id=at(),a.startTime={type:"prevTaskEnd",id:t},a.endTime={data:i[0]};break;case 2:a.id=at(),a.startTime={type:"getStartDate",startData:i[0]},a.endTime={data:i[1]};break;case 3:a.id=at(i[0]),a.startTime={type:"getStartDate",startData:i[1]},a.endTime={data:i[2]};break}return a},"parseData"),Ct,yt,P=[],ne={},gs=c(function(t,n){const r={section:ot,type:ot,processed:!1,manualEndTime:!1,renderEndTime:null,raw:{data:n},task:t,classes:[]},i=ys(yt,n);r.raw.startTime=i.startTime,r.raw.endTime=i.endTime,r.id=i.id,r.prevTaskId=yt,r.active=i.active,r.done=i.done,r.crit=i.crit,r.milestone=i.milestone,r.vert=i.vert,r.order=_t,_t++;const a=P.push(r);yt=r.id,ne[r.id]=a-1},"addTask"),st=c(function(t){const n=ne[t];return P[n]},"findTaskById"),vs=c(function(t,n){const r={section:ot,type:ot,description:t,task:t,classes:[]},i=ks(Ct,n);r.startTime=i.startTime,r.endTime=i.endTime,r.id=i.id,r.active=i.active,r.done=i.done,r.crit=i.crit,r.milestone=i.milestone,r.vert=i.vert,Ct=r,gt.push(r)},"addTaskOrg"),Gt=c(function(){const t=c(function(r){const i=P[r];let a="";switch(P[r].raw.startTime.type){case"prevTaskEnd":{const k=st(i.prevTaskId);i.startTime=k.endTime;break}case"getStartDate":a=Dt(void 0,q,P[r].raw.startTime.startData),a&&(P[r].startTime=a);break}return P[r].startTime&&(P[r].endTime=ie(P[r].startTime,q,P[r].raw.endTime.data,dt),P[r].endTime&&(P[r].processed=!0,P[r].manualEndTime=R(P[r].raw.endTime.data,"YYYY-MM-DD",!0).isValid(),ee(P[r],q,ut,lt))),P[r].processed},"compileTask");let n=!0;for(const[r,i]of P.entries())t(r),n=n&&i.processed;return n},"compileTasks"),ps=c(function(t,n){let r=n;rt().securityLevel!=="loose"&&(r=Ae(n)),t.split(",").forEach(function(i){st(i)!==void 0&&(ae(i,()=>{window.open(r,"_self")}),Mt.set(i,r))}),re(t,"clickable")},"setLink"),re=c(function(t,n){t.split(",").forEach(function(r){let i=st(r);i!==void 0&&i.classes.push(n)})},"setClass"),Ts=c(function(t,n,r){if(rt().securityLevel!=="loose"||n===void 0)return;let i=[];if(typeof r=="string"){i=r.split(/,(?=(?:(?:[^"]*"){2})*[^"]*$)/);for(let k=0;k<i.length;k++){let T=i[k].trim();T.startsWith('"')&&T.endsWith('"')&&(T=T.substr(1,T.length-2)),i[k]=T}}i.length===0&&i.push(t),st(t)!==void 0&&ae(t,()=>{$e.runFunc(n,...i)})},"setClickFun"),ae=c(function(t,n){$t.push(function(){const r=nt?`${nt}-${t}`:t,i=document.querySelector(`[id="${r}"]`);i!==null&&i.addEventListener("click",function(){n()})},function(){const r=nt?`${nt}-${t}`:t,i=document.querySelector(`[id="${r}-text"]`);i!==null&&i.addEventListener("click",function(){n()})})},"pushFun"),xs=c(function(t,n,r){t.split(",").forEach(function(i){Ts(i,n,r)}),re(t,"clickable")},"setClickEvent"),bs=c(function(t){$t.forEach(function(n){n(t)})},"bindFunctions"),ws={getConfig:c(()=>rt().gantt,"getConfig"),clear:Ne,setDateFormat:Xe,getDateFormat:ss,enableInclusiveEndDates:Ke,endDatesAreInclusive:Je,enableTopAxis:Qe,topAxisEnabled:Ze,setAxisFormat:ze,getAxisFormat:He,setTickInterval:je,getTickInterval:qe,setTodayMarker:Ge,getTodayMarker:Ue,setAccTitle:me,getAccTitle:he,setDiagramTitle:fe,getDiagramTitle:de,setDiagramId:Be,setDisplayMode:ts,getDisplayMode:es,setAccDescription:ue,getAccDescription:le,addSection:cs,getSections:ls,getTasks:us,addTask:gs,findTaskById:st,addTaskOrg:vs,setIncludes:is,getIncludes:ns,setExcludes:rs,getExcludes:as,setClickEvent:xs,setLink:ps,getLinks:os,bindFunctions:bs,parseDuration:se,isInvalidDate:te,setWeekday:ds,getWeekday:fs,setWeekend:hs};function Wt(t,n,r){let i=!0;for(;i;)i=!1,r.forEach(function(a){const k="^\\s*"+a+"\\s*$",T=new RegExp(k);t[0].match(T)&&(n[a]=!0,t.shift(1),i=!0)})}c(Wt,"getTaskTags");R.extend(Re);var _s=c(function(){et.debug("Something is calling, setConf, remove the call")},"setConf"),Ut={monday:Ee,tuesday:Se,wednesday:Ce,thursday:De,friday:_e,saturday:we,sunday:be},Ds=c((t,n)=>{let r=[...t].map(()=>-1/0),i=[...t].sort((k,T)=>k.startTime-T.startTime||k.order-T.order),a=0;for(const k of i)for(let T=0;T<r.length;T++)if(k.startTime>=r[T]){r[T]=k.endTime,k.order=T+n,T>a&&(a=T);break}return a},"getMaxIntersections"),K,bt=1e4,Cs=c(function(t,n,r,i){const a=rt().gantt;i.db.setDiagramId(n);const k=rt().securityLevel;let T;k==="sandbox"&&(T=mt("#i"+n));const C=k==="sandbox"?mt(T.nodes()[0].contentDocument.body):mt("body"),Y=k==="sandbox"?T.nodes()[0].contentDocument:document,A=Y.getElementById(n);K=A.parentElement.offsetWidth,K===void 0&&(K=1200),a.useWidth!==void 0&&(K=a.useWidth);const D=i.db.getTasks();let $=[];for(const u of D)$.push(u.type);$=f($);const L={};let O=2*a.topPadding;if(i.db.getDisplayMode()==="compact"||a.displayMode==="compact"){const u={};for(const g of D)u[g.section]===void 0?u[g.section]=[g]:u[g.section].push(g);let y=0;for(const g of Object.keys(u)){const v=Ds(u[g],y)+1;y+=v,O+=v*(a.barHeight+a.barGap),L[g]=v}}else{O+=D.length*(a.barHeight+a.barGap);for(const u of $)L[u]=D.filter(y=>y.type===u).length}A.setAttribute("viewBox","0 0 "+K+" "+O);const W=C.select(`[id="${n}"]`),_=ke().domain([ye(D,function(u){return u.startTime}),ge(D,function(u){return u.endTime})]).rangeRound([0,K-a.leftPadding-a.rightPadding]);function J(u,y){const g=u.startTime,v=y.startTime;let m=0;return g>v?m=1:g<v&&(m=-1),m}c(J,"taskCompare"),D.sort(J),B(D,K,O),ve(W,O,K,a.useMaxWidth),W.append("text").text(i.db.getDiagramTitle()).attr("x",K/2).attr("y",a.titleTopMargin).attr("class","titleText");function B(u,y,g){const v=a.barHeight,m=v+a.barGap,o=a.topPadding,l=a.leftPadding,h=pe().domain([0,$.length]).range(["#00B9FA","#F95002"]).interpolate(Te);H(m,o,l,y,g,u,i.db.getExcludes(),i.db.getIncludes()),Q(l,o,y,g),Z(u,m,o,l,v,h,y),I(m,o),p(l,o,y,g)}c(B,"makeGantt");function Z(u,y,g,v,m,o,l){u.sort((e,b)=>e.vert===b.vert?0:e.vert?1:-1);const d=[...new Set(u.map(e=>e.order))].map(e=>u.find(b=>b.order===e));W.append("g").selectAll("rect").data(d).enter().append("rect").attr("x",0).attr("y",function(e,b){return b=e.order,b*y+g-2}).attr("width",function(){return l-a.rightPadding/2}).attr("height",y).attr("class",function(e){for(const[b,M]of $.entries())if(e.type===M)return"section section"+b%a.numberSectionStyles;return"section section0"}).enter();const x=W.append("g").selectAll("rect").data(u).enter(),s=i.db.getLinks();if(x.append("rect").attr("id",function(e){return n+"-"+e.id}).attr("rx",3).attr("ry",3).attr("x",function(e){return e.milestone?_(e.startTime)+v+.5*(_(e.endTime)-_(e.startTime))-.5*m:_(e.startTime)+v}).attr("y",function(e,b){return b=e.order,e.vert?a.gridLineStartPadding:b*y+g}).attr("width",function(e){return e.milestone?m:e.vert?.08*m:_(e.renderEndTime||e.endTime)-_(e.startTime)}).attr("height",function(e){return e.vert?D.length*(a.barHeight+a.barGap)+a.barHeight*2:m}).attr("transform-origin",function(e,b){return b=e.order,(_(e.startTime)+v+.5*(_(e.endTime)-_(e.startTime))).toString()+"px "+(b*y+g+.5*m).toString()+"px"}).attr("class",function(e){const b="task";let M="";e.classes.length>0&&(M=e.classes.join(" "));let E=0;for(const[V,w]of $.entries())e.type===w&&(E=V%a.numberSectionStyles);let S="";return e.active?e.crit?S+=" activeCrit":S=" active":e.done?e.crit?S=" doneCrit":S=" done":e.crit&&(S+=" crit"),S.length===0&&(S=" task"),e.milestone&&(S=" milestone "+S),e.vert&&(S=" vert "+S),S+=E,S+=" "+M,b+S}),x.append("text").attr("id",function(e){return n+"-"+e.id+"-text"}).text(function(e){return e.task}).attr("font-size",a.fontSize).attr("x",function(e){let b=_(e.startTime),M=_(e.renderEndTime||e.endTime);if(e.milestone&&(b+=.5*(_(e.endTime)-_(e.startTime))-.5*m,M=b+m),e.vert)return _(e.startTime)+v;const E=this.getBBox().width;return E>M-b?M+E+1.5*a.leftPadding>l?b+v-5:M+v+5:(M-b)/2+b+v}).attr("y",function(e,b){return e.vert?a.gridLineStartPadding+D.length*(a.barHeight+a.barGap)+60:(b=e.order,b*y+a.barHeight/2+(a.fontSize/2-2)+g)}).attr("text-height",m).attr("class",function(e){const b=_(e.startTime);let M=_(e.endTime);e.milestone&&(M=b+m);const E=this.getBBox().width;let S="";e.classes.length>0&&(S=e.classes.join(" "));let V=0;for(const[U,ct]of $.entries())e.type===ct&&(V=U%a.numberSectionStyles);let w="";return e.active&&(e.crit?w="activeCritText"+V:w="activeText"+V),e.done?e.crit?w=w+" doneCritText"+V:w=w+" doneText"+V:e.crit&&(w=w+" critText"+V),e.milestone&&(w+=" milestoneText"),e.vert&&(w+=" vertText"),E>M-b?M+E+1.5*a.leftPadding>l?S+" taskTextOutsideLeft taskTextOutside"+V+" "+w:S+" taskTextOutsideRight taskTextOutside"+V+" "+w+" width-"+E:S+" taskText taskText"+V+" "+w+" width-"+E}),rt().securityLevel==="sandbox"){let e;e=mt("#i"+n);const b=e.nodes()[0].contentDocument;x.filter(function(M){return s.has(M.id)}).each(function(M){var E=b.querySelector("#"+CSS.escape(n+"-"+M.id)),S=b.querySelector("#"+CSS.escape(n+"-"+M.id+"-text"));const V=E.parentNode;var w=b.createElement("a");w.setAttribute("xlink:href",s.get(M.id)),w.setAttribute("target","_top"),V.appendChild(w),w.appendChild(E),w.appendChild(S)})}}c(Z,"drawRects");function H(u,y,g,v,m,o,l,h){if(l.length===0&&h.length===0)return;let d,x;for(const{startTime:E,endTime:S}of o)(d===void 0||E<d)&&(d=E),(x===void 0||S>x)&&(x=S);if(!d||!x)return;if(R(x).diff(R(d),"year")>5){et.warn("The difference between the min and max time is more than 5 years. This will cause performance issues. Skipping drawing exclude days.");return}const s=i.db.getDateFormat(),F=[];let e=null,b=R(d);for(;b.valueOf()<=x;)i.db.isInvalidDate(b,s,l,h)?e?e.end=b:e={start:b,end:b}:e&&(F.push(e),e=null),b=b.add(1,"d");W.append("g").selectAll("rect").data(F).enter().append("rect").attr("id",E=>n+"-exclude-"+E.start.format("YYYY-MM-DD")).attr("x",E=>_(E.start.startOf("day"))+g).attr("y",a.gridLineStartPadding).attr("width",E=>_(E.end.endOf("day"))-_(E.start.startOf("day"))).attr("height",m-y-a.gridLineStartPadding).attr("transform-origin",function(E,S){return(_(E.start)+g+.5*(_(E.end)-_(E.start))).toString()+"px "+(S*u+.5*m).toString()+"px"}).attr("class","exclude-range")}c(H,"drawExcludeDays");function G(u,y,g,v){if(g<=0||u>y)return 1/0;const m=y-u,o=R.duration({[v??"day"]:g}).asMilliseconds();return o<=0?1/0:Math.ceil(m/o)}c(G,"getEstimatedTickCount");function Q(u,y,g,v){const m=i.db.getDateFormat(),o=i.db.getAxisFormat();let l;o?l=o:m==="D"?l="%d":l=a.axisFormat??"%Y-%m-%d";let h=xe(_).tickSize(-v+y+a.gridLineStartPadding).tickFormat(Rt(l));const x=/^([1-9]\d*)(millisecond|second|minute|hour|day|week|month)$/.exec(i.db.getTickInterval()||a.tickInterval);if(x!==null){const s=parseInt(x[1],10);if(isNaN(s)||s<=0)et.warn(`Invalid tick interval value: "${x[1]}". Skipping custom tick interval.`);else{const F=x[2],e=i.db.getWeekday()||a.weekday,b=_.domain(),M=b[0],E=b[1],S=G(M,E,s,F);if(S>bt)et.warn(`The tick interval "${s}${F}" would generate ${S} ticks, which exceeds the maximum allowed (${bt}). This may indicate an invalid date or time range. Skipping custom tick interval.`);else switch(F){case"millisecond":h.ticks(jt.every(s));break;case"second":h.ticks(Ht.every(s));break;case"minute":h.ticks(zt.every(s));break;case"hour":h.ticks(Bt.every(s));break;case"day":h.ticks(Nt.every(s));break;case"week":h.ticks(Ut[e].every(s));break;case"month":h.ticks(Vt.every(s));break}}}if(W.append("g").attr("class","grid").attr("transform","translate("+u+", "+(v-50)+")").call(h).selectAll("text").style("text-anchor","middle").attr("fill","#000").attr("stroke","none").attr("font-size",10).attr("dy","1em"),i.db.topAxisEnabled()||a.topAxis){let s=Ie(_).tickSize(-v+y+a.gridLineStartPadding).tickFormat(Rt(l));if(x!==null){const F=parseInt(x[1],10);if(isNaN(F)||F<=0)et.warn(`Invalid tick interval value: "${x[1]}". Skipping custom tick interval.`);else{const e=x[2],b=i.db.getWeekday()||a.weekday,M=_.domain(),E=M[0],S=M[1];if(G(E,S,F,e)<=bt)switch(e){case"millisecond":s.ticks(jt.every(F));break;case"second":s.ticks(Ht.every(F));break;case"minute":s.ticks(zt.every(F));break;case"hour":s.ticks(Bt.every(F));break;case"day":s.ticks(Nt.every(F));break;case"week":s.ticks(Ut[b].every(F));break;case"month":s.ticks(Vt.every(F));break}}}W.append("g").attr("class","grid").attr("transform","translate("+u+", "+y+")").call(s).selectAll("text").style("text-anchor","middle").attr("fill","#000").attr("stroke","none").attr("font-size",10)}}c(Q,"makeGrid");function I(u,y){let g=0;const v=Object.keys(L).map(m=>[m,L[m]]);W.append("g").selectAll("text").data(v).enter().append(function(m){const o=m[0].split(Me.lineBreakRegex),l=-(o.length-1)/2,h=Y.createElementNS("http://www.w3.org/2000/svg","text");h.setAttribute("dy",l+"em");for(const[d,x]of o.entries()){const s=Y.createElementNS("http://www.w3.org/2000/svg","tspan");s.setAttribute("alignment-baseline","central"),s.setAttribute("x","10"),d>0&&s.setAttribute("dy","1em"),s.textContent=x,h.appendChild(s)}return h}).attr("x",10).attr("y",function(m,o){if(o>0)for(let l=0;l<o;l++)return g+=v[o-1][1],m[1]*u/2+g*u+y;else return m[1]*u/2+y}).attr("font-size",a.sectionFontSize).attr("class",function(m){for(const[o,l]of $.entries())if(m[0]===l)return"sectionTitle sectionTitle"+o%a.numberSectionStyles;return"sectionTitle"})}c(I,"vertLabels");function p(u,y,g,v){const m=i.db.getTodayMarker();if(m==="off")return;const o=W.append("g").attr("class","today"),l=new Date,h=o.append("line");h.attr("x1",_(l)+u).attr("x2",_(l)+u).attr("y1",a.titleTopMargin).attr("y2",v-a.titleTopMargin).attr("class","today"),m!==""&&h.attr("style",m.replace(/,/g,";"))}c(p,"drawToday");function f(u){const y={},g=[];for(let v=0,m=u.length;v<m;++v)Object.prototype.hasOwnProperty.call(y,u[v])||(y[u[v]]=!0,g.push(u[v]));return g}c(f,"checkUnique")},"draw"),Ss={setConf:_s,draw:Cs},Es=c(t=>`
  .mermaid-main-font {
        font-family: ${t.fontFamily};
  }

  .exclude-range {
    fill: ${t.excludeBkgColor};
  }

  .section {
    stroke: none;
    opacity: 0.2;
  }

  .section0 {
    fill: ${t.sectionBkgColor};
  }

  .section2 {
    fill: ${t.sectionBkgColor2};
  }

  .section1,
  .section3 {
    fill: ${t.altSectionBkgColor};
    opacity: 0.2;
  }

  .sectionTitle0 {
    fill: ${t.titleColor};
  }

  .sectionTitle1 {
    fill: ${t.titleColor};
  }

  .sectionTitle2 {
    fill: ${t.titleColor};
  }

  .sectionTitle3 {
    fill: ${t.titleColor};
  }

  .sectionTitle {
    text-anchor: start;
    font-family: ${t.fontFamily};
  }


  /* Grid and axis */

  .grid .tick {
    stroke: ${t.gridColor};
    opacity: 0.8;
    shape-rendering: crispEdges;
  }

  .grid .tick text {
    font-family: ${t.fontFamily};
    fill: ${t.textColor};
  }

  .grid path {
    stroke-width: 0;
  }


  /* Today line */

  .today {
    fill: none;
    stroke: ${t.todayLineColor};
    stroke-width: 2px;
  }


  /* Task styling */

  /* Default task */

  .task {
    stroke-width: 2;
  }

  .taskText {
    text-anchor: middle;
    font-family: ${t.fontFamily};
  }

  .taskTextOutsideRight {
    fill: ${t.taskTextDarkColor};
    text-anchor: start;
    font-family: ${t.fontFamily};
  }

  .taskTextOutsideLeft {
    fill: ${t.taskTextDarkColor};
    text-anchor: end;
  }


  /* Special case clickable */

  .task.clickable {
    cursor: pointer;
  }

  .taskText.clickable {
    cursor: pointer;
    fill: ${t.taskTextClickableColor} !important;
    font-weight: bold;
  }

  .taskTextOutsideLeft.clickable {
    cursor: pointer;
    fill: ${t.taskTextClickableColor} !important;
    font-weight: bold;
  }

  .taskTextOutsideRight.clickable {
    cursor: pointer;
    fill: ${t.taskTextClickableColor} !important;
    font-weight: bold;
  }


  /* Specific task settings for the sections*/

  .taskText0,
  .taskText1,
  .taskText2,
  .taskText3 {
    fill: ${t.taskTextColor};
  }

  .task0,
  .task1,
  .task2,
  .task3 {
    fill: ${t.taskBkgColor};
    stroke: ${t.taskBorderColor};
  }

  .taskTextOutside0,
  .taskTextOutside2
  {
    fill: ${t.taskTextOutsideColor};
  }

  .taskTextOutside1,
  .taskTextOutside3 {
    fill: ${t.taskTextOutsideColor};
  }


  /* Active task */

  .active0,
  .active1,
  .active2,
  .active3 {
    fill: ${t.activeTaskBkgColor};
    stroke: ${t.activeTaskBorderColor};
  }

  .activeText0,
  .activeText1,
  .activeText2,
  .activeText3 {
    fill: ${t.taskTextDarkColor} !important;
  }


  /* Completed task */

  .done0,
  .done1,
  .done2,
  .done3 {
    stroke: ${t.doneTaskBorderColor};
    fill: ${t.doneTaskBkgColor};
    stroke-width: 2;
  }

  .doneText0,
  .doneText1,
  .doneText2,
  .doneText3 {
    fill: ${t.taskTextDarkColor} !important;
  }

  /* Done task text displayed outside the bar sits against the diagram background,
     not against the done-task bar, so it must use the outside/contrast color. */
  .doneText0.taskTextOutsideLeft,
  .doneText0.taskTextOutsideRight,
  .doneText1.taskTextOutsideLeft,
  .doneText1.taskTextOutsideRight,
  .doneText2.taskTextOutsideLeft,
  .doneText2.taskTextOutsideRight,
  .doneText3.taskTextOutsideLeft,
  .doneText3.taskTextOutsideRight {
    fill: ${t.taskTextOutsideColor} !important;
  }


  /* Tasks on the critical line */

  .crit0,
  .crit1,
  .crit2,
  .crit3 {
    stroke: ${t.critBorderColor};
    fill: ${t.critBkgColor};
    stroke-width: 2;
  }

  .activeCrit0,
  .activeCrit1,
  .activeCrit2,
  .activeCrit3 {
    stroke: ${t.critBorderColor};
    fill: ${t.activeTaskBkgColor};
    stroke-width: 2;
  }

  .doneCrit0,
  .doneCrit1,
  .doneCrit2,
  .doneCrit3 {
    stroke: ${t.critBorderColor};
    fill: ${t.doneTaskBkgColor};
    stroke-width: 2;
    cursor: pointer;
    shape-rendering: crispEdges;
  }

  .milestone {
    transform: rotate(45deg) scale(0.8,0.8);
  }

  .milestoneText {
    font-style: italic;
  }
  .doneCritText0,
  .doneCritText1,
  .doneCritText2,
  .doneCritText3 {
    fill: ${t.taskTextDarkColor} !important;
  }

  /* Done-crit task text outside the bar — same reasoning as doneText above. */
  .doneCritText0.taskTextOutsideLeft,
  .doneCritText0.taskTextOutsideRight,
  .doneCritText1.taskTextOutsideLeft,
  .doneCritText1.taskTextOutsideRight,
  .doneCritText2.taskTextOutsideLeft,
  .doneCritText2.taskTextOutsideRight,
  .doneCritText3.taskTextOutsideLeft,
  .doneCritText3.taskTextOutsideRight {
    fill: ${t.taskTextOutsideColor} !important;
  }

  .vert {
    stroke: ${t.vertLineColor};
  }

  .vertText {
    font-size: 15px;
    text-anchor: middle;
    fill: ${t.vertLineColor} !important;
  }

  .activeCritText0,
  .activeCritText1,
  .activeCritText2,
  .activeCritText3 {
    fill: ${t.taskTextDarkColor} !important;
  }

  .titleText {
    text-anchor: middle;
    font-size: 18px;
    fill: ${t.titleColor||t.textColor};
    font-family: ${t.fontFamily};
  }
`,"getStyles"),Is=Es,$s={parser:Ve,db:ws,renderer:Ss,styles:Is};export{$s as diagram};
