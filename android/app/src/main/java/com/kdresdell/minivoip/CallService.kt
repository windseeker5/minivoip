package com.kdresdell.minivoip

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import org.json.JSONObject

// Firebase messages from the server (push/fcm.go). Each carries the same
// notification JSON the browser's service worker gets (sw.js), in "n".
class CallService : FirebaseMessagingService() {
    override fun onMessageReceived(msg: RemoteMessage) {
        val n = JSONObject(msg.data["n"] ?: return)
        val id = n.optLong("id")
        val call = n.optString("call")
        val title = n.optString("title", "minivoip")
        val body = n.optString("body")
        when (n.optString("kind")) {
            "ring" -> when {
                App.visible -> Server.receipt(id, "ring", "app on screen")
                n.optLong("until") in 1..System.currentTimeMillis() -> {
                    Ring.message(this, "call-$call", "Missed call", "A website visitor called. They may leave a message.")
                    Server.receipt(id, "missed", "late ring")
                }
                else -> {
                    val fresh = Ring.ring(this, call, n.optString("body"), n.optLong("until"))
                    Server.receipt(id, "ring", if (fresh) "android call screen" else "android already ringing")
                }
            }
            "answered" -> {
                Ring.stop(this, call)
                Server.receipt(id, "cleared", "android")
            }
            "missed" -> {
                Ring.stop(this, call)
                Ring.message(this, "call-$call", title, body)
                Server.receipt(id, "missed", "android")
            }
            else -> {
                Ring.message(this, n.optString("tag", "minivoip"), title, body)
                Server.receipt(id, n.optString("kind"), "android")
            }
        }
    }

    // A new token: tell the server, if we're logged in (else the app pages
    // send it after logging in).
    override fun onNewToken(token: String) {
        Server.post("/app/push/subscribe", JSONObject().put("endpoint", "fcm:$token"))
    }
}
