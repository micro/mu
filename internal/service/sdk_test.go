package service

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

type NamedSDKProbe struct{}
type NamedSDKRequest struct {
	Query string `json:"query" required:"true"`
	Limit int    `json:"limit"`
}
type NamedSDKResponse struct {
	Items []string `json:"items"`
	Next  string   `json:"next,omitempty"`
}

func (NamedSDKProbe) Search(context.Context, *NamedSDKRequest, *NamedSDKResponse) error { return nil }
func TestNamedSDKMatchesRPCContract(t *testing.T) {
	specs := []Spec{{Name: "places", Handler: NamedSDKProbe{}, Endpoints: map[string]Endpoint{"Search": {Doc: "Find places"}}}}
	types := TypeScriptSDK(specs)
	for _, want := range []string{`"places":`, `"search"(args:`, `"query": string`, `"limit"?: number`, `"items": Array<string>`, `"next"?: string`} {
		if !strings.Contains(types, want) {
			t.Errorf("missing typed contract %s", want)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	source := JavaScriptSDK(specs) + `
 let invocation;
 const mu=createClient(async (...args)=>{invocation=args;return {items:['Coffee']};});
 const result=await mu.places.search({query:'coffee'});
 if(JSON.stringify(invocation)!=='["places","search",{"query":"coffee"}]'||result.items[0]!=='Coffee')throw Error('Wrong SDK dispatch');
 let request;
 globalThis.fetch=async (url, options)=>{
   request={url,...options};
   return {ok:true,json:async()=>({result:'summary',data:{items:['Coffee']}})};
 };
 const remote=createClient({baseURL:'https://example.test/',token:'services-test-token'});
 const response=await remote.places.search({query:'coffee'});
 if(request.url!=='https://example.test/api/v1/places/search')throw Error('Wrong URL');
 if(request.method!=='POST'||request.headers.Authorization!=='Bearer services-test-token')throw Error('Missing authentication');
 if(JSON.parse(request.body).query!=='coffee'||response.items[0]!=='Coffee')throw Error('Lost structured response');
 globalThis.fetch=async()=>({ok:false,status:403,json:async()=>({error:'Access denied'})});
 try {await remote.places.search({query:'coffee'});throw Error('Accepted refusal');}
 catch(error){if(error.status!==403||error.message!=='Access denied')throw error;}
 `
	cmd := exec.Command(node, "--input-type=module", "-e", source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("named SDK failed: %v %s", err, out)
	}
}
