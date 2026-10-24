package com.kdresdell.minivoip

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import org.json.JSONObject

// Decline on the call notification. The caller is offered voicemail.
class DeclineReceiver : BroadcastReceiver() {
    override fun onReceive(ctx: Context, intent: Intent) {
        val call = intent.getStringExtra("call") ?: return
        val pending = goAsync()
        decline(ctx, call) { pending.finish() }
    }

    companion object {
        fun decline(ctx: Context, call: String, done: () -> Unit = {}) {
            Ring.stop(ctx, call)
            Server.post("/app/calls/$call/decline", JSONObject()) { done() }
        }
    }
}
