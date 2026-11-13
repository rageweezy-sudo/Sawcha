'use strict';
const token = document.querySelector('meta[name=sawcha-session]').content;
const $ = id => document.getElementById(id);
let report = null;
let history = [];
let busy = false;
async function api(url, data) {
  const response = await fetch(url,{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-Sawcha-Token':token},body:JSON.stringify(data)});
  const out = await response.json();
  if(!response.ok) throw new Error(out.error || `Request failed (${response.status})`);
  return out;
}
function setStatus(id,msg,error=false){const e=$(id);e.textContent=msg;e.classList.toggle('error',error)}
function fact(name,value){const e=document.createElement('div');e.className='fact';const a=document.createElement('span');a.textContent=name;const b=document.createElement('strong');b.textContent=value == null?'Unavailable':String(value);e.append(a,b);return e}
function renderReport(){
 $('rawReport').textContent=JSON.stringify(report,null,2);
 $('summary').replaceChildren();
 const d=report?.windowsData || {};
 const cpu=d.cpu||{},os=d.os||{};
 const gpu=Array.isArray(d.gpu)?d.gpu[0]:{};
 $('summary').append(fact('CPU',cpu.Name),fact('LOGICAL THREADS',cpu.NumberOfLogicalProcessors),fact('GPU',gpu?.Name),fact('WINDOWS',os.Caption||os.Version),fact('MEMORY AVAILABLE (MB)',os.FreePhysicalMemorySize||os.FreePhysicalMemory),fact('OS BUILD',os.BuildNumber));
 $('copy').disabled=$('export').disabled=$('copyPrompt').disabled=false;
 setStatus('scanstatus',`Report captured at ${report.timestampUtc}. Sections: ${Object.keys(d).length}. Warnings: ${report.warnings.length}. No changes were made.`);
}
async function doScan(deep){
 if(busy)return;busy=true;
 $('scan').disabled=$('deep').disabled=true;
 setStatus('scanstatus',deep?'Collecting deep drivers, counters and event IDs (may take up to 30 seconds)...':'Collecting Windows system data (may take up to 15 seconds)...');
 try{report=await api('/api/scan',{deep});renderReport()}catch(e){setStatus('scanstatus',e.message,true)}
 finally{busy=false;$('scan').disabled=$('deep').disabled=false}
}
$('scan').onclick=()=>doScan(false);
$('deep').onclick=()=>doScan(true);
$('copy').onclick=async()=>{if(!report)return;try{await navigator.clipboard.writeText(JSON.stringify(report,null,2));setStatus('scanstatus','Sanitized report copied. Review before sharing.')}catch(e){setStatus('scanstatus','Clipboard permission unavailable: use SAVE JSON instead.',true)}};
$('export').onclick=()=>{if(!report)return;const blob=new Blob([JSON.stringify(report,null,2)],{type:'application/json'});const u=URL.createObjectURL(blob);const link=document.createElement('a');link.href=u;link.download='SAWCHA_Latency_Lab_Report.json';link.click();setTimeout(()=>URL.revokeObjectURL(u),1000)};
$('copyPrompt').onclick=async()=>{if(!report)return;const text='Please analyze this SAWCHA Windows latency diagnostic snapshot. Explain the safest evidence-based optimizations, possible driver bottlenecks, and exact measurements/rollback. Do NOT infer kernel access from the report.\n\n'+JSON.stringify(report,null,2);try{await navigator.clipboard.writeText(text);setStatus('scanstatus','Diagnostic analysis prompt copied. Paste it into ChatGPT yourself after reviewing the report.')}catch(e){setStatus('scanstatus','Copy failed; use SAVE JSON and upload to ChatGPT.',true)}};
$('trace').onclick=async()=>{
 if(!$('traceconsent').checked){setStatus('tracestatus','Tick the ETL privacy/overhead consent box first.',true);return}
 if(busy)return;busy=true;$('trace').disabled=true;
 setStatus('tracestatus','Recording 12 seconds with Windows Performance Recorder; trace remains on your PC. Do not start a match during the recording.');
 try{const out=await api('/api/trace',{confirmSensitiveTrace:true});setStatus('tracestatus',`ETW trace saved at:\n${out.path}\n\n${out.notice}`)}catch(e){setStatus('tracestatus',e.message,true)}
 finally{busy=false;$('trace').disabled=false}
};
function addMessage(text,kind){const el=document.createElement('div');el.className='message '+kind;el.textContent=text;$('conversation').append(el);$('conversation').scrollTop=$('conversation').scrollHeight}
async function ask(local){
 const question=$('question').value.trim();if(!question)return;
 $('ask').disabled=$('local').disabled=true;addMessage(question,'human');$('question').value='';
 try{
   let result;
   if(local){result=await api('/api/local',{question});}
   else{
     if(!$('apikey').value){addMessage('Enter your personal OpenAI API key, or use Offline Analysis. An API key is separate from a ChatGPT subscription.','bot');return}
     result=await api('/api/chat',{question,apiKey:$('apikey').value,includeReport:$('include').checked,history:history.slice(-8)});
   }
   addMessage(result.answer,'bot');
   if(!local){history.push({role:'user',text:question},{role:'assistant',text:result.answer.slice(0,1800)});history=history.slice(-8)}
 }catch(e){addMessage('Unable to complete analysis: '+e.message,'bot')}
 finally{$('ask').disabled=$('local').disabled=false}
}
$('local').onclick=()=>ask(true);
$('ask').onclick=()=>ask(false);
$('question').addEventListener('keydown',e=>{if(e.key==='Enter'&&(e.ctrlKey||e.metaKey)){e.preventDefault();ask(false)}});
