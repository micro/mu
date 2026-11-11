// SPDX-License-Identifier: Apache-2.0

package proto_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	mucpv1 "mu/proto/mucp/v1"
)

func TestMessagePreservesPayloadAcrossEncodings(t *testing.T) {
	for _, body := range [][]byte{[]byte(`{"prompt":"Hello","voice":true}`), {0, 255, 1, 128}} {
		original := &mucpv1.Message{Headers: map[string]string{
			"micro-protocol": "mucp/1", "micro-service": "assistant", "micro-endpoint": "Assistant.Ask",
			"micro-id": "12345678-1234-1234-1234-123456789abc", "content-type": "application/json",
		}, Body: body}
		wire, err := proto.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		restored := new(mucpv1.Message)
		if err := proto.Unmarshal(wire, restored); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(original, restored) {
			t.Fatal("binary envelope changed metadata or payload")
		}
		readable, err := protojson.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var object struct {
			Body string `json:"body"`
		}
		if err := json.Unmarshal(readable, &object); err != nil {
			t.Fatal(err)
		}
		decoded, err := base64.StdEncoding.DecodeString(object.Body)
		if err != nil || !bytes.Equal(decoded, body) {
			t.Fatal("JSON envelope did not preserve exact payload bytes")
		}
		if err := protojson.Unmarshal(readable, restored); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(original, restored) {
			t.Fatal("JSON envelope changed metadata or payload")
		}
	}
}
