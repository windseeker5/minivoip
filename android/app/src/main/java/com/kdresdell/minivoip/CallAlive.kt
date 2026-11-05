package com.kdresdell.minivoip

import android.app.Notification
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder

// Keeps the microphone open while a call or a meeting is on. Android only gives
// it to the app on screen, or to one with a service like this: without it, a
// screen that locks would silence the owner, in the call and in the notes.
class CallAlive : Service() {
    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val back = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )
        val note = Notification.Builder(this, Ring.ON_A_CALL)
            .setSmallIcon(R.drawable.ic_call)
            .setContentTitle("On a call")
            .setContentText("Tap to go back to it.")
            .setContentIntent(back)
            .setOngoing(true)
            .build()
        // Android can refuse, say if the app is no longer in front. The call goes on.
        // Always tried first, even to stop: a service told to start that stops without
        // this crashes the app, and the call with it.
        try {
            if (Build.VERSION.SDK_INT >= 30) {
                startForeground(ID, note, ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE)
            } else {
                startForeground(ID, note)
            }
        } catch (e: RuntimeException) {
            stopSelf()
            return START_NOT_STICKY
        }
        if (intent?.action == STOP) {
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
        return START_NOT_STICKY
    }

    companion object {
        private const val ID = 3
        private const val STOP = "stop"

        @Volatile
        private var asked = false

        fun start(ctx: Context) {
            try {
                ctx.startForegroundService(Intent(ctx, CallAlive::class.java))
                asked = true
            } catch (e: RuntimeException) {
                // Not allowed right now: the call goes on while the screen is on.
            }
        }

        // stop goes through the service itself: it may not have started yet.
        fun stop(ctx: Context) {
            if (!asked) return
            asked = false
            try {
                ctx.startService(Intent(ctx, CallAlive::class.java).setAction(STOP))
            } catch (e: RuntimeException) {
                ctx.stopService(Intent(ctx, CallAlive::class.java))
            }
        }
    }
}
