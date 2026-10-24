package com.kdresdell.minivoip

import android.app.Application

class App : Application() {
    override fun onCreate() {
        super.onCreate()
        Ring.channels(this)
        // Before any Firebase message is handled, also when a push wakes
        // the app from nothing.
        Server.startFirebase(this)
    }

    companion object {
        // The app pages are on screen: they ring by themselves.
        @Volatile
        var visible = false
    }
}
