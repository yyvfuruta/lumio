package io.github.yyvfuruta.lumio;

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

/** Stores only the session, encrypted using a key bound to this installation. */
final class SessionStore {
    private static final String ALIAS = "lumio.session.v1";
    private final SharedPreferences prefs;
    SessionStore(Context context) { prefs = context.getSharedPreferences("session", Context.MODE_PRIVATE); }
    private SecretKey key() throws Exception {
        KeyStore store = KeyStore.getInstance("AndroidKeyStore"); store.load(null);
        if (store.containsAlias(ALIAS)) return (SecretKey) store.getKey(ALIAS, null);
        KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
        generator.init(new KeyGenParameterSpec.Builder(ALIAS,
                KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build());
        return generator.generateKey();
    }
    void save(String url, String token, long expiry) throws Exception {
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding"); cipher.init(Cipher.ENCRYPT_MODE, key());
        cipher.updateAAD(url.getBytes(StandardCharsets.UTF_8));
        String data = Base64.encodeToString(cipher.doFinal(token.getBytes(StandardCharsets.UTF_8)), Base64.NO_WRAP);
        if (!prefs.edit().putString("url", url).putString("token", data)
                .putString("iv", Base64.encodeToString(cipher.getIV(), Base64.NO_WRAP))
                .putLong("expires", expiry).commit()) throw new java.io.IOException("Não foi possível salvar a sessão.");
    }
    String url() { return prefs.getString("url", ""); }
    String token() {
        if (prefs.getLong("expires", 0) <= System.currentTimeMillis()) return "";
        try {
            Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
            cipher.init(Cipher.DECRYPT_MODE, key(), new GCMParameterSpec(128, Base64.decode(prefs.getString("iv", ""), Base64.NO_WRAP)));
            cipher.updateAAD(url().getBytes(StandardCharsets.UTF_8));
            return new String(cipher.doFinal(Base64.decode(prefs.getString("token", ""), Base64.NO_WRAP)), StandardCharsets.UTF_8);
        } catch (Exception e) { clear(); return ""; }
    }
    void clear() { prefs.edit().clear().apply(); }
}
