import java.io.*;
import java.util.*;
import java.security.SecureRandom;
import javax.crypto.*;
import javax.crypto.spec.*;
import com.google.gson.*;
import org.whispersystems.libsignal.*;
import org.whispersystems.libsignal.ecc.*;
import org.whispersystems.libsignal.state.*;
import org.whispersystems.libsignal.state.impl.*;
import org.whispersystems.libsignal.protocol.*;
import org.whispersystems.libsignal.util.KeyHelper;

public class Peer {
 static Gson g=new Gson();static InMemorySignalProtocolStore store;static IdentityKeyPair identity;static SignedPreKeyRecord signed;static List<PreKeyRecord> keys;static SignalProtocolAddress remote;
 static String b64(byte[] b){return Base64.getEncoder().encodeToString(b);}static byte[] dec(String s){return Base64.getDecoder().decode(s);}static String str(JsonObject o,String k){return o.get(k).getAsString();}
 static Map<String,Object> handle(JsonObject r)throws Exception {
  Map<String,Object> out=new LinkedHashMap<>();String cmd=str(r,"cmd");
  if(cmd.equals("init")){
   identity=KeyHelper.generateIdentityKeyPair();store=new InMemorySignalProtocolStore(identity,42);signed=KeyHelper.generateSignedPreKey(identity,1);store.storeSignedPreKey(1,signed);keys=KeyHelper.generatePreKeys(1,100);
   Map<String,String> pre=new LinkedHashMap<>();for(PreKeyRecord k:keys){store.storePreKey(k.getId(),k);pre.put(""+k.getId(),b64(k.getKeyPair().getPublicKey().serialize()));}
   out.put("identity",b64(identity.getPublicKey().serialize()));out.put("signed",b64(signed.getKeyPair().getPublicKey().serialize()));out.put("signature",b64(signed.getSignature()));out.put("prekeys",pre);
  }else if(cmd.equals("encrypt")){
   if(r.has("bundle")){
    JsonObject b=r.getAsJsonObject("bundle");remote=new SignalProtocolAddress(str(r,"jid"),r.get("device").getAsInt());
    PreKeyBundle pb=new PreKeyBundle(r.get("device").getAsInt(),r.get("device").getAsInt(),b.get("prekey_id").getAsInt(),Curve.decodePoint(dec(str(b,"prekey")),0),b.get("signed_id").getAsInt(),Curve.decodePoint(dec(str(b,"signed")),0),dec(str(b,"signature")),new IdentityKey(dec(str(b,"identity")),0));
    new SessionBuilder(store,remote).process(pb);
   }
   byte[] key=new byte[16],iv=new byte[12];new SecureRandom().nextBytes(key);new SecureRandom().nextBytes(iv);
   Cipher aes=Cipher.getInstance("AES/GCM/NoPadding");aes.init(Cipher.ENCRYPT_MODE,new SecretKeySpec(key,"AES"),new GCMParameterSpec(128,iv));byte[] cipher=aes.doFinal(str(r,"text").getBytes("UTF-8"));
   byte[] material=new byte[32];System.arraycopy(key,0,material,0,16);System.arraycopy(cipher,cipher.length-16,material,16,16);
   CiphertextMessage msg=new SessionCipher(store,remote).encrypt(material);
   out.put("key",b64(msg.serialize()));out.put("prekey",msg.getType()==CiphertextMessage.PREKEY_TYPE);out.put("iv",b64(iv));out.put("payload",b64(Arrays.copyOf(cipher,cipher.length-16)));
  }else if(cmd.equals("decrypt")){
   SessionCipher cipher=new SessionCipher(store,remote);byte[] k=dec(str(r,"key"));byte[] material=r.get("prekey").getAsBoolean()?cipher.decrypt(new PreKeySignalMessage(k)):cipher.decrypt(new SignalMessage(k));
   byte[] payload=dec(str(r,"payload")),tagged=new byte[payload.length+16];System.arraycopy(payload,0,tagged,0,payload.length);System.arraycopy(material,16,tagged,payload.length,16);
   Cipher aes=Cipher.getInstance("AES/GCM/NoPadding");aes.init(Cipher.DECRYPT_MODE,new SecretKeySpec(Arrays.copyOf(material,16),"AES"),new GCMParameterSpec(128,dec(str(r,"iv"))));out.put("text",new String(aes.doFinal(tagged),"UTF-8"));
  }
  return out;
 }
 public static void main(String[] args)throws Exception{BufferedReader in=new BufferedReader(new InputStreamReader(System.in));String line;while((line=in.readLine())!=null){try{System.out.println(g.toJson(handle(JsonParser.parseString(line).getAsJsonObject())));}catch(Exception e){e.printStackTrace(System.err);System.out.println(g.toJson(Map.of("error",e.toString())));}}}
}
