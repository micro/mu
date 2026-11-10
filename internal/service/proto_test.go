package service

import (
	chatpb "mu/proto/service/chat/v1"
	"reflect"
	"strings"
	"testing"
)

func TestProtoSDKUsesJSONTypesAndRequiredFields(t *testing.T) {
	request := sdkType(reflect.TypeOf(chatpb.SendRequest{}), map[reflect.Type]bool{}, true)
	if strings.Contains(request, `"message"?`) || !strings.Contains(request, `"message": string`) {
		t.Fatal(request)
	}
	response := sdkType(reflect.TypeOf(chatpb.Message{}), map[reflect.Type]bool{}, false)
	if !strings.Contains(response, `"timestamp"?: string`) || strings.Contains(response, "nanos") {
		t.Fatal(response)
	}
}
