package com.kdresdell.minivoip

import android.Manifest
import android.annotation.SuppressLint
import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.webkit.CookieManager
import android.webkit.JavascriptInterface
import android.webkit.PermissionRequest
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import org.json.JSONObject

// The app pages (/app) in a web view: one design for the computer, the
// browser and this app. The page finds us as window.MinivoipAndroid
// (assets/js/phone.js) for ringing, which the web can't do.
class MainActivity : Activity() {
    private lateinit var web: WebView
    private var pendingMic: PermissionRequest? = null
    private var started = false

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(state: Bundle?) {
        super.onCreate(state)
        web = WebView(this)
        web.settings.javaScriptEnabled = true
        web.settings.domStorageEnabled = true
        web.settings.mediaPlaybackRequiresUserGesture = false
        CookieManager.getInstance().setAcceptCookie(true)
        web.webViewClient = object : WebViewClient() {
            // Other sites open in the browser.
            override fun shouldOverrideUrlLoading(view: WebView, req: WebResourceRequest): Boolean {
                if (req.url.host == Uri.parse(Server.base).host) return false
                startActivity(Intent(Intent.ACTION_VIEW, req.url))
                return true
            }
        }
        web.webChromeClient = object : WebChromeClient() {
            // The page asks for the microphone (calls) or the camera (video
            // calls, the meeting room): ask the phone first if needed.
            override fun onPermissionRequest(req: PermissionRequest) {
                runOnUiThread {
                    val need = media(req).map { it.second }.filter { checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED }
                    if (need.isEmpty()) {
                        grant(req)
                    } else {
                        pendingMic = req
                        requestPermissions(need.toTypedArray(), MIC)
                    }
                }
            }
        }
        web.addJavascriptInterface(Bridge(), "MinivoipAndroid")
        setContentView(web)
        open(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        open(intent)
    }

    // open loads the app pages; from the ring screen's Answer, with the call
    // to answer, and over the lock screen like a phone call.
    private fun open(intent: Intent) {
        val call = intent.getStringExtra("answer")
        setShowWhenLocked(call != null)
        setTurnScreenOn(call != null)
        web.loadUrl(Server.base + "/app" + if (call != null) "?answer=" + Uri.encode(call) else "")
    }

    override fun onResume() {
        super.onResume()
        App.visible = true
        Ring.stop(this)
        if (started) report() // back from the settings
    }

    override fun onPause() {
        App.visible = false
        CookieManager.getInstance().flush()
        super.onPause()
    }

    @Deprecated("Back goes back in the pages first.")
    override fun onBackPressed() {
        if (web.canGoBack()) web.goBack() else super.onBackPressed()
    }

    // media pairs what the page asks for with the phone's permission for it.
    private fun media(req: PermissionRequest): List<Pair<String, String>> = req.resources.mapNotNull {
        when (it) {
            PermissionRequest.RESOURCE_AUDIO_CAPTURE -> it to Manifest.permission.RECORD_AUDIO
            PermissionRequest.RESOURCE_VIDEO_CAPTURE -> it to Manifest.permission.CAMERA
            else -> null
        }
    }

    // grant gives the page what the phone allows, or says no.
    private fun grant(req: PermissionRequest) {
        val ok = media(req).filter { checkSelfPermission(it.second) == PackageManager.PERMISSION_GRANTED }.map { it.first }
        if (ok.isEmpty()) req.deny() else req.grant(ok.toTypedArray())
    }

    override fun onRequestPermissionsResult(code: Int, perms: Array<out String>, results: IntArray) {
        if (code == MIC) {
            pendingMic?.let { grant(it) }
            pendingMic = null
        } else {
            report()
        }
    }

    // report gives the page this phone's Firebase token and whether calls
    // can ring on the lock screen.
    private fun report() {
        Server.token(this) { token ->
            val ready = Ring.ready(this)
            runOnUiThread {
                web.evaluateJavascript("window.minivoipAndroid && minivoipAndroid(${JSONObject.quote(token ?: "")}, $ready)", null)
            }
        }
    }

    inner class Bridge {
        // start asks for notifications and the microphone (once), then
        // reports.
        @JavascriptInterface
        fun start() = runOnUiThread {
            started = true
            val need = mutableListOf<String>()
            if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                need += Manifest.permission.POST_NOTIFICATIONS
            }
            if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
                need += Manifest.permission.RECORD_AUDIO
            }
            if (need.isEmpty()) report() else requestPermissions(need.toTypedArray(), START)
        }

        // settings opens what still blocks ringing: notifications, then
        // full-screen calls (Android 14).
        @JavascriptInterface
        fun settings() = runOnUiThread {
            val nm = getSystemService(android.app.NotificationManager::class.java)
            when {
                !nm.areNotificationsEnabled() || !Ring.ready(this@MainActivity) && Build.VERSION.SDK_INT < 34 ->
                    startActivity(Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, packageName))
                Build.VERSION.SDK_INT >= 34 && !nm.canUseFullScreenIntent() ->
                    startActivity(Intent(Settings.ACTION_MANAGE_APP_USE_FULL_SCREEN_INTENT, Uri.parse("package:$packageName")))
                else -> report()
            }
        }

        @JavascriptInterface
        fun clearRing() = Ring.stop(this@MainActivity)
    }

    companion object {
        private const val MIC = 1
        private const val START = 2

        fun answer(ctx: Context, call: String): Intent =
            Intent(ctx, MainActivity::class.java)
                .putExtra("answer", call)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    }
}
