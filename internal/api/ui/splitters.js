'use strict';
(function(){
  const root=document.documentElement,workspace=document.querySelector('.workspace'),upper=document.querySelector('.upper-pane');
  const clamp=(value,min,max)=>Math.max(min,Math.min(max,value));
  function restore(){const width=Number(localStorage.getItem('utautts.settingsWidth')),height=Number(localStorage.getItem('utautts.pitchHeight'));if(width)root.style.setProperty('--settings-width',`${clamp(width,220,520)}px`);if(height)root.style.setProperty('--pitch-height',`${clamp(height,170,window.innerHeight*.6)}px`)}
  function bind(splitter,move,finish){splitter.addEventListener('pointerdown',event=>{if(matchMedia('(max-width:780px)').matches)return;splitter.setPointerCapture(event.pointerId);splitter.classList.add('dragging');document.body.classList.add('resizing')});splitter.addEventListener('pointermove',event=>{if(!splitter.hasPointerCapture(event.pointerId))return;move(event)});const end=event=>{if(!splitter.hasPointerCapture(event.pointerId))return;splitter.releasePointerCapture(event.pointerId);splitter.classList.remove('dragging');document.body.classList.remove('resizing');finish()};splitter.addEventListener('pointerup',end);splitter.addEventListener('pointercancel',end)}
  const vertical=document.getElementById('verticalSplitter'),horizontal=document.getElementById('horizontalSplitter');
  bind(vertical,event=>{const rect=upper.getBoundingClientRect();root.style.setProperty('--settings-width',`${clamp(rect.right-event.clientX,220,Math.min(520,rect.width-420))}px`)},()=>localStorage.setItem('utautts.settingsWidth',parseFloat(getComputedStyle(root).getPropertyValue('--settings-width'))));
  bind(horizontal,event=>{const rect=workspace.getBoundingClientRect();root.style.setProperty('--pitch-height',`${clamp(rect.bottom-event.clientY,170,rect.height*.62)}px`)},()=>localStorage.setItem('utautts.pitchHeight',parseFloat(getComputedStyle(root).getPropertyValue('--pitch-height'))));
  restore();
})();
