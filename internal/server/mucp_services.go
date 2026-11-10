package server

import (
	"fmt"
	"net/http"
	"reflect"

	microerrors "go-micro.dev/v6/errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"mu/internal/api"
	"mu/internal/mucp"
	"mu/internal/service"
)

// contractOperations binds an explicitly selected, implemented service contract.
// It never scans the schema directory or exposes other registered services.
// All requests still traverse the existing service gateway and its account,
// authorization and charging checks.
func contractOperations(spec service.Spec, contract protoreflect.ServiceDescriptor) []mucp.Operation {
	var out []mucp.Operation
	for i := 0; i < contract.Methods().Len(); i++ {
		method := contract.Methods().Get(i)
		name := string(method.Name())
		endpoint, ok := spec.Endpoints[name]
		handler, exists := reflect.TypeOf(spec.Handler).MethodByName(name)
		if !ok || !exists || handler.Type.NumIn() != 4 {
			panic(fmt.Sprintf("unimplemented contract: %s.%s", spec.Name, name))
		}
		for n, desc := range []protoreflect.MessageDescriptor{method.Input(), method.Output()} {
			typ := handler.Type.In(n + 2)
			if typ.Kind() != reflect.Pointer {
				panic("contract requires pointer messages")
			}
			message, ok := reflect.New(typ.Elem()).Interface().(proto.Message)
			if !ok || message.ProtoReflect().Descriptor().FullName() != desc.FullName() {
				panic(fmt.Sprintf("contract type mismatch: %s.%s", spec.Name, name))
			}
		}
		out = append(out, mucp.Operation{Service: spec.Name, Endpoint: string(contract.Name()) + "." + name, Writes: endpoint.Writes || endpoint.Destructive,
			Request: func() proto.Message { return dynamicpb.NewMessage(method.Input()) },
			Handle: func(_ http.ResponseWriter, r *http.Request, owner, _ string, q proto.Message) (proto.Message, error) {
				response := dynamicpb.NewMessage(method.Output())
				err := service.Call(service.WithAccount(r.Context(), owner), spec.Name, service.HandlerName(spec.Handler)+"."+name, q, response)
				if err != nil {
					e := microerrors.FromError(err)
					code := map[int32]string{400: "invalid_arguments", 401: "unauthenticated", 403: "forbidden", 404: "not_found", 409: "conflict", 413: "invalid_arguments", 422: "invalid_arguments", 429: "rate_limited"}[e.Code]
					if code != "" {
						return nil, api.Fail(int(e.Code), code, e.Detail)
					}
				}
				return response, err
			},
		})
	}
	return out
}
