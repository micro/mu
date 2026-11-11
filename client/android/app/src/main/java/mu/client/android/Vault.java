package mu.client.android;

import android.content.Context;
import android.content.SharedPreferences;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;
import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;
import org.json.JSONObject;

final class Vault {
    private static final Object RENEWAL = new Object();
    private static final String ALIAS = "micro.account";
    private final SharedPreferences preferences;
    Vault(Context context) { preferences = context.getSharedPreferences("account", Context.MODE_PRIVATE); }
    private SecretKey key() throws Exception {
        KeyStore store = KeyStore.getInstance("AndroidKeyStore");
        store.load(null);
        if (!store.containsAlias(ALIAS)) {
            KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
            generator.init(new KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build());
            generator.generateKey();
        }
        return (SecretKey) store.getKey(ALIAS, null);
    }
    JSONObject read() throws Exception { synchronized (Vault.class) {
        String encrypted = preferences.getString("data", "");
        if (encrypted.isEmpty()) return new JSONObject();
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.DECRYPT_MODE, key(), new GCMParameterSpec(128,
                Base64.decode(preferences.getString("iv", ""), Base64.NO_WRAP)));
        return new JSONObject(new String(cipher.doFinal(Base64.decode(encrypted, Base64.NO_WRAP)), StandardCharsets.UTF_8));
    }
    }
    void write(JSONObject value) throws Exception { synchronized (Vault.class) {
        JSONObject current = read();
        if (!value.optString("client").isEmpty() && value.optString("client").equals(current.optString("client")) && current.optLong("expires") > value.optLong("expires")) {
            value.put("token", current.getString("token")).put("refresh", current.optString("refresh")).put("expires", current.getLong("expires"));
        }
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.ENCRYPT_MODE, key());
        String data = Base64.encodeToString(cipher.doFinal(value.toString().getBytes(StandardCharsets.UTF_8)), Base64.NO_WRAP);
        if (!preferences.edit().putString("data", data)
                .putString("iv", Base64.encodeToString(cipher.getIV(), Base64.NO_WRAP)).commit())
            throw new java.io.IOException("Could not save sign-in.");
    }
    }
    void clear() { synchronized (Vault.class) { preferences.edit().clear().commit(); } }

    interface Renewal { JSONObject exchange(String server, JSONObject account) throws Exception; }
    String accessToken(String server, String connection) throws Exception {
        return accessToken(server, connection, OAuth::refresh);
    }
    String accessToken(String server, String connection, Renewal renewal) throws Exception {
        synchronized (RENEWAL) {
            JSONObject current = read();
            if (!server.equals(current.optString("server")) ||
                !connection.equals(current.optString("client", current.optString("token"))) || connection.isEmpty())
                throw new java.io.IOException("Please sign in again.");
            if (current.optLong("expires") <= System.currentTimeMillis() ||
                (!current.optString("refresh").isEmpty() && current.optLong("expires") <= System.currentTimeMillis() + 60000)) {
                if (current.optString("refresh").isEmpty()) throw new java.io.IOException("Please sign in again.");
                JSONObject reply = renewal.exchange(server, current);
                long seconds = reply.getLong("expires_in");
                if (!"Bearer".equals(reply.optString("token_type")) || seconds <= 0 ||
                    reply.getString("access_token").isEmpty() || reply.getString("refresh_token").isEmpty())
                    throw new java.io.IOException("Invalid session renewal response.");
                current.put("token", reply.getString("access_token")).put("refresh", reply.getString("refresh_token"))
                    .put("expires", System.currentTimeMillis() + Math.min(seconds, 86400) * 1000);
                synchronized (Vault.class) {
                    JSONObject latest = read();
                    if (!connection.equals(latest.optString("client")) || !server.equals(latest.optString("server")))
                        throw new java.io.IOException("Account disconnected.");
                    latest.put("token", current.getString("token")).put("refresh", current.getString("refresh"))
                        .put("expires", current.getLong("expires"));
                    write(latest);
                }
            }
            return current.getString("token");
        }
    }

}
