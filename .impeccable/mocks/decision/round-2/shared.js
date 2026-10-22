// Deterministic voice waveform, played part in the logo's colours.
(function(){
  const stops=['#f3840f','#f46422','#e31a5a','#9f1574','#5a2692','#143495'];
  const hex=h=>[1,3,5].map(i=>parseInt(h.slice(i,i+2),16));
  const mix=t=>{t=Math.min(.9999,Math.max(0,t));const p=t*(stops.length-1),i=Math.floor(p),f=p-i;const a=hex(stops[i]),b=hex(stops[i+1]);return 'rgb('+a.map((v,k)=>Math.round(v+(b[k]-v)*f)).join(',')+')'};
  document.querySelectorAll('[data-wave]').forEach(el=>{
    const n=+el.dataset.wave, played=+(el.dataset.played||0), h=+(el.dataset.h||40), rest=el.dataset.rest||'#d9dce2', span=+(el.dataset.span||1);
    let s=+(el.dataset.seed||11), prev=.5; const rnd=()=>{s=(s*9301+49297)%233280;return s/233280};
    for(let i=0;i<n;i++){
      const env=Math.pow(Math.sin(Math.PI*(i+.5)/n),.45);
      let v=.55*prev+.45*rnd(); prev=v;
      const gap=(i%11===7||i%17===3)?.35:1;
      const b=document.createElement('i');
      b.style.height=Math.max(3,Math.round(h*env*v*gap*1.25))+'px';
      b.style.background=(i/n<played)?mix(i/n/played*span):rest;
      el.appendChild(b);
    }
  });
  if(window.lucide) lucide.createIcons({attrs:{'stroke-width':1.75}});
})();
