// Pinned runtimes; models are downloaded only after the user enables voice.
let recognizer, speaker, lastProgress=0;
function progress(p) {
 if (p.status === 'progress' && Date.now()-lastProgress>500) { lastProgress=Date.now(); postMessage({type:'status', text:'Downloading speech model… '+Math.round(p.progress||0)+'%'}); }
}
let queue=Promise.resolve();
self.onmessage = ({data}) => { queue=queue.then(async () => {
 try {
  if(data.type==='load') {
   const {pipeline,env}=await import('https://cdn.jsdelivr.net/npm/@huggingface/transformers@3.8.1');
   env.allowLocalModels=false;
   env.backends.onnx.wasm.numThreads=1;
   recognizer=await pipeline('automatic-speech-recognition','Xenova/whisper-tiny.en',{device:'wasm',dtype:'q8',progress_callback:progress});
   postMessage({type:'ready'});
  } else if(data.type==='transcribe') {
   const result=await recognizer(data.audio,{return_timestamps:false});
   postMessage({type:'transcript',text:result.text||''});
  } else if(data.type==='speak') {
   if(!speaker) {
    postMessage({type:'status',text:'Loading spoken replies…'});
    const {KokoroTTS}=await import('https://cdn.jsdelivr.net/npm/kokoro-js@1.2.1/dist/kokoro.web.js');
    speaker=await KokoroTTS.from_pretrained('onnx-community/Kokoro-82M-v1.0-ONNX',{device:'wasm',dtype:'q8',progress_callback:progress});
   }
   const audio=await speaker.generate(data.text,{voice:'bf_emma'});
   const samples=audio.audio;
   postMessage({type:'audio',id:data.id,samples,rate:audio.sampling_rate},[samples.buffer]);
  }
 } catch(error) {
  postMessage({type:'error',operation:data.type,text:'Speech could not run on this device. '+String(error.message||error).slice(0,180)});
 }
}); };
