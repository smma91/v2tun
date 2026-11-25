package io.github.smma91.v2tun

import android.app.Application
import io.github.smma91.v2tun.libbox.Libbox
import io.github.smma91.v2tun.libbox.SetupOptions
import io.github.smma91.v2tun.mobile.Mobile
import java.io.File

class App : Application() {
    override fun onCreate() {
        super.onCreate()
        instance = this
        // Servers, settings and translations (shared with the desktop app).
        Mobile.init(filesDir.absolutePath, assets.open("i18n.json").use { it.readBytes() })
        // sing-box's mobile layer.
        val o = SetupOptions()
        o.basePath = filesDir.absolutePath
        o.workingPath = File(filesDir, "work").apply { mkdirs() }.absolutePath
        o.tempPath = cacheDir.absolutePath
        o.logMaxLines = 300
        runCatching { Libbox.setup(o) }
    }

    companion object {
        lateinit var instance: App
            private set
    }
}

/** Translates an English UI text (the keys of the desktop's i18n.json). */
fun tr(s: String): String = Mobile.t(s)
