// SPDX-License-Identifier: Apache-2.0

package proto_test

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	assistant "mu/proto/assistant/v1"
	contacts "mu/proto/service/contacts/v1"
	"os"
	"testing"
)

func TestCompositionWireFixtures(t *testing.T) {
	for name, message := range map[string]proto.Message{"home": &assistant.HomeResponse{}, "inbox": &assistant.ListInboxResponse{}, "services": &assistant.ListServicesResponse{}, "contacts": &contacts.ListResponse{}, "operation": &assistant.Operation{}} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/" + name + "-response.json")
			if err != nil {
				t.Fatal(err)
			}
			if err := protojson.Unmarshal(raw, message); err != nil {
				t.Fatal(err)
			}
			if _, err := protojson.Marshal(message); err != nil {
				t.Fatal(err)
			}
		})
	}
}
