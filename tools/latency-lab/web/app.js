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


let winSlopAudit = null;
let lastRemovalPlan = null;
let lastRemovalReceipt = null;
function auditDetail(label, value) {
 const line=document.createElement('div');line.className='auditline';
 const title=document.createElement('b');title.textContent=label;
 const detail=document.createElement('span');detail.textContent=value;
 line.append(title,detail);return line;
}
function selectedSlopIDs() {
 return [...document.querySelectorAll('.slopchoice:checked')].map(input=>input.value);
}
function clearRemovalPlan() {
 lastRemovalPlan=null;$('slopremove').disabled=true;
 $('sloppreview').textContent='Selection changed. Preview selected apps before removal.';
}
function renderWinSlopAudit(audit) {
 winSlopAudit=audit;
 const host=$('slopfindings');host.replaceChildren();
 const groups=new Map();
 for(const item of audit.findings||[]) {
   if(!groups.has(item.category))groups.set(item.category,[]);
   groups.get(item.category).push(item);
 }
 for(const [category,findings] of groups) {
   const group=document.createElement('section');group.className='auditgroup';
   const head=document.createElement('h3');head.textContent=category;group.append(head);
   for(const item of findings) {
     const block=document.createElement('details');block.className='auditentry';
     const title=document.createElement('summary');title.textContent=item.title+' · '+item.state;
     block.append(title,auditDetail('Observation',item.evidence),auditDetail('What it means',item.explanation),auditDetail('Latency relevance',item.performance));
     if(category==='Optional apps' && item.state==='Installed for current user' && item.id.startsWith('app:')) {
       const select=document.createElement('label');select.className='check removal-check';
       const toggle=document.createElement('input');toggle.type='checkbox';toggle.className='slopchoice';toggle.value=item.id.slice(4);
       toggle.addEventListener('change',()=>{clearRemovalPlan();$('slopplan').disabled=selectedSlopIDs().length===0;});
       const label=document.createElement('span');label.textContent='SELECT FOR ACTUAL UNINSTALL (CURRENT USER)';
       select.append(toggle,label);block.append(select);
     }
     group.append(block);
   }
   host.append(group);
 }
 clearRemovalPlan();$('slopplan').disabled=true;$('slopcopy').disabled=false;
 setStatus('slopstatus',`Found ${audit.findings.filter(f=>f.category==='Optional apps'&&f.state==='Installed for current user').length} supported optional apps at ${audit.timestampUtc}. No Windows settings were modified by the scan. Click an installed app to expand its selection checkbox.`);
}
async function runSlopScan() {
 if(busy)return;
 busy=true;$('slopscan').disabled=true;
 setStatus('slopstatus','Scanning installed apps and optional Windows preferences...');
 try{
    const audit=await api('/api/winslop/inspect',{});renderWinSlopAudit(audit);
    if(report) {report=await api('/api/report',{});renderReport();}
 }catch(e){setStatus('slopstatus',e.message,true)}
 finally{busy=false;$('slopscan').disabled=false}
}
$('slopscan').onclick=runSlopScan;
$('slopcopy').onclick=async()=>{
 if(!winSlopAudit)return;
 try{await navigator.clipboard.writeText(JSON.stringify(winSlopAudit,null,2));setStatus('slopstatus','Windows feature audit copied. Review before sharing.');}
 catch(e){setStatus('slopstatus','Clipboard unavailable.',true)}
};
$('slopplan').onclick=async()=>{
 if(busy)return;
 const ids=selectedSlopIDs();
 if(ids.length===0||ids.length>6){setStatus('sloppreview','Choose 1–6 installed optional apps.',true);return;}
 try{
   const plan=await api('/api/winslop/plan',{ids});
   lastRemovalPlan=plan;
   $('sloppreview').textContent=plan.scope+'\n\n'+plan.apps.map((a,i)=>(i+1)+'. '+a.title+' ('+a.id+')\n   '+a.warning).join('\n\n')+'\n\n'+plan.warning;
   $('slopremove').disabled=false;
 }catch(e){clearRemovalPlan();setStatus('sloppreview',e.message,true)}
};
$('slopremove').onclick=async()=>{
 if(busy||!lastRemovalPlan)return;
 const ids=selectedSlopIDs();
 const planned=lastRemovalPlan.apps.map(a=>a.id);
 if(ids.length!==planned.length||ids.some((id,i)=>id!==planned[i])){clearRemovalPlan();setStatus('sloppreview','Selections changed. Preview again first.',true);return;}
 if(!$('slopack').checked||$('slopconfirm').value!=='REMOVE SELECTED APPS'){
   setStatus('sloppreview','You must acknowledge possible data loss and type REMOVE SELECTED APPS exactly.',true);return;
 }
 const description=lastRemovalPlan.apps.map(a=>'• '+a.title).join('\n');
 if(!window.confirm('This will REALLY uninstall the following applications from your current Windows account:\n\n'+description+'\n\nApp data may be lost and automatic rollback is unavailable. Continue?'))return;
 busy=true;$('slopremove').disabled=true;$('slopplan').disabled=true;$('slopscan').disabled=true;
 setStatus('slopstatus','Removing selected AppX packages and verifying Windows registration. Do not close this window. This may take up to 48 seconds.');
 try{
   const receipt=await api('/api/winslop/remove',{ids,confirmDataLoss:true,confirmation:'REMOVE SELECTED APPS'});
   lastRemovalReceipt=receipt;$('slopreceipt').disabled=false;
   const lines=receipt.results.map(x=>x.id+': '+x.status.toUpperCase()+' — '+x.message);
   setStatus('slopstatus','Windows returned results:\n'+lines.join('\n')+'\n\nSave the removal receipt. Apps for other Windows accounts were not affected.');
   $('slopconfirm').value='';$('slopack').checked=false;clearRemovalPlan();
   // Run a fresh post-change inventory; never display the old audit as current.
   const audit=await api('/api/winslop/inspect',{});
   renderWinSlopAudit(audit);
   setStatus('slopstatus','Removal finished and current-user apps rescanned:\n'+lines.join('\n')+'\n\nSave the removal receipt for your records.');
   if(report){report=await api('/api/report',{});renderReport();}
 }catch(e){setStatus('slopstatus','Removal could not be fully verified: '+e.message+' Rescan before retrying; some packages may have been removed.',true)}
 finally{busy=false;$('slopscan').disabled=false;$('slopremove').disabled=true;$('slopplan').disabled=selectedSlopIDs().length===0}
};
$('slopreceipt').onclick=()=>{
 if(!lastRemovalReceipt)return;
 const blob=new Blob([JSON.stringify(lastRemovalReceipt,null,2)],{type:'application/json'});
 const u=URL.createObjectURL(blob);const a=document.createElement('a');a.href=u;a.download='SAWCHA_Removal_Receipt.json';a.click();setTimeout(()=>URL.revokeObjectURL(u),1000);
};
