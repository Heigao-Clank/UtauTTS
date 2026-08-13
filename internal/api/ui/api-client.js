'use strict';
window.utauAPI={
  async request(path,options={}){try{const response=await fetch(path,options);if(!response.ok){let detail;try{detail=await response.json()}catch{detail={}}throw new Error(detail.error||`HTTP ${response.status}`)}window.dispatchEvent(new CustomEvent('utautts-api-ok',{detail:{path}}));return response}catch(error){window.dispatchEvent(new CustomEvent('utautts-api-error',{detail:{path,error}}));throw error}},
  async json(path,options={}){const response=await this.request(path,options);try{return await response.json()}catch{throw new Error('バックエンドから不正なJSON応答を受信しました')}},
  get(path){return this.json(path)},
  post(path,data,options={}){return this.json(path,{...options,method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)})},
  async blob(path,data){return(await this.request(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)})).blob()},
  audio(data){return this.blob('/api/synthesize/audio',data)},batch(items){return this.blob('/api/synthesize/batch',{items})}
};
