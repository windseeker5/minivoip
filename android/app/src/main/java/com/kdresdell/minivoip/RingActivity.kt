package com.kdresdell.minivoip

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.Bundle
import android.view.Gravity
import android.view.View
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView
import java.lang.ref.WeakReference

// The ring screen: fills the phone, also when locked (look C, like the app's
// own call screen). Answer opens the app pages, which answer the call.
class RingActivity : Activity() {
    private lateinit var call: String

    override fun onCreate(state: Bundle?) {
        super.onCreate(state)
        current = WeakReference(this)
        call = intent.getStringExtra("call") ?: return finish()
        setContentView(layout(intent.getStringExtra("text") ?: ""))
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        call = intent.getStringExtra("call") ?: return
        setContentView(layout(intent.getStringExtra("text") ?: ""))
    }

    override fun onDestroy() {
        if (current?.get() == this) current = null
        super.onDestroy()
    }

    private fun dp(v: Int) = (v * resources.displayMetrics.density).toInt()

    private fun layout(text: String): View {
        val ink = getColor(R.color.ink)
        val muted = getColor(R.color.muted)
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER_HORIZONTAL
            setBackgroundColor(getColor(R.color.ground))
            setPadding(dp(28), dp(96), dp(28), dp(64))
        }
        root.addView(TextView(this).apply {
            this.text = "INCOMING CALL"
            setTextColor(getColor(R.color.green))
            textSize = 13f
            letterSpacing = 0.12f
            typeface = Typeface.DEFAULT_BOLD
        })
        root.addView(TextView(this).apply {
            this.text = "Website caller"
            setTextColor(ink)
            textSize = 40f
            typeface = Typeface.create("sans-serif-medium", Typeface.NORMAL)
            gravity = Gravity.CENTER
            setPadding(0, dp(16), 0, dp(12))
        })
        root.addView(TextView(this).apply {
            this.text = text
            setTextColor(muted)
            textSize = 18f
            gravity = Gravity.CENTER
        })
        root.addView(View(this), LinearLayout.LayoutParams(0, 0, 1f))
        val buttons = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER
        }
        buttons.addView(button("Decline", getColor(R.color.red)) { decline() })
        buttons.addView(View(this), LinearLayout.LayoutParams(dp(72), 1))
        buttons.addView(button("Answer", getColor(R.color.green)) { answer() })
        root.addView(buttons)
        return root
    }

    private fun button(label: String, color: Int, click: () -> Unit): View {
        val col = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER_HORIZONTAL
        }
        col.addView(ImageView(this).apply {
            setImageResource(R.drawable.ic_call)
            scaleType = ImageView.ScaleType.FIT_CENTER
            setPadding(dp(22), dp(22), dp(22), dp(22))
            background = GradientDrawable().apply { shape = GradientDrawable.OVAL; setColor(color) }
            contentDescription = label
            setOnClickListener { click() }
            if (label == "Decline") rotation = 135f
        }, LinearLayout.LayoutParams(dp(80), dp(80)))
        col.addView(TextView(this).apply {
            text = label
            textSize = 16f
            setTextColor(getColor(R.color.ink))
            setPadding(0, dp(10), 0, 0)
        })
        return col
    }

    private fun answer() {
        Ring.stop(this)
        startActivity(MainActivity.answer(this, call))
        finish()
    }

    private fun decline() {
        DeclineReceiver.decline(this, call)
        finish()
    }

    companion object {
        private var current: WeakReference<RingActivity>? = null

        fun intent(ctx: Context, call: String, text: String): Intent =
            Intent(ctx, RingActivity::class.java)
                .putExtra("call", call)
                .putExtra("text", text)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_NO_USER_ACTION)

        // close takes the ring screen away (answered elsewhere, rang out).
        fun close() {
            current?.get()?.let { it.runOnUiThread { it.finish() } }
        }
    }
}
