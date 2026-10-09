if (typeof document !== 'undefined') {
document.addEventListener('DOMContentLoaded', () => {
 // Buy credits with an authorization signed in the customer's own wallet.
 const cryptoForm=document.getElementById('crypto-checkout');
 if(cryptoForm){
  const amount=cryptoForm.querySelector('[name="amount"]'),button=cryptoForm.querySelector('button[type="submit"]'),status=document.getElementById('crypto-status'),total=document.getElementById('crypto-total');
  let pending=false,timer;
  const request=async body=>{
   const response=await fetch('/account/crypto',{method:body?'POST':'GET',credentials:'same-origin',headers:{'Accept':'application/json','Content-Type':'application/json','X-CSRF-Token':cryptoForm.dataset.csrf},...(body?{body:JSON.stringify(body)}:{})});
   const result=await response.json();
   if(!response.ok){const error=new Error(result.error||'Could not check payment.');error.paymentRejected=response.status>=400&&response.status<500;throw error;}
   return result;
  };
  const show=payment=>{
   pending=payment.status==='pending';button.disabled=pending;amount.disabled=pending;
   if(pending){status.textContent='Payment is being confirmed. You can close this page; your credits will be added automatically. Do not pay again.';clearTimeout(timer);timer=setTimeout(check,5000);}
   else if(payment.status==='paid'){status.textContent=payment.credits.toLocaleString()+' credits added to your account.';}
   else if(payment.status==='expired'){status.textContent='The payment expired without a confirmed transfer. You can try again.';}
   else if(status.textContent.startsWith('Checking whether')){status.textContent='No payment was submitted. You can try again.';}
  };
  const check=async()=>{try{show(await request());}catch(error){status.textContent=error.message+' Reload this page to check your payment before paying again.';button.disabled=true;timer=setTimeout(check,10000);}};
  amount.addEventListener('input',()=>{const n=Number(amount.value);total.textContent=Number.isInteger(n)&&n>=1&&n<=500?n+' USDC = '+(n*100).toLocaleString()+' credits':'Enter 1–500 USDC.';});
  cryptoForm.addEventListener('submit',async event=>{
   event.preventDefault();if(pending)return;button.disabled=true;amount.disabled=true;
   let signed=false;
   try{
    const previous=await request();if(previous.status==='pending'){show(previous);return;}
    const provider=window.ethereum;
    if(!provider?.request)throw new Error('Open this page in your wallet’s browser, or enable a compatible browser wallet.');
    status.textContent='Connect your wallet…';
    const accounts=await provider.request({method:'eth_requestAccounts'});
    if(!accounts?.[0])throw new Error('No wallet account selected.');
    if(await provider.request({method:'eth_chainId'})!=='0x2105')await provider.request({method:'wallet_switchEthereumChain',params:[{chainId:'0x2105'}]});
    if(await provider.request({method:'eth_chainId'})!=='0x2105')throw new Error('Switch your wallet to Base to continue.');
    const payment=await request({amount:amount.value,from:accounts[0]});
    status.textContent='Confirm '+(payment.credits/100)+' USDC for '+payment.credits.toLocaleString()+' credits in your wallet.';
    const signature=await provider.request({method:'eth_signTypedData_v4',params:[accounts[0],JSON.stringify(payment.typedData)]});
    signed=true;pending=true;amount.disabled=true;
    show(await request({nonce:payment.nonce,signature}));
   }catch(error){
    if(signed&&!error.paymentRejected){status.textContent='Checking whether your payment was submitted. Do not pay again.';timer=setTimeout(check,2000);}
    else{pending=false;status.textContent=error.code===4001?'Payment canceled in your wallet.':error.message||'Could not start payment.';button.disabled=false;amount.disabled=false;}
   }
  });
  button.disabled=true;check();
  window.addEventListener('pagehide',()=>clearTimeout(timer));
 }
});
}
