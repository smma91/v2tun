package io.github.smma91.v2tun

import android.content.Context
import android.content.Intent
import android.net.VpnService
import androidx.core.content.ContextCompat
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

enum class Status { STOPPED, STARTING, STARTED, STOPPING }

/** Bytes per second and in total, while connected. */
data class Traffic(val up: Long, val down: Long, val upTotal: Long, val downTotal: Long)

/** What the internet sees: the server's public IP and its country. */
data class Exit(val ip: String, val country: String, val city: String, val org: String)

/** The VPN's state, shared by the service, the screen, the tile and the notification. */
object Vpn {
    private val _status = MutableStateFlow(Status.STOPPED)
    val status: StateFlow<Status> = _status

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    internal val traffic = MutableStateFlow<Traffic?>(null)
    internal val exit = MutableStateFlow<Exit?>(null)

    /** Whether the app is on screen (live speed is collected only then, to save battery). */
    var uiVisible = false
        set(v) {
            field = v
            V2tunVpnService.instance?.updateWatching()
        }

    /** False while the device has no real network (Wi-Fi / mobile data off). */
    val online = MutableStateFlow(true)

    var since = 0L
        private set

    internal fun set(s: Status, error: String? = null) {
        if (s == Status.STARTED) since = System.currentTimeMillis()
        if (s != Status.STARTED) {
            traffic.value = null
            exit.value = null
        }
        _status.value = s
        if (error != null || s == Status.STARTING) _error.value = error
    }

    /** Android must have granted the VPN permission (VpnService.prepare) before. */
    fun start(ctx: Context) {
        if (VpnService.prepare(ctx) != null) return
        set(Status.STARTING)
        ContextCompat.startForegroundService(
            ctx, Intent(ctx, V2tunVpnService::class.java).setAction(V2tunVpnService.ACTION_START)
        )
    }

    fun stop(ctx: Context) {
        if (_status.value == Status.STOPPED) return
        ctx.startService(Intent(ctx, V2tunVpnService::class.java).setAction(V2tunVpnService.ACTION_STOP))
    }
}
