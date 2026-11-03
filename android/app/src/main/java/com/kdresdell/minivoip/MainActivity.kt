package com.kdresdell.minivoip

import android.Manifest
import android.annotation.SuppressLint
import android.app.Activity
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.view.View
import android.webkit.CookieManager
import android.webkit.JavascriptInterface
import android.webkit.PermissionRequest
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import org.json.JSONObject
import java.util.UUID

// The app pages (/app) in a web view: one design for the computer, the
// browser and this app. The page finds us as window.MinivoipAndroid
// (assets/js/phone.js) for ringing, which the web can't do.
class MainActivity : Activity() {
    private lateinit var web: WebView
    private var pendingMic: PermissionRequest? = null
    private var started = false
    private var loaded = false
    private var overLock = false
    private var pendingAnswer: String? = null
    private val handler = Handler(Looper.getMainLooper())
    // Over the lock screen, a page whose script never ran (no network right
    // after Answer, say) must not stay up: it was never shown, so leave.
    private val neverShown = Runnable { if (overLock && web.visibility != View.VISIBLE) finish() }

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
                browser(req.url)
                return true
            }
        }
        web.webChromeClient = object : WebChromeClient() {
            // The page asks for the microphone (calls) or the camera (video
            // calls, the meeting room): ask the phone first if needed.
            override fun onPermissionRequest(req: PermissionRequest) {
                runOnUiThread {
                    // Only our own pages get the microphone and camera.
                    if (req.origin.host != Uri.parse(Server.base).host || req.origin.scheme != Uri.parse(Server.base).scheme) {
                        req.deny()
                        return@runOnUiThread
                    }
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
        // "Update the app" downloads with the browser: a web view can't.
        web.setDownloadListener { url, _, _, _, _ -> browser(Uri.parse(url)) }
        web.addJavascriptInterface(Bridge(), "MinivoipAndroid")
        setContentView(web)
        open(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        open(intent)
    }

    // open loads the app pages; from the ring screen's Answer, with the call
    // to answer, and over the lock screen like a phone call. The launcher
    // screen is open to other apps on the phone, so only an Answer carrying
    // our own pass counts.
    private fun open(intent: Intent) {
        val call = intent.getStringExtra("answer")?.takeIf { spendPass(this, intent.getStringExtra("pass")) }
        intent.removeExtra("answer")
        intent.removeExtra("pass")
        overLock = call != null
        setShowWhenLocked(overLock)
        setTurnScreenOn(overLock)
        pendingAnswer = call
        // Over the lock screen nothing shows until the page has hidden its tabs
        // and taken the call (takeAnswer).
        handler.removeCallbacks(neverShown)
        web.visibility = if (overLock) View.INVISIBLE else View.VISIBLE
        if (overLock) handler.postDelayed(neverShown, 10_000)
        // A plain return to the app leaves the page alone: a call may be on.
        if (call != null || !loaded) {
            loaded = true
            web.loadUrl(Server.base + "/app")
        }
    }

    // browser opens a web address outside the app; anything else is refused.
    private fun browser(url: Uri) {
        if (url.scheme != "https" && url.scheme != "http") return
        try {
            startActivity(Intent(Intent.ACTION_VIEW, url).addCategory(Intent.CATEGORY_BROWSABLE))
        } catch (e: ActivityNotFoundException) {
            // No browser: nothing to open it with.
        }
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
        // Over the lock screen, Back leaves: the app's pages stay behind the lock.
        if (overLock) finish() else if (web.canGoBack()) web.goBack() else super.onBackPressed()
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

        // takeAnswer hands the page the call to answer, once. It isn't in the
        // address, so no link can answer a call.
        @JavascriptInterface
        fun takeAnswer(): String? {
            runOnUiThread { web.visibility = View.VISIBLE }
            return pendingAnswer.also { pendingAnswer = null }
        }

        // callEnded puts the lock screen back in front of the app.
        @JavascriptInterface
        fun callEnded() = runOnUiThread {
            overLock = false
            setShowWhenLocked(false)
            setTurnScreenOn(false)
        }

        // version is this app's version name ("0.3"), so the page can say
        // when a newer one is ready (0.2 and older don't have it).
        @JavascriptInterface
        fun version(): String = BuildConfig.VERSION_NAME
    }

    companion object {
        private const val MIC = 1
        private const val START = 2

        // pass is a secret kept in this app's private storage, good for one
        // Answer: reopening the app later can't replay it.
        private fun pass(ctx: Context): String {
            val prefs = ctx.getSharedPreferences("answer", Context.MODE_PRIVATE)
            prefs.getString("pass", null)?.let { return it }
            val fresh = UUID.randomUUID().toString()
            prefs.edit().putString("pass", fresh).commit()
            return fresh
        }

        private fun spendPass(ctx: Context, given: String?): Boolean {
            val prefs = ctx.getSharedPreferences("answer", Context.MODE_PRIVATE)
            val ok = given != null && given == prefs.getString("pass", null)
            if (ok) prefs.edit().remove("pass").commit()
            return ok
        }

        fun answer(ctx: Context, call: String): Intent =
            Intent(ctx, MainActivity::class.java)
                .putExtra("answer", call)
                .putExtra("pass", pass(ctx))
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    }
}
