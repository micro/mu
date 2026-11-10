package tool

import (
	chatpb "mu/proto/service/chat/v1"
	grouppb "mu/proto/service/groups/v1"
	"reflect"
	"testing"
)

func TestProtoInputsRetainToolMetadata(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(chatpb.SendRequest{}), reflect.TypeOf(grouppb.UpdateRequest{})} {
		fields := params(typ)
		required := 0
		for _, field := range fields {
			if field.Required {
				required++
			}
			if field.Name == "message" && field.Description != "What to say" {
				t.Fatal("description lost", field)
			}
		}
		if required != 2 {
			t.Fatalf("required fields lost for %s: %+v", typ, fields)
		}
	}
}
