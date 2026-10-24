package com.kdresdell.minivoip

import android.content.Context
import android.util.Log
import android.webkit.CookieManager
import com.google.firebase.FirebaseApp
import com.google.firebase.FirebaseOptions
import com.google.firebase.messaging.FirebaseMessaging
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import kotlin.concurrent.thread

// Talking to the minivoip server, and starting Firebase with the settings
// the server hands out (GET /api/android/config, push/fcm.go), so the app
// carries no Firebase file of its own.
object Server {
    const val TAG = "minivoip"
    val base: String = BuildConfig.SERVER

    private fun prefs(ctx: Context) = ctx.getSharedPreferences("minivoip", Context.MODE_PRIVATE)

    // POST as the logged-in owner: the web view's session cookie, and our
    // own Origin so the server's same-site check passes. Runs on a thread.
    fun post(path: String, body: JSONObject, done: (Int) -> Unit = {}) = thread {
        val code = try {
            val c = URL(base + path).openConnection() as HttpURLConnection
            c.requestMethod = "POST"
            c.connectTimeout = 10_000
            c.readTimeout = 10_000
            c.doOutput = true
            c.setRequestProperty("Content-Type", "application/json")
            c.setRequestProperty("Origin", base)
            CookieManager.getInstance().getCookie(base)?.let { c.setRequestProperty("Cookie", it) }
            c.outputStream.use { it.write(body.toString().toByteArray()) }
            c.responseCode.also { c.disconnect() }
        } catch (e: Exception) {
            Log.w(TAG, "post $path: $e")
            0
        }
        done(code)
    }

    // startFirebase starts Firebase from the saved settings. False until the
    // settings have been fetched once.
    fun startFirebase(ctx: Context): Boolean {
        if (FirebaseApp.getApps(ctx).isNotEmpty()) return true
        val saved = prefs(ctx).getString("firebase", null) ?: return false
        val j = JSONObject(saved)
        FirebaseApp.initializeApp(
            ctx, FirebaseOptions.Builder()
                .setProjectId(j.getString("projectId"))
                .setApplicationId(j.getString("appId"))
                .setApiKey(j.getString("apiKey"))
                .setGcmSenderId(j.getString("senderId"))
                .build()
        )
        return true
    }

    // token gets this phone's Firebase token (fetching the settings first
    // if needed); null when the server has no Firebase set up.
    fun token(ctx: Context, done: (String?) -> Unit) = thread {
        if (!startFirebase(ctx)) {
            try {
                val c = URL("$base/api/android/config").openConnection() as HttpURLConnection
                c.connectTimeout = 10_000
                c.readTimeout = 10_000
                if (c.responseCode == 200) {
                    prefs(ctx).edit().putString("firebase", c.inputStream.bufferedReader().readText()).apply()
                }
                c.disconnect()
            } catch (e: Exception) {
                Log.w(TAG, "firebase config: $e")
            }
            if (!startFirebase(ctx)) return@thread done(null)
        }
        FirebaseMessaging.getInstance().token.addOnCompleteListener {
            done(if (it.isSuccessful) it.result else null)
        }
    }

    // receipt tells the server a push arrived and what the app did with it
    // (the push_log table).
    fun receipt(id: Long, shownAs: String, note: String) {
        if (id == 0L) return
        post("/app/push/receipt", JSONObject().put("id", id).put("shownAs", shownAs).put("note", note))
    }
}
