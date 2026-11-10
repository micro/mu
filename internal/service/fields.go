package service

import (
	"reflect"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	servicepb "mu/proto/service"
)

// FieldMetadata reads the contract's input documentation. Legacy handlers keep
// their struct tags; protobuf handlers use options from the source .proto file.
func FieldMetadata(t reflect.Type, f reflect.StructField) (description string, required bool) {
	description, required = f.Tag.Get("description"), f.Tag.Get("required") == "true"
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	message, ok := reflect.New(t).Interface().(proto.Message)
	if !ok {
		return
	}
	name := strings.Split(f.Tag.Get("json"), ",")[0]
	fields := message.ProtoReflect().Descriptor().Fields()
	fd := fields.ByJSONName(name)
	if fd == nil {
		fd = fields.ByName(protoreflect.Name(name))
	}
	if fd == nil {
		return
	}
	if v, ok := proto.GetExtension(fd.Options(), servicepb.E_Description).(string); ok {
		description = v
	}
	if v, ok := proto.GetExtension(fd.Options(), servicepb.E_Required).(bool); ok {
		required = v
	}
	return
}
