package chat

// Optional cross-implementation gate against signal-protocol-java 2.6.2,
// the exact Signal dependency in Conversations. The peer also uses JCA's
// AES-GCM, so neither the session nor payload codec shares our Go code.
import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	omemo "github.com/jim-ww/omemo-go"
	"mu/internal/data"
	"os"
	"os/exec"
	"testing"
)

func TestOMEMOConversationsSignalInterop(t *testing.T) {
	classpath := os.Getenv("OMEMO_JAVA_CLASSPATH")
	if classpath == "" {
		t.Skip("set OMEMO_JAVA_CLASSPATH to Signal Java 2.6.2, Curve25519 Java 0.4.1, protobuf Java 2.5.0 and Gson jars")
	}
	cmd := exec.Command("java", "-cp", classpath, "testdata/omemo/Peer.java")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { in.Close(); cmd.Wait() })
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 4096), 512*1024)
	call := func(req any) map[string]json.RawMessage {
		t.Helper()
		b, _ := json.Marshal(req)
		if _, err := fmt.Fprintln(in, string(b)); err != nil {
			t.Fatal(err)
		}
		if !scan.Scan() {
			t.Fatalf("Java peer stopped: %s", stderr.String())
		}
		var r map[string]json.RawMessage
		if err := json.Unmarshal(scan.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		if e := r["error"]; len(e) > 0 {
			t.Fatalf("Java peer: %s / %s", e, stderr.String())
		}
		return r
	}
	str := func(r map[string]json.RawMessage, k string) string {
		var s string
		if err := json.Unmarshal(r[k], &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	s, c := omemoFixture(t)
	peer := call(map[string]any{"cmd": "init"})
	var prekeys map[string]string
	json.Unmarshal(peer["prekeys"], &prekeys)
	x := omemoBundleXML{Identity: str(peer, "identity"), Signed: omemoSignedXML{ID: 1, Data: str(peer, "signed")}, Signature: str(peer, "signature")}
	for id, key := range prekeys {
		var n uint32
		fmt.Sscan(id, &n)
		x.Prekeys = append(x.Prekeys, omemoKeyXML{ID: n, Data: key})
	}
	bundle, err := x.bundle(omemo.Device{JID: s.bare(), ID: 42})
	if err != nil {
		t.Fatal(err)
	}
	publishPeer(t, s, c, bundle)
	state, err := loadOMEMO()
	if err != nil {
		t.Fatal(err)
	}
	manager, err := state.manager()
	if err != nil {
		t.Fatal(err)
	}
	own, err := manager.Bundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b := bundleXML(own)
	req := map[string]any{"cmd": "encrypt", "jid": agentJID(), "device": own.Device.ID, "text": "from Conversations Signal", "bundle": map[string]any{"identity": b.Identity, "signed": b.Signed.Data, "signed_id": b.Signed.ID, "signature": b.Signature, "prekey": b.Prekeys[0].Data, "prekey_id": b.Prekeys[0].ID}}
	for i := 0; i < 3; i++ {
		enc := call(req)
		var prekey bool
		json.Unmarshal(enc["prekey"], &prekey)
		x := omemoEnvelope{}
		x.Header.ID = 42
		x.Header.IV = str(enc, "iv")
		x.Header.Keys = []omemoRecipientXML{{ID: uint32(own.Device.ID), Prekey: prekey, Data: str(enc, "key")}}
		x.Payload = str(enc, "payload")
		// Authenticate the entire payload before committing ratchet state.
		before, _ := data.LoadFile(omemoStoreKey)
		bad := x
		ciphertext, _ := base64.StdEncoding.DecodeString(bad.Payload)
		ciphertext[0] ^= 0x80
		bad.Payload = b64(ciphertext)
		if err := s.receiveOMEMO(stanza{ID: fmt.Sprint(i)}, bad); err == nil {
			t.Fatal("tampered payload accepted")
		}
		after, _ := data.LoadFile(omemoStoreKey)
		if !bytes.Equal(before, after) {
			t.Fatal("tampering advanced persistent ratchet")
		}
		if err := s.receiveOMEMO(stanza{ID: fmt.Sprint(i)}, x); err != nil {
			t.Fatal(err)
		}
		rows := Everything(s.acc.ID, 20)
		if rows[len(rows)-1].Text != "from Conversations Signal" {
			t.Fatal("incorrect plaintext")
		}
		// Send two prekey messages before replying: Conversations keeps the prekey
		// wrapper until an acknowledgement. The second must reuse the same session.
		if i != 0 {
			c.Reset()
			if !SayTo(s.acc.ID, agentJID(), "encrypted Go reply") {
				t.Fatalf("reply: %s", c.String())
			}
			rows = Everything(s.acc.ID, 20)
			var reply omemoEnvelope
			xml.Unmarshal([]byte(rows[len(rows)-1].OMEMO), &reply)
			var k omemoRecipientXML
			for _, candidate := range reply.Header.Keys {
				if candidate.ID == 42 {
					k = candidate
				}
			}
			dec := call(map[string]any{"cmd": "decrypt", "key": k.Data, "prekey": k.Prekey, "iv": reply.Header.IV, "payload": reply.Payload})
			if str(dec, "text") != "encrypted Go reply" {
				t.Fatal("Java could not decrypt Go reply")
			}
		}
		delete(req, "bundle")
	}
}
