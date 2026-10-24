package com.kdresdell.minivoip

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Person
import android.content.Context
import android.content.Intent
import android.media.AudioAttributes
import android.media.RingtoneManager
import android.os.Build

// Ringing like a phone call: a call notification on the "calls" channel
// with the phone's ringtone, repeating until answered (FLAG_INSISTENT), and
// the ring screen (RingActivity) over the lock screen.
object Ring {
    private const val CALLS = "calls"
    private const val MESSAGES = "messages"
    private const val RING_ID = 1
    private const val MESSAGE_ID = 2

    private fun nm(ctx: Context) = ctx.getSystemService(NotificationManager::class.java)

    fun channels(ctx: Context) {
        val calls = NotificationChannel(CALLS, "Incoming calls", NotificationManager.IMPORTANCE_HIGH).apply {
            description = "Rings when a website visitor calls."
            setSound(
                RingtoneManager.getDefaultUri(RingtoneManager.TYPE_RINGTONE),
                AudioAttributes.Builder()
                    .setUsage(AudioAttributes.USAGE_NOTIFICATION_RINGTONE)
                    .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                    .build()
            )
            enableVibration(true)
            vibrationPattern = longArrayOf(0, 800, 600, 800, 600)
            lockscreenVisibility = Notification.VISIBILITY_PUBLIC
        }
        val messages = NotificationChannel(MESSAGES, "Missed calls and voicemail", NotificationManager.IMPORTANCE_HIGH)
        nm(ctx).createNotificationChannels(listOf(calls, messages))
    }

    // ready: notifications are on and calls may fill the lock screen.
    fun ready(ctx: Context): Boolean {
        val nm = nm(ctx)
        if (!nm.areNotificationsEnabled()) return false
        if (nm.getNotificationChannel(CALLS)?.importance == NotificationManager.IMPORTANCE_NONE) return false
        return Build.VERSION.SDK_INT < 34 || nm.canUseFullScreenIntent()
    }

    @Volatile
    var ringing: String? = null
        private set

    // ring starts ringing for a call; false if it already rings (the server
    // repeats its push every few seconds).
    fun ring(ctx: Context, call: String, text: String, until: Long): Boolean {
        if (ringing == call && nm(ctx).activeNotifications.any { it.id == RING_ID }) return false
        ringing = call
        val screen = PendingIntent.getActivity(
            ctx, 1, RingActivity.intent(ctx, call, text),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val answer = PendingIntent.getActivity(
            ctx, 2, MainActivity.answer(ctx, call),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val decline = PendingIntent.getBroadcast(
            ctx, 3, Intent(ctx, DeclineReceiver::class.java).putExtra("call", call),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val caller = Person.Builder().setName("Website caller").setImportant(true).build()
        val b = Notification.Builder(ctx, CALLS)
            .setSmallIcon(R.drawable.ic_call)
            .setContentTitle("Incoming call")
            .setContentText(text)
            .setCategory(Notification.CATEGORY_CALL)
            .setVisibility(Notification.VISIBILITY_PUBLIC)
            .setOngoing(true)
            .setContentIntent(screen)
            .setFullScreenIntent(screen, true)
            .setColor(ctx.getColor(R.color.green))
        if (until > System.currentTimeMillis()) b.setTimeoutAfter(until - System.currentTimeMillis())
        if (Build.VERSION.SDK_INT >= 31) {
            b.setStyle(Notification.CallStyle.forIncomingCall(caller, decline, answer))
        } else {
            b.addAction(Notification.Action.Builder(null, "Decline", decline).build())
            b.addAction(Notification.Action.Builder(null, "Answer", answer).build())
        }
        val n = b.build()
        n.flags = n.flags or Notification.FLAG_INSISTENT
        nm(ctx).notify(RING_ID, n)
        return true
    }

    // stop ends the ringing for a call ("" for whichever call rings).
    fun stop(ctx: Context, call: String = "") {
        if (call != "" && ringing != null && call != ringing) return
        ringing = null
        nm(ctx).cancel(RING_ID)
        RingActivity.close()
    }

    // message shows a plain notification: missed call, voicemail, test.
    fun message(ctx: Context, tag: String, title: String, text: String) {
        val open = PendingIntent.getActivity(
            ctx, 4, Intent(ctx, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val n = Notification.Builder(ctx, MESSAGES)
            .setSmallIcon(R.drawable.ic_call)
            .setContentTitle(title)
            .setContentText(text)
            .setContentIntent(open)
            .setAutoCancel(true)
            .setColor(ctx.getColor(R.color.ink))
            .build()
        nm(ctx).notify(tag, MESSAGE_ID, n)
    }
}
